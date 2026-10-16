// Package notes reads Kindle's My Clippings.txt into notes and writes notes
// out as Markdown.
package notes

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/aloglu/bookshelf/internal/store"
)

// Clipping is one note, folded together from the file's highlights and notes.
type Clipping struct {
	Title, Author string
	Quote, Text   string       // a highlight's passage; the reader's own note on it, or on its own
	Where         *store.Where // the page when Kindle gives one, otherwise the location
	Loc           *store.Where // the location, when it has one
	At            time.Time
}

var (
	headLine = regexp.MustCompile(`^(.*?)\s*\(([^()]*)\)\s*$`)
	pageRe   = regexp.MustCompile(`(?i)\bpage\s+(\d+)`)
	locRe    = regexp.MustCompile(`(?i)\blocation\s+(\d+)(?:\s*-\s*(\d+))?`)
	whenRe   = regexp.MustCompile(`(?i)added on\s+(?:[A-Za-z]+,?\s+)?(.+?)\s*$`)
	sepRe    = regexp.MustCompile(`(?m)^={5,}\s*$`)
)

// bom is the byte order mark Kindle starts the file with.
const bom = "\ufeff"

// Kindle writes the date as "Saturday, 10 October 2026 21:14:09", or in the
// American order "Saturday, October 10, 2026 9:14:09 PM".
var whenLayouts = []string{"2 January 2006 15:04:05", "January 2, 2006 3:04:05 PM", "January 2, 2006 15:04:05", "2 January 2006 3:04:05 PM"}

