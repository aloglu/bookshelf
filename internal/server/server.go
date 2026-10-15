// Package server serves the shelf, its admin and the JSON API from one address.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aloglu/bookshelf/internal/auth"
	"github.com/aloglu/bookshelf/internal/backup"
	"github.com/aloglu/bookshelf/internal/lookup"
	"github.com/aloglu/bookshelf/internal/store"
	"github.com/aloglu/bookshelf/internal/updates"
)

// Server is the HTTP side of Bookshelf.
type Server struct {
	Store   *store.Store
	Auth    *auth.Auth
	Lookup  *lookup.Client
	Backups *backup.Backups
	Updates *updates.Checker // nil: no update check
	DataDir string
	Version string
	Web     fs.FS // the built frontend; nil serves a placeholder
	Log     *slog.Logger

	views sync.Map // cached library views, by snapshot and audience
}

// CoversDir holds every cover and thumbnail.
func (s *Server) CoversDir() string { return filepath.Join(s.DataDir, "covers") }

// IconDir holds the owner's own icon, if there is one.
func (s *Server) IconDir() string { return filepath.Join(s.DataDir, "icon") }

// Handler is every route.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	mux.HandleFunc("GET /api/session", s.getSession)
	mux.HandleFunc("POST /api/session", s.signIn)
	mux.HandleFunc("DELETE /api/session", s.owner(s.signOut))

	mux.HandleFunc("GET /api/library", s.getLibrary)
	mux.HandleFunc("GET /covers/{name}", s.getCover)
	mux.HandleFunc("GET /icon/{name}", s.getIcon)
	mux.HandleFunc("GET /manifest.json", s.manifest)
	mux.HandleFunc("PUT /api/icon", s.owner(s.putIcon))
	mux.HandleFunc("DELETE /api/icon", s.owner(s.deleteIcon))

	mux.HandleFunc("POST /api/books", s.owner(s.addBook))
	mux.HandleFunc("PATCH /api/books/{id}", s.owner(s.editBook))
	mux.HandleFunc("DELETE /api/books/{id}", s.owner(s.removeBook))
	mux.HandleFunc("POST /api/books/batch", s.owner(s.batch))
	mux.HandleFunc("PUT /api/books/{id}/cover", s.owner(s.putCover))
	mux.HandleFunc("DELETE /api/books/{id}/cover", s.owner(s.deleteCover))
	mux.HandleFunc("PUT /api/books/{id}/spine", s.owner(s.putSpine))
	mux.HandleFunc("GET /api/lookup", s.owner(s.lookup))

	mux.HandleFunc("GET /api/readings", s.owner(s.listReadings))
	mux.HandleFunc("POST /api/readings", s.owner(s.addReading))
	mux.HandleFunc("PATCH /api/readings/{id}", s.owner(s.editReading))
	mux.HandleFunc("DELETE /api/readings/{id}", s.owner(s.removeReading))

	mux.HandleFunc("GET /api/books/{id}/entries", s.listEntries)
	mux.HandleFunc("PATCH /api/books/{id}/notes", s.owner(s.setNotesHidden))
	mux.HandleFunc("GET /api/notes", s.listNotes)
	mux.HandleFunc("GET /api/notes/export", s.owner(s.exportNotes))
	mux.HandleFunc("POST /api/notes/import", s.owner(s.importNotes))
	mux.HandleFunc("POST /api/books/{id}/entries", s.owner(s.addEntry))
	mux.HandleFunc("PATCH /api/entries/{id}", s.owner(s.editEntry))
	mux.HandleFunc("DELETE /api/entries/{id}", s.owner(s.removeEntry))

	mux.HandleFunc("GET /api/backups", s.owner(s.listBackups))
	mux.HandleFunc("POST /api/backups", s.owner(s.backupNow))
	mux.HandleFunc("GET /api/backups/{name}", s.owner(s.downloadBackup))
	mux.HandleFunc("POST /api/backups/{name}/restore", s.owner(s.restoreBackup))
	mux.HandleFunc("POST /api/restore", s.owner(s.restoreUpload))
	mux.HandleFunc("POST /api/import", s.owner(s.importFile))
	mux.HandleFunc("GET /api/export", s.owner(s.export))

	mux.HandleFunc("GET /api/settings", s.owner(s.getSettings))
	mux.HandleFunc("GET /api/updates", s.owner(s.getUpdates))
	mux.HandleFunc("PATCH /api/settings", s.owner(s.patchSettings))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "No such address.") })
	mux.HandleFunc("/", s.app)
	return s.guard(mux)
}

// contentPolicy lets the page run only its own scripts (and the barcode
// reader's WebAssembly), talk only to this server, and show covers from here,
// from the lookup's https hosts, or drawn in the browser. Inline styles are
// allowed: every spine carries its own colors and size.
const contentPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; media-src 'self' blob:; font-src 'self'; connect-src 'self'; " +
	"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'"

// guard adds the headers every answer carries, refuses changes that don't
// come from this site's own pages, and logs failures.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Content-Security-Policy", contentPolicy)
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.sameOrigin(r) {
			fail(w, http.StatusForbidden, "This change didn’t come from Bookshelf’s own page.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin is the CSRF check: a change must come from a page of this site.
// Browsers say where a request comes from in Sec-Fetch-Site, which pages can't
// forge and which holds behind any proxy (it compares the page with the address
// the browser asked for). Browsers too old to send it are checked by Origin
// against the host; tools that send neither (curl) carry no visitor's cookies.
func (s *Server) sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := r.Host
	if s.Auth.Trusted(r) {
		if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
			host = strings.TrimSpace(strings.Split(fh, ",")[0])
		}
	}
	return strings.EqualFold(u.Host, host)
}

// owner lets a request through only when it carries a valid session.
func (s *Server) owner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.Auth.Session(r); !ok {
			fail(w, http.StatusUnauthorized, "Sign in first.")
			return
		}
		next(w, r)
	}
}

func (s *Server) isOwner(r *http.Request) bool {
	_, ok := s.Auth.Session(r)
	return ok
}

// ---------------------------------------------------------------- responses

type apiError struct {
	Error string `json:"error"`
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &store.InvalidError{Msg: "The request isn’t valid: " + err.Error()}
	}
	return nil
}

// failErr answers with the right status for a store or lookup error.
func (s *Server) failErr(w http.ResponseWriter, r *http.Request, err error) {
	var inv *store.InvalidError
	switch {
	case errors.As(err, &inv):
		fail(w, http.StatusUnprocessableEntity, inv.Msg)
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "That isn’t in the library.")
	case errors.Is(err, store.ErrDuplicateISBN), errors.Is(err, store.ErrDuplicateIssue):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, context.Canceled):
	default:
		s.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		fail(w, http.StatusInternalServerError, "Something went wrong. The log has the details.")
	}
}

// ---------------------------------------------------------------- the app

func (s *Server) app(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Web == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!doctype html><title>Bookshelf</title><p>Bookshelf " + s.Version + " is running. The web interface is not built into this program.</p>"))
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name != "" && name != "index.html" {
		if f, err := s.Web.Open(name); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") { // hashed names never change
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, s.Web, name)
				return
			}
		}
	}
	// Every other address (a book's own address, /stats, /signin) is the app.
	s.page(w, r)
}

// timeNow is replaceable in tests.
var timeNow = time.Now
