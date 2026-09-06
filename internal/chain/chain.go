// Package chain turns sentences into links. It validates what a visitor
// wrote, stores it, and writes it to the ledger one link after another,
// so that every memo can point at the one before it.
package chain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/nazboyko/kindness-chain/internal/solana"
	"github.com/nazboyko/kindness-chain/internal/store"
)

// Link is a stored link. The store owns the row, this package owns what
// happens to it.
type Link = store.Link

// Config is the pledge as the site presents it.
type Config struct {
	PerLinkCents  int
	CapCents      int
	CharityName   string
	CharityURL    string
	PledgerName   string
	SignerAddress string // the public key behind every memo, when known
}

// Stats is the state of the pledge and of the chain.
type Stats struct {
	Confirmed     int64 // visitor links on the chain; link #0 is not one
	Pending       int64
	PledgedCents  int
	CapCents      int
	PerLinkCents  int
	CharityName   string
	CharityURL    string
	PledgerName   string
	SignerAddress string
	HeadN         int64 // the newest confirmed link
	HeadSignature string
	Paused        bool // sends keep failing; links wait, nothing is lost
}

// Broadcaster is told about every change worth pushing to open pages.
type Broadcaster interface {
	LinkChanged(link Link)
	StatsChanged(stats Stats)
}

// nopBroadcaster tells nobody.
type nopBroadcaster struct{}

func (nopBroadcaster) LinkChanged(Link)   {}
func (nopBroadcaster) StatsChanged(Stats) {}

// ErrBusy is returned when too many links are already waiting.
var ErrBusy = errors.New("too many links are waiting for the chain, try again in a minute")

const (
	queueSize = 1000
	// duplicateWindow is how long the same sentence is refused after it
	// was added once.
	duplicateWindow = 24 * time.Hour
)

// Service is the one place links are created and confirmed.
type Service struct {
	cfg    Config
	store  *store.Store
	ledger solana.Client
	events Broadcaster
	queue  chan int64
	paused atomic.Bool
	log    *log.Logger

	retryDelay    time.Duration // the first wait after a failed send
	maxRetryDelay time.Duration
}

// New wires a service. Call Start before Run and before serving.
func New(cfg Config, st *store.Store, ledger solana.Client, events Broadcaster) *Service {
	if events == nil {
		events = nopBroadcaster{}
	}
	return &Service{
		cfg:           cfg,
		store:         st,
		ledger:        ledger,
		events:        events,
		queue:         make(chan int64, queueSize),
		log:           log.Default(),
		retryDelay:    2 * time.Second,
		maxRetryDelay: 30 * time.Second,
	}
}

// Start writes the pledge as link #0 when the store is empty and queues
// every link an earlier run left pending, so a restart loses nothing.
func (s *Service) Start(ctx context.Context) error {
	pledge := genesisText(s.cfg)
	wrote, err := s.store.InsertGenesis(ctx, pledge, s.cfg.PledgerName, fingerprint(pledge), now())
	if err != nil {
		return err
	}
	if wrote {
		s.log.Printf("wrote the pledge as link #0")
	}
	// links from before duplicate checks existed get their fingerprint now
	older, err := s.store.WithoutFingerprint(ctx)
	if err != nil {
		return err
	}
	for _, link := range older {
		if err := s.store.SetFingerprint(ctx, link.N, fingerprint(link.Act)); err != nil {
			return err
		}
	}
	if len(older) > 0 {
		s.log.Printf("fingerprinted %d older links", len(older))
	}
	pending, err := s.store.PendingInOrder(ctx)
	if err != nil {
		return err
	}
	for i, n := range pending {
		select {
		case s.queue <- n:
		default:
			s.log.Printf("%d pending links do not fit the queue and wait for the next start", len(pending)-i)
			return nil
		}
	}
	if len(pending) > 0 {
		s.log.Printf("queued %d pending links", len(pending))
	}
	return nil
}

// Add validates a sentence, stores it as pending and hands it to the
// worker. The returned link has its number; the chain fills in the rest.
func (s *Service) Add(ctx context.Context, act, by string) (Link, error) {
	act, by, err := Validate(act, by)
	if err != nil {
		return Link{}, err
	}
	createdAt := now()
	if err := fitsOnChain(act, by, createdAt); err != nil {
		return Link{}, err
	}
	duplicate, err := s.Duplicate(ctx, act)
	if err != nil {
		return Link{}, err
	}
	if duplicate {
		return Link{}, ErrDuplicate
	}
	if len(s.queue) >= cap(s.queue) {
		return Link{}, ErrBusy
	}
	n, err := s.store.Insert(ctx, act, by, fingerprint(act), createdAt)
	if err != nil {
		return Link{}, fmt.Errorf("store link: %w", err)
	}
	select {
	case s.queue <- n:
	default:
		// filled up since the check above; the row is safe and Start
		// queues it on the next run
		s.log.Printf("queue full, link %d waits for the next start", n)
	}
	return Link{N: n, Act: act, By: by, CreatedAt: createdAt, Status: store.StatusPending}, nil
}

// Duplicate reports whether the same sentence, give or take punctuation
// and case, was added within the last day.
func (s *Service) Duplicate(ctx context.Context, act string) (bool, error) {
	seen, err := s.store.SeenSince(ctx, fingerprint(act), now().Add(-duplicateWindow))
	if err != nil {
		return false, fmt.Errorf("duplicate check: %w", err)
	}
	return seen, nil
}

// Get returns one link.
func (s *Service) Get(ctx context.Context, n int64) (Link, error) {
	return s.store.Get(ctx, n)
}

// List returns up to limit links below before, newest first.
func (s *Service) List(ctx context.Context, before int64, limit int) ([]Link, error) {
	return s.store.ListDesc(ctx, before, limit)
}

// Stats reads the state of the pledge.
func (s *Service) Stats(ctx context.Context) (Stats, error) {
	counts, err := s.store.Counts(ctx)
	if err != nil {
		return Stats{}, err
	}
	head, ok, err := s.store.LastConfirmed(ctx)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{
		Confirmed:     counts.Confirmed,
		Pending:       counts.Pending,
		PledgedCents:  Pledged(counts.Confirmed, s.cfg.PerLinkCents, s.cfg.CapCents),
		CapCents:      s.cfg.CapCents,
		PerLinkCents:  s.cfg.PerLinkCents,
		CharityName:   s.cfg.CharityName,
		CharityURL:    s.cfg.CharityURL,
		PledgerName:   s.cfg.PledgerName,
		SignerAddress: s.cfg.SignerAddress,
		Paused:        s.paused.Load(),
	}
	if ok {
		stats.HeadN = head.N
		stats.HeadSignature = head.Signature
	}
	return stats, nil
}

// Pledged is what the confirmed links add up to, capped.
func Pledged(confirmed int64, perLinkCents, capCents int) int {
	total := confirmed * int64(perLinkCents)
	if total > int64(capCents) {
		return capCents
	}
	return int(total)
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}
