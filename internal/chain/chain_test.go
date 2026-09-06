package chain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/nazboyko/kindness-chain/internal/solana"
	"github.com/nazboyko/kindness-chain/internal/store"
)

var testConfig = Config{
	PerLinkCents: 10,
	CapCents:     5000,
	CharityName:  "International Institute of Minnesota",
	CharityURL:   "https://iimn.org",
	PledgerName:  "Nazar",
}

// recorder is a Broadcaster the tests can wait on.
type recorder struct {
	links chan Link
	stats chan Stats
}

func newRecorder() *recorder {
	return &recorder{links: make(chan Link, 100), stats: make(chan Stats, 100)}
}

func (r *recorder) LinkChanged(l Link)   { r.links <- l }
func (r *recorder) StatsChanged(s Stats) { r.stats <- s }

func (r *recorder) nextLink(t *testing.T) Link {
	t.Helper()
	select {
	case l := <-r.links:
		return l
	case <-time.After(5 * time.Second):
		t.Fatal("no link event within 5s")
		return Link{}
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func newService(t *testing.T, st *store.Store, ledger solana.Client) (*Service, *recorder) {
	t.Helper()
	rec := newRecorder()
	svc := New(testConfig, st, ledger, rec)
	svc.retryDelay = time.Millisecond
	svc.maxRetryDelay = 5 * time.Millisecond
	svc.log = log.New(io.Discard, "", 0)
	return svc, rec
}

// start runs Start and the worker until the test ends.
func start(t *testing.T, svc *Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func TestLinksAreConfirmedInOrderAndChained(t *testing.T) {
	ctx := context.Background()
	fake := solana.NewFake()
	svc, rec := newService(t, openStore(t), fake)
	start(t, svc)

	genesis := rec.nextLink(t)
	if genesis.N != 0 || genesis.Status != store.StatusConfirmed || genesis.PrevSignature != GenesisPrev {
		t.Fatalf("genesis = %+v", genesis)
	}
	if genesis.Act != GenesisText(testConfig) || genesis.By != "Nazar" {
		t.Errorf("genesis text = %q by %q", genesis.Act, genesis.By)
	}

	acts := []string{"I carried groceries for a neighbour", "I called my grandmother", "I let someone merge in traffic"}
	for _, act := range acts {
		if _, err := svc.Add(ctx, act, "Olena"); err != nil {
			t.Fatal(err)
		}
	}

	prev := genesis.Signature
	for i, act := range acts {
		link := rec.nextLink(t)
		if link.N != int64(i+1) || link.Status != store.StatusConfirmed {
			t.Fatalf("event %d = %+v", i, link)
		}
		if link.PrevSignature != prev {
			t.Errorf("link %d prev = %s, want %s", link.N, link.PrevSignature, prev)
		}
		onChain, err := fake.GetMemo(ctx, link.Signature)
		if err != nil {
			t.Fatal(err)
		}
		memo, err := DecodeMemo(onChain)
		if err != nil {
			t.Fatal(err)
		}
		if memo.N != link.N || memo.Act != act || memo.By != "Olena" || memo.Prev != prev {
			t.Errorf("memo on chain = %+v", memo)
		}
		if string(onChain) != link.Memo {
			t.Errorf("stored memo differs from the chain")
		}
		prev = link.Signature
	}
	if sent := fake.Sent(); len(sent) != 4 {
		t.Errorf("sent %d memos, want 4", len(sent))
	}

	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Confirmed != 3 || stats.PledgedCents != 30 || stats.HeadN != 3 || stats.HeadSignature != prev || stats.Paused {
		t.Errorf("stats = %+v", stats)
	}
}

func TestRestartPicksUpPendingLinks(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	if _, err := st.InsertGenesis(ctx, "pledge text for the test", "Nazar", "pledge text for the test", now()); err != nil {
		t.Fatal(err)
	}
	for _, act := range []string{"first pending sentence", "second pending sentence"} {
		if _, err := st.Insert(ctx, act, "", Fingerprint(act), now()); err != nil {
			t.Fatal(err)
		}
	}

	svc, rec := newService(t, st, solana.NewFake())
	start(t, svc)
	for want := int64(0); want <= 2; want++ {
		link := rec.nextLink(t)
		if link.N != want || link.Status != store.StatusConfirmed {
			t.Fatalf("after restart got %+v, want link %d confirmed", link, want)
		}
	}
	if pending, _ := st.PendingInOrder(ctx); len(pending) != 0 {
		t.Errorf("still pending: %v", pending)
	}
}

func TestTransientFailuresKeepTheLinkAndPauseTheChain(t *testing.T) {
	ctx := context.Background()
	fake := solana.NewFake()
	svc, rec := newService(t, openStore(t), fake)
	start(t, svc)
	rec.nextLink(t) // genesis

	network := errors.New("dial tcp: connection refused")
	fake.FailNext(network, network, network, network)
	if _, err := svc.Add(ctx, "I carried groceries for a neighbour", ""); err != nil {
		t.Fatal(err)
	}

	link := rec.nextLink(t)
	if link.N != 1 || link.Status != store.StatusConfirmed {
		t.Fatalf("link after transient failures = %+v", link)
	}
	sawPaused := false
	for {
		select {
		case s := <-rec.stats:
			if s.Paused {
				sawPaused = true
			}
			continue
		default:
		}
		break
	}
	if !sawPaused {
		t.Error("the chain was never reported as paused")
	}
	stats, _ := svc.Stats(ctx)
	if stats.Paused || stats.Confirmed != 1 {
		t.Errorf("stats after recovery = %+v", stats)
	}
}

func TestRejectedLinkFailsAndTheChainMovesOn(t *testing.T) {
	ctx := context.Background()
	fake := solana.NewFake()
	svc, rec := newService(t, openStore(t), fake)
	start(t, svc)
	genesis := rec.nextLink(t)

	fake.FailNext(fmt.Errorf("%w: custom program error", solana.ErrRejected))
	if _, err := svc.Add(ctx, "I carried groceries for a neighbour", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, "I called my grandmother today", ""); err != nil {
		t.Fatal(err)
	}

	failed := rec.nextLink(t)
	if failed.N != 1 || failed.Status != store.StatusFailed || failed.Error == "" {
		t.Fatalf("rejected link = %+v", failed)
	}
	next := rec.nextLink(t)
	if next.N != 2 || next.Status != store.StatusConfirmed || next.PrevSignature != genesis.Signature {
		t.Fatalf("link after a failure = %+v, want it chained to genesis %s", next, genesis.Signature)
	}
	stats, _ := svc.Stats(ctx)
	if stats.Confirmed != 1 || stats.PledgedCents != 10 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestAddRefusesBadInput(t *testing.T) {
	svc, _ := newService(t, openStore(t), solana.NewFake())
	_, err := svc.Add(context.Background(), "too short", "")
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Message != MsgTooShort {
		t.Errorf("error = %v, want %q", err, MsgTooShort)
	}
}

func TestDuplicateSentencesAreRefusedForADay(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	svc, _ := newService(t, st, solana.NewFake())

	if _, err := svc.Add(ctx, "I called my grandmother today!", "Olena"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Add(ctx, "  i called MY grandmother, today ", "")
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Message != MsgDuplicate {
		t.Fatalf("second copy: %v, want %q", err, MsgDuplicate)
	}
	if _, err := svc.Add(ctx, "I called my grandmother yesterday.", ""); err != nil {
		t.Errorf("a different sentence was refused: %v", err)
	}

	// the same sentence from two days ago no longer counts
	old := "I watered the plants for a travelling friend."
	if _, err := st.Insert(ctx, old, "", Fingerprint(old), now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, old, ""); err != nil {
		t.Errorf("an old sentence was refused: %v", err)
	}
}

func TestStartFingerprintsOlderLinks(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	if _, err := st.InsertGenesis(ctx, "the pledge", "Nazar", "", now()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Insert(ctx, "An older sentence, kept.", "", "", now()); err != nil {
		t.Fatal(err)
	}
	svc, _ := newService(t, st, solana.NewFake())
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if missing, _ := st.WithoutFingerprint(ctx); len(missing) != 0 {
		t.Errorf("still without fingerprint: %v", missing)
	}
	if dup, _ := svc.Duplicate(ctx, "an older sentence kept"); !dup {
		t.Error("the backfilled fingerprint does not match")
	}
}
