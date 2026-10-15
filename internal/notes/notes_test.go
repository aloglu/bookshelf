package notes

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aloglu/bookshelf/internal/store"
)

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/My Clippings.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseClippings(t *testing.T) {
	got := ParseClippings(fixture(t), time.UTC)
	var mm []Clipping
	for _, c := range got {
		if c.Title == "Middlemarch" {
			mm = append(mm, c)
		}
	}
	// A note beside a highlight joins it, the longer copy of a highlight wins, a bookmark is dropped.
	if len(mm) != 2 {
		t.Fatalf("Middlemarch should keep 2 entries, got %d: %+v", len(mm), mm)
	}
	if !strings.HasPrefix(mm[0].Quote, "What do we live for") || !strings.HasPrefix(mm[0].Text, "Dorothea to Rosamond") {
		t.Fatalf("the note should sit with its highlight: %+v", mm[0])
	}
	if mm[0].Where == nil || mm[0].Where.Kind != store.WherePage || mm[0].Where.N != 412 {
		t.Fatalf("a page wins when Kindle gives one: %+v", mm[0].Where)
	}
	if !strings.Contains(mm[1].Quote, "Who can quit young lives") || mm[1].Where.N != 380 {
		t.Fatalf("the longer copy should replace the shorter: %+v", mm[1])
	}
	if mm[0].At.Format("2006-01-02 15:04:05") != "2026-10-10 21:14:09" {
		t.Fatalf("date: %v", mm[0].At)
	}
	var walden *Clipping
	for i, c := range got {
		if c.Title == "Walden" {
			walden = &got[i]
			break
		}
	}
	if walden == nil || walden.Author != "Henry David Thoreau" || walden.Where.Kind != store.WhereLoc || walden.Where.N != 214 || walden.Where.To != 215 {
		t.Fatalf("a location when there is no page: %+v", walden)
	}
	// A note beside a highlight that runs over several locations, with no page, joins it.
	locs := "Walden (Henry David Thoreau)\n- Your Highlight on Location 2203-2210 | Added on Saturday, 14 June 2025 06:40:05\n\nA long passage.\n==========\n" +
		"Walden (Henry David Thoreau)\n- Your Note on Location 2208 | Added on Saturday, 14 June 2025 06:41:05\n\nMine.\n==========\n"
	if c := ParseClippings(locs, time.UTC); len(c) != 1 || c[0].Text != "Mine." {
		t.Fatalf("a note inside a location range: %+v", c)
	}
	// Kindle's clock is the reader's: 23:12 in Istanbul is 20:12 UTC.
	ist, _ := time.LoadLocation("Europe/Istanbul")
	if c := ParseClippings(fixture(t), ist); c[0].At.UTC().Hour() != 18 {
		t.Fatalf("time zone: %v", c[0].At)
	}
	if got := ParseClippings("not a clippings file\n", time.UTC); len(got) != 0 {
		t.Fatalf("garbage: %+v", got)
	}
	// The American date order, and a file without the byte order mark or with Unix line ends.
	us := "Walden (Henry David Thoreau)\n- Your Highlight at location 5-6 | Added on Saturday, June 14, 2025 6:40:05 AM\n\nSimplify.\n==========\n"
	if c := ParseClippings(us, time.UTC); len(c) != 1 || c[0].At.Hour() != 6 || c[0].Where.N != 5 {
		t.Fatalf("american dates: %+v", c)
	}
}

func shelf(t *testing.T) (*store.Store, map[string]store.Book) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	books := map[string]store.Book{}
	for _, b := range []store.BookInput{{Title: "Middlemarch", Author: "George Eliot"}, {Title: "Walden; or, Life in the Woods", Author: "Henry David Thoreau"}, {Title: "Moby-Dick: or, The Whale", Author: "Herman Melville"}} {
		bk, err := s.AddBook(context.Background(), b)
		if err != nil {
			t.Fatal(err)
		}
		books[b.Title] = bk
	}
	return s, books
}

