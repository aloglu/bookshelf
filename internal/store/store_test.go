package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

func open(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func intp(v int) *int { return &v }

func add(t *testing.T, s *Store, title, isbn string, pages int) Book {
	t.Helper()
	b, err := s.AddBook(ctx, BookInput{Title: title, Author: "A. Writer", ISBN: isbn, Pages: intp(pages), Binding: "Paperback"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A second Bookshelf on the same folder is refused at once, never kept waiting.
func TestSecondOpenIsRefusedAtOnce(t *testing.T) {
	_, dir := open(t)
	start := time.Now()
	_, err := Open(dir)
	if !errors.Is(err, ErrInUse) {
		t.Fatalf("second open: got %v, want ErrInUse", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("second open waited %v", d)
	}
}

// Exclusive locking mode keeps SQLite's WAL index in memory: no -shm file.
func TestNoSharedMemoryFile(t *testing.T) {
	s, dir := open(t)
	for i := range 20 {
		add(t, s, "Book "+strconv.Itoa(i), "", 100)
	}
	if _, err := os.Stat(filepath.Join(dir, DatabaseName+"-shm")); !os.IsNotExist(err) {
		t.Fatalf("a -shm file exists (err=%v)", err)
	}
}

// Everything is still there after closing and opening again.
func TestReopenKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := add(t, s, "Austerlitz", "9780375756566", 304)
	r, _ := s.StartReading(ctx, b.ID)
	s.SetPage(ctx, r.ID, 112)
	s.UpdateSettings(ctx, func(st *Settings) { st.Name = "Our books"; st.WoodDay = "cherry" })
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	snap := s.Snapshot()
	got, ok := snap.Book(b.ID)
	if !ok || got.Title != "Austerlitz" || got.ISBN != "9780375756566" || *got.Pages != 304 {
		t.Fatalf("book after reopen: %+v", got)
	}
	cur, ok := snap.InProgress(got.ID)
	if !ok || cur.Page() != 112 || cur.Started.Precision != Instant {
		t.Fatalf("reading after reopen: %+v", cur)
	}
	if snap.Settings.Name != "Our books" || snap.Settings.WoodDay != "cherry" {
		t.Fatalf("settings after reopen: %+v", snap.Settings)
	}
}

func TestSlugsAndISBNsAreUnique(t *testing.T) {
	s, _ := open(t)
	a := add(t, s, "Moby-Dick", "9780142437247", 654)
	b := add(t, s, "Moby-Dick", "", 670)
	if a.Slug != "moby-dick" || b.Slug != "moby-dick-2" {
		t.Fatalf("slugs %q %q", a.Slug, b.Slug)
	}
	if _, err := s.AddBook(ctx, BookInput{Title: "Another", ISBN: "9780142437247"}); !errors.Is(err, ErrDuplicateISBN) {
		t.Fatalf("duplicate ISBN: %v", err)
	}
	// Renaming keeps a slug unique; keeping the title keeps the slug.
	b2, err := s.UpdateBook(ctx, b.ID, BookInput{Title: "Moby-Dick", Pages: intp(671)})
	if err != nil || b2.Slug != "moby-dick-2" {
		t.Fatalf("unchanged title changed slug: %q %v", b2.Slug, err)
	}
	if _, err := s.UpdateBook(ctx, a.ID, BookInput{Title: "Moby-Dick; or, The Whale", ISBN: "9780142437247"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Snapshot().Book(a.ID); got.Slug != "moby-dick-or-the-whale" {
		t.Fatalf("renamed slug %q", got.Slug)
	}
	// The site's own addresses are never a book's: a book titled "Notes" is at /notes-2.
	for _, title := range []string{"Notes", "Settings", "Signin", "Stats", "Healthz"} {
		if got := add(t, s, title, "", 100); reservedSlugs[got.Slug] || !strings.HasSuffix(got.Slug, "-2") {
			t.Errorf("%q got the address %q", title, got.Slug)
		}
	}
}

func TestIssuesAreUnique(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if _, err := s.AddBook(ctx, BookInput{Title: "Cogito", ISSN: "13002880", Issue: "70–71"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddBook(ctx, BookInput{Title: "Cogito", ISSN: "13002880", Issue: "76"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddBook(ctx, BookInput{Title: "Cogito", ISSN: "13002880", Issue: "70–71"}); !errors.Is(err, ErrDuplicateIssue) {
		t.Fatalf("same issue: %v", err)
	}
	if _, err := s.AddBook(ctx, BookInput{Title: "Both", ISBN: "9780679723424", ISSN: "13002880"}); err == nil {
		t.Fatal("a book took an ISBN and an ISSN")
	}
	b, err := s.AddBook(ctx, BookInput{Title: "Stray", Issue: "4"})
	if err != nil || b.Issue != "" {
		t.Fatalf("an issue without an ISSN: %v %q", err, b.Issue)
	}
}

func TestBookInputIsChecked(t *testing.T) {
	s, _ := open(t)
	var inv *InvalidError
	if _, err := s.AddBook(ctx, BookInput{Title: "  "}); !errors.As(err, &inv) {
		t.Fatalf("empty title: %v", err)
	}
	if _, err := s.AddBook(ctx, BookInput{Title: "X", ISBN: "0679723420"}); !errors.As(err, &inv) {
		t.Fatalf("ISBN-10 must be normalized by the caller: %v", err)
	}
	b, err := s.AddBook(ctx, BookInput{Title: "  The   Road ", Binding: "Mass Market Paperback"})
	if err != nil || b.Title != "The Road" || b.Binding != Paperback {
		t.Fatalf("tidying: %+v %v", b, err)
	}
}

func TestSeveralBooksInProgress(t *testing.T) {
	s, _ := open(t)
	a := add(t, s, "A", "", 300)
	b := add(t, s, "B", "", 200)
	ra, _ := s.StartReading(ctx, a.ID)
	rb, err := s.StartReading(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if now := snap.NowReading(); len(now) != 2 || now[0].ID != rb.ID || now[1].ID != ra.ID {
		t.Fatalf("now reading: %+v", now)
	}
	if again, err := s.StartReading(ctx, a.ID); err != nil || again.ID != ra.ID || len(s.Snapshot().Readings[a.ID]) != 1 {
		t.Fatalf("starting a book being read: %+v %v", again, err)
	}
	if _, err := s.EndReading(ctx, ra.ID, OutcomeSetAside); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPage(ctx, ra.ID, 10); err == nil {
		t.Fatal("a set-aside reading took a page")
	}
	if _, err := s.SetPage(ctx, rb.ID, 201); err == nil {
		t.Fatal("a page past the end was accepted")
	}
}

func TestRereads(t *testing.T) {
	s, _ := open(t)
	b := add(t, s, "Pride and Prejudice", "9780141439518", 435)

	// Read long ago (year only), read again (date unknown), then tracked.
	old, err := s.MarkRead(ctx, b.ID, When{Time: time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC), Precision: Year})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkRead(ctx, b.ID, When{Precision: Unknown}); err != nil {
		t.Fatal(err)
	}
	cur, _ := s.StartReading(ctx, b.ID)
	s.SetPage(ctx, cur.ID, 50)
	s.SetPage(ctx, cur.ID, 120)

	if _, err := s.EndReading(ctx, cur.ID, OutcomeFinished); err != nil {
		t.Fatal(err)
	}

	list := s.Snapshot().Readings[b.ID]
	if len(list) != 3 {
		t.Fatalf("%d readings, want 3", len(list))
	}
	var tracked Reading
	for _, r := range list {
		if r.ID == cur.ID {
			tracked = r
		}
	}
	if tracked.Page() != 120 || len(tracked.Progress) != 2 || tracked.Outcome != OutcomeFinished {
		t.Fatalf("tracked reading: %+v", tracked)
	}

	// Correcting and deleting.
	if _, err := s.CorrectReading(ctx, old.ID, When{Precision: Unknown}, When{Time: time.Date(2014, 6, 3, 0, 0, 0, 0, time.UTC), Precision: Day}, OutcomeFinished); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CorrectReading(ctx, cur.ID, When{Precision: Unknown}, When{Precision: Unknown}, OutcomeSetAside); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteReading(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Snapshot().Readings[b.ID]); n != 2 {
		t.Fatalf("%d readings after delete, want 2", n)
	}
	if _, err := s.MarkRead(ctx, b.ID, When{Time: time.Now().AddDate(2, 0, 0), Precision: Day}); err == nil {
		t.Fatal("a reading in the future was accepted")
	}

	// Deleting the book takes its readings with it.
	if _, err := s.DeleteBooks(ctx, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Snapshot().Readings[b.ID]); n != 0 {
		t.Fatalf("readings left after deleting the book: %d", n)
	}
}

func TestNotes(t *testing.T) {
	s, _ := open(t)
	b := add(t, s, "The Great Gatsby", "9780743273565", 180)
	other := add(t, s, "Stoner", "9781590171998", 278)
	r, _ := s.MarkRead(ctx, b.ID, When{Time: time.Date(2025, 8, 3, 0, 0, 0, 0, time.UTC), Precision: Day})
	theirs, _ := s.MarkRead(ctx, other.ID, When{Precision: Unknown})

	note, err := s.AddEntry(ctx, b.ID, EntryInput{Text: "  Sadder than I remembered.  ", ReadingID: r.ID, Public: true})
	if err != nil {
		t.Fatal(err)
	}
	if note.Text != "Sadder than I remembered." || note.Quote != "" || note.Where != nil || note.ReadingID != r.ID || !note.Public {
		t.Fatalf("note: %+v", note)
	}
	// Words with no passage are just words; with a passage they stay before it.
	if e, err := s.AddEntry(ctx, b.ID, EntryInput{Pre: "Just words.", Text: "More."}); err != nil || e.Pre != "" || e.Text != "Just words.\nMore." {
		t.Fatalf("words before nothing: %+v %v", e, err)
	}
	hl, err := s.AddEntry(ctx, b.ID, EntryInput{Pre: "Closing line:", Quote: "So we beat on, boats against the current.", Text: "Oh.", Where: &Where{Kind: WherePage, N: 180}})
	if err != nil || hl.Pre != "Closing line:" || hl.Text != "Oh." || hl.Where.N != 180 {
		t.Fatalf("passage with words either side: %+v %v", hl, err)
	}
	for name, in := range map[string]EntryInput{
		"nothing":                 {Text: "  "},
		"words around no passage": {Pre: " "},
		"a page past the end":     {Quote: "x", Where: &Where{Kind: WherePage, N: 181}},
		"place zero":              {Quote: "x", Where: &Where{Kind: WhereChapter, N: 0}},
		"an unknown place":        {Quote: "x", Where: &Where{Kind: "line", N: 3}},
		"another book's reading":  {Text: "x", ReadingID: theirs.ID},
		"a date in the future":    {Text: "x", CreatedAt: ptr(time.Now().Add(48 * time.Hour))},
	} {
		if _, err := s.AddEntry(ctx, b.ID, in); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := s.AddEntry(ctx, "nope", EntryInput{Text: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing book: %v", err)
	}
	// Chapters and locations have no upper limit; only a location may be a range.
	if e, err := s.AddEntry(ctx, b.ID, EntryInput{Quote: "x", Where: &Where{Kind: WhereLoc, N: 4000, To: 4003}}); err != nil || e.Where.To != 4003 {
		t.Fatalf("a location range: %+v %v", e, err)
	}
	if e, err := s.AddEntry(ctx, b.ID, EntryInput{Quote: "y", Where: &Where{Kind: WhereChapter, N: 400, To: 402}}); err != nil || e.Where.To != 0 {
		t.Fatalf("a chapter is one number: %+v %v", e, err)
	}

	// Editing keeps when it was first written; putting one back (undo) keeps its date.
	edited, err := s.UpdateEntry(ctx, hl.ID, EntryInput{Quote: hl.Quote, Text: "The last line.", Where: hl.Where, Public: true, CreatedAt: ptr(time.Unix(0, 0))})
	if err != nil || !edited.CreatedAt.Equal(hl.CreatedAt) || edited.Text != "The last line." || edited.Pre != "" || !edited.Public {
		t.Fatalf("edited: %+v %v", edited, err)
	}
	if err := s.DeleteEntry(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	back, err := s.AddEntry(ctx, b.ID, EntryInput{Text: note.Text, ReadingID: note.ReadingID, Public: note.Public, CreatedAt: &note.CreatedAt})
	if err != nil || !back.CreatedAt.Equal(note.CreatedAt) {
		t.Fatalf("put back: %+v %v", back, err)
	}
	if list := s.Snapshot().Entries[b.ID]; list[0].ID != back.ID {
		t.Fatalf("entries are kept oldest first: %+v", list)
	}

	// A book's notes can be kept from visitors.
	if err := s.SetNotesHidden(ctx, b.ID, true); err != nil {
		t.Fatal(err)
	}
	if bk, _ := s.Snapshot().Book(b.ID); !bk.NotesHidden {
		t.Fatal("notes were not hidden")
	}
	if err := s.SetNotesHidden(ctx, "nope", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing book: %v", err)
	}

	// Deleting a reading keeps what was written during it; deleting the book doesn't.
	if err := s.DeleteReading(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.Snapshot().Entry(back.ID); e.ReadingID != "" {
		t.Fatalf("still tied to a deleted reading: %+v", e)
	}
	if _, err := s.DeleteBooks(ctx, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&count)
	if count != 0 || len(s.Snapshot().Entries[b.ID]) != 0 {
		t.Fatalf("entries left after deleting the book: %d", count)
	}
}

// A 2.0 library's notes (one on each finished reading) become notes tied to
// their readings when 2.1 opens it, and (through schema 2) become entries with
// a place and no kind.
func TestOpeningA20Library(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, DatabaseName))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{schemaV1, "PRAGMA user_version = 1",
		`INSERT INTO books (id, title, slug, added_at, updated_at) VALUES ('b1', 'Stoner', 'stoner', '2024-01-01T00:00:00.000000000Z', '2024-01-01T00:00:00.000000000Z')`,
		`INSERT INTO readings (id, book_id, started_precision, ended_at, ended_precision, outcome) VALUES ('r1', 'b1', 'unknown', '2024-03-12', 'day', 'finished')`,
		`INSERT INTO notes (reading_id, text, public, created_at, updated_at) VALUES ('r1', 'Better the second time.', 1, '2024-03-12T10:00:00.000000000Z', '2024-03-13T10:00:00.000000000Z')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list := s.Snapshot().Entries["b1"]
	if len(list) != 1 {
		t.Fatalf("entries: %+v", list)
	}
	e := list[0]
	if e.Text != "Better the second time." || !e.Public || e.ReadingID != "r1" || len(e.ID) != 16 ||
		!e.CreatedAt.Equal(time.Date(2024, 3, 12, 10, 0, 0, 0, time.UTC)) || !e.UpdatedAt.Equal(time.Date(2024, 3, 13, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("moved note: %+v", e)
	}
	var notes int
	s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'notes'`).Scan(&notes)
	if notes != 0 {
		t.Fatal("the old notes table is still there")
	}
}

// A library from an early 2.1 (schema 2) had notes and highlights with a page;
// they become entries with a passage and a place of kind page.
func TestOpeningASchema2Library(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, DatabaseName))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{schemaV1, schemaV2, "PRAGMA user_version = 2",
		`INSERT INTO books (id, title, slug, added_at, updated_at) VALUES ('b1', 'Stoner', 'stoner', '2024-01-01T00:00:00.000000000Z', '2024-01-01T00:00:00.000000000Z')`,
		`INSERT INTO entries (id, book_id, kind, text, quote, page, public, created_at, updated_at) VALUES
			('n1', 'b1', 'note', 'Sad.', '', NULL, 0, '2024-03-12T10:00:00.000000000Z', '2024-03-12T10:00:00.000000000Z'),
			('h1', 'b1', 'highlight', 'Why.', 'A passage.', 33, 1, '2024-03-13T10:00:00.000000000Z', '2024-03-13T10:00:00.000000000Z')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list := s.Snapshot().Entries["b1"]
	if len(list) != 2 || list[0].Text != "Sad." || list[0].Where != nil || list[1].Quote != "A passage." || list[1].Text != "Why." ||
		list[1].Where == nil || list[1].Where.Kind != WherePage || list[1].Where.N != 33 || !list[1].Public {
		t.Fatalf("moved entries: %+v", list)
	}
	if b, _ := s.Snapshot().Book("b1"); b.NotesHidden {
		t.Fatal("a book's notes start out open to visitors")
	}
}

// Settings saved before notes had settings keep visitors reading the public notes.
func TestNotesSettingsDefaultOn(t *testing.T) {
	s, _ := open(t)
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('settings', '{"name":"Old","sort":"title","theme":"system","permalink":"isbn","sources":["openlibrary"],"wallDay":"#e9e0d0","wallNight":"#17110d","woodDay":"oak","woodNight":"walnut","ribbonDay":"#8e2532","ribbonNight":"#8e2532","bookSize":100,"maxHeight":100,"backupEvery":"day","backupKeep":14}')`); err != nil {
		t.Fatal(err)
	}
	if err := s.reload(ctx); err != nil {
		t.Fatal(err)
	}
	if st := s.Snapshot().Settings; st.Name != "Old" || !st.Notes || st.Timeline || st.StartsPublic {
		t.Fatalf("notes settings of an older library: %+v", st)
	}
}

// A note keeps the place it has though its book's page count was lowered past it: restoring a backup, editing
// the note, and putting it back after deleting it. Only a page given anew must be one of the book's.
func TestANoteKeepsItsPlace(t *testing.T) {
	s, _ := open(t)
	b := add(t, s, "Stoner", "9781590171998", 278)
	s.AddEntry(ctx, b.ID, EntryInput{Quote: "A passage.", Where: &Where{Kind: WherePage, N: 270}})
	lib := s.Snapshot().Library()
	small := 200
	lib.Books[0].Pages = &small
	if err := s.Replace(ctx, lib); err != nil {
		t.Fatalf("restore: %v", err)
	}
	es := s.Snapshot().Entries[b.ID]
	if len(es) != 1 || es[0].Where == nil || es[0].Where.N != 270 || es[0].Quote != "A passage." {
		t.Fatalf("restored: %+v", es)
	}
	e := es[0]
	edited, err := s.UpdateEntry(ctx, e.ID, EntryInput{Quote: "A passage, edited.", Where: e.Where})
	if err != nil || edited.Where.N != 270 {
		t.Fatalf("editing the words: %+v %v", edited, err)
	}
	if _, err := s.UpdateEntry(ctx, e.ID, EntryInput{Quote: "A passage.", Where: &Where{Kind: WherePage, N: 250}}); err == nil {
		t.Fatal("a new page past the book's 200 was taken")
	}
	if err := s.DeleteEntry(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	created := edited.CreatedAt
	back, err := s.AddEntry(ctx, b.ID, EntryInput{Quote: edited.Quote, Where: edited.Where, CreatedAt: &created})
	if err != nil || back.Where.N != 270 {
		t.Fatalf("putting it back: %+v %v", back, err)
	}
	if _, err := s.AddEntry(ctx, b.ID, EntryInput{Quote: "New.", Where: &Where{Kind: WherePage, N: 270}}); err == nil {
		t.Fatal("a new note on page 270 of 200 was taken")
	}
}

func TestSettingsAreValidated(t *testing.T) {
	s, _ := open(t)
	if _, err := s.UpdateSettings(ctx, func(st *Settings) { st.WallDay = "red" }); err == nil {
		t.Fatal("a bad color was accepted")
	}
	if s.Snapshot().Settings.WallDay != "#e9e0d0" {
		t.Fatal("a rejected change was kept")
	}
	for _, bad := range []string{"javascript:alert(1)", "http://www.goodreads.com/x", "https://goodreads.com.evil.example/x", "https://user@www.goodreads.com/x"} {
		if _, err := s.UpdateSettings(ctx, func(st *Settings) { st.Social.Goodreads = bad }); err == nil {
			t.Fatalf("a social link took %q", bad)
		}
	}
	if _, err := s.UpdateSettings(ctx, func(st *Settings) { st.Social.Goodreads = "https://www.goodreads.com/user/show/1-a" }); err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateSettings(ctx, func(st *Settings) { st.Sources = []string{"goodreads", "openlibrary"}; st.WallDay = "#A1B2C3" })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Sources, ",") != "openlibrary,goodreads" || got.WallDay != "#a1b2c3" {
		t.Fatalf("tidying: %+v", got)
	}
}

// Settings saved before 2.0.5 had one bookcase switch over the parts (off, none of them showed) and one
// ribbon color. They open looking the same, with the shelves still under the books.
func TestOlderBookcaseSettingsOpenTheSame(t *testing.T) {
	for _, c := range []struct {
		saved string
		want  CaseParts
	}{
		{`{"name":"Ours","bookcase":false,"caseParts":{"sides":true,"top":true,"back":true}}`, CaseParts{Shelves: true}},
		{`{"name":"Ours","bookcase":true,"caseParts":{"sides":true,"top":false,"back":true},"ribbon":"#23365e"}`, CaseParts{Shelves: true, Sides: true, Back: true}},
	} {
		dir := t.TempDir()
		s, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('settings', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, c.saved); err != nil {
			t.Fatal(err)
		}
		s.Close()
		if s, err = Open(dir); err != nil {
			t.Fatal(err)
		}
		st := s.Snapshot().Settings
		if st.CaseParts != c.want || st.Bookcase != nil || st.Name != "Ours" {
			t.Fatalf("%s opened as %+v (bookcase %v)", c.saved, st.CaseParts, st.Bookcase)
		}
		// The one ribbon color, where there was one, is the day's and the evening's; otherwise the default.
		ribbon := "#8e2532"
		if strings.Contains(c.saved, "ribbon") {
			ribbon = "#23365e"
		}
		if st.RibbonDay != ribbon || st.RibbonNight != ribbon || st.Ribbon != "" {
			t.Fatalf("%s: ribbons %q and %q (old %q)", c.saved, st.RibbonDay, st.RibbonNight, st.Ribbon)
		}
		// Saving again keeps the parts and drops the old switch.
		if st, err = s.UpdateSettings(ctx, func(st *Settings) { st.CaseParts.Top = true }); err != nil || st.Bookcase != nil || !st.CaseParts.Top || st.CaseParts.Shelves != c.want.Shelves {
			t.Fatalf("saved again: %+v %v", st, err)
		}
		s.Close()
	}
}

// Hundreds of concurrent readers while changes are made: every read is
// answered from memory, and the process holds a single file lock throughout.
func TestConcurrentReadsHoldOneLock(t *testing.T) {
	s, dir := open(t)
	for i := range 50 {
		add(t, s, "Book "+strconv.Itoa(i), "", 100+i)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if len(s.Snapshot().Books) < 50 {
					t.Error("a reader saw a partial library")
					return
				}
			}
		}()
	}
	maxLocks := 0
	for i := range 40 {
		add(t, s, "Late "+strconv.Itoa(i), "", 200)
		if n := locksHeld(t, dir); n > maxLocks {
			maxLocks = n
		}
	}
	close(stop)
	wg.Wait()
	t.Logf("most file locks held at once: %d", maxLocks)
	if runtime.GOOS == "linux" && maxLocks != 1 {
		t.Fatalf("the process held %d file locks on the library at once, want 1", maxLocks)
	}
}

// locksHeld counts this process's POSIX locks on the database (Linux only).
func locksHeld(t *testing.T, dir string) int {
	if runtime.GOOS != "linux" {
		return 0
	}
	info, err := os.Stat(filepath.Join(dir, DatabaseName))
	if err != nil {
		t.Fatal(err)
	}
	ino := strconv.FormatUint(inode(info), 10)
	raw, err := os.ReadFile("/proc/locks")
	if err != nil {
		t.Skip("no /proc/locks")
	}
	pid := strconv.Itoa(os.Getpid())
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		// 1: POSIX  ADVISORY  WRITE 1234 00:2a:5678 1073741824 1073741824
		if len(f) >= 6 && f[1] == "POSIX" && f[4] == pid && strings.HasSuffix(f[5], ":"+ino) {
			n++
		}
	}
	return n
}

func ptr[T any](v T) *T { return &v }

func TestPlainText(t *testing.T) {
	for in, want := range map[string]string{
		"Bookshelf":                             "Bookshelf",
		"[Ali’s books](https://alog.lu)":        "Ali’s books",
		"**Our** *shelf* of `books`":            "Our shelf of books",
		"__Big__ _small_ snake_case_name":       "Big small snake_case_name",
		"See <https://alog.lu>\nand more":       "See https://alog.lu and more",
		"2 * 3 * 4":                             "2 * 3 * 4",
		"[a](https://x.y) and [b](https://z.w)": "a and b",
	} {
		if got := PlainText(in); got != want {
			t.Errorf("PlainText(%q) = %q, want %q", in, got, want)
		}
	}
}
