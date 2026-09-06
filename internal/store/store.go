// Package store keeps the links in SQLite. It is the source of truth
// for what visitors wrote and the index into what the chain holds.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure Go driver, registered as "sqlite"
)

// Link statuses. A link starts pending, and the worker moves it to
// confirmed once the cluster has the memo, or to failed if the cluster
// rejected it for good.
const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusFailed    = "failed"
)

var (
	// ErrNotFound is returned for a link number that does not exist.
	ErrNotFound = errors.New("link not found")
	// errNotPending is returned when a link is marked twice.
	errNotPending = errors.New("link is not pending")
)

// Link is one row: a sentence, who wrote it, and what the chain says
// about it.
type Link struct {
	N             int64
	Act           string
	By            string
	CreatedAt     time.Time
	Status        string
	Signature     string
	PrevSignature string
	Memo          string
	ConfirmedAt   time.Time
	Error         string
}

// Counts is how many links are on the chain and how many are waiting.
// Confirmed leaves out link #0, which is the pledge itself and does not
// count toward it.
type Counts struct {
	Confirmed int64
	Pending   int64
}

// Store is an open database.
type Store struct {
	db *sql.DB
}

const createTable = `
CREATE TABLE IF NOT EXISTS links (
	n              INTEGER PRIMARY KEY,
	act            TEXT NOT NULL,
	author         TEXT NOT NULL DEFAULT '',
	created_at     TEXT NOT NULL,
	status         TEXT NOT NULL CHECK (status IN ('pending', 'confirmed', 'failed')),
	signature      TEXT,
	prev_signature TEXT,
	memo           TEXT,
	confirmed_at   TEXT,
	error          TEXT,
	fingerprint    TEXT NOT NULL DEFAULT ''
);
`

const createIndexes = `
CREATE INDEX IF NOT EXISTS links_by_status ON links (status, n);
CREATE INDEX IF NOT EXISTS links_by_fingerprint ON links (fingerprint, created_at);
`

// Open opens or creates the database at path and applies the schema.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrate creates the table for a new database and brings an older one
// up to date. The fingerprint column arrived after the first deploy, so
// a database from before it gets the column added in place.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(createTable); err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	has, err := hasColumn(db, "links", "fingerprint")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE links ADD COLUMN fingerprint TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add fingerprint column: %w", err)
		}
	}
	if _, err := db.Exec(createIndexes); err != nil {
		return fmt.Errorf("create indexes: %w", err)
	}
	return nil
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close releases the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