func TestPlanMatchesByTitleAndSkipsWhatIsKept(t *testing.T) {
	s, books := shelf(t)
	ctx := context.Background()
	clips := ParseClippings(fixture(t), time.UTC)
	plan := Plan(s.Snapshot(), clips)
	by := map[string]Group{}
	for _, g := range plan {
		by[g.Title] = g
	}
	if g := by["Middlemarch"]; g.BookID != books["Middlemarch"].ID || len(g.Fresh) != 2 {
		t.Fatalf("Middlemarch: %+v", g)
	}
	if g := by["Frankenstein"]; g.BookID != "" || len(g.Fresh) != 0 || g.Total != 1 {
		t.Fatalf("a book that isn't on the shelf is skipped: %+v", g)
	}
	if g := by["Moby-Dick"]; g.BookID != books["Moby-Dick: or, The Whale"].ID {
		t.Fatalf("the part before a colon is enough: %+v", g)
	}
	added, err := s.AddEntries(ctx, Entries(plan, false))
	if err != nil || added == 0 {
		t.Fatalf("add: %d %v", added, err)
	}
	mm := s.Snapshot().Entries[books["Middlemarch"].ID]
	if len(mm) != 2 || mm[0].Public || mm[0].CreatedAt.Year() != 2026 || mm[0].CreatedAt.Month() != time.October {
		t.Fatalf("imported entries keep Kindle's dates: %+v", mm)
	}
	// Running it again adds nothing.
	again := Plan(s.Snapshot(), clips)
	if n := len(Entries(again, false)); n != 0 {
		t.Fatalf("a second import should add nothing, would add %d", n)
	}
	for _, g := range again {
		if g.BookID != "" && g.Same != g.Total {
			t.Fatalf("%s: same %d of %d", g.Title, g.Same, g.Total)
		}
	}
}

func TestMarkdown(t *testing.T) {
	s, books := shelf(t)
	ctx := context.Background()
	mm := books["Middlemarch"]
	r, _ := s.StartReading(ctx, mm.ID)
	s.AddEntry(ctx, mm.ID, store.EntryInput{Pre: "Read on the train.", Quote: "Every limit is a beginning.\nTruly.", Text: "Yes.", Where: &store.Where{Kind: store.WhereChapter, N: 63}, ReadingID: r.ID, Public: true})
	s.AddEntry(ctx, mm.ID, store.EntryInput{Text: "Only mine.", Where: &store.Where{Kind: store.WhereLoc, N: 214, To: 215}})
	snap := s.Snapshot()
	got := BookMarkdown(snap, mm, true, time.UTC)
	for _, want := range []string{"title: \"Middlemarch\"", "notes: 2", "# Middlemarch", "*George Eliot*", "Read on the train.\n\n> Every limit is a beginning.\n> Truly.\n\nYes.", "*Chapter 63 · ", "*Location 214–215 · ", "· Private*"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	pub := BookMarkdown(snap, mm, false, time.UTC)
	if strings.Contains(pub, "Only mine") || strings.Contains(pub, "Private") || !strings.Contains(pub, "notes: 1") {
		t.Errorf("public export leaks or miscounts:\n%s", pub)
	}
	if BookMarkdown(snap, books["Walden; or, Life in the Woods"], true, time.UTC) != "" {
		t.Error("a book without notes gives nothing")
	}
	all := AllMarkdown(snap, false, time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), time.UTC)
	if !strings.Contains(all, "# Notes from Bookshelf") || !strings.Contains(all, "## Middlemarch") || !strings.Contains(all, "public notes only") {
		t.Errorf("all:\n%s", all)
	}
	s.SetNotesHidden(ctx, mm.ID, true)
	if AllMarkdown(s.Snapshot(), false, time.Now(), time.UTC) != "" {
		t.Error("a book kept from visitors stays out of what visitors export")
	}
}

func TestHeadDropsSubtitleAndEdition(t *testing.T) {
	for in, want := range map[string]string{
		"Frankenstein; Or, The Modern Prometheus": "Frankenstein",
		"Middlemarch (Penguin Classics)":          "Middlemarch",
		"Moby-Dick: or, The Whale":                "Moby-Dick",
		"Walden":                                  "Walden",
	} {
		if got := head(in); got != want {
			t.Errorf("head(%q) = %q, want %q", in, got, want)
		}
	}
}
