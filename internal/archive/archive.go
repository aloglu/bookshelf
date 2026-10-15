// Package archive reads and writes Bookshelf archives (.bookshelf files), the
// one format used for backups and for full exports: a zip holding
// manifest.json, library.json (books, readings, notes and highlights, and
// settings; never sessions) and every cover under covers/.
package archive

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aloglu/bookshelf/internal/covers"
	"github.com/aloglu/bookshelf/internal/store"
)

// bad is a problem with the owner's file, which they can correct.
func bad(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return &store.InvalidError{Msg: strings.ToUpper(msg[:1]) + msg[1:] + "."}
}

// Format is this archive layout's version: 3 since Bookshelf 2.1's notes, which
// are one kind of entry (words before a passage, the passage, words after it,
// and a place). Format 2 (an early 2.1) had notes and highlights, with a page;
// format 1 (2.0) kept one note on each finished reading. Both are read as
// entries.
const Format = 3

// Extension is an archive's file extension.
const Extension = ".bookshelf"

const (
	maxEntries  = 100_000
	maxUnpacked = 8 << 30 // 8 GB in all
	maxJSON     = 256 << 20
)

// Manifest describes an archive.
type Manifest struct {
	Kind      string    `json:"kind"` // always "bookshelf"
	Format    int       `json:"format"`
	Version   string    `json:"version"` // the Bookshelf that wrote it
	CreatedAt time.Time `json:"createdAt"`
	Books     int       `json:"books"`
}

// ErrNotArchive means the file isn't a Bookshelf archive.
var ErrNotArchive error = &store.InvalidError{Msg: "This isn’t a Bookshelf archive."}

// Write writes lib and the covers it uses (read from coversDir) to w.
func Write(w io.Writer, lib store.Library, coversDir, version string, now time.Time) error {
	z := zip.NewWriter(w)
	put := func(name string, modified time.Time) (io.Writer, error) {
		return z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified})
	}
	m := Manifest{Kind: "bookshelf", Format: Format, Version: version, CreatedAt: now.UTC(), Books: len(lib.Books)}
	for _, entry := range []struct {
		name string
		v    any
	}{{"manifest.json", m}, {"library.json", lib}} {
		f, err := put(entry.name, now)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entry.v); err != nil {
			return err
		}
	}
	for _, b := range lib.Books {
		if b.Cover == "" {
			continue
		}
		for _, name := range []string{b.Cover, covers.ThumbName(b.Cover)} {
			if err := addFile(z, put, coversDir, name); err != nil {
				return err
			}
		}
	}
	return z.Close()
}

