// Package backup keeps a library's backups in the data folder, restores
// them, imports other files, and tidies the covers folder.
package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aloglu/bookshelf/internal/archive"
	"github.com/aloglu/bookshelf/internal/covers"
	"github.com/aloglu/bookshelf/internal/store"
)

// Kinds of backup.
const (
	Automatic     = "automatic"
	Manual        = "manual"
	BeforeImport  = "before-import"
	BeforeRestore = "before-restore"
)

var kinds = []string{Automatic, Manual, BeforeImport, BeforeRestore}

// Entry is one backup file.
type Entry struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
	Books     int       `json:"books"`
	Size      int64     `json:"size"`
}

// Backups manages one library's backups.
type Backups struct {
	Dir       string // the backups folder
	CoversDir string
	Store     *store.Store
	Version   string
	Log       *slog.Logger

	mu  sync.Mutex // one backup, restore or import at a time
	now func() time.Time
}

// New makes a Backups for a data folder.
func New(dataDir string, st *store.Store, version string, log *slog.Logger) *Backups {
	return &Backups{Dir: filepath.Join(dataDir, "backups"), CoversDir: filepath.Join(dataDir, "covers"), Store: st, Version: version, Log: log, now: time.Now}
}

const layout = "2006-01-02-150405"

func (b *Backups) name(kind string, at time.Time) string {
	return "bookshelf-" + at.UTC().Format(layout) + "-" + kind + archive.Extension
}

func parseName(name string) (kind string, at time.Time, ok bool) {
	rest, found := strings.CutPrefix(name, "bookshelf-")
	if !found || !strings.HasSuffix(rest, archive.Extension) || len(rest) < len(layout)+2 {
		return "", time.Time{}, false
	}
	at, err := time.Parse(layout, rest[:len(layout)])
	kind = strings.TrimSuffix(rest[len(layout)+1:], archive.Extension)
	return kind, at, err == nil && slices.Contains(kinds, kind)
}

// Create writes a backup of the library as it is now, then removes the
// oldest backups beyond the number to keep.
func (b *Backups) Create(kind string) (Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.create(kind)
}

