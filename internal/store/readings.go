package store

import (
	"context"
	"database/sql"
	"time"
)

// StartReading begins a tracked reading of a book ("Reading now"). Several
// books can be read at once, but a book has only one reading in progress:
// starting a book already being read returns that reading. A book read
// before can be started again; that is a re-read and earlier readings are
// left as they are.
func (s *Store) StartReading(ctx context.Context, bookID string) (Reading, error) {
	id := newID()
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Book(bookID); !ok {
			return ErrNotFound
		}
		if cur, ok := snap.InProgress(bookID); ok {
			id = cur.ID
			return nil
		}
		started := When{Time: s.now(), Precision: Instant}
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

// DeleteReading removes a reading and its page log. Notes and highlights
// written during it stay, no longer tied to a reading.
func (s *Store) DeleteReading(ctx context.Context, readingID string) error {
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Reading(readingID); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM readings WHERE id=?`, readingID)
		return err
	})
}
