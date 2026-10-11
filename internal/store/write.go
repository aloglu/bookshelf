package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	goslug "github.com/gosimple/slug"
)

// Errors a change can fail with. Callers show their text to the owner.
var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicateISBN = errors.New("a book with this ISBN is already on the shelf")
)

// InvalidError is a value the owner can correct.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &InvalidError{Msg: fmt.Sprintf(format, args...)}
}

// change runs fn in one transaction, taking turns with every other change, and
// replaces the snapshot when it commits.
func (s *Store) change(ctx context.Context, fn func(tx *sql.Tx, snap *Snapshot) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx, s.snap.Load()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.reload(ctx)
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// stamp is fixed-width UTC, so stamps compare correctly as text in SQL.
const stampLayout = "2006-01-02T15:04:05.000000000Z"

func stamp(t time.Time) string { return t.UTC().Format(stampLayout) }

// ---------------------------------------------------------------- books

// BookInput is what the owner gives for a new or edited book. ISBN must
// already be normalized to bare ISBN-13 digits (see package isbn), or "".
type BookInput struct {
	Title      string
	Author     string
	Translator string
	Publisher  string
	Published  *int
	Pages      *int
	Binding    string
	ISBN       string
	Hidden     bool
}

var isbnDigits = regexp.MustCompile(`^97[89]\d{10}$`)

func (in *BookInput) clean() error {
	trim := func(p *string) { *p = strings.Join(strings.Fields(*p), " ") }
	for _, p := range []*string{&in.Title, &in.Author, &in.Translator, &in.Publisher} {
		trim(p)
	}
	if in.Title == "" {
		return invalid("A book needs a title.")
	}
	if len([]rune(in.Title)) > 300 {
		return invalid("The title is longer than 300 characters.")
	}
	if in.Published != nil && (*in.Published < -3000 || *in.Published > 3000) {
		return invalid("The year doesn’t look right.")
	}
	if in.Pages != nil && (*in.Pages < 1 || *in.Pages > 100000) {
		return invalid("The page count doesn’t look right.")
	}
	in.Binding = NormalizeBinding(in.Binding)
	if in.ISBN != "" && !isbnDigits.MatchString(in.ISBN) {
		return invalid("The ISBN isn’t valid.")
	}
	return nil
}

// NormalizeBinding maps any binding text to one of the four bindings.
func NormalizeBinding(value string) string {
	v := strings.ToLower(value)
	switch {
	case strings.Contains(v, "leather"):
		return Leather
	case strings.Contains(v, "cloth"):
		return Cloth
	case strings.Contains(v, "hard"):
		return Hardcover
	default:
		return Paperback // mass market counts as paperback
	}
}

// uniqueSlug makes a slug from title that no other book (but self) has.
func uniqueSlug(snap *Snapshot, title, self string) string {
	base := goslug.Make(title)
	if base == "" {
		base = "book"
	}
	taken := map[string]bool{}
	for _, b := range snap.Books {
		if b.ID != self {
			taken[b.Slug] = true
		}
	}
	slug := base
	for n := 2; taken[slug]; n++ {
		slug = fmt.Sprintf("%s-%d", base, n)
	}
	return slug
}

func checkISBN(snap *Snapshot, isbn, self string) error {
	if isbn == "" {
		return nil
	}
	for _, b := range snap.Books {
		if b.ID != self && b.ISBN == isbn {
			return ErrDuplicateISBN
		}
	}
	return nil
}

func nullable(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullISBN(isbn string) any {
	if isbn == "" {
		return nil
	}
	return isbn
}

// AddBook adds a book and returns it.
func (s *Store) AddBook(ctx context.Context, in BookInput) (Book, error) {
	if err := in.clean(); err != nil {
		return Book{}, err
	}
	id := newID()
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if err := checkISBN(snap, in.ISBN, ""); err != nil {
			return err
		}
		now := stamp(s.now())
		_, err := tx.ExecContext(ctx, `INSERT INTO books (id, title, author, translator, publisher, published, pages, binding, isbn, slug, hidden, added_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, in.Title, in.Author, in.Translator, in.Publisher, nullable(in.Published), nullable(in.Pages), in.Binding,
			nullISBN(in.ISBN), uniqueSlug(snap, in.Title, ""), in.Hidden, now, now)
		return err
	})
	if err != nil {
		return Book{}, err
	}
	b, _ := s.Snapshot().Book(id)
	return b, nil
}

// UpdateBook replaces a book's details.
func (s *Store) UpdateBook(ctx context.Context, id string, in BookInput) (Book, error) {
	if err := in.clean(); err != nil {
		return Book{}, err
	}
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		old, ok := snap.Book(id)
		if !ok {
			return ErrNotFound
		}
		if err := checkISBN(snap, in.ISBN, id); err != nil {
			return err
		}
		slug := old.Slug
		if in.Title != old.Title {
			slug = uniqueSlug(snap, in.Title, id)
		}
		_, err := tx.ExecContext(ctx, `UPDATE books SET title=?, author=?, translator=?, publisher=?, published=?, pages=?, binding=?, isbn=?, slug=?, hidden=?, updated_at=? WHERE id=?`,
			in.Title, in.Author, in.Translator, in.Publisher, nullable(in.Published), nullable(in.Pages), in.Binding,
			nullISBN(in.ISBN), slug, in.Hidden, stamp(s.now()), id)
		return err
	})
	if err != nil {
		return Book{}, err
	}
	b, _ := s.Snapshot().Book(id)
	return b, nil
}

// SetHidden hides or shows books from visitors.
func (s *Store) SetHidden(ctx context.Context, ids []string, hidden bool) error {
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		for _, id := range ids {
			if _, ok := snap.Book(id); !ok {
				return ErrNotFound
			}
			if _, err := tx.ExecContext(ctx, `UPDATE books SET hidden=?, updated_at=? WHERE id=?`, hidden, stamp(s.now()), id); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteBooks removes books, with their readings and notes. It returns the
// cover files that are no longer used, for the caller to remove.
func (s *Store) DeleteBooks(ctx context.Context, ids []string) ([]string, error) {
	var covers []string
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		for _, id := range ids {
			b, ok := snap.Book(id)
			if !ok {
				return ErrNotFound
			}
			if b.Cover != "" {
				covers = append(covers, b.Cover)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM books WHERE id=?`, id); err != nil {
				return err
			}
		}
		return nil
	})
	return covers, err
}

