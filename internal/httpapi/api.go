// Package httpapi is the HTTP face of the chain: a small JSON API, a
// server-sent event stream, and the embedded frontend.
package httpapi

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/nazboyko/kindness-chain/internal/chain"
	"github.com/nazboyko/kindness-chain/internal/solana"
)

// Chain is what the handlers need from the chain service.
type Chain interface {
	Add(ctx context.Context, act, by string) (chain.Link, error)
	Duplicate(ctx context.Context, act string) (bool, error)
	Get(ctx context.Context, n int64) (chain.Link, error)
	List(ctx context.Context, before int64, limit int) ([]chain.Link, error)
	Stats(ctx context.Context) (chain.Stats, error)
}

// Config is what the API needs to know about its surroundings.
type Config struct {
	Cluster          string // names the cluster in explorer links
	RateLimitPerHour int    // links one address may add per hour
	GlobalPerMinute  int    // links the whole chain accepts per minute
	PowBits          int    // proof-of-work difficulty; 0 turns it off
}

// challengeTTL is how long a proof-of-work seed stays valid.
const challengeTTL = 10 * time.Minute

// API holds the handlers and what they depend on.
type API struct {
	cfg    Config
	chain  Chain
	ledger solana.Client
	hub    *Hub
	perIP  *limiter
	global *limiter
	pow    *challenges // nil when proof of work is off
	dist   fs.FS
	log    *log.Logger
}

// New wires the API. The hub is created separately because the chain
// service needs it before the API exists.
func New(cfg Config, service Chain, ledger solana.Client, hub *Hub, dist fs.FS) *API {
	api := &API{
		cfg:    cfg,
		chain:  service,
		ledger: ledger,
		hub:    hub,
		perIP:  newLimiter(cfg.RateLimitPerHour, time.Hour),
		global: newLimiter(cfg.GlobalPerMinute, time.Minute),
		dist:   dist,
		log:    log.Default(),
	}
	if cfg.PowBits > 0 {
		api.pow = newChallenges(cfg.PowBits, challengeTTL)
	}
	return api
}

// Handler routes everything: the API, the event stream, health, and the
// frontend for whatever is left.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/stats", a.handleStats)
	mux.HandleFunc("GET /api/links", a.handleList)
	mux.HandleFunc("GET /api/links/{n}", a.handleGet)
	mux.HandleFunc("POST /api/links", a.handleAdd)
	mux.HandleFunc("GET /api/challenge", a.handleChallenge)
	mux.HandleFunc("GET /api/verify/{n}", a.handleVerify)
	mux.HandleFunc("GET /api/events", a.handleEvents)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})
	mux.Handle("/", static(a.dist))
	return a.logRequests(secureHeaders(mux))
}
