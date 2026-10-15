package server

import (
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aloglu/bookshelf/internal/notes"
	"github.com/aloglu/bookshelf/internal/store"
)

// notesOpen says whether visitors may read a book's public notes: the owner has not switched notes off
// for visitors, and has not kept this book's notes from them.
func notesOpen(st store.Settings, b store.Book) bool { return st.Notes && !b.NotesHidden }

// readableBy is a book's notes as one audience may read them: all of them for the owner; for visitors only
// the public ones, never which reading one came from, and nothing for a hidden book, a book whose notes
// are kept from them, notes switched off, or a private library.
func readableBy(snap *store.Snapshot, b store.Book, owner bool) []store.Entry {
	out := []store.Entry{}
	for _, e := range snap.Entries[b.ID] {
		switch {
		case owner:
			out = append(out, e)
		case e.Public && !b.Hidden && !snap.Settings.Private && notesOpen(snap.Settings, b):
			e.ReadingID = ""
			out = append(out, e)
		}
	}
	return out
}

// listEntries is a book's notes, oldest first.
func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	snap, owner := s.Store.Snapshot(), s.isOwner(r)
	b, ok := snap.Book(r.PathValue("id"))
	if !ok || (!owner && (b.Hidden || snap.Settings.Private || !notesOpen(snap.Settings, b))) {
		fail(w, http.StatusNotFound, "No such book.")
		return
	}
	writeJSON(w, http.StatusOK, readableBy(snap, b, owner))
}

func (s *Server) setNotesHidden(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hidden bool `json:"hidden"`
	}
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	if err := s.Store.SetNotesHidden(r.Context(), r.PathValue("id"), in.Hidden); err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.book(r.PathValue("id")))
}

const timelinePage = 100

// listNotes is every book's notes together, newest first, a page at a time (before and beforeId are the
// time and id of the last one seen: several can share a time; count asks for more than a page at once, as
// many as the page showed before, when it is come back to). Visitors get it only when the owner allows it, and only what they may read. The owner gets
// everything, or with view=visitors exactly what a visitor would see.
func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	snap, owner := s.Store.Snapshot(), s.isOwner(r)
	st := snap.Settings
	if !owner && (st.Private || !st.Notes || !st.Timeline) {
		fail(w, http.StatusNotFound, "There is no such page.")
		return
	}
	asOwner := owner && r.URL.Query().Get("view") != "visitors"
	var all []store.Entry
	for _, b := range snap.Books {
		if asOwner {
			all = append(all, snap.Entries[b.ID]...)
			continue
		}
		if b.Hidden || !notesOpen(st, b) {
			continue
		}
		for _, e := range snap.Entries[b.ID] {
			if e.Public {
				e.ReadingID = ""
				all = append(all, e)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.After(all[j].CreatedAt)
		}
		return all[i].ID > all[j].ID
	})
	total := len(all)
	if before := r.URL.Query().Get("before"); before != "" {
		if t, err := time.Parse(time.RFC3339Nano, before); err == nil {
			id := r.URL.Query().Get("beforeId")
			i := sort.Search(len(all), func(i int) bool {
				return all[i].CreatedAt.Before(t) || (all[i].CreatedAt.Equal(t) && all[i].ID < id)
			})
			all = all[i:]
		}
	}
	size := timelinePage
	if n, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && n > size {
		size = n
	}
	more := len(all) > size
	if more {
		all = all[:size]
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": all, "total": total, "more": more})
}

// exportNotes is a book's notes, or all of them, as a Markdown file. With private=1 it holds the owner's
// private notes too; with download=1 the browser saves it as a file.
func (s *Server) exportNotes(w http.ResponseWriter, r *http.Request) {
	snap, q := s.Store.Snapshot(), r.URL.Query()
	private, zone := q.Get("private") == "1", readerZone(q.Get("tz"))
	var text, name string
	if id := q.Get("book"); id != "" {
		b, ok := snap.Book(id)
		if !ok {
			fail(w, http.StatusNotFound, "No such book.")
			return
		}
		text, name = notes.BookMarkdown(snap, b, private, zone), b.Slug+"-notes.md"
	} else {
		text, name = notes.AllMarkdown(snap, private, time.Now(), zone), "bookshelf-notes.md"
	}
	if text == "" {
		text = "(No notes to export.)\n"
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	if q.Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.NewReplacer(`"`, "", `\`, "").Replace(name)+`"`)
	}
	io.WriteString(w, text)
}

// readerZone is the time zone the browser says its reader is in (tz), or UTC.
func readerZone(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil && tz != "" {
		return loc
	}
	return time.UTC
}

const maxClippings = 32 << 20

type importGroup struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	BookID string `json:"bookId,omitempty"` // "" when the book isn't on the shelf
	Book   string `json:"book,omitempty"`   // its title on the shelf
	Add    int    `json:"add"`
	Same   int    `json:"same"`
	Total  int    `json:"total"`
}

// importNotes reads a Kindle My Clippings.txt (the request's body) and matches its notes to books on the
// shelf. With preview=1 it only says what it would do; otherwise it adds the notes it hasn't already got,
// public or the owner's alone (public=1).
func (s *Server) importNotes(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxClippings))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "That file is too large to be a clippings file.")
		return
	}
	q := r.URL.Query()
	clips := notes.ParseClippings(string(raw), readerZone(q.Get("tz"))) // Kindle writes its own clock's time
	if len(clips) == 0 {
		fail(w, http.StatusUnprocessableEntity, "That doesn’t look like a Kindle clippings file.")
		return
	}
	plan := notes.Plan(s.Store.Snapshot(), clips)
	var groups []importGroup
	var add, books int
	for _, g := range plan {
		groups = append(groups, importGroup{Title: g.Title, Author: g.Author, BookID: g.BookID, Book: g.BookTitle, Add: len(g.Fresh), Same: g.Same, Total: g.Total})
		add += len(g.Fresh)
		if len(g.Fresh) > 0 {
			books++
		}
	}
	if q.Get("preview") == "1" {
		writeJSON(w, http.StatusOK, map[string]any{"clippings": len(clips), "add": add, "books": books, "groups": groups})
		return
	}
	n, err := s.Store.AddEntries(r.Context(), notes.Entries(plan, q.Get("public") == "1"))
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": n, "books": books, "clippings": len(clips), "groups": groups})
}