func parseWhen(s string, loc *time.Location) time.Time {
	for _, l := range whenLayouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

type clip struct {
	Clipping
	note  bool
	locN  int // the location, for matching a note to the highlight beside it
	locTo int
}

// ParseClippings reads a My Clippings.txt, whose times are the Kindle’s own clock, in the reader’s time
// zone. A note made beside a highlight
// becomes part of it, a longer copy of a highlight replaces the shorter one it
// grew from, and bookmarks are left out. Clippings come back in file order.
func ParseClippings(text string, zone *time.Location) []Clipping {
	text = strings.TrimPrefix(text, bom)
	var raw []clip
	for _, block := range sepRe.Split(strings.ReplaceAll(text, "\r\n", "\n"), -1) {
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		if len(lines) < 3 {
			continue
		}
		head, meta, body := strings.TrimSpace(strings.TrimPrefix(lines[0], bom)), lines[1], strings.TrimSpace(strings.Join(lines[2:], "\n"))
		var c clip
		switch {
		case strings.Contains(meta, "Your Highlight"):
		case strings.Contains(meta, "Your Note"):
			c.note = true
		default:
			continue
		}
		if body == "" {
			continue
		}
		c.Title = head
		if m := headLine.FindStringSubmatch(head); m != nil {
			c.Title, c.Author = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
		}
		var page, loc, locTo int
		if m := pageRe.FindStringSubmatch(meta); m != nil {
			page, _ = strconv.Atoi(m[1])
		}
		if m := locRe.FindStringSubmatch(meta); m != nil {
			loc, _ = strconv.Atoi(m[1])
			locTo, _ = strconv.Atoi(m[2])
		}
		switch {
		case page > 0:
			c.Where = &store.Where{Kind: store.WherePage, N: page}
		case loc > 0:
			c.Where = &store.Where{Kind: store.WhereLoc, N: loc}
			if locTo > loc {
				c.Where.To = locTo
			}
		}
		c.locN = loc
		if loc > 0 {
			c.Loc = &store.Where{Kind: store.WhereLoc, N: loc}
			if locTo > loc {
				c.Loc.To = locTo
			}
		}
		if m := whenRe.FindStringSubmatch(meta); m != nil {
			c.At = parseWhen(m[1], zone)
		}
		if c.note {
			c.Text = body
		} else {
			c.Quote = body
		}
		c.locTo = locTo // the location still tells a note which highlight it sits by
		raw = append(raw, c)
	}

	var items []*clip
	for i := range raw {
		c := &raw[i]
		var same []*clip // earlier items of this book that hold a passage
		for _, it := range items {
			if it.Title == c.Title && it.Quote != "" {
				same = append(same, it)
			}
		}
		if c.note {
			var host *clip
			for j := len(same) - 1; j >= 0; j-- {
				h := same[j]
				end := h.locTo
				if end == 0 {
					end = h.locN
				}
				if h.locN != 0 && c.locN != 0 && c.locN >= h.locN && c.locN <= end+1 {
					host = h
					break
				}
			}
			if host != nil {
				host.Text = c.Text
				continue
			}
			items = append(items, c)
			continue
		}
		var dup *clip
		for _, it := range same {
			if near(it, c) && (strings.HasPrefix(it.Quote, c.Quote) || strings.HasPrefix(c.Quote, it.Quote)) {
				dup = it
				break
			}
		}
		if dup != nil {
			if len(c.Quote) > len(dup.Quote) {
				dup.Quote, dup.Where, dup.Loc, dup.locTo, dup.At = c.Quote, c.Where, c.Loc, c.locTo, c.At
			}
			continue
		}
		items = append(items, c)
	}
	out := make([]Clipping, len(items))
	for i, it := range items {
		out[i] = it.Clipping
	}
	return out
}

// near says whether two highlights are at the same place: their locations overlap or touch (a highlight
// lengthened on the Kindle is written again, from the same spot). Without locations, only the very same
// passage counts. The same words highlighted elsewhere in the book are another highlight.
func near(a, b *clip) bool {
	if a.locN == 0 || b.locN == 0 {
		return a.Quote == b.Quote
	}
	end := func(c *clip) int {
		if c.locTo > c.locN {
			return c.locTo
		}
		return c.locN
	}
	return a.locN <= end(b)+1 && b.locN <= end(a)+1
}

// plain maps the accented letters of the Latin alphabets to their bare forms.
var plain = func() map[rune]string {
	m := map[rune]string{'ß': "ss", 'æ': "ae", 'œ': "oe", 'ø': "o", 'ı': "i", 'đ': "d", 'ł': "l"}
	for bare, accented := range map[string]string{"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ď", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥ", "i": "ìíîïĩīĭį", "j": "ĵ", "k": "ķ",
		"l": "ĺļľ", "n": "ñńņň", "o": "òóôõöōŏő", "r": "ŕŗř", "s": "śŝşš", "t": "ţť", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž"} {
		for _, r := range accented {
			m[r] = bare
		}
	}
	return m
}()

// Fold lowercases, strips accents and reduces everything but letters and digits to single spaces, so
// titles and passages written a little differently still match.
func Fold(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if p, ok := plain[r]; ok {
				b.WriteString(p)
			} else {
				b.WriteRune(r)
			}
			space = false
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Group is one book's clippings, matched to a book on the shelf or not.
type Group struct {
	Title, Author string
	BookID        string // "" when the book isn't on the shelf
	BookTitle     string
	Fresh         []Clipping // not yet among the book's notes
	Same          int        // already there
	Total         int
}

// Plan matches clippings to the books on the shelf, by title (the part before a subtitle is enough), taking
// the author's surname to choose between books that share a title, and leaves out those already kept:
// same passage, or the same words when there is no passage.
func Plan(snap *store.Snapshot, clips []Clipping) []Group {
	var groups []*Group
	byTitle := map[string]*Group{}
	for _, c := range clips {
		g := byTitle[c.Title]
		if g == nil {
			g = &Group{Title: c.Title, Author: c.Author}
			byTitle[c.Title] = g
			groups = append(groups, g)
		}
		g.Total++
		g.Fresh = append(g.Fresh, c)
	}
	out := make([]Group, 0, len(groups))
	// What each book already has, and what this import has taken for it so far: two Kindle titles can be one
	// book on the shelf (a sample and the bought edition), and a note can be in the file twice.
	have := map[string]map[string]bool{}
	for _, g := range groups {
		b, ok := match(snap, g.Title, g.Author)
		if !ok {
			g.Fresh = nil
			out = append(out, *g)
			continue
		}
		g.BookID, g.BookTitle = b.ID, b.Title
		kept := have[b.ID]
		if kept == nil {
			kept = map[string]bool{}
			for _, e := range snap.Entries[b.ID] {
				kept[key(e.Quote, e.Text)] = true
			}
			have[b.ID] = kept
		}
		var fresh []Clipping
		for _, c := range g.Fresh {
			if kept[key(c.Quote, c.Text)] {
				g.Same++
				continue
			}
			kept[key(c.Quote, c.Text)] = true
			// Kindle's page numbers are the print edition's; one that runs past this copy's pages is left for the location.
			if w := c.Where; w != nil && w.Kind == store.WherePage && b.Pages != nil && w.N > *b.Pages {
				c.Where = c.Loc
			}
			fresh = append(fresh, c)
		}
		g.Fresh = fresh
		out = append(out, *g)
	}
	return out
}

func key(quote, text string) string {
	if quote != "" {
		return "q:" + Fold(quote)
	}
	return "t:" + Fold(text)
}

// head is a title without its subtitle or edition: Kindle writes "Frankenstein; Or, The Modern Prometheus" and
// "Middlemarch (Penguin Classics)" where the shelf may have "Frankenstein" and "Middlemarch: A Study of Provincial Life".
func head(title string) string {
	if i := strings.IndexAny(title, ":;("); i > 0 {
		return strings.TrimSpace(title[:i])
	}
	return title
}

// match finds the shelf's book for a Kindle title: the very same title first, and only failing that one
// that shares the part before a subtitle (so "X: Book Two" is never taken for "X: Book One").
func match(snap *store.Snapshot, title, author string) (store.Book, bool) {
	t, h := Fold(title), Fold(head(title))
	var exact, shared []store.Book
	for _, b := range snap.Books {
		switch bt := Fold(b.Title); {
		case bt == t:
			exact = append(exact, b)
		case bt == h || Fold(head(b.Title)) == h:
			shared = append(shared, b)
		}
	}
	found := exact
	if len(found) == 0 {
		found = shared
	}
	if len(found) > 1 && author != "" {
		sur := Fold(author)
		if f := strings.Fields(sur); len(f) > 0 {
			sur = f[len(f)-1]
		}
		for _, b := range found {
			if strings.Contains(Fold(b.Author), sur) {
				return b, true
			}
		}
	}
	if len(found) == 0 {
		return store.Book{}, false
	}
	return found[0], true
}

// Entries are the new notes a plan would write.
func Entries(plan []Group, public bool) []store.BookEntry {
	var out []store.BookEntry
	for _, g := range plan {
		for _, c := range g.Fresh {
			in := store.EntryInput{Quote: c.Quote, Text: c.Text, Where: c.Where, Public: public}
			if !c.At.IsZero() {
				at := c.At
				in.CreatedAt = &at
			}
			out = append(out, store.BookEntry{BookID: g.BookID, Entry: in})
		}
	}
	return out
}
