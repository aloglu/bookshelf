// Package store keeps a Bookshelf library in one SQLite file.
//
// It is built so that it can never deadlock a FUSE-backed folder such as an
// Unraid user share (see "Storage safety on Unraid" in docs/v2/PLAN.md):
//
//   - The database is opened once, on one connection, in EXCLUSIVE locking
//     mode, and a write at open takes the exclusive lock, which is then held
//     for as long as the store is open. No request ever waits on a file lock;
//     a second Bookshelf on the same folder is refused at once.
//   - Exclusive mode is set before the first WAL access, so SQLite keeps the
//     WAL index in memory and never creates a shared-memory (-shm) file.
//   - The whole library is held in memory as an immutable Snapshot. Reads use
//     the snapshot and touch no files; changes take turns, run in one
//     transaction, and replace the snapshot when they commit.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// DatabaseName is the library's file inside the data folder.
const DatabaseName = "bookshelf.db"

// ErrInUse is returned by Open when another Bookshelf holds the library.
var ErrInUse = errors.New("another Bookshelf is already using this data folder")

// Store is an open library.
type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex // changes take turns
	snap atomic.Pointer[Snapshot]
	now  func() time.Time
}

// Open opens (creating if needed) the library in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data folder: %w", err)
	}
	path := filepath.Join(dir, DatabaseName)
	// The order of these pragmas matters: locking mode before the first
	// access in WAL mode, and no waiting on a lock held by someone else.
	dsn := "file:" + path +
		"?_pragma=busy_timeout(0)" +
		"&_pragma=locking_mode(EXCLUSIVE)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	s := &Store{db: db, path: path, now: time.Now}
	if err := s.migrate(); err != nil {
		db.Close()
		if isBusy(err) {
			return nil, fmt.Errorf("%w (%s)", ErrInUse, dir)
		}
		return nil, err
	}
	if err := s.reload(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the library and its lock.
func (s *Store) Close() error { return s.db.Close() }

// Path is the database file.
func (s *Store) Path() string { return s.path }

// Snapshot is the library as of the last change. It is never modified.
func (s *Store) Snapshot() *Snapshot { return s.snap.Load() }

func isBusy(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "(5)")
}

