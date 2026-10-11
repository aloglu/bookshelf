package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aloglu/bookshelf/internal/archive"
	"github.com/aloglu/bookshelf/internal/backup"
	"github.com/aloglu/bookshelf/internal/store"
)

// maxImport is the largest file accepted for an import or a restore.
const maxImport = 2 << 30

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	list, err := s.Backups.List()
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) backupNow(w http.ResponseWriter, r *http.Request) {
	e, err := s.Backups.Create(backup.Manual)
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	p, ok := s.Backups.Path(r.PathValue("name"))
	if !ok {
		s.failErr(w, r, store.ErrNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(p)+`"`)
	http.ServeFile(w, r, p)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.Backups.Restore(r.Context(), r.PathValue("name")); err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(true))
}

// upload reads the file sent with a request (multipart field "file", or the
// whole body) into a temporary file, so archives are never held in memory.
func upload(w http.ResponseWriter, r *http.Request) (*os.File, string, error) {
	body := http.MaxBytesReader(w, r.Body, maxImport)
	name := r.URL.Query().Get("name")
	var src io.Reader = body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		mr, err := r.MultipartReader()
		if err != nil {
			return nil, "", &store.InvalidError{Msg: "Choose a file."}
		}
		part, err := mr.NextPart()
		if err != nil {
			return nil, "", &store.InvalidError{Msg: "Choose a file."}
		}
		name, src = part.FileName(), part
	}
	tmp, err := os.CreateTemp("", "bookshelf-upload-*")
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		discard(tmp)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, "", &store.InvalidError{Msg: "The file is larger than 2 GB."}
		}
		return nil, "", err
	}
	return tmp, name, nil
}

// discard closes and removes an uploaded file.
func discard(f *os.File) {
	f.Close()
	os.Remove(f.Name())
}

// slow gives a request that uploads and unpacks a whole library (a restore,
// an import) longer than the server's usual deadlines: an archive with
// thousands of covers can take minutes to send from a phone or to unpack on
// a small computer.
func slow(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	rc.SetReadDeadline(time.Now().Add(time.Hour))
	rc.SetWriteDeadline(time.Now().Add(time.Hour))
}

func (s *Server) restoreUpload(w http.ResponseWriter, r *http.Request) {
	slow(w)
	f, _, err := upload(w, r)
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	defer discard(f)
	info, _ := f.Stat()
	if err := s.Backups.RestoreFrom(r.Context(), f, info.Size()); err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.view(true))
}

func (s *Server) importFile(w http.ResponseWriter, r *http.Request) {
	slow(w)
	f, name, err := upload(w, r)
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	defer discard(f)
	info, _ := f.Stat()
	added, skipped, err := s.Backups.Import(r.Context(), name, f, info.Size())
	if err != nil {
		s.failErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"added": added, "skipped": skipped})
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	snap := s.Store.Snapshot()
	stamp := time.Now().Format("2006-01-02")
	var buf bytes.Buffer
	var name, kind string
	switch r.URL.Query().Get("format") {
	case "csv":
		name, kind = "bookshelf-"+stamp+".csv", "text/csv; charset=utf-8"
		archive.WriteCSV(&buf, snap.Books)
	case "json":
		name, kind = "bookshelf-"+stamp+".json", "application/json"
		archive.WriteJSON(&buf, snap.Books)
	case "archive":
		// Streamed straight to the browser: an archive with covers can be large.
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="bookshelf-`+stamp+archive.Extension+`"`)
		if err := archive.Write(w, snap.Library(), s.CoversDir(), s.Version, time.Now()); err != nil {
			s.Log.Error("export failed", "err", err)
		}
		return
	default:
		fail(w, http.StatusUnprocessableEntity, "Choose csv, json or archive.")
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Write(buf.Bytes())
}
