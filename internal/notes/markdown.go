package notes

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aloglu/bookshelf/internal/store"
)

// WhereText is a place as it is read: "Page 12", "Chapter 63", "Location 214–215".
func WhereText(w *store.Where) string {
	if w == nil {
		return ""
	}
	n := strconv.Itoa(w.N)
	switch w.Kind {
	case store.WhereChapter:
		return "Chapter " + n
	case store.WhereLoc:
		if w.To > w.N {
			return "Location " + n + "–" + strconv.Itoa(w.To)
		}
		return "Location " + n
	}
	return "Page " + n
}

var ordinals = []string{"First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh", "Eighth", "Ninth", "Tenth"}

func ordinal(i int) string {
	if i < len(ordinals) {
		return ordinals[i]
	}
	return strconv.Itoa(i+1) + "th"
}

func entryMarkdown(e store.Entry, private bool, zone *time.Location) string {
	var parts []string
	if e.Pre != "" {
		parts = append(parts, e.Pre)
	}
	if e.Quote != "" {
		parts = append(parts, "> "+strings.ReplaceAll(e.Quote, "\n", "\n> "))
	}
	if e.Text != "" {
		parts = append(parts, e.Text)
	}
	meta := []string{}
	if w := WhereText(e.Where); w != "" {
		meta = append(meta, w)
	}
	meta = append(meta, e.CreatedAt.In(zone).Format("Jan 2, 2006"))
	if private && !e.Public {
		meta = append(meta, "Private")
	}
	return strings.Join(parts, "\n\n") + "\n\n*" + strings.Join(meta, " · ") + "*\n"
}

func oldestFirst(list []store.Entry) []store.Entry {
	out := append([]store.Entry(nil), list...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// visible are a book's notes as the audience may read them: every one, or exactly what visitors can read
// (public notes, and none at all where the site keeps them from visitors).
func visible(snap *store.Snapshot, b store.Book, private bool) []store.Entry {
	var out []store.Entry
	for _, e := range snap.Entries[b.ID] {
		if private || (e.Public && snap.VisitorsReadNotes(b)) {
			out = append(out, e)
		}
	}
	return out
}

func bookMarkdown(snap *store.Snapshot, b store.Book, private bool, level int, zone *time.Location) string {
	es := visible(snap, b, private)
	if len(es) == 0 {
		return ""
	}
	h, sub := strings.Repeat("#", level), strings.Repeat("#", level+1)
	var out strings.Builder
	fmt.Fprintf(&out, "%s %s\n\n*%s", h, b.Title, b.Author)
	if b.Published != nil {
		fmt.Fprintf(&out, " · %d", *b.Published)
	}
	out.WriteString("*\n\n")
	readings := snap.Readings[b.ID]
	var groups []string
	byReading := map[string][]store.Entry{}
	var loose []store.Entry
	for _, e := range es {
		if e.ReadingID != "" {
			byReading[e.ReadingID] = append(byReading[e.ReadingID], e)
		} else {
			loose = append(loose, e)
		}
	}
	if len(readings) < 2 || !private { // visitors are never told about readings
		for _, e := range oldestFirst(es) {
			out.WriteString(entryMarkdown(e, private, zone))
			out.WriteString("\n")
		}
		return strings.TrimRight(out.String(), "\n") + "\n"
	}
	for i, r := range readings {
		l := byReading[r.ID]
		if len(l) == 0 {
			continue
		}
		state := "under way"
		if r.Outcome != store.OutcomeReading && r.Ended.Known() {
			ended := r.Ended.Time // a day is kept as it was given; only a moment is the reader's
			if r.Ended.Precision == store.Instant {
				ended = ended.In(zone)
			}
			state = "finished " + ended.Format("Jan 2, 2006")
			if r.Ended.Precision == store.Year {
				state = "finished " + strconv.Itoa(r.Ended.Time.Year())
			}
		}
		groups = append(groups, fmt.Sprintf("%s %s reading — %s\n\n", sub, ordinal(i), state))
		for _, e := range oldestFirst(l) {
			groups[len(groups)-1] += entryMarkdown(e, private, zone) + "\n"
		}
	}
	if len(loose) > 0 {
		g := sub + " Not tied to a reading\n\n"
		for _, e := range oldestFirst(loose) {
			g += entryMarkdown(e, private, zone) + "\n"
		}
		groups = append(groups, g)
	}
	out.WriteString(strings.Join(groups, ""))
	return strings.TrimRight(out.String(), "\n") + "\n"
}

// BookMarkdown is one book's notes as a Markdown file, with its details in front matter. It is empty when
// there is nothing to say. Only what visitors can read unless private is set. Dates are given in zone, the reader's.
func BookMarkdown(snap *store.Snapshot, b store.Book, private bool, zone *time.Location) string {
	body := bookMarkdown(snap, b, private, 1, zone)
	if body == "" {
		return ""
	}
	front := "---\ntitle: " + strconv.Quote(b.Title) + "\nauthor: " + strconv.Quote(b.Author) + "\n"
	if b.Published != nil {
		front += "year: " + strconv.Itoa(*b.Published) + "\n"
	}
	front += "notes: " + strconv.Itoa(len(visible(snap, b, private))) + "\n---\n\n"
	return front + body
}

// AllMarkdown is every book's notes in one file, a heading for each book (in the order of the shelf).
// Without private, it holds what visitors can read and nothing more.
func AllMarkdown(snap *store.Snapshot, private bool, now time.Time, zone *time.Location) string {
	var blocks []string
	for _, b := range snap.Books {
		if m := bookMarkdown(snap, b, private, 2, zone); m != "" {
			blocks = append(blocks, m)
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	books := "1 book"
	if len(blocks) != 1 {
		books = strconv.Itoa(len(blocks)) + " books"
	}
	tail := ""
	if !private {
		tail = " · public notes only"
	}
	return "# Notes from " + store.PlainText(snap.Settings.Name) + "\n\n*" + books + " · exported " + now.In(zone).Format("January 2, 2006") + tail + "*\n\n" + strings.Join(blocks, "\n")
}
