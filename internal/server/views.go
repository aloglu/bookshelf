package server

import (
	"hash/fnv"
	"net/http"
	"path/filepath"
	"time"

	"github.com/aloglu/bookshelf/internal/covers"
	"github.com/aloglu/bookshelf/internal/isbn"
	"github.com/aloglu/bookshelf/internal/store"
)

// A library view is what one audience may see, built once per change.
// Visitors never receive hidden books, the reading history, or the owner-only
// settings; when the library is private they receive nothing.

type bookView struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Author     string     `json:"author"`
	Translator string     `json:"translator"`
	Publisher  string     `json:"publisher"`
	Published  *int       `json:"published"`
	Pages      *int       `json:"pages"`
	Binding    string     `json:"binding"`
	ISBN       string     `json:"isbn"`
	ISSN       string     `json:"issn"`
	Issue      string     `json:"issue"`
	Identifier string     `json:"identifier"` // the ISBN hyphenated, or “ISSN 1300-2880”
	Slug       string     `json:"slug"`
	Address    string     `json:"address"` // the book's own address, in the owner's chosen style
	Cover      *coverView `json:"cover"`
	Spine      spineView  `json:"spine"`
	AddedAt    time.Time  `json:"addedAt"`

	// How many notes and highlights it has: the public ones for visitors,
	// all of them for the owner. They are fetched with the book's notes page.
	Entries int `json:"entries,omitempty"`

	// Owner only.
	Hidden      bool      `json:"hidden,omitempty"`
	NotesHidden bool      `json:"notesHidden,omitempty"` // the book's notes are kept from visitors
	NotesPublic int       `json:"notesPublic,omitempty"` // how many of its notes are marked public
	NotesLast   time.Time `json:"notesLast,omitzero"`    // when its newest note was written
	ReadCount   int       `json:"readCount,omitempty"`
}

type coverView struct {
	URL   string `json:"url"`
	Thumb string `json:"thumb"`
}

type spineView struct {
	Color string `json:"color"`
	Text  string `json:"text"`
	Own   bool   `json:"own"` // set by hand rather than from the cover
}

type readingView struct {
	ID      string     `json:"id"`
	BookID  string     `json:"bookId"`
	Started store.When `json:"started"`
	Page    int        `json:"page"`
}

type libraryView struct {
	Owner    bool          `json:"owner"`
	User     string        `json:"user,omitempty"`
	Settings any           `json:"settings"`
	Books    []bookView    `json:"books"`
	Reading  []readingView `json:"reading"` // books being read now, the most recently started first

	covers map[string]bool // cover and thumbnail files this audience may fetch
}

// publicSettings are the settings a visitor's page needs.
type publicSettings struct {
	Name         string          `json:"name"`
	Sort         string          `json:"sort"`
	SortDesc     bool            `json:"sortDesc"`
	Theme        string          `json:"theme"`
	Footer       bool            `json:"footer"`
	FooterText   string          `json:"footerText"`
	Stats        bool            `json:"stats"`
	Permalink    string          `json:"permalink"`
	Sources      []string        `json:"sources"`
	Social       store.Social    `json:"social"`
	WallDay      string          `json:"wallDay"`
	WallNight    string          `json:"wallNight"`
	WoodDay      string          `json:"woodDay"`
	WoodNight    string          `json:"woodNight"`
	CaseParts    store.CaseParts `json:"caseParts"`
	RibbonDay    string          `json:"ribbonDay"`
	RibbonNight  string          `json:"ribbonNight"`
	Bindings     bool            `json:"bindings"`
	BookSize     int             `json:"bookSize"`
	MaxHeight    int             `json:"maxHeight"`
	Icon         string          `json:"icon"`
	IconShown    bool            `json:"iconShown"`
	ShowReading  bool            `json:"showReading"`
	ShowProgress bool            `json:"showProgress"`
	Notes        bool            `json:"notes"`    // visitors may read public notes
	Timeline     bool            `json:"timeline"` // and all of them together, newest first
}

func public(st store.Settings) publicSettings {
	return publicSettings{st.Name, st.Sort, st.SortDesc, st.Theme, st.Footer, st.FooterText, st.Stats, st.Permalink, st.Sources, st.Social,
		st.WallDay, st.WallNight, st.WoodDay, st.WoodNight, st.CaseParts, st.RibbonDay, st.RibbonNight, st.Bindings, st.BookSize, st.MaxHeight,
		st.Icon, st.IconShown, st.ShowReading, st.ShowProgress, st.Notes, st.Timeline}
}

type viewKey struct {
	snap  *store.Snapshot
	owner bool
}

