package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var day = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func TestGenesisAndInsert(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	wrote, err := s.InsertGenesis(ctx, "the pledge", "Nazar", "the pledge", day)
	if err != nil || !wrote {
		t.Fatalf("first genesis: wrote=%v err=%v", wrote, err)
	}
	wrote, err = s.InsertGenesis(ctx, "the pledge again", "Nazar", "the pledge again", day)
	if err != nil || wrote {
		t.Fatalf("second genesis: wrote=%v err=%v", wrote, err)
	}

	n, err := s.Insert(ctx, "I carried groceries for a neighbour", "Olena", "i carried groceries for a neighbour", day.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("first visitor link got number %d, want 1", n)
	}

	genesis, err := s.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if genesis.Act != "the pledge" || genesis.By != "Nazar" || genesis.Status != StatusPending {
		t.Errorf("genesis row = %+v", genesis)
	}
	if !genesis.CreatedAt.Equal(day) {
		t.Errorf("created_at round trip = %v, want %v", genesis.CreatedAt, day)
	}
	if _, err := s.Get(ctx, 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing link error = %v, want ErrNotFound", err)
	}
}

func TestStatusTransitions(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	for i := range 3 {
		if _, err := s.Insert(ctx, "act", "", "act", day.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	if _, ok, err := s.LastConfirmed(ctx); err != nil || ok {
		t.Fatalf("head before any confirmation: ok=%v err=%v", ok, err)
	}

	if err := s.MarkConfirmed(ctx, 1, "sig1", "genesis", `{"n":1}`, day); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkConfirmed(ctx, 1, "sig1", "genesis", `{"n":1}`, day); !errors.Is(err, errNotPending) {
		t.Errorf("confirming twice = %v, want errNotPending", err)
	}
	if err := s.MarkFailed(ctx, 2, "rejected"); err != nil {
		t.Fatal(err)
	}

	head, ok, err := s.LastConfirmed(ctx)
	if err != nil || !ok || head.N != 1 || head.Signature != "sig1" || head.PrevSignature != "genesis" {
		t.Errorf("head = %+v ok=%v err=%v", head, ok, err)
	}
	pending, err := s.PendingInOrder(ctx)
	if err != nil || len(pending) != 1 || pending[0] != 3 {
		t.Errorf("pending = %v err=%v", pending, err)
	}
	counts, err := s.Counts(ctx)
	if err != nil || counts != (Counts{Confirmed: 1, Pending: 1}) {
		t.Errorf("counts = %+v err=%v", counts, err)
	}
	failed, _ := s.Get(ctx, 2)
	if failed.Status != StatusFailed || failed.Error != "rejected" {
		t.Errorf("failed row = %+v", failed)
	}
}

func TestGenesisDoesNotCountTowardThePledge(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if _, err := s.InsertGenesis(ctx, "pledge", "Nazar", "pledge", day); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkConfirmed(ctx, 0, "sig0", "genesis", "{}", day); err != nil {
		t.Fatal(err)
	}
	counts, err := s.Counts(ctx)
	if err != nil || counts.Confirmed != 0 {
		t.Errorf("counts after confirming only genesis = %+v err=%v", counts, err)
	}
}

func TestListDesc(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	for i := range 5 {
		if _, err := s.Insert(ctx, "act", "", "act", day.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name   string
		before int64
		limit  int
		want   []int64
	}{
		{"newest page", 0, 2, []int64{5, 4}},
		{"older page", 4, 2, []int64{3, 2}},
		{"last page is short", 2, 10, []int64{1}},
		{"nothing older", 1, 10, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			links, err := s.ListDesc(ctx, tc.before, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]int64, len(links))
			for i, l := range links {
				got[i] = l.N
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestSeenSince(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if _, err := s.Insert(ctx, "I called my grandmother.", "", "i called my grandmother", day); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.SeenSince(ctx, "i called my grandmother", day.Add(-time.Hour)); !seen {
		t.Error("a fresh duplicate was not seen")
	}
	if seen, _ := s.SeenSince(ctx, "i called my grandmother", day.Add(time.Hour)); seen {
		t.Error("an old link counted as recent")
	}
	if seen, _ := s.SeenSince(ctx, "something else", day.Add(-time.Hour)); seen {
		t.Error("a different fingerprint matched")
	}
	if err := s.MarkFailed(ctx, 1, "rejected"); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.SeenSince(ctx, "i called my grandmother", day.Add(-time.Hour)); seen {
		t.Error("a failed link still blocks a retry")
	}
}

func TestMigrationAddsFingerprint(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// the table as the first deploy created it, without the column
	_, err = old.Exec(`CREATE TABLE links (
		n INTEGER PRIMARY KEY, act TEXT NOT NULL, author TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('pending', 'confirmed', 'failed')),
		signature TEXT, prev_signature TEXT, memo TEXT, confirmed_at TEXT, error TEXT);
		INSERT INTO links (n, act, created_at, status) VALUES (0, 'the pledge', '2026-09-06T00:00:00Z', 'confirmed')`)
	if err != nil {
		t.Fatal(err)
	}
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open an older database: %v", err)
	}
	defer s.Close()
	missing, err := s.WithoutFingerprint(ctx)
	if err != nil || len(missing) != 1 || missing[0].N != 0 {
		t.Fatalf("links without fingerprint = %v err=%v", missing, err)
	}
	if err := s.SetFingerprint(ctx, 0, "the pledge"); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.SeenSince(ctx, "the pledge", day.Add(-24*time.Hour)); !seen {
		t.Error("the backfilled fingerprint is not found")
	}
	if _, err := s.Insert(ctx, "new", "", "new", day); err != nil {
		t.Errorf("insert after migration: %v", err)
	}
}