// SetCover records a book's cover file and the spine colors worked out from
// it. It returns the cover file it replaced, if any, for the caller to remove.
func (s *Store) SetCover(ctx context.Context, id, cover, spine, spineText string) (string, error) {
	var old string
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		b, ok := snap.Book(id)
		if !ok {
			return ErrNotFound
		}
		old = b.Cover
		_, err := tx.ExecContext(ctx, `UPDATE books SET cover=?, spine_color=?, spine_text_color=?, spine_color_override='', updated_at=? WHERE id=?`,
			cover, spine, spineText, stamp(s.now()), id)
		return err
	})
	if old == cover {
		old = ""
	}
	return old, err
}

// SetSpineOverride sets the owner's own spine color, or clears it with "".
func (s *Store) SetSpineOverride(ctx context.Context, id, color string) error {
	color = strings.ToLower(strings.TrimSpace(color))
	if color != "" && !hexColor.MatchString(color) {
		return invalid("The spine color must look like #a1b2c3.")
	}
	return s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		if _, ok := snap.Book(id); !ok {
			return ErrNotFound
		}
		_, err := tx.ExecContext(ctx, `UPDATE books SET spine_color_override=?, updated_at=? WHERE id=?`, color, stamp(s.now()), id)
		return err
	})
}

// ---------------------------------------------------------------- settings

// UpdateSettings applies edit to the settings, validates them and saves them.
func (s *Store) UpdateSettings(ctx context.Context, edit func(*Settings)) (Settings, error) {
	err := s.change(ctx, func(tx *sql.Tx, snap *Snapshot) error {
		next := snap.Settings
		next.Sources = append([]string(nil), snap.Settings.Sources...)
		edit(&next)
		if err := next.Validate(); err != nil {
			return invalid("%s", err.Error())
		}
		raw, err := json.Marshal(next)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('settings', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw))
		return err
	})
	return s.Snapshot().Settings, err
}

// ---------------------------------------------------------------- sessions

// AddSession records a signed-in device.
func (s *Store) AddSession(ctx context.Context, ss Session) error {
	return s.change(ctx, func(tx *sql.Tx, _ *Snapshot) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO sessions (token_hash, created_at, expires_at, last_seen, user_agent) VALUES (?, ?, ?, ?, ?)`,
			ss.TokenHash, stamp(ss.CreatedAt), stamp(ss.ExpiresAt), stamp(ss.LastSeen), ss.UserAgent)
		return err
	})
}

// RemoveSession signs a device out.
func (s *Store) RemoveSession(ctx context.Context, tokenHash string) error {
	return s.change(ctx, func(tx *sql.Tx, _ *Snapshot) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
		return err
	})
}

// SessionAge is how long a session lasts after it was last used.
const SessionAge = 7 * 24 * time.Hour

// TouchSession records that a device was seen and extends its session, at
// most once an hour so that requests don't write to the disk.
func (s *Store) TouchSession(ctx context.Context, tokenHash string) error {
	ss, ok := s.Snapshot().Sessions[tokenHash]
	if !ok || s.now().Sub(ss.LastSeen) < time.Hour {
		return nil
	}
	now := s.now()
	return s.change(ctx, func(tx *sql.Tx, _ *Snapshot) error {
		_, err := tx.ExecContext(ctx, `UPDATE sessions SET last_seen=?, expires_at=? WHERE token_hash=?`, stamp(now), stamp(now.Add(SessionAge)), tokenHash)
		return err
	})
}

// PruneSessions removes expired sessions.
func (s *Store) PruneSessions(ctx context.Context) error {
	now := s.now()
	stale := false
	for _, ss := range s.Snapshot().Sessions {
		if now.After(ss.ExpiresAt) {
			stale = true
		}
	}
	if !stale {
		return nil
	}
	return s.change(ctx, func(tx *sql.Tx, _ *Snapshot) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, stamp(now))
		return err
	})
}
