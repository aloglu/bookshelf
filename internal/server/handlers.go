package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aloglu/bookshelf/internal/auth"
	"github.com/aloglu/bookshelf/internal/covers"
	"github.com/aloglu/bookshelf/internal/isbn"
	"github.com/aloglu/bookshelf/internal/lookup"
	"github.com/aloglu/bookshelf/internal/store"
	"github.com/aloglu/bookshelf/internal/updates"
)

// ---------------------------------------------------------------- session

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	if s.isOwner(r) {
		writeJSON(w, http.StatusOK, map[string]any{"owner": true, "user": s.Auth.User()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"owner": false})
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	var in struct{ User, Password string }
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	token, err := s.Auth.SignIn(r.Context(), s.Auth.Client(r), strings.TrimSpace(in.User), in.Password, r.UserAgent())
	var locked *auth.LockedError
	switch {
	case errors.As(err, &locked):
		minutes := int(locked.Wait.Minutes()) + 1
		fail(w, http.StatusTooManyRequests, "Too many wrong passwords. Try again in "+plural(minutes, "minute")+".")
		return
	case errors.Is(err, auth.ErrWrong):
		fail(w, http.StatusUnauthorized, "That user name and password don’t match.")
		return
	case err != nil:
		s.failErr(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: s.https(r), MaxAge: int(auth.SessionAge / time.Second)})
	writeJSON(w, http.StatusOK, map[string]any{"owner": true, "user": s.Auth.User()})
}

