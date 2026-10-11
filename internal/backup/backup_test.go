package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aloglu/bookshelf/internal/archive"
	"github.com/aloglu/bookshelf/internal/covers"
	"github.com/aloglu/bookshelf/internal/store"
)

var ctx = context.Background()

func setup(t *testing.T) (*Backups, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	b := New(dir, st, "test", nil)
	clock := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	b.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	return b, st, dir
}

func intp(v int) *int { return &v }

func importFile(b *Backups, name string, raw []byte) (added, skipped int, err error) {
	return b.Import(ctx, name, bytes.NewReader(raw), int64(len(raw)))
}

func pngCover() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 150))
	for y := range 150 {
		for x := range 100 {
			img.Set(x, y, color.RGBA{90, 20, 20, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// A library with a covered book, a reading with a page log, a finished
// reading with a note, and changed settings.
func fill(t *testing.T, b *Backups, st *store.Store) (store.Book, store.Book) {
	t.Helper()
	a, _ := st.AddBook(ctx, store.BookInput{Title: "Austerlitz", Author: "W. G. Sebald", ISBN: "9780375756566", Pages: intp(304)})
	res, err := covers.Save(b.CoversDir, a.ID, bytes.NewReader(pngCover()))
	if err != nil {
		t.Fatal(err)
	}
	st.SetCover(ctx, a.ID, res.Name, res.Spine, res.SpineText)
	p, _ := st.AddBook(ctx, store.BookInput{Title: "Pale Fire", Author: "Vladimir Nabokov", Pages: intp(315), Hidden: true})
	r, _ := st.StartReading(ctx, a.ID)
	st.SetPage(ctx, r.ID, 112)
	old, _ := st.MarkRead(ctx, p.ID, store.When{Time: time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC), Precision: store.Year})
	st.SetNote(ctx, old.ID, "A puzzle box.", true)
	st.UpdateSettings(ctx, func(s *store.Settings) { s.Name = "Our books"; s.WoodDay = "cherry"; s.BackupKeep = 3 })
	a, _ = st.Snapshot().Book(a.ID)
	return a, p
}

func TestBackupAndRestoreKeepEverything(t *testing.T) {
	b, st, _ := setup(t)
	a, p := fill(t, b, st)
	e, err := b.Create(Manual)
	if err != nil {
		t.Fatal(err)
	}
	if e.Books != 2 || e.Kind != Manual {
		t.Fatalf("entry %+v", e)
	}
	// Change everything, then restore.
	st.DeleteBooks(ctx, []string{a.ID, p.ID})
	st.UpdateSettings(ctx, func(s *store.Settings) { s.Name = "Changed" })
	b.CleanCovers(0)
	if entries, _ := os.ReadDir(b.CoversDir); len(entries) != 0 {
		t.Fatal("covers were not cleaned after deleting their book")
	}
	if err := b.Restore(ctx, e.Name); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	got, ok := snap.Book(a.ID)
	if !ok || got.Cover != a.Cover || got.SpineColor != a.SpineColor || got.ISBN != "9780375756566" {
		t.Fatalf("restored book: %+v", got)
	}
	if !fileExists(filepath.Join(b.CoversDir, a.Cover)) || !fileExists(filepath.Join(b.CoversDir, covers.ThumbName(a.Cover))) {
		t.Fatal("the cover files were not restored")
	}
	if hp, _ := snap.Book(p.ID); !hp.Hidden {
		t.Fatal("hidden was lost")
	}
	cur, ok := snap.InProgress(a.ID)
	if !ok || cur.Page() != 112 {
		t.Fatalf("reading in progress: %+v", cur)
	}
	rs := snap.Readings[p.ID]
	if len(rs) != 1 || rs[0].Note == nil || rs[0].Note.Text != "A puzzle box." || !rs[0].Note.Public || rs[0].Ended.Precision != store.Year {
		t.Fatalf("finished reading: %+v", rs)
	}
	if snap.Settings.Name != "Our books" || snap.Settings.WoodDay != "cherry" {
		t.Fatalf("settings: %+v", snap.Settings)
	}
	// The restore backed up what it replaced.
	list, _ := b.List()
	if list[0].Kind != BeforeRestore {
		t.Fatalf("no before-restore backup: %+v", list)
	}
}

func TestKeepsTheNewest(t *testing.T) {
	b, st, _ := setup(t)
	fill(t, b, st) // keep 3
	var names []string
	for range 5 {
		e, err := b.Create(Automatic)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, e.Name)
	}
	list, _ := b.List()
	if len(list) != 3 || list[0].Name != names[4] || list[2].Name != names[2] {
		t.Fatalf("kept %v", list)
	}
	// Zip entries carry real times.
	f, _ := os.Open(filepath.Join(b.Dir, list[0].Name))
	info, _ := f.Stat()
	z, _ := zip.NewReader(f, info.Size())
	for _, zf := range z.File {
		if zf.Modified.Year() < 2000 {
			t.Fatalf("%s has no time (%v)", zf.Name, zf.Modified)
		}
	}
	f.Close()
}

func TestImportSkipsWhatIsThere(t *testing.T) {
	b, st, _ := setup(t)
	fill(t, b, st)
	csv := "\ufeffTitle,Author,ISBN,Pages,Binding,Year\n" +
		"Austerlitz,W. G. Sebald,978-0-375-75656-6,304,Paperback,2001\n" + // already there (same ISBN)
		"Pale Fire,Vladimir Nabokov,,315,,1962\n" + // already there (same title and author, no ISBN)
		"Middlemarch,George Eliot,=\"9780141439549\",880,Mass Market Paperback,1871\n" +
		",,,,,\n"
	added, skipped, err := importFile(b, "books.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || skipped != 2 {
		t.Fatalf("added %d skipped %d", added, skipped)
	}
	var mm store.Book
	for _, bk := range st.Snapshot().Books {
		if bk.Title == "Middlemarch" {
			mm = bk
		}
	}
	if mm.ISBN != "9780141439549" || *mm.Published != 1871 || mm.Binding != store.Paperback {
		t.Fatalf("imported row: %+v", mm)
	}
	list, _ := b.List()
	if len(list) == 0 || list[0].Kind != BeforeImport {
		t.Fatal("no backup before the import")
	}

	// JSON, as Bookshelf exports it.
	var buf bytes.Buffer
	archive.WriteJSON(&buf, []store.Book{{Title: "Dune", Author: "Frank Herbert", ISBN: "9780441172719"}})
	if added, _, err := importFile(b, "books.json", buf.Bytes()); err != nil || added != 1 {
		t.Fatalf("json import: %d %v", added, err)
	}
}

func TestImportAnArchiveFromAnotherLibrary(t *testing.T) {
	src, srcStore, _ := setup(t)
	fill(t, src, srcStore)
	var arc bytes.Buffer
	if err := archive.Write(&arc, srcStore.Snapshot().Library(), src.CoversDir, "test", time.Now()); err != nil {
		t.Fatal(err)
	}

	b, st, _ := setup(t)
	st.AddBook(ctx, store.BookInput{Title: "Pale Fire", Author: "Vladimir Nabokov"})
	cur, _ := st.StartReading(ctx, st.Snapshot().Books[0].ID)
	added, skipped, err := importFile(b, "theirs.bookshelf", arc.Bytes())
	if err != nil || added != 1 || skipped != 1 {
		t.Fatalf("archive import: %d %d %v", added, skipped, err)
	}
	snap := st.Snapshot()
	if c, _ := snap.InProgress(cur.BookID); c.ID != cur.ID {
		t.Fatal("an import changed the reading in progress")
	}
	var aus store.Book
	for _, bk := range snap.Books {
		if bk.Title == "Austerlitz" {
			aus = bk
		}
	}
	if aus.Cover == "" || !fileExists(filepath.Join(b.CoversDir, aus.Cover)) {
		t.Fatal("the imported book's cover is missing")
	}
	if rs := snap.Readings[aus.ID]; len(rs) != 1 || rs[0].Outcome != store.OutcomeReading || rs[0].Page() != 112 {
		t.Fatalf("imported reading: %+v", rs)
	}
	if snap.Settings.Name != "Bookshelf" {
		t.Fatal("an import changed the library's settings")
	}
}

func TestRefusesABoobyTrappedArchive(t *testing.T) {
	b, st, dir := setup(t)
	fill(t, b, st)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("manifest.json")
	w.Write([]byte(`{"kind":"bookshelf","format":1}`))
	w, _ = z.Create("library.json")
	w.Write([]byte(`{"books":[{"title":"X","cover":"../../evil.webp"}],"readings":[]}`))
	w, _ = z.Create("covers/../../evil.webp")
	w.Write([]byte("gotcha"))
	z.Close()
	if _, _, err := importFile(b, "trap.bookshelf", buf.Bytes()); err == nil || !strings.Contains(err.Error(), "unexpected file") {
		t.Fatalf("got %v", err)
	}
	if fileExists(filepath.Join(dir, "..", "evil.webp")) || fileExists(filepath.Join(filepath.Dir(dir), "evil.webp")) {
		t.Fatal("a file was written outside the data folder")
	}
	if _, _, err := importFile(b, "x.bookshelf", []byte("PK\x03\x04 not really")); err == nil {
		t.Fatal("a broken archive was accepted")
	}
}

func TestDue(t *testing.T) {
	b, st, _ := setup(t)
	if b.Due() {
		t.Fatal("an empty library is due a backup")
	}
	fill(t, b, st)
	if !b.Due() {
		t.Fatal("the first backup is not due")
	}
	b.Create(Automatic)
	if b.Due() {
		t.Fatal("due again straight after a backup")
	}
	st.UpdateSettings(ctx, func(s *store.Settings) { s.BackupEvery = "never" })
	if b.Due() {
		t.Fatal("due with backups turned off")
	}
}
