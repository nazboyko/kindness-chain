package solana

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Fake is an in-memory chain for tests and local runs. Signatures are
// deterministic, memos are kept in a map, and errors can be scripted
// for the next sends to exercise retry paths.
type Fake struct {
	// Delay, when set, is how long a send takes to "confirm". Local runs
	// use it so the page shows the pending state for a moment.
	Delay time.Duration

	mu    sync.Mutex
	memos map[string][]byte
	sent  []string
	fails []error
}

// NewFake returns an empty fake chain.
func NewFake() *Fake {
	return &Fake{memos: map[string][]byte{}}
}

// FailNext queues errors that the following SendMemo calls return, in
// order, before sends succeed again.
func (f *Fake) FailNext(errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fails = append(f.fails, errs...)
}

// Sent lists the signatures of every accepted memo, oldest first.
func (f *Fake) Sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

// SendMemo records the memo under the next signature.
func (f *Fake) SendMemo(ctx context.Context, memo []byte) (string, error) {
	if err := Validate(memo); err != nil {
		return "", err
	}
	if f.Delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(f.Delay):
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.fails) > 0 {
		err := f.fails[0]
		f.fails = f.fails[1:]
		return "", err
	}
	sig := fmt.Sprintf("fake-%06d", len(f.sent)+1)
	f.memos[sig] = append([]byte(nil), memo...)
	f.sent = append(f.sent, sig)
	return sig, nil
}

// GetMemo returns what was recorded under the signature.
func (f *Fake) GetMemo(_ context.Context, signature string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	memo, ok := f.memos[signature]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), memo...), nil
}
