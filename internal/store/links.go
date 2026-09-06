package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

const linkColumns = `n, act, author, created_at, status, signature, prev_signature, memo, confirmed_at, error`

// Insert adds a pending link and returns its number. The fingerprint
// is what duplicate checks compare.
func (s *Store) Insert(ctx context.Context, act, by, fingerprint string, createdAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO links (act, author, fingerprint, created_at, status) VALUES (?, ?, ?, ?, ?)`,
		act, by, fingerprint, formatTime(createdAt), StatusPending,
	)
	if err != nil {
		return 0, fmt.Errorf("insert link: %w", err)
	}
	n, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert link: %w", err)
	}
	return n, nil
}

// InsertGenesis writes link #0 if the table is empty, and reports
// whether it did.
func (s *Store) InsertGenesis(ctx context.Context, act, by, fingerprint string, createdAt time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO links (n, act, author, fingerprint, created_at, status)
		 SELECT 0, ?, ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM links)`,
		act, by, fingerprint, formatTime(createdAt), StatusPending,
	)
	if err != nil {
		return false, fmt.Errorf("insert genesis: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("insert genesis: %w", err)
	}
	return rows == 1, nil
}

// Get returns one link.
func (s *Store) Get(ctx context.Context, n int64) (Link, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+linkColumns+` FROM links WHERE n = ?`, n)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	if err != nil {
		return Link{}, fmt.Errorf("get link %d: %w", n, err)
	}
	return link, nil
}

// ListDesc returns up to limit links numbered below before, newest
// first. A before of zero or less means start from the newest.
func (s *Store) ListDesc(ctx context.Context, before int64, limit int) ([]Link, error) {
	if before <= 0 {
		before = math.MaxInt64
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+linkColumns+` FROM links WHERE n < ? ORDER BY n DESC LIMIT ?`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()
	links := []Link{}
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("list links: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// MarkConfirmed records where a pending link landed on the chain.
func (s *Store) MarkConfirmed(ctx context.Context, n int64, signature, prev, memo string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE links SET status = ?, signature = ?, prev_signature = ?, memo = ?, confirmed_at = ?, error = NULL
		 WHERE n = ? AND status = ?`,
		StatusConfirmed, signature, prev, memo, formatTime(at), n, StatusPending,
	)
	return outcome(res, err, "confirm link")
}

// MarkFailed records that the chain rejected a pending link for good.
func (s *Store) MarkFailed(ctx context.Context, n int64, reason string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE links SET status = ?, error = ? WHERE n = ? AND status = ?`,
		StatusFailed, reason, n, StatusPending,
	)
	return outcome(res, err, "fail link")
}

func outcome(res sql.Result, err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if rows == 0 {
		return errNotPending
	}
	return nil
}

// LastConfirmed is the head of the chain: the highest numbered link
// the cluster has. The bool is false while nothing is confirmed yet.
func (s *Store) LastConfirmed(ctx context.Context) (Link, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+linkColumns+` FROM links WHERE status = ? ORDER BY n DESC LIMIT 1`, StatusConfirmed)
	link, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, false, nil
	}
	if err != nil {
		return Link{}, false, fmt.Errorf("last confirmed: %w", err)
	}
	return link, true, nil
}

// PendingInOrder lists the numbers of every pending link, oldest first.
func (s *Store) PendingInOrder(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT n FROM links WHERE status = ? ORDER BY n`, StatusPending)
	if err != nil {
		return nil, fmt.Errorf("pending links: %w", err)
	}
	defer rows.Close()
	ns := []int64{}
	for rows.Next() {
		var n int64
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("pending links: %w", err)
		}
		ns = append(ns, n)
	}
	return ns, rows.Err()
}

// SeenSince reports whether a link with this fingerprint was added at
// or after since and is on the chain or on its way there.
func (s *Store) SeenSince(ctx context.Context, fingerprint string, since time.Time) (bool, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT n FROM links WHERE fingerprint = ? AND created_at >= ? AND status != ? LIMIT 1`,
		fingerprint, formatTime(since), StatusFailed,
	).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("duplicate check: %w", err)
	}
	return true, nil
}

// WithoutFingerprint lists links from before fingerprints existed.
func (s *Store) WithoutFingerprint(ctx context.Context) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+linkColumns+` FROM links WHERE fingerprint = '' ORDER BY n`)
	if err != nil {
		return nil, fmt.Errorf("links without fingerprint: %w", err)
	}
	defer rows.Close()
	links := []Link{}
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("links without fingerprint: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// SetFingerprint fills in the fingerprint of an older link.
func (s *Store) SetFingerprint(ctx context.Context, n int64, fingerprint string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE links SET fingerprint = ? WHERE n = ?`, fingerprint, n); err != nil {
		return fmt.Errorf("set fingerprint: %w", err)
	}
	return nil
}

// Counts tallies the links that count and the links that wait.
func (s *Store) Counts(ctx context.Context) (Counts, error) {
	var c Counts
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(CASE WHEN status = ? AND n > 0 THEN 1 END),
			COUNT(CASE WHEN status = ? THEN 1 END)
		FROM links`, StatusConfirmed, StatusPending).Scan(&c.Confirmed, &c.Pending)
	if err != nil {
		return Counts{}, fmt.Errorf("count links: %w", err)
	}
	return c, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanLink(row scanner) (Link, error) {
	var (
		link                                  Link
		createdAt                             string
		signature, prev, memo, confirmedAt, e sql.NullString
	)
	err := row.Scan(&link.N, &link.Act, &link.By, &createdAt, &link.Status, &signature, &prev, &memo, &confirmedAt, &e)
	if err != nil {
		return Link{}, err
	}
	link.CreatedAt = parseTime(createdAt)
	link.Signature = signature.String
	link.PrevSignature = prev.String
	link.Memo = memo.String
	link.ConfirmedAt = parseTime(confirmedAt.String)
	link.Error = e.String
	return link, nil
}