func (s *Server) https(r *http.Request) bool {
	return r.TLS != nil || (s.Auth.Trusted(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}

func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	if h, ok := s.Auth.Session(r); ok {
		if err := s.Auth.SignOut(r.Context(), h); err != nil {
			s.failErr(w, r, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"owner": false})
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// ---------------------------------------------------------------- books

type bookIn struct {
	Title      string `json:"title"`
	Author     string `json:"author"`
	Translator string `json:"translator"`
	Publisher  string `json:"publisher"`
	Published  *int   `json:"published"`
	Pages      *int   `json:"pages"`
	Binding    string `json:"binding"`
	Identifier string `json:"identifier"` // the one “ISBN or ISSN” field, as typed or scanned
	Issue      string `json:"issue"`      // a periodical's issue
	Hidden     bool   `json:"hidden"`
	CoverURL   string `json:"coverUrl"` // adding only: a cover found by the lookup
}

func (in bookIn) input() (store.BookInput, error) {
	id, err := isbn.Parse(in.Identifier)
	if err != nil {
		return store.BookInput{}, &store.InvalidError{Msg: "That ISBN or ISSN doesn’t add up. Check the digits."}
	}
	return store.BookInput{Title: in.Title, Author: in.Author, Translator: in.Translator, Publisher: in.Publisher,
		Published: in.Published, Pages: in.Pages, Binding: in.Binding, ISBN: id.ISBN, ISSN: id.ISSN, Issue: in.Issue,
		Hidden: in.Hidden}, nil
}

func (s *Server) book(id string) bookView {
	for _, b := range s.view(true).Books {
		if b.ID == id {
			return b
		}
	}
	return bookView{}
}

func (s *Server) addBook(w http.ResponseWriter, r *http.Request) {
	var in bookIn
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	bi, err := in.input()
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	b, err := s.Store.AddBook(r.Context(), bi)
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	if in.CoverURL != "" {
		// A cover that can't be fetched doesn't stop the book being added;
		// the book gets a plain cloth cover and the owner can add one later.
		if err := s.fetchCover(r, b.ID, in.CoverURL); err != nil {
			s.Log.Warn("cover not fetched", "book", b.ID, "err", err)
		}
	}
	writeJSON(w, http.StatusCreated, s.book(b.ID))
}

func (s *Server) fetchCover(r *http.Request, id, url string) error {
	body, err := s.Lookup.FetchCover(r.Context(), url)
	if err != nil {
		return err
	}
	defer body.Close()
	return s.saveCover(r, id, body)
}

func (s *Server) saveCover(r *http.Request, id string, src io.Reader) error {
	res, err := covers.Save(s.CoversDir(), id, src)
	if err != nil {
		return err
	}
	old, err := s.Store.SetCover(r.Context(), id, res.Name, res.Spine, res.SpineText)
	if err != nil {
		covers.Remove(s.CoversDir(), res.Name)
		return err
	}
	return covers.Remove(s.CoversDir(), old)
}

func (s *Server) editBook(w http.ResponseWriter, r *http.Request) {
	var in bookIn
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	bi, err := in.input()
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	if _, err := s.Store.UpdateBook(r.Context(), r.PathValue("id"), bi); err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.book(r.PathValue("id")))
}

func (s *Server) removeBook(w http.ResponseWriter, r *http.Request) {
	s.removeBooks(w, r, []string{r.PathValue("id")})
}

func (s *Server) removeBooks(w http.ResponseWriter, r *http.Request, ids []string) {
	files, err := s.Store.DeleteBooks(r.Context(), ids)
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	for _, f := range files {
		if err := covers.Remove(s.CoversDir(), f); err != nil {
			s.Log.Warn("cover not removed", "file", f, "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) batch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"` // hide, show, remove
	}
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	switch in.Action {
	case "hide", "show":
		if err := s.Store.SetHidden(r.Context(), in.IDs, in.Action == "hide"); err != nil {
			s.failErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "remove":
		s.removeBooks(w, r, in.IDs)
	default:
		fail(w, http.StatusUnprocessableEntity, "Choose hide, show or remove.")
	}
}

func (s *Server) putCover(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Store.Snapshot().Book(id); !ok {
		s.failErr(w, r, store.ErrNotFound)
		return
	}
	var src io.Reader = http.MaxBytesReader(w, r.Body, covers.MaxUpload+1)
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "multipart/form-data" {
		f, _, err := r.FormFile("cover")
		if err != nil {
			fail(w, http.StatusUnprocessableEntity, "Choose an image for the cover.")
			return
		}
		defer f.Close()
		src = f
	}
	if err := s.saveCover(r, id, src); err != nil {
		if errors.Is(err, covers.ErrNotImage) || strings.Contains(err.Error(), "covers can be up to") || strings.Contains(err.Error(), "larger than") {
			fail(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.book(id))
}

func (s *Server) deleteCover(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	old, err := s.Store.SetCover(r.Context(), id, "", "", "")
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	covers.Remove(s.CoversDir(), old)
	writeJSON(w, http.StatusOK, s.book(id))
}

func (s *Server) putSpine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Color string `json:"color"` // "" goes back to the cover's own color
	}
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	if err := s.Store.SetSpineOverride(r.Context(), r.PathValue("id"), in.Color); err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.book(r.PathValue("id")))
}

// lookup finds a book's details by its ISBN. A periodical's ISSN (or its
// 977 barcode) can't be looked up: the answer says so, with the ISSN read,
// so the details can be entered by hand.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) {
	id, err := isbn.Parse(r.URL.Query().Get("id"))
	if err != nil || id == (isbn.ID{}) {
		fail(w, http.StatusUnprocessableEntity, "That isn’t a valid ISBN or ISSN. Check the digits.")
		return
	}
	if id.ISSN != "" {
		writeJSON(w, http.StatusOK, map[string]any{"periodical": true, "issn": id.ISSN, "identifier": id.Display()})
		return
	}
	n := id.ISBN
	for _, b := range s.view(true).Books {
		if b.ISBN == n {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "“" + b.Title + "” is already on your shelf.", "book": b.ID})
			return
		}
	}
	d, err := s.Lookup.Find(r.Context(), n)
	switch {
	case errors.Is(err, lookup.ErrNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, lookup.ErrBusy):
		fail(w, http.StatusServiceUnavailable, "The book sources are busy. Try again in a minute.")
	case err != nil:
		s.Log.Warn("lookup failed", "isbn", n, "err", err)
		fail(w, http.StatusBadGateway, "The book sources couldn’t be reached. Try again in a moment.")
	default:
		d.Binding = store.NormalizeBinding(d.Binding)
		writeJSON(w, http.StatusOK, d)
	}
}

// ---------------------------------------------------------------- readings

type whenIn struct {
	Date      string          `json:"date"` // 2015-06-01, 2015, or ""
	Precision store.Precision `json:"precision"`
}

func (in whenIn) when() (store.When, error) {
	w := store.When{Precision: in.Precision}
	var err error
	switch in.Precision {
	case store.Day:
		w.Time, err = time.Parse("2006-01-02", in.Date)
	case store.Year:
		w.Time, err = time.Parse("2006", in.Date)
	case store.Unknown, "":
		w.Precision = store.Unknown
	default:
		err = errors.New("precision")
	}
	if err != nil {
		return w, &store.InvalidError{Msg: "That date doesn’t look right."}
	}
	return w, nil
}

func (s *Server) listReadings(w http.ResponseWriter, r *http.Request) {
	snap := s.Store.Snapshot()
	book := r.URL.Query().Get("book")
	out := []store.Reading{}
	for _, b := range snap.Books {
		if book == "" || b.ID == book {
			out = append(out, snap.Readings[b.ID]...)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) addReading(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BookID string  `json:"bookId"`
		Read   *whenIn `json:"read"` // present: mark as read, finished then
	}
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	var rd store.Reading
	var err error
	if in.Read != nil {
		var when store.When
		if when, err = in.Read.when(); err == nil {
			rd, err = s.Store.MarkRead(r.Context(), in.BookID, when)
		}
	} else {
		rd, err = s.Store.StartReading(r.Context(), in.BookID)
	}
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, rd)
}

func (s *Server) editReading(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Page    *int    `json:"page"`    // a reading in progress reached this page
		End     string  `json:"end"`     // finished or set_aside: end it now
		Started *whenIn `json:"started"` // correcting a past reading
		Ended   *whenIn `json:"ended"`
		Outcome string  `json:"outcome"`
	}
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	id := r.PathValue("id")
	var rd store.Reading
	var err error
	switch {
	case in.Page != nil:
		rd, err = s.Store.SetPage(r.Context(), id, *in.Page)
	case in.End != "":
		rd, err = s.Store.EndReading(r.Context(), id, in.End)
	case in.Started != nil && in.Ended != nil:
		var st, en store.When
		if st, err = in.Started.when(); err == nil {
			if en, err = in.Ended.when(); err == nil {
				rd, err = s.Store.CorrectReading(r.Context(), id, st, en, in.Outcome)
			}
		}
	default:
		err = &store.InvalidError{Msg: "Nothing to change."}
	}
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rd)
}

func (s *Server) removeReading(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteReading(r.Context(), r.PathValue("id")); err != nil {
		s.failErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) putNote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text   string `json:"text"`
		Public bool   `json:"public"`
	}
	if err := readJSON(r, &in); err != nil {
		s.failErr(w, r, err)
		return
	}
	rd, err := s.Store.SetNote(r.Context(), r.PathValue("id"), in.Text, in.Public)
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rd)
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteNote(r.Context(), r.PathValue("id")); err != nil {
		s.failErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- settings

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Snapshot().Settings)
}

// getUpdates says which version runs and whether a newer one is out.
func (s *Server) getUpdates(w http.ResponseWriter, r *http.Request) {
	st := updates.Status{Version: s.Version}
	if s.Updates != nil {
		st = s.Updates.Status()
	}
	writeJSON(w, http.StatusOK, st)
}

// patchSettings applies the fields given and leaves the rest as they are.
func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "The request is too large.")
		return
	}
	// The fields given are decoded over a copy of the settings, inside the
	// change (so it can't race another one), and only a whole, valid request
	// is applied.
	var bad error
	st, err := s.Store.UpdateSettings(r.Context(), func(st *store.Settings) {
		next := *st
		next.Sources = append([]string(nil), st.Sources...)
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if bad = dec.Decode(&next); bad == nil {
			*st = next
		}
	})
	if bad != nil {
		fail(w, http.StatusUnprocessableEntity, "The request isn’t valid: "+bad.Error())
		return
	}
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	if st.CheckUpdates && s.Updates != nil && !s.Updates.Status().CheckedAt.After(time.Now().Add(-updates.Every)) {
		s.Updates.CheckSoon() // turned on (or never answered): look now rather than in half a day
	}
	writeJSON(w, http.StatusOK, st)
}
