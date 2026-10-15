package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// Library is everything that makes up a library, for backups, exports,
// imports and restores. Sessions are never part of it.
type Library struct {
	Books    []Book    `json:"books"`
	Readings []Reading `json:"readings"`
	Entries  []Entry   `json:"entries"`
	Settings Settings  `json:"settings"`
}

// Library is the snapshot's contents.
func (s *Snapshot) Library() Library {
	lib := Library{Books: append([]Book(nil), s.Books...), Readings: []Reading{}, Entries: []Entry{}, Settings: s.Settings}
	for _, b := range s.Books {
		lib.Readings = append(lib.Readings, s.Readings[b.ID]...)
		lib.Entries = append(lib.Entries, s.Entries[b.ID]...)
	}
	return lib
}

// Replace swaps the whole library for lib, in one transaction (a restore).
// Signed-in sessions are kept.
func (s *Store) Replace(ctx context.Context, lib Library) error {
	if err := lib.Settings.Validate(); err != nil {
		return invalid("The backup's settings aren't valid: %s", err.Error())
	}
	return s.change(ctx, func(tx *sql.Tx, _ *Snapshot) error {
		for _, table := range []string{"entries", "reading_progress", "readings", "books", "settings"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				return err
			}
		}
		empty := &Snapshot{byID: map[string]int{}, Readings: map[string][]Reading{}}
		if _, _, err := insertLibrary(ctx, tx, empty, lib, false, s.now()); err != nil {
			return err
		}
		raw, _ := json.Marshal(lib.Settings)
		_, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('settings', ?)`, string(raw))
		return err
	})
}

// Merge adds the books in lib that aren't on the shelf yet (an import): a
// book is already there if it has the same ISBN, or, without one, the same
// title and author. Their readings, notes and highlights come with them. The library's
// own settings are left alone. It returns how many books were added and
// skipped.
func (s *Store) Merge(ctx context.Context, lib Library) (added, skipped int, err error) {
	err = s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		added, skipped, err = insertLibrary(ctx, tx, snap, lib, true, s.now())
		return err
	})
	return added, skipped, err
}

// sameBook says whether two books are the same: by ISBN, by periodical
// issue, or, with neither, by title and author.
func sameBook(a, b Book) bool {
	switch {
	case a.ISBN != "" || b.ISBN != "":
		return a.ISBN == b.ISBN
	case a.ISSN != "" || b.ISSN != "":
		return a.ISSN == b.ISSN && strings.EqualFold(a.Issue, b.Issue)
	}
	return strings.EqualFold(a.Title, b.Title) && strings.EqualFold(a.Author, b.Author)
}

// insertLibrary writes lib's books, readings and entries into tx, beside what snap
// already holds. With skipExisting, books already in snap are left out;
// otherwise every book must be valid and new.
func insertLibrary(ctx context.Context, tx *sql.Tx, snap *Snapshot, lib Library, skipExisting bool, now time.Time) (added, skipped int, err error) {
	// The working set grows as books are added, so slugs and ISBNs stay unique
	// within the import as well as against the shelf.
	work := &Snapshot{byID: map[string]int{}, Readings: map[string][]Reading{}}
	work.Books = append(work.Books, snap.Books...)
	for i, b := range work.Books {
		work.byID[b.ID] = i
	}
	newIDs := map[string]string{} // archive id -> id on this shelf
	for _, b := range lib.Books {
		in := BookInput{Title: b.Title, Author: b.Author, Translator: b.Translator, Publisher: b.Publisher,
			Published: b.Published, Pages: b.Pages, Binding: b.Binding, ISBN: b.ISBN, ISSN: b.ISSN, Issue: b.Issue, Hidden: b.Hidden}
		if err := in.clean(); err != nil {
			if skipExisting {
				skipped++
				continue
			}
			return 0, 0, invalid("“%s”: %s", b.Title, err.Error())
		}
		cand := b
		cand.Title, cand.Author, cand.ISBN, cand.ISSN, cand.Issue = in.Title, in.Author, in.ISBN, in.ISSN, in.Issue
		dup := false
		for _, have := range work.Books {
			if sameBook(have, cand) {
				dup = true
				break
			}
		}
		if dup {
			if skipExisting {
				skipped++
				continue
			}
			return 0, 0, invalid("“%s” appears twice in the backup.", b.Title)
		}
		id := b.ID
		if _, taken := work.byID[id]; taken || id == "" {
			id = newID()
		}
		newIDs[b.ID] = id
		addedAt, updatedAt := b.AddedAt, b.UpdatedAt
		if addedAt.IsZero() {
			addedAt = now
		}
		if updatedAt.IsZero() {
			updatedAt = addedAt
		}
		cand.ID, cand.Slug = id, uniqueSlug(work, in.Title, "")
		cand.Translator, cand.Publisher, cand.Binding = in.Translator, in.Publisher, in.Binding
		if _, err := tx.ExecContext(ctx, `INSERT INTO books (id, title, author, translator, publisher, published, pages, binding, isbn, issn, issue, slug, hidden,
			notes_hidden, cover, spine_color, spine_text_color, spine_color_override, added_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, cand.Title, cand.Author, cand.Translator, cand.Publisher, nullable(in.Published), nullable(in.Pages), cand.Binding, nullISBN(cand.ISBN), cand.ISSN, cand.Issue,
			cand.Slug, in.Hidden, b.NotesHidden, b.Cover, b.SpineColor, b.SpineTextColor, b.SpineColorOverride, stamp(addedAt), stamp(updatedAt)); err != nil {
			return 0, 0, err
		}
		work.byID[id] = len(work.Books)
		work.Books = append(work.Books, cand)
		added++
	}

	inProgress := map[string]bool{}   // books given a reading in progress
	readingIDs := map[string]string{} // archive id -> id on this shelf
	for _, r := range lib.Readings {
		book, ok := newIDs[r.BookID]
		if !ok {
			continue // its book was skipped
		}
		outcome := r.Outcome
		if outcome == OutcomeReading {
			if inProgress[book] { // a book has only one reading in progress
				outcome = OutcomeSetAside
			}
			inProgress[book] = true
		}
		id := newID()
		readingIDs[r.ID] = id
		if _, err := tx.ExecContext(ctx, `INSERT INTO readings (id, book_id, started_at, started_precision, ended_at, ended_precision, outcome) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, book, r.Started.encode(), precisionOrUnknown(r.Started.Precision), r.Ended.encode(), precisionOrUnknown(r.Ended.Precision), outcome); err != nil {
			return 0, 0, err
		}
		for _, p := range r.Progress {
			if _, err := tx.ExecContext(ctx, `INSERT INTO reading_progress (reading_id, page, at) VALUES (?, ?, ?)`, id, p.Page, stamp(p.At)); err != nil {
				return 0, 0, err
			}
		}
	}

	for _, e := range lib.Entries {
		book, ok := newIDs[e.BookID]
		if !ok {
			continue // its book was skipped
		}
		in := EntryInput{Pre: e.Pre, Quote: e.Quote, Text: e.Text, Where: e.Where, Public: e.Public}
		b := work.Books[work.byID[book]]
		err := in.check(work, b, now.Add(24*time.Hour))
		if err != nil && in.Where != nil { // a place that no longer fits the book (its page count was lowered) is let go
			in.Where = nil
			err = in.check(work, b, now.Add(24*time.Hour))
		}
		if err != nil {
			if skipExisting {
				continue
			}
			return 0, 0, invalid("A note in the backup isn’t valid: %s", err.Error())
		}
		created := orNow(e.CreatedAt, now)
		if _, err := tx.ExecContext(ctx, insertEntry,
			newID(), book, nullString(readingIDs[e.ReadingID]), in.Pre, in.Quote, in.Text, in.Where.kind(), in.Where.n(), in.Where.to(), in.Public, stamp(created), stamp(orNow(e.UpdatedAt, created))); err != nil {
			return 0, 0, err
		}
	}
	return added, skipped, nil
}

func orNow(t, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}
	return t
}

func precisionOrUnknown(p Precision) Precision {
	if p == "" {
		return Unknown
	}
	return p
}
