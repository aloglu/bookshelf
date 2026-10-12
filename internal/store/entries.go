package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// EntryInput is a note or highlight as the owner writes it.
type EntryInput struct {
	Kind      string     `json:"kind"`
	Text      string     `json:"text"`
	Quote     string     `json:"quote"`
	Page      *int       `json:"page"`
	ReadingID string     `json:"readingId"`
	Public    bool       `json:"public"`
	CreatedAt *time.Time `json:"createdAt"` // only to put back an entry just deleted (undo)
}

const maxEntryText = 200_000

// check cleans the input and checks it against the book it belongs to.
func (in *EntryInput) check(snap *Snapshot, b Book, now time.Time) error {
	in.Text, in.Quote = strings.TrimSpace(in.Text), strings.TrimSpace(in.Quote)
	switch in.Kind {
	case KindNote:
		in.Quote, in.Page = "", nil
		if in.Text == "" {
			return invalid("Write something first.")
		}
	case KindHighlight:
		if in.Quote == "" {
			return invalid("Copy in the passage first.")
		}
	default:
		return invalid("An entry is a note or a highlight.")
	}
	if len(in.Text) > maxEntryText || len(in.Quote) > maxEntryText {
		return invalid("That’s too long.")
	}
	if in.Page != nil && (*in.Page < 1 || (b.Pages != nil && *in.Page > *b.Pages)) {
		if b.Pages != nil {
			return invalid("Pages run from 1 to %d.", *b.Pages)
		}
		return invalid("That page isn’t in the book.")
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

// AddEntry writes a new note or highlight about a book.
func (s *Store) AddEntry(ctx context.Context, bookID string, in EntryInput) (Entry, error) {
	id := newID()
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		b, ok := snap.Book(bookID)
		if !ok {
			return ErrNotFound
		}
		now := s.now()
		if err := in.check(snap, b, now); err != nil {
			return err
		}
		created := now
		if in.CreatedAt != nil {
			created = *in.CreatedAt
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO entries (id, book_id, reading_id, kind, text, quote, page, public, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, bookID, nullString(in.ReadingID), in.Kind, in.Text, in.Quote, nullable(in.Page), in.Public, stamp(created), stamp(now))
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	e, _ := s.Snapshot().Entry(id)
	return e, nil
}

// UpdateEntry rewrites a note or highlight. It keeps when it was first written.
func (s *Store) UpdateEntry(ctx context.Context, id string, in EntryInput) (Entry, error) {
	in.CreatedAt = nil
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		e, ok := snap.Entry(id)
		if !ok {
			return ErrNotFound
		}
		b, _ := snap.Book(e.BookID)
		if err := in.check(snap, b, s.now()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE entries SET reading_id=?, kind=?, text=?, quote=?, page=?, public=?, updated_at=? WHERE id=?`,
			nullString(in.ReadingID), in.Kind, in.Text, in.Quote, nullable(in.Page), in.Public, stamp(s.now()), id)
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	e, _ := s.Snapshot().Entry(id)
	return e, nil
}

// DeleteEntry removes a note or highlight.
func (s *Store) DeleteEntry(ctx context.Context, id string) error {
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Entry(id); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE id=?`, id)
		return err
	})
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