// migrate creates the schema. It always writes (user_version), so the
// exclusive lock is taken here, at open, and held from then on.
func (s *Store) migrate() error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("this library was written by a newer Bookshelf (schema %d); update Bookshelf", version)
	}
	if version < 1 {
		if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}
	if version < 2 {
		if _, err := tx.ExecContext(ctx, schemaV2); err != nil {
			return fmt.Errorf("add notes and highlights: %w", err)
		}
	}
	if version < 3 {
		if _, err := tx.ExecContext(ctx, schemaV3); err != nil {
			return fmt.Errorf("change entries to passages and places: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

const schemaVersion = 3

const schemaV1 = `
CREATE TABLE books (
	id                   TEXT PRIMARY KEY,
	title                TEXT NOT NULL,
	author               TEXT NOT NULL DEFAULT '',
	translator           TEXT NOT NULL DEFAULT '',
	publisher            TEXT NOT NULL DEFAULT '',
	published            INTEGER,
	pages                INTEGER,
	binding              TEXT NOT NULL DEFAULT 'paperback' CHECK (binding IN ('paperback','hardcover','cloth','leather')),
	isbn                 TEXT,
	issn                 TEXT NOT NULL DEFAULT '',
	issue                TEXT NOT NULL DEFAULT '',
	slug                 TEXT NOT NULL UNIQUE,
	hidden               INTEGER NOT NULL DEFAULT 0,
	cover                TEXT NOT NULL DEFAULT '',
	spine_color          TEXT NOT NULL DEFAULT '',
	spine_text_color     TEXT NOT NULL DEFAULT '',
	spine_color_override TEXT NOT NULL DEFAULT '',
	added_at             TEXT NOT NULL,
	updated_at           TEXT NOT NULL
);
CREATE UNIQUE INDEX books_isbn ON books(isbn) WHERE isbn IS NOT NULL AND isbn <> '';
CREATE UNIQUE INDEX books_issue ON books(issn, issue) WHERE issn <> '';

CREATE TABLE readings (
	id                TEXT PRIMARY KEY,
	book_id           TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	started_at        TEXT NOT NULL DEFAULT '',
	started_precision TEXT NOT NULL CHECK (started_precision IN ('instant','day','year','unknown')),
	ended_at          TEXT NOT NULL DEFAULT '',
	ended_precision   TEXT NOT NULL CHECK (ended_precision IN ('instant','day','year','unknown')),
	outcome           TEXT NOT NULL CHECK (outcome IN ('reading','finished','set_aside'))
);
CREATE INDEX readings_book ON readings(book_id);
CREATE UNIQUE INDEX readings_one_in_progress ON readings(book_id) WHERE outcome = 'reading';

CREATE TABLE reading_progress (
	reading_id TEXT NOT NULL REFERENCES readings(id) ON DELETE CASCADE,
	page       INTEGER NOT NULL,
	at         TEXT NOT NULL
);
CREATE INDEX reading_progress_reading ON reading_progress(reading_id, at);

CREATE TABLE notes (
	reading_id TEXT PRIMARY KEY REFERENCES readings(id) ON DELETE CASCADE,
	text       TEXT NOT NULL,
	public     INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	last_seen  TEXT NOT NULL,
	user_agent TEXT NOT NULL DEFAULT ''
);
`

// schemaV2 (2.1) gives notes and highlights a table of their own, any number
// to a book, in place of one note on each finished reading. Those notes move
// across as notes tied to their readings.
const schemaV2 = `
CREATE TABLE entries (
	id         TEXT PRIMARY KEY,
	book_id    TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	reading_id TEXT REFERENCES readings(id) ON DELETE SET NULL,
	kind       TEXT NOT NULL CHECK (kind IN ('note','highlight')),
	text       TEXT NOT NULL DEFAULT '',
	quote      TEXT NOT NULL DEFAULT '',
	page       INTEGER,
	public     INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX entries_book ON entries(book_id, created_at);
CREATE INDEX entries_reading ON entries(reading_id);

INSERT INTO entries (id, book_id, reading_id, kind, text, public, created_at, updated_at)
	SELECT lower(hex(randomblob(8))), r.book_id, n.reading_id, 'note', n.text, n.public, n.created_at, n.updated_at
	FROM notes n JOIN readings r ON r.id = n.reading_id;
DROP TABLE notes;
`

// schemaV3 (2.1) makes an entry one kind of thing: words before a passage,
// the passage, words after it, and a place (a page, a chapter or a location).
// A note keeps its words; a highlight keeps its passage and its thought; a
// page becomes a place of kind page. A book can also keep its notes from
// visitors.
const schemaV3 = `
CREATE TABLE entries_new (
	id         TEXT PRIMARY KEY,
	book_id    TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	reading_id TEXT REFERENCES readings(id) ON DELETE SET NULL,
	pre        TEXT NOT NULL DEFAULT '',
	quote      TEXT NOT NULL DEFAULT '',
	text       TEXT NOT NULL DEFAULT '',
	where_kind TEXT NOT NULL DEFAULT '' CHECK (where_kind IN ('','page','chapter','loc')),
	where_n    INTEGER,
	where_to   INTEGER,
	public     INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
INSERT INTO entries_new (id, book_id, reading_id, pre, quote, text, where_kind, where_n, public, created_at, updated_at)
	SELECT id, book_id, reading_id, '', quote, text, CASE WHEN page IS NULL THEN '' ELSE 'page' END, page, public, created_at, updated_at FROM entries;
DROP TABLE entries;
ALTER TABLE entries_new RENAME TO entries;
CREATE INDEX entries_book ON entries(book_id, created_at);
CREATE INDEX entries_reading ON entries(reading_id);
ALTER TABLE books ADD COLUMN notes_hidden INTEGER NOT NULL DEFAULT 0;
`
