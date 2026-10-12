package store

import (
	"context"
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

func TestRereadsAndNotes(t *testing.T) {
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

	if _, err := s.SetNote(ctx, cur.ID, "Halfway thoughts", false); err == nil {
		t.Fatal("a note was allowed on a reading in progress")
	}
	if _, err := s.SetNote(ctx, old.ID, "Better than I remembered.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EndReading(ctx, cur.ID, OutcomeFinished); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetNote(ctx, cur.ID, "Second time round: Mr Collins is funnier.", false); err != nil {
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
	if tracked.Page() != 120 || len(tracked.Progress) != 2 || tracked.Note == nil || tracked.Note.Public {
		t.Fatalf("tracked reading: %+v", tracked)
	}
	if r, _ := s.Snapshot().Reading(old.ID); r.Note == nil || r.Note.Text != "Better than I remembered." || !r.Note.Public {
		t.Fatalf("the first reading lost its note: %+v", r)
	}

	// Correcting and deleting.
	if _, err := s.CorrectReading(ctx, old.ID, When{Precision: Unknown}, When{Time: time.Date(2014, 6, 3, 0, 0, 0, 0, time.UTC), Precision: Day}, OutcomeFinished); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CorrectReading(ctx, old.ID, When{Precision: Unknown}, When{Precision: Unknown}, OutcomeSetAside); err == nil {
		t.Fatal("a reading with a note was set aside")
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

	// Deleting the book takes its readings and notes with it.
	if _, err := s.DeleteBooks(ctx, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Snapshot().Readings[b.ID]); n != 0 {
		t.Fatalf("readings left after deleting the book: %d", n)
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&count)
	if count != 0 {
		t.Fatalf("notes left after deleting the book: %d", count)
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
