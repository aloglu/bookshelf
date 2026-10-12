package server

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/aloglu/bookshelf/internal/covers"
	"github.com/aloglu/bookshelf/internal/store"
)

// The library's icon is Bookshelf's own (in the built frontend) or one the
// owner chose. Browsers read a tab's icon and a home screen's before any
// script runs, so the page itself is written with the chosen icon, and so is
// the web app manifest, which also carries the library's name.

func (s *Server) putIcon(w http.ResponseWriter, r *http.Request) {
	var src io.Reader = http.MaxBytesReader(w, r.Body, covers.MaxUpload+1)
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "multipart/form-data" {
		f, _, err := r.FormFile("icon")
		if err != nil {
			fail(w, http.StatusUnprocessableEntity, "Choose a picture for the icon.")
			return
		}
		defer f.Close()
		src = f
	}
	name, err := covers.SaveIcon(s.IconDir(), src)
	if err != nil {
		if errors.Is(err, covers.ErrNotImage) || strings.Contains(err.Error(), "larger than") || strings.Contains(err.Error(), "pixels on a side") {
			fail(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.failErr(w, r, err)
		return
	}
	old := s.Store.Snapshot().Settings.Icon
	st, err := s.Store.UpdateSettings(r.Context(), func(st *store.Settings) { st.Icon = name })
	if err != nil {
		covers.RemoveIcon(s.IconDir(), name)
		s.failErr(w, r, err)
		return
	}
	covers.RemoveIcon(s.IconDir(), old)
	writeJSON(w, http.StatusOK, st)
}

// deleteIcon goes back to Bookshelf's own icon.
func (s *Server) deleteIcon(w http.ResponseWriter, r *http.Request) {
	old := s.Store.Snapshot().Settings.Icon
	st, err := s.Store.UpdateSettings(r.Context(), func(st *store.Settings) { st.Icon = "" })
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	covers.RemoveIcon(s.IconDir(), old)
	writeJSON(w, http.StatusOK, st)
}

// getIcon serves the owner's icon. Its file names change with every new icon,
// so they can be cached for good.
func (s *Server) getIcon(w http.ResponseWriter, r *http.Request) {
	name, current := r.PathValue("name"), s.Store.Snapshot().Settings.Icon
	for _, size := range covers.IconSizes {
		if current != "" && name == covers.IconFile(current, size) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Set("Content-Type", "image/png")
			http.ServeFile(w, r, filepath.Join(s.IconDir(), name))
			return
		}
	}
	http.NotFound(w, r)
}

// iconURLs are the icon's addresses by size: the owner's, or Bookshelf's own.
func (s *Server) iconURLs() map[int]string {
	if name := s.Store.Snapshot().Settings.Icon; name != "" {
		urls := map[int]string{}
		for _, size := range covers.IconSizes {
			urls[size] = "/icon/" + covers.IconFile(name, size)
		}
		return urls
	}
	return map[int]string{32: "/favicon-32.png", 180: "/apple-touch-icon.png", 192: "/icon-192.png", 512: "/icon-512.png"}
}

// manifest describes Bookshelf to a phone that puts it on its home screen:
// the library's name and its icon.
func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	urls, own := s.iconURLs(), s.Store.Snapshot().Settings.Icon != ""
	icons := []map[string]string{
		{"src": urls[192], "sizes": "192x192", "type": "image/png"},
		{"src": urls[512], "sizes": "512x512", "type": "image/png"},
	}
	if !own { // Bookshelf's own icon has a version made for Android's shapes
		icons = append(icons, map[string]string{"src": "/icon-maskable-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"})
	}
	name := s.Store.Snapshot().Settings.Name
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(map[string]any{
		"name": name, "short_name": name, "start_url": "/", "scope": "/", "display": "standalone",
		"background_color": "#17110d", "icons": icons,
	})
}

// page is the app's page, written with the library's name and, if the owner
// chose one, their icon.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	raw, err := fs.ReadFile(s.Web, "index.html")
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	page := strings.Replace(string(raw), "<title>Bookshelf</title>", "<title>"+html.EscapeString(s.Store.Snapshot().Settings.Name)+"</title>", 1)
	if s.Store.Snapshot().Settings.Icon != "" {
		urls := s.iconURLs()
		page = strings.NewReplacer(
			`  <link rel="icon" href="/favicon.svg" type="image/svg+xml">`+"\n", "", // the owner's icon has no SVG version
			`href="/favicon-32.png"`, `href="`+urls[32]+`"`,
			`href="/apple-touch-icon.png"`, `href="`+urls[180]+`"`,
		).Replace(page)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	io.WriteString(w, page)
}
