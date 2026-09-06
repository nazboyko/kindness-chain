package store

import (
	"context"
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

	wrote, err := s.InsertGenesis(ctx, "the pledge", "Nazar", day)
	if err != nil || !wrote {
		t.Fatalf("first genesis: wrote=%v err=%v", wrote, err)
	}
	wrote, err = s.InsertGenesis(ctx, "the pledge again", "Nazar", day)
	if err != nil || wrote {
		t.Fatalf("second genesis: wrote=%v err=%v", wrote, err)
	}

	n, err := s.Insert(ctx, "I carried groceries for a neighbour", "Olena", day.Add(time.Minute))
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
		if _, err := s.Insert(ctx, "act", "", day.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	if _, ok, err := s.LastConfirmed(ctx); err != nil || ok {
		t.Fatalf("head before any confirmation: ok=%v err=%v", ok, err)
	}

	if err := s.MarkConfirmed(ctx, 1, "sig1", "genesis", `{"n":1}`, day); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkConfirmed(ctx, 1, "sig1", "genesis", `{"n":1}`, day); !errors.Is(err, ErrNotPending) {
		t.Errorf("confirming twice = %v, want ErrNotPending", err)
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
	if err != nil || counts != (Counts{Confirmed: 1, Pending: 1, Failed: 1}) {
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
	if _, err := s.InsertGenesis(ctx, "pledge", "Nazar", day); err != nil {
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
		if _, err := s.Insert(ctx, "act", "", day.Add(time.Duration(i)*time.Second)); err != nil {
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
