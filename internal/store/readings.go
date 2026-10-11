package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// StartReading begins a tracked reading of a book ("Reading now"). Only one
// reading is in progress at a time: one already in progress, of this book or
// another, is set aside now. A book read before can be started again; that
// is a re-read and earlier readings are left as they are.
func (s *Store) StartReading(ctx context.Context, bookID string) (Reading, error) {
	id := newID()
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Book(bookID); !ok {
			return ErrNotFound
		}
		now := s.now()
		if cur, ok := snap.Current(); ok {
			if err := endReading(ctx, tx, cur.ID, OutcomeSetAside, When{Time: now, Precision: Instant}); err != nil {
				return err
			}
		}
		started := When{Time: now, Precision: Instant}
		_, err := tx.ExecContext(ctx, `INSERT INTO readings (id, book_id, started_at, started_precision, ended_at, ended_precision, outcome) VALUES (?, ?, ?, ?, '', 'unknown', 'reading')`,
			id, bookID, started.encode(), started.Precision)
		return err
	})
	if err != nil {
		return Reading{}, err
	}
	r, _ := s.Snapshot().Reading(id)
	return r, nil
}

// MarkRead records a finished reading that wasn't tracked: the book was read,
// finished when (a day, a year, or unknown). It can be repeated for a book
// read more than once.
func (s *Store) MarkRead(ctx context.Context, bookID string, finished When) (Reading, error) {
	if err := checkWhen(finished, s.now()); err != nil {
		return Reading{}, err
	}
	id := newID()
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Book(bookID); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO readings (id, book_id, started_at, started_precision, ended_at, ended_precision, outcome) VALUES (?, ?, '', 'unknown', ?, ?, 'finished')`,
			id, bookID, finished.encode(), finished.Precision)
		return err
	})
	if err != nil {
		return Reading{}, err
	}
	r, _ := s.Snapshot().Reading(id)
	return r, nil
}

func checkWhen(w When, now time.Time) error {
	switch w.Precision {
	case Unknown:
		return nil
	case Day, Year, Instant:
		if w.Time.After(now.Add(24 * time.Hour)) {
			return invalid("That date is in the future.")
		}
		if w.Time.Year() < 1000 {
			return invalid("That date doesn’t look right.")
		}
		return nil
	default:
		return invalid("Unknown date precision.")
	}
}

// SetPage records the page reached in a reading in progress.
func (s *Store) SetPage(ctx context.Context, readingID string, page int) (Reading, error) {
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		r, ok := snap.Reading(readingID)
		if !ok {
			return ErrNotFound
		}
		if r.Outcome != OutcomeReading {
			return invalid("Only a book being read has a current page.")
		}
		b, _ := snap.Book(r.BookID)
		if page < 0 || (b.Pages != nil && page > *b.Pages) {
			return invalid("That page isn’t in the book.")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO reading_progress (reading_id, page, at) VALUES (?, ?, ?)`, readingID, page, stamp(s.now()))
		return err
	})
	if err != nil {
		return Reading{}, err
	}
	r, _ := s.Snapshot().Reading(readingID)
	return r, nil
}

// EndReading finishes a reading in progress or sets it aside, now.
func (s *Store) EndReading(ctx context.Context, readingID, outcome string) (Reading, error) {
	if outcome != OutcomeFinished && outcome != OutcomeSetAside {
		return Reading{}, invalid("A reading ends finished or set aside.")
	}
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		r, ok := snap.Reading(readingID)
		if !ok {
			return ErrNotFound
		}
		if r.Outcome != OutcomeReading {
			return invalid("This reading has already ended.")
		}
		return endReading(ctx, tx, readingID, outcome, When{Time: s.now(), Precision: Instant})
	})
	if err != nil {
		return Reading{}, err
	}
	r, _ := s.Snapshot().Reading(readingID)
	return r, nil
}

func endReading(ctx context.Context, tx *sql.Tx, id, outcome string, at When) error {
	_, err := tx.ExecContext(ctx, `UPDATE readings SET outcome=?, ended_at=?, ended_precision=? WHERE id=?`, outcome, at.encode(), at.Precision, id)
	return err
}

// CorrectReading changes a past reading's dates and outcome, to fix mistakes.
// A reading can't be put back in progress this way.
func (s *Store) CorrectReading(ctx context.Context, readingID string, started, ended When, outcome string) (Reading, error) {
	now := s.now()
	if outcome != OutcomeFinished && outcome != OutcomeSetAside {
		return Reading{}, invalid("A past reading is finished or set aside.")
	}
	for _, w := range []When{started, ended} {
		if err := checkWhen(w, now); err != nil {
			return Reading{}, err
		}
	}
	if started.Known() && ended.Known() && ended.Time.Before(started.Time) && started.Precision != Year && ended.Precision != Year {
		return Reading{}, invalid("It ends before it starts.")
	}
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		r, ok := snap.Reading(readingID)
		if !ok {
			return ErrNotFound
		}
		if r.Outcome == OutcomeReading {
			return invalid("Finish or set aside the reading first.")
		}
		if outcome != OutcomeFinished && r.Note != nil {
			return invalid("A reading with a note stays finished; delete the note first.")
		}
		_, err := tx.ExecContext(ctx, `UPDATE readings SET started_at=?, started_precision=?, ended_at=?, ended_precision=?, outcome=? WHERE id=?`,
			started.encode(), started.Precision, ended.encode(), ended.Precision, outcome, readingID)
		return err
	})
	if err != nil {
		return Reading{}, err
	}
	r, _ := s.Snapshot().Reading(readingID)
	return r, nil
}

// DeleteReading removes a reading, with its page log and note.
func (s *Store) DeleteReading(ctx context.Context, readingID string) error {
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Reading(readingID); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM readings WHERE id=?`, readingID)
		return err
	})
}

// SetNote writes the note on a finished reading.
func (s *Store) SetNote(ctx context.Context, readingID, text string, public bool) (Reading, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Reading{}, invalid("The note is empty.")
	}
	if len(text) > 200_000 {
		return Reading{}, invalid("The note is too long.")
	}
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		r, ok := snap.Reading(readingID)
		if !ok {
			return ErrNotFound
		}
		if r.Outcome != OutcomeFinished {
			return invalid("Notes are for books that have been read.")
		}
		now := stamp(s.now())
		_, err := tx.ExecContext(ctx, `INSERT INTO notes (reading_id, text, public, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(reading_id) DO UPDATE SET text=excluded.text, public=excluded.public, updated_at=excluded.updated_at`,
			readingID, text, public, now, now)
		return err
	})
	if err != nil {
		return Reading{}, err
	}
	r, _ := s.Snapshot().Reading(readingID)
	return r, nil
}

// DeleteNote removes the note on a reading.
func (s *Store) DeleteNote(ctx context.Context, readingID string) error {
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Reading(readingID); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE reading_id=?`, readingID)
		return err
	})
}
