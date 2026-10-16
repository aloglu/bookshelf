package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Snapshot is the whole library at one moment. It is shared between readers
// and must not be modified; every change builds a new one.
type Snapshot struct {
	Books    []Book               // in the order they were added
	byID     map[string]int       // index into Books
	Readings map[string][]Reading // by book id, oldest first
	Entries  map[string][]Entry   // notes and highlights by book id, oldest first
	Settings Settings
	Sessions map[string]Session // by token hash
}

// VisitorsReadNotes says whether visitors may read a book's public notes: the library is open to them, notes
// are switched on, and neither the book nor its notes are hidden from them. The site and the Markdown export
// both go by it.
func (s *Snapshot) VisitorsReadNotes(b Book) bool {
	return !s.Settings.Private && s.Settings.Notes && !b.Hidden && !b.NotesHidden
}

// Book returns the book with id.
func (s *Snapshot) Book(id string) (Book, bool) {
	i, ok := s.byID[id]
	if !ok {
		return Book{}, false
	}
	return s.Books[i], true
}

// NowReading is every reading in progress (one book or several), the most
// recently started first.
func (s *Snapshot) NowReading() []Reading {
	var now []Reading
	for _, list := range s.Readings {
		for _, r := range list {
			if r.Outcome == OutcomeReading {
				now = append(now, r)
			}
		}
	}
	sort.Slice(now, func(i, j int) bool { return readingOrder(now[i]) > readingOrder(now[j]) })
	return now
}

// InProgress is the book's reading in progress, if it is being read.
func (s *Snapshot) InProgress(bookID string) (Reading, bool) {
	for _, r := range s.Readings[bookID] {
		if r.Outcome == OutcomeReading {
			return r, true
		}
	}
	return Reading{}, false
}

// Entry returns the note or highlight with id.
func (s *Snapshot) Entry(id string) (Entry, bool) {
	for _, list := range s.Entries {
		for _, e := range list {
			if e.ID == id {
				return e, true
			}
		}
	}
	return Entry{}, false
}

// Reading returns the reading with id.
func (s *Snapshot) Reading(id string) (Reading, bool) {
	for _, list := range s.Readings {
		for _, r := range list {
			if r.ID == id {
				return r, true
			}
		}
	}
	return Reading{}, false
}

func (s *Store) reload(ctx context.Context) error {
	snap, err := load(ctx, s.db)
	if err != nil {
		return err
	}
	s.snap.Store(snap)
	return nil
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func load(ctx context.Context, q querier) (*Snapshot, error) {
	snap := &Snapshot{byID: map[string]int{}, Readings: map[string][]Reading{}, Entries: map[string][]Entry{}, Sessions: map[string]Session{}}

	rows, err := q.QueryContext(ctx, `SELECT id, title, author, translator, publisher, published, pages, binding,
		COALESCE(isbn, ''), issn, issue, slug, hidden, notes_hidden, cover, spine_color, spine_text_color, spine_color_override, added_at, updated_at
		FROM books ORDER BY added_at, id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var b Book
		var published, pages sql.NullInt64
		var added, updated string
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Translator, &b.Publisher, &published, &pages, &b.Binding,
			&b.ISBN, &b.ISSN, &b.Issue, &b.Slug, &b.Hidden, &b.NotesHidden, &b.Cover, &b.SpineColor, &b.SpineTextColor, &b.SpineColorOverride, &added, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		if published.Valid {
			v := int(published.Int64)
			b.Published = &v
		}
		if pages.Valid {
			v := int(pages.Int64)
			b.Pages = &v
		}
		b.AddedAt, _ = time.Parse(time.RFC3339Nano, added)
		b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		snap.byID[b.ID] = len(snap.Books)
		snap.Books = append(snap.Books, b)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	progress := map[string][]Progress{}
	rows, err = q.QueryContext(ctx, `SELECT reading_id, page, at FROM reading_progress ORDER BY at`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, at string
		var p Progress
		if err := rows.Scan(&id, &p.Page, &at); err != nil {
			rows.Close()
			return nil, err
		}
		p.At, _ = time.Parse(time.RFC3339Nano, at)
		progress[id] = append(progress[id], p)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = q.QueryContext(ctx, `SELECT id, book_id, COALESCE(reading_id, ''), pre, quote, text, where_kind, where_n, where_to, public, created_at, updated_at
		FROM entries ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e Entry
		var kind string
		var n, to sql.NullInt64
		var created, updated string
		if err := rows.Scan(&e.ID, &e.BookID, &e.ReadingID, &e.Pre, &e.Quote, &e.Text, &kind, &n, &to, &e.Public, &created, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		if kind != "" && n.Valid {
			e.Where = &Where{Kind: kind, N: int(n.Int64), To: int(to.Int64)}
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		snap.Entries[e.BookID] = append(snap.Entries[e.BookID], e)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}

	rows, err = q.QueryContext(ctx, `SELECT id, book_id, started_at, started_precision, ended_at, ended_precision, outcome FROM readings`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r Reading
		var sa, sp, ea, ep string
		if err := rows.Scan(&r.ID, &r.BookID, &sa, &sp, &ea, &ep, &r.Outcome); err != nil {
			rows.Close()
			return nil, err
		}
		if r.Started, err = decodeWhen(sa, Precision(sp)); err != nil {
			rows.Close()
			return nil, fmt.Errorf("reading %s: %w", r.ID, err)
		}
		if r.Ended, err = decodeWhen(ea, Precision(ep)); err != nil {
			rows.Close()
			return nil, fmt.Errorf("reading %s: %w", r.ID, err)
		}
		r.Progress = progress[r.ID]
		snap.Readings[r.BookID] = append(snap.Readings[r.BookID], r)
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	for id, list := range snap.Readings {
		sort.SliceStable(list, func(i, j int) bool { return readingOrder(list[i]) < readingOrder(list[j]) })
		snap.Readings[id] = list
	}

	snap.Settings = DefaultSettings()
	rows, err = q.QueryContext(ctx, `SELECT value FROM settings WHERE key = 'settings'`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &snap.Settings); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read settings: %w", err)
		}
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	snap.Settings.upgrade()

	rows, err = q.QueryContext(ctx, `SELECT token_hash, created_at, expires_at, last_seen, user_agent FROM sessions`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ss Session
		var c, e, l string
		if err := rows.Scan(&ss.TokenHash, &c, &e, &l, &ss.UserAgent); err != nil {
			rows.Close()
			return nil, err
		}
		ss.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		ss.ExpiresAt, _ = time.Parse(time.RFC3339Nano, e)
		ss.LastSeen, _ = time.Parse(time.RFC3339Nano, l)
		snap.Sessions[ss.TokenHash] = ss
	}
	return snap, closeRows(rows)
}

func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	return rows.Close()
}

// readingOrder sorts readings by when they started; undated ones by when they
// ended, and fully undated ones first.
func readingOrder(r Reading) string {
	switch {
	case r.Started.Known():
		return stamp(r.Started.Time)
	case r.Ended.Known():
		return stamp(r.Ended.Time)
	default:
		return ""
	}
}