func (b *Backups) create(kind string) (Entry, error) {
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		return Entry{}, err
	}
	now := b.now()
	name := b.name(kind, now)
	for i := 1; fileExists(filepath.Join(b.Dir, name)); i++ { // two within a second
		now = now.Add(time.Second)
		name = b.name(kind, now)
	}
	snap := b.Store.Snapshot() // immutable: the backup is of one consistent moment
	tmp, err := os.CreateTemp(b.Dir, ".backup-*")
	if err != nil {
		return Entry{}, err
	}
	defer os.Remove(tmp.Name())
	if err := archive.Write(tmp, snap.Library(), b.CoversDir, b.Version, now); err != nil {
		tmp.Close()
		return Entry{}, fmt.Errorf("write backup: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return Entry{}, err
	}
	if err := tmp.Close(); err != nil {
		return Entry{}, err
	}
	path := filepath.Join(b.Dir, name)
	if err := os.Rename(tmp.Name(), path); err != nil {
		return Entry{}, err
	}
	os.Chtimes(path, now, now)
	b.prune(snap.Settings.BackupKeep)
	info, _ := os.Stat(path)
	return Entry{Name: name, Kind: kind, CreatedAt: now.UTC().Truncate(time.Second), Books: len(snap.Books), Size: info.Size()}, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// prune keeps the newest keep backups.
func (b *Backups) prune(keep int) {
	list, err := b.list()
	if err != nil || len(list) <= keep {
		return
	}
	for _, e := range list[keep:] {
		if err := os.Remove(filepath.Join(b.Dir, e.Name)); err != nil && b.Log != nil {
			b.Log.Warn("removing an old backup", "file", e.Name, "err", err)
		}
	}
}

// List is every backup, newest first.
func (b *Backups) List() ([]Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.list()
}

func (b *Backups) list() ([]Entry, error) {
	entries, err := os.ReadDir(b.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Entry{}
	for _, de := range entries {
		kind, at, ok := parseName(de.Name())
		if !ok || de.IsDir() {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		e := Entry{Name: de.Name(), Kind: kind, CreatedAt: at, Size: info.Size()}
		if f, err := os.Open(filepath.Join(b.Dir, de.Name())); err == nil {
			if a, err := archive.Open(f, info.Size()); err == nil {
				e.Books = a.Manifest.Books
			}
			f.Close()
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Path is a backup's file, if name is one of the backups.
func (b *Backups) Path(name string) (string, bool) {
	if _, _, ok := parseName(name); !ok || name != filepath.Base(name) {
		return "", false
	}
	p := filepath.Join(b.Dir, name)
	return p, fileExists(p)
}

// Restore replaces the library with a backup, after backing up what is there
// now. If anything goes wrong the library is left as it was.
func (b *Backups) Restore(ctx context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.Path(name)
	if !ok {
		return store.ErrNotFound
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return b.restore(ctx, f, info.Size())
}

// RestoreFrom replaces the library with an archive the owner uploaded.
func (b *Backups) RestoreFrom(ctx context.Context, r io.ReaderAt, size int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.restore(ctx, r, size)
}

func (b *Backups) restore(ctx context.Context, r io.ReaderAt, size int64) error {
	a, err := archive.Open(r, size)
	if err != nil {
		return err
	}
	if _, err := b.create(BeforeRestore); err != nil {
		return fmt.Errorf("back up before restoring: %w", err)
	}
	written, err := a.Extract(b.CoversDir)
	if err != nil {
		b.removeCovers(written)
		return fmt.Errorf("restore covers: %w", err)
	}
	if err := b.Store.Replace(ctx, a.Library); err != nil {
		b.removeCovers(written)
		return err
	}
	b.cleanCovers(0)
	return nil
}

func (b *Backups) removeCovers(names []string) {
	for _, n := range names {
		os.Remove(filepath.Join(b.CoversDir, n))
	}
}

// Import adds the books in a file (a .bookshelf archive, or CSV or JSON)
// that aren't on the shelf yet, after backing up the library. It returns
// how many books were added and how many were already there. The file is
// read where it lies, never whole into memory.
func (b *Backups) Import(ctx context.Context, filename string, r io.ReaderAt, size int64) (added, skipped int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	backUp := func() error {
		if _, err := b.create(BeforeImport); err != nil {
			return fmt.Errorf("back up before importing: %w", err)
		}
		return nil
	}
	head := make([]byte, 512)
	n, _ := r.ReadAt(head, 0)
	head = head[:n]
	var lib store.Library
	var written []string
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case ext == archive.Extension || ext == ".zip" || bytes.HasPrefix(head, []byte("PK\x03\x04")):
		a, err := archive.Open(r, size)
		if err != nil {
			return 0, 0, err
		}
		lib = a.Library
		if err := backUp(); err != nil {
			return 0, 0, err
		}
		if written, err = a.Extract(b.CoversDir); err != nil {
			b.removeCovers(written)
			return 0, 0, fmt.Errorf("import covers: %w", err)
		}
	default:
		read := archive.ReadCSV
		if ext == ".json" || bytes.HasPrefix(bytes.TrimSpace(head), []byte("[")) {
			read = archive.ReadJSON
		}
		if lib.Books, err = read(io.NewSectionReader(r, 0, size)); err != nil {
			return 0, 0, err
		}
		if err := backUp(); err != nil { // now that the file is known to be readable
			return 0, 0, err
		}
	}
	added, skipped, err = b.Store.Merge(ctx, lib)
	if err != nil {
		b.removeCovers(written)
		return 0, 0, err
	}
	b.cleanCovers(0) // covers of books that were skipped
	return added, skipped, nil
}

// CleanCovers removes cover files no book uses, older than age (a cover
// uploaded in a form that was then cancelled, or left by a failed change).
func (b *Backups) CleanCovers(age time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanCovers(age)
}

func (b *Backups) cleanCovers(age time.Duration) {
	used := map[string]bool{}
	for _, bk := range b.Store.Snapshot().Books {
		if bk.Cover != "" {
			used[bk.Cover], used[covers.ThumbName(bk.Cover)] = true, true
		}
	}
	entries, err := os.ReadDir(b.CoversDir)
	if err != nil {
		return
	}
	cutoff := b.now().Add(-age)
	for _, de := range entries {
		if used[de.Name()] || de.IsDir() {
			continue
		}
		info, err := de.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(b.CoversDir, de.Name())); err == nil && b.Log != nil {
			b.Log.Info("removed an unused cover", "file", de.Name())
		}
	}
}

// Due reports whether a scheduled backup should be made now.
func (b *Backups) Due() bool {
	every := b.Store.Snapshot().Settings.BackupEvery
	var gap time.Duration
	switch every {
	case "day":
		gap = 24 * time.Hour
	case "week":
		gap = 7 * 24 * time.Hour
	default:
		return false
	}
	list, err := b.List()
	if err != nil {
		return false
	}
	for _, e := range list {
		if e.Kind == Automatic || e.Kind == Manual {
			return b.now().Sub(e.CreatedAt) >= gap
		}
	}
	return len(b.Store.Snapshot().Books) > 0 // the first backup, once there is something to keep
}

// Run makes scheduled backups and tidies the covers folder until ctx ends.
func (b *Backups) Run(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if b.Due() {
			if _, err := b.Create(Automatic); err != nil && b.Log != nil {
				b.Log.Error("scheduled backup failed", "err", err)
			}
		}
		b.CleanCovers(24 * time.Hour)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
