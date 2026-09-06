package chain

import (
	"context"
	"errors"
	"time"

	"github.com/nazboyko/kindness-chain/internal/solana"
	"github.com/nazboyko/kindness-chain/internal/store"
)

// pausedAfter is how many failed sends in a row flag the chain as paused.
const pausedAfter = 3

// Run is the worker. One goroutine takes links in order and writes each
// one behind the last confirmed link, so a memo always points at a
// signature that is already on the chain. It returns when ctx ends; a
// send in flight at that moment still completes and is recorded.
func (s *Service) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-s.queue:
			s.process(ctx, n)
		}
	}
}

func (s *Service) process(ctx context.Context, n int64) {
	// a shutdown must not cut a memo off between reaching the cluster
	// and being recorded, so the work itself ignores cancellation and
	// only the waits between attempts honour it
	work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()

	link, err := s.store.Get(work, n)
	if err != nil {
		s.log.Printf("link %d: %v", n, err)
		return
	}
	if link.Status != store.StatusPending {
		return
	}
	head, _, err := s.store.LastConfirmed(work)
	if err != nil {
		s.log.Printf("link %d: %v", n, err)
		return
	}
	memo, err := EncodeMemo(link, head.Signature)
	if err != nil {
		s.fail(work, link, err)
		return
	}
	prev := head.Signature
	if prev == "" {
		prev = GenesisPrev
	}

	delay := s.retryDelay
	for attempt := 1; ; attempt++ {
		signature, err := s.ledger.SendMemo(work, memo)
		if err == nil {
			s.confirm(work, link, signature, prev, memo)
			return
		}
		if permanent(err) {
			s.fail(work, link, err)
			return
		}
		s.log.Printf("link %d: send attempt %d failed: %v", n, attempt, err)
		if attempt >= pausedAfter && !s.paused.Swap(true) {
			s.log.Printf("chain paused: sends keep failing, links wait and nothing is lost")
			s.publishStats(work)
		}
		if !wait(ctx, delay) {
			return // shutting down; the row stays pending for the next start
		}
		delay = min(delay*2, s.maxRetryDelay)
	}
}

func (s *Service) confirm(ctx context.Context, link Link, signature, prev string, memo []byte) {
	at := now()
	if err := s.store.MarkConfirmed(ctx, link.N, signature, prev, string(memo), at); err != nil {
		s.log.Printf("link %d: confirmed as %s but not recorded: %v", link.N, signature, err)
		return
	}
	link.Status = store.StatusConfirmed
	link.Signature = signature
	link.PrevSignature = prev
	link.Memo = string(memo)
	link.ConfirmedAt = at
	s.log.Printf("link %d confirmed as %s", link.N, signature)
	if s.paused.Swap(false) {
		s.log.Printf("chain resumed")
	}
	s.events.LinkChanged(link)
	s.publishStats(ctx)
}

func (s *Service) fail(ctx context.Context, link Link, cause error) {
	if err := s.store.MarkFailed(ctx, link.N, cause.Error()); err != nil {
		s.log.Printf("link %d: failed but not recorded: %v", link.N, err)
		return
	}
	link.Status = store.StatusFailed
	link.Error = cause.Error()
	s.log.Printf("link %d failed for good: %v", link.N, cause)
	s.events.LinkChanged(link)
	s.publishStats(ctx)
}

func (s *Service) publishStats(ctx context.Context) {
	stats, err := s.Stats(ctx)
	if err != nil {
		s.log.Printf("stats: %v", err)
		return
	}
	s.events.StatsChanged(stats)
}

// permanent tells a memo the cluster will never take from a cluster
// that is not answering right now.
func permanent(err error) bool {
	return errors.Is(err, solana.ErrMemoTooLarge) ||
		errors.Is(err, solana.ErrMemoNotUTF8) ||
		errors.Is(err, solana.ErrRejected)
}

func wait(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