func addFile(z *zip.Writer, put func(string, time.Time) (io.Writer, error), dir, name string) error {
	f, err := os.Open(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil // a missing cover file shouldn't stop a backup
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	// Covers are already compressed; store them as they are.
	out, err := z.CreateHeader(&zip.FileHeader{Name: "covers/" + name, Method: zip.Store, Modified: info.ModTime()})
	if err != nil {
		return err
	}
	_, err = io.Copy(out, f)
	return err
}

// Archive is an opened archive.
type Archive struct {
	Manifest Manifest
	Library  store.Library
	covers   map[string]*zip.File
}

// Open reads an archive's manifest and library; covers are extracted later
// with Extract.
func Open(r io.ReaderAt, size int64) (*Archive, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, ErrNotArchive
	}
	if len(z.File) > maxEntries {
		return nil, bad("the archive holds too many files")
	}
	a := &Archive{covers: map[string]*zip.File{}}
	var total uint64
	var manifest, library *zip.File
	for _, f := range z.File {
		total += f.UncompressedSize64
		switch {
		case f.Name == "manifest.json":
			manifest = f
		case f.Name == "library.json":
			library = f
		case strings.HasPrefix(f.Name, "covers/") && !f.FileInfo().IsDir():
			name := path.Base(f.Name)
			// Only plain file names, never paths: nothing can be written outside the covers folder.
			if name != strings.TrimPrefix(f.Name, "covers/") || name == "." || name == ".." || strings.ContainsAny(name, `\/:`) || !strings.HasSuffix(name, ".webp") {
				return nil, bad("the archive holds an unexpected file %q", f.Name)
			}
			a.covers[name] = f
		}
	}
	if total > maxUnpacked {
		return nil, bad("the archive unpacks to more than %d GB", maxUnpacked>>30)
	}
	if manifest == nil || library == nil {
		return nil, ErrNotArchive
	}
	if err := readJSON(manifest, &a.Manifest); err != nil || a.Manifest.Kind != "bookshelf" {
		return nil, ErrNotArchive
	}
	if a.Manifest.Format > Format {
		return nil, bad("this archive was made by a newer Bookshelf (%s); update Bookshelf first", a.Manifest.Version)
	}
	a.Library.Settings = store.DefaultSettings()
	if err := readJSON(library, &a.Library); err != nil {
		return nil, bad("the archive’s library can’t be read (%v)", err)
	}
	switch {
	case a.Manifest.Format == 2:
		var old struct {
			Entries []struct {
				BookID    string    `json:"bookId"`
				ReadingID string    `json:"readingId"`
				Kind      string    `json:"kind"`
				Text      string    `json:"text"`
				Quote     string    `json:"quote"`
				Page      *int      `json:"page"`
				Public    bool      `json:"public"`
				CreatedAt time.Time `json:"createdAt"`
				UpdatedAt time.Time `json:"updatedAt"`
			} `json:"entries"`
		}
		if err := readJSON(library, &old); err != nil {
			return nil, bad("the archive’s library can’t be read (%v)", err)
		}
		a.Library.Entries = nil
		for _, e := range old.Entries {
			n := store.Entry{BookID: e.BookID, ReadingID: e.ReadingID, Text: e.Text, Public: e.Public, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
			if e.Kind == "highlight" {
				n.Quote = e.Quote
			}
			if e.Page != nil && e.Kind == "highlight" {
				n.Where = &store.Where{Kind: store.WherePage, N: *e.Page}
			}
			a.Library.Entries = append(a.Library.Entries, n)
		}
	case a.Manifest.Format < 2:
		var old struct {
			Readings []struct {
				ID     string `json:"id"`
				BookID string `json:"bookId"`
				Note   *struct {
					Text      string    `json:"text"`
					Public    bool      `json:"public"`
					CreatedAt time.Time `json:"createdAt"`
					UpdatedAt time.Time `json:"updatedAt"`
				} `json:"note"`
			} `json:"readings"`
		}
		if err := readJSON(library, &old); err != nil {
			return nil, bad("the archive’s library can’t be read (%v)", err)
		}
		for _, r := range old.Readings {
			if n := r.Note; n != nil && strings.TrimSpace(n.Text) != "" {
				a.Library.Entries = append(a.Library.Entries, store.Entry{BookID: r.BookID, ReadingID: r.ID,
					Text: n.Text, Public: n.Public, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt})
			}
		}
	}
	return a, nil
}

func readJSON(f *zip.File, v any) error {
	if f.UncompressedSize64 > maxJSON {
		return fmt.Errorf("%s is too large", f.Name)
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(io.LimitReader(rc, maxJSON)).Decode(v)
}

// Extract copies the archive's covers that the library uses into dir,
// skipping those already there (cover names are unique). It returns the
// files it wrote, so a failed restore or import can remove them.
func (a *Archive) Extract(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	want := map[string]bool{}
	for _, b := range a.Library.Books {
		if b.Cover != "" {
			want[b.Cover], want[covers.ThumbName(b.Cover)] = true, true
		}
	}
	for name, f := range a.covers {
		if !want[name] {
			continue
		}
		dest := filepath.Join(dir, name)
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if err := extractOne(f, dest); err != nil {
			return written, err
		}
		written = append(written, name)
	}
	return written, nil
}

func extractOne(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".restore-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(rc, int64(f.UncompressedSize64)+1)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	os.Chtimes(tmp.Name(), f.Modified, f.Modified)
	return os.Rename(tmp.Name(), dest)
}