// view is the library for one audience, cached until the next change.
func (s *Server) view(owner bool) *libraryView {
	snap := s.Store.Snapshot()
	key := viewKey{snap, owner}
	if v, ok := s.views.Load(key); ok {
		return v.(*libraryView)
	}
	v := build(snap, owner)
	if owner {
		v.User = s.Auth.User()
	}
	// Forget views of older snapshots.
	s.views.Range(func(k, _ any) bool {
		if k.(viewKey).snap != snap {
			s.views.Delete(k)
		}
		return true
	})
	s.views.Store(key, v)
	return v
}

// clothColors are the plain cloth covers a book without a cover is bound in.
var clothColors = []string{"#5b2a2a", "#2f4a3a", "#2b3a55", "#6b4a2b", "#463f3a", "#6e3328", "#3c4a4f"}

func build(snap *store.Snapshot, owner bool) *libraryView {
	st := snap.Settings
	v := &libraryView{Owner: owner, Books: []bookView{}, covers: map[string]bool{}}
	if owner {
		v.Settings = st
	} else {
		v.Settings = public(st)
	}
	for _, b := range snap.Books {
		if b.Hidden && !owner {
			continue
		}
		bv := bookView{ID: b.ID, Title: b.Title, Author: b.Author, Translator: b.Translator, Publisher: b.Publisher,
			Published: b.Published, Pages: b.Pages, Binding: b.Binding, ISBN: b.ISBN, ISSN: b.ISSN, Issue: b.Issue,
			Identifier: isbn.ID{ISBN: b.ISBN, ISSN: b.ISSN}.Display(), Slug: b.Slug, AddedAt: b.AddedAt}
		switch {
		case st.Permalink == "title" || b.ISBN == "":
			bv.Address = b.Slug
		case st.Permalink == "hyphens":
			bv.Address = bv.Identifier
		default:
			bv.Address = b.ISBN
		}
		if b.Cover != "" {
			thumb := covers.ThumbName(b.Cover)
			bv.Cover = &coverView{URL: "/covers/" + b.Cover, Thumb: "/covers/" + thumb}
			v.covers[b.Cover], v.covers[thumb] = true, true
		}
		switch {
		case b.SpineColorOverride != "":
			bv.Spine = spineView{Color: b.SpineColorOverride, Text: covers.TextFor(b.SpineColorOverride), Own: true}
		case b.SpineColor != "":
			bv.Spine = spineView{Color: b.SpineColor, Text: b.SpineTextColor}
		default:
			h := fnv.New32a()
			h.Write([]byte(b.ID))
			c := clothColors[h.Sum32()%uint32(len(clothColors))]
			bv.Spine = spineView{Color: c, Text: covers.TextFor(c)}
		}
		for _, e := range snap.Entries[b.ID] {
			if owner || (e.Public && snap.VisitorsReadNotes(b)) {
				bv.Entries++
			}
		}
		if owner {
			bv.Hidden = b.Hidden
			bv.NotesHidden = b.NotesHidden
			for _, e := range snap.Entries[b.ID] {
				if e.Public {
					bv.NotesPublic++
				}
				if e.CreatedAt.After(bv.NotesLast) {
					bv.NotesLast = e.CreatedAt
				}
			}
			for _, r := range snap.Readings[b.ID] {
				if r.Outcome == store.OutcomeFinished {
					bv.ReadCount++
				}
			}
		}
		v.Books = append(v.Books, bv)
	}
	// Visitors are told what is being read only if the owner allows it, and how far along only if that too.
	v.Reading = []readingView{}
	for _, cur := range snap.NowReading() {
		b, ok := snap.Book(cur.BookID)
		if !ok || (!owner && (b.Hidden || !st.ShowReading)) {
			continue
		}
		rv := readingView{ID: cur.ID, BookID: cur.BookID, Started: cur.Started, Page: cur.Page()}
		if !owner && !st.ShowProgress {
			rv.Started, rv.Page = store.When{Precision: store.Unknown}, 0
		}
		v.Reading = append(v.Reading, rv)
	}
	return v
}

func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	owner := s.isOwner(r)
	if !owner && s.Store.Snapshot().Settings.Private {
		writeJSON(w, http.StatusForbidden, map[string]any{"private": true, "name": s.Store.Snapshot().Settings.Name})
		return
	}
	writeJSON(w, http.StatusOK, s.view(owner))
}

// getCover serves a cover or thumbnail, only of a book this audience may see.
// File names change whenever a cover changes, so they can be cached for good.
func (s *Server) getCover(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	owner := s.isOwner(r)
	if name != filepath.Base(name) || (!owner && s.Store.Snapshot().Settings.Private) || !s.view(owner).covers[name] {
		http.NotFound(w, r)
		return
	}
	if owner {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("Content-Type", "image/webp")
	http.ServeFile(w, r, filepath.Join(s.CoversDir(), name))
}
