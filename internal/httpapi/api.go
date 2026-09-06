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
	Get(ctx context.Context, n int64) (chain.Link, error)
	List(ctx context.Context, before int64, limit int) ([]chain.Link, error)
	Stats(ctx context.Context) (chain.Stats, error)
}

// Config is what the API needs to know about its surroundings.
type Config struct {
	Cluster          string // names the cluster in explorer links
	RateLimitPerHour int
}

// API holds the handlers and what they depend on.
type API struct {
	cfg     Config
	chain   Chain
	ledger  solana.Client
	hub     *Hub
	limiter *Limiter
	dist    fs.FS
	log     *log.Logger
}

// New wires the API. The hub is created separately because the chain
// service needs it before the API exists.
func New(cfg Config, service Chain, ledger solana.Client, hub *Hub, dist fs.FS) *API {
	return &API{
		cfg:     cfg,
		chain:   service,
		ledger:  ledger,
		hub:     hub,
		limiter: NewLimiter(cfg.RateLimitPerHour, time.Hour),
		dist:    dist,
		log:     log.Default(),
	}
}

// Handler routes everything: the API, the event stream, health, and the
// frontend for whatever is left.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/stats", a.handleStats)
	mux.HandleFunc("GET /api/links", a.handleList)
	mux.HandleFunc("GET /api/links/{n}", a.handleGet)
	mux.HandleFunc("POST /api/links", a.handleAdd)
	mux.HandleFunc("GET /api/verify/{n}", a.handleVerify)
	mux.HandleFunc("GET /api/events", a.handleEvents)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})
	mux.Handle("/", Static(a.dist))
	return a.logRequests(secureHeaders(mux))
}
