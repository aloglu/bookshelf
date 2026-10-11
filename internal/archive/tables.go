package archive

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/aloglu/bookshelf/internal/isbn"
	"github.com/aloglu/bookshelf/internal/store"
)

// CSV and JSON exports carry books only: a list anyone can open in a
// spreadsheet or another program. Covers, readings and notes travel in a
// full archive.

var columns = []string{"title", "author", "translator", "publisher", "published", "pages", "binding", "isbn", "issn", "issue", "hidden"}

// BookRow is one book in a CSV or JSON export.
type BookRow struct {
	Title      string `json:"title"`
	Author     string `json:"author"`
	Translator string `json:"translator"`
	Publisher  string `json:"publisher"`
	Published  *int   `json:"published"`
	Pages      *int   `json:"pages"`
	Binding    string `json:"binding"`
	ISBN       string `json:"isbn"`
	ISSN       string `json:"issn"`
	Issue      string `json:"issue"`
	Hidden     bool   `json:"hidden"`
}

func rowOf(b store.Book) BookRow {
	return BookRow{b.Title, b.Author, b.Translator, b.Publisher, b.Published, b.Pages, b.Binding, b.ISBN, b.ISSN, b.Issue, b.Hidden}
}

// WriteCSV writes books as CSV with a header row. A byte-order mark lets
// spreadsheet programs read accented names correctly.
func WriteCSV(w io.Writer, books []store.Book) error {
	io.WriteString(w, "\ufeff")
	cw := csv.NewWriter(w)
	cw.Write(columns)
	for _, b := range books {
		r := rowOf(b)
		cw.Write([]string{r.Title, r.Author, r.Translator, r.Publisher, num(r.Published), num(r.Pages), r.Binding, isbn.Hyphenate(r.ISBN), isbn.HyphenateISSN(r.ISSN), r.Issue, strconv.FormatBool(r.Hidden)})
	}
	cw.Flush()
	return cw.Error()
}

func num(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// WriteJSON writes books as a JSON list.
func WriteJSON(w io.Writer, books []store.Book) error {
	rows := make([]BookRow, 0, len(books))
	for _, b := range books {
		rows = append(rows, rowOf(b))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

// ReadCSV reads books from CSV. Columns are found by their header, in any
// order and case; unknown columns are ignored, so other programs' exports
// (with a title column) can be read. Rows without a title are skipped.
func ReadCSV(r io.Reader) ([]store.Book, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	head, err := cr.Read()
	if err != nil {
		return nil, bad("the CSV file is empty or can’t be read")
	}
	at := map[string]int{}
	for i, h := range head {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		switch h {
		case "isbn13", "isbn-13":
			h = "isbn"
		case "year", "year published", "original publication year":
			h = "published"
		case "number of pages", "page count":
			h = "pages"
		}
		if _, seen := at[h]; !seen {
			at[h] = i
		}
	}
	if _, ok := at["title"]; !ok {
		return nil, bad("the CSV file has no “title” column")
	}
	get := func(rec []string, col string) string {
		if i, ok := at[col]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	var books []store.Book
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, bad("line %d of the CSV file can’t be read", line)
		}
		b := bookOf(BookRow{Title: get(rec, "title"), Author: get(rec, "author"), Translator: get(rec, "translator"),
			Publisher: get(rec, "publisher"), Published: atoi(get(rec, "published")), Pages: atoi(get(rec, "pages")),
			Binding: get(rec, "binding"), ISBN: get(rec, "isbn"), ISSN: get(rec, "issn"), Issue: get(rec, "issue"),
			Hidden: truthy(get(rec, "hidden"))})
		if b.Title != "" {
			books = append(books, b)
		}
	}
	return books, nil
}

// ReadJSON reads books from a JSON list (as WriteJSON writes it).
func ReadJSON(r io.Reader) ([]store.Book, error) {
	var rows []BookRow
	if err := json.NewDecoder(io.LimitReader(r, maxJSON)).Decode(&rows); err != nil {
		return nil, bad("the JSON file isn’t a list of books")
	}
	var books []store.Book
	for _, row := range rows {
		if b := bookOf(row); b.Title != "" {
			books = append(books, b)
		}
	}
	return books, nil
}

// bookOf turns a row into a book. A bad identifier shouldn't lose the book;
// it comes in without one. An ISSN in the ISBN column is recognized.
func bookOf(r BookRow) store.Book {
	id, _ := isbn.Parse(strings.Trim(r.ISBN, `="`)) // spreadsheet exports often wrap ISBNs as ="…"
	if id.ISBN == "" && r.ISSN != "" {
		id.ISSN, _ = isbn.NormalizeISSN(strings.Trim(r.ISSN, `="`))
	}
	b := store.Book{Title: r.Title, Author: r.Author, Translator: r.Translator, Publisher: r.Publisher,
		Published: r.Published, Pages: r.Pages, Binding: store.NormalizeBinding(r.Binding), ISBN: id.ISBN, ISSN: id.ISSN, Hidden: r.Hidden}
	if b.ISSN != "" {
		b.Issue = r.Issue
	}
	return b
}

func atoi(s string) *int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &v
}

func truthy(s string) bool {
	switch strings.ToLower(s) {
	case "true", "yes", "1", "hidden":
		return true
	}
	return false
}
