package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// EntryInput is a note as the owner writes it: a passage, words, or both, with
// words on either side of the passage, and where in the book it comes from.
type EntryInput struct {
	Pre       string     `json:"pre"`
	Quote     string     `json:"quote"`
	Text      string     `json:"text"`
	Where     *Where     `json:"where"`
	ReadingID string     `json:"readingId"`
	Public    bool       `json:"public"`
	CreatedAt *time.Time `json:"createdAt"` // to put back an entry just deleted (undo), or an imported one's own date
}

const maxEntryText = 200_000

const insertEntry = `INSERT INTO entries (id, book_id, reading_id, pre, quote, text, where_kind, where_n, where_to, public, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func (w *Where) kind() string {
	if w == nil {
		return ""
	}
	return w.Kind
}

func (w *Where) n() any {
	if w == nil {
		return nil
	}
	return w.N
}

func (w *Where) to() any {
	if w == nil || w.To == 0 {
		return nil
	}
	return w.To
}

// check cleans the input and checks it against the book it belongs to. kept is a place the note already has
// (editing it, or putting it back just after deleting it): it stays, even past the book's pages since the
// book's page count was lowered; a page given anew must be one of the book's.
func (in *EntryInput) check(snap *Snapshot, b Book, now time.Time, kept *Where) error {
	in.Pre, in.Quote, in.Text = strings.TrimSpace(in.Pre), strings.TrimSpace(in.Quote), strings.TrimSpace(in.Text)
	if in.Quote == "" && in.Pre != "" { // words with no passage are just words
		in.Text, in.Pre = strings.TrimSpace(in.Pre+"\n"+in.Text), ""
	}
	if in.Quote == "" && in.Text == "" {
		return invalid("Write something, or copy in a passage.")
	}
	if len(in.Pre) > maxEntryText || len(in.Quote) > maxEntryText || len(in.Text) > maxEntryText {
		return invalid("That’s too long.")
	}
	if w := in.Where; w != nil {
		switch w.Kind {
		case WherePage, WhereChapter, WhereLoc:
		default:
			return invalid("A place is a page, a chapter or a location.")
		}
		if w.N < 1 {
			return invalid("Give the place as a number, 1 or more.")
		}
		if w.Kind == WherePage && b.Pages != nil && w.N > *b.Pages && !(kept != nil && kept.Kind == w.Kind && kept.N == w.N) {
			return invalid("Pages run from 1 to %d.", *b.Pages)
		}
		if w.To != 0 && (w.Kind != WhereLoc || w.To <= w.N) {
			w.To = 0
		}
	}
	if in.ReadingID != "" {
		r, ok := snap.Reading(in.ReadingID)
		if !ok || r.BookID != b.ID {
			return invalid("That reading isn’t one of this book’s.")
		}
	}
	if in.CreatedAt != nil && in.CreatedAt.After(now) {
		return invalid("That date is in the future.")
	}
	return nil
}

// AddEntry writes a new note about a book.
func (s *Store) AddEntry(ctx context.Context, bookID string, in EntryInput) (Entry, error) {
	id := newID()
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		b, ok := snap.Book(bookID)
		if !ok {
			return ErrNotFound
		}
		now := s.now()
		var kept *Where
		if in.CreatedAt != nil { // putting back a note just deleted: its place is its own
			kept = in.Where
		}
		if err := in.check(snap, b, now, kept); err != nil {
			return err
		}
		created := now
		if in.CreatedAt != nil {
			created = *in.CreatedAt
		}
		_, err := tx.ExecContext(ctx, insertEntry,
			id, bookID, nullString(in.ReadingID), in.Pre, in.Quote, in.Text, in.Where.kind(), in.Where.n(), in.Where.to(), in.Public, stamp(created), stamp(now))
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	e, _ := s.Snapshot().Entry(id)
	return e, nil
}

// AddEntries writes many notes at once (an import), each to its own book,
// in one change. Notes that are not valid are left out; it returns how many
// were written.
func (s *Store) AddEntries(ctx context.Context, items []BookEntry) (added int, err error) {
	err = s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		now := s.now()
		added = 0
		for _, it := range items {
			b, ok := snap.Book(it.BookID)
			if !ok {
				continue
			}
			in := it.Entry
			if in.check(snap, b, now, nil) != nil {
				continue
			}
			created := now
			if in.CreatedAt != nil {
				created = *in.CreatedAt
			}
			if _, err := tx.ExecContext(ctx, insertEntry,
				newID(), it.BookID, nullString(in.ReadingID), in.Pre, in.Quote, in.Text, in.Where.kind(), in.Where.n(), in.Where.to(), in.Public, stamp(created), stamp(now)); err != nil {
				return err
			}
			added++
		}
		return nil
	})
	return added, err
}

// BookEntry is a note for one book.
type BookEntry struct {
	BookID string
	Entry  EntryInput
}

// UpdateEntry rewrites a note. It keeps when it was first written.
func (s *Store) UpdateEntry(ctx context.Context, id string, in EntryInput) (Entry, error) {
	in.CreatedAt = nil
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		e, ok := snap.Entry(id)
		if !ok {
			return ErrNotFound
		}
		b, _ := snap.Book(e.BookID)
		if err := in.check(snap, b, s.now(), e.Where); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE entries SET reading_id=?, pre=?, quote=?, text=?, where_kind=?, where_n=?, where_to=?, public=?, updated_at=? WHERE id=?`,
			nullString(in.ReadingID), in.Pre, in.Quote, in.Text, in.Where.kind(), in.Where.n(), in.Where.to(), in.Public, stamp(s.now()), id)
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	e, _ := s.Snapshot().Entry(id)
	return e, nil
}

// DeleteEntry removes a note.
func (s *Store) DeleteEntry(ctx context.Context, id string) error {
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Entry(id); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE id=?`, id)
		return err
	})
}

// SetNotesHidden keeps a book's notes from visitors, or lets them read the public ones again.
func (s *Store) SetNotesHidden(ctx context.Context, id string, hidden bool) error {
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Book(id); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `UPDATE books SET notes_hidden=? WHERE id=?`, hidden, id)
		return err
	})
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
