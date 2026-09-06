// Command server runs the Kindness Chain site: the API, the event
// stream and the frontend, all from one process and one binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nazboyko/kindness-chain/internal/chain"
	"github.com/nazboyko/kindness-chain/internal/config"
	"github.com/nazboyko/kindness-chain/internal/httpapi"
	"github.com/nazboyko/kindness-chain/internal/solana"
	"github.com/nazboyko/kindness-chain/internal/store"
	"github.com/nazboyko/kindness-chain/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config:\n%w", err)
	}

	// Take the port before any other startup work, so a stale process
	// still holding it is the first line of the log and not the last.
	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("listen on :%s: %w", cfg.Port, err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ledger, signer, err := newLedger(cfg)
	if err != nil {
		return err
	}
	hub := httpapi.NewHub(cfg.SolanaCluster)
	service := chain.New(chain.Config{
		PerLinkCents:  cfg.PledgePerLinkCents,
		CapCents:      cfg.PledgeCapCents,
		CharityName:   cfg.CharityName,
		CharityURL:    cfg.CharityURL,
		PledgerName:   cfg.PledgerName,
		SignerAddress: signer,
	}, st, ledger, hub)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := service.Start(ctx); err != nil {
		return fmt.Errorf("start chain: %w", err)
	}
	workerDone := make(chan struct{})
	go func() {
		service.Run(ctx)
		close(workerDone)
	}()

	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return fmt.Errorf("embedded frontend: %w", err)
	}
	api := httpapi.New(httpapi.Config{
		Cluster:          cfg.SolanaCluster,
		RateLimitPerHour: cfg.RateLimitPerHour,
		GlobalPerMinute:  cfg.GlobalPerMinute,
		PowBits:          cfg.PowBits,
	}, service, ledger, hub, dist)

	srv := &http.Server{
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// no WriteTimeout: the event stream stays open as long as a tab does
	}
	// open event streams would otherwise hold the shutdown until its timeout
	srv.RegisterOnShutdown(hub.Close)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("listening on :%s", cfg.Port)
	if err := srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-shutdownDone

	// a memo in flight is worth waiting for: once it is on the chain it
	// must be recorded, or the next start would write it again
	select {
	case <-workerDone:
	case <-time.After(20 * time.Second):
		log.Print("the worker is still busy, exiting anyway")
	}
	return nil
}

// newLedger picks the real chain or the in-memory fake. It also
// returns the signing address when there is one.
func newLedger(cfg config.Config) (solana.Client, string, error) {
	if cfg.FakeChain {
		log.Print("FAKE_CHAIN is set: links are confirmed in memory and nothing is written to Solana")
		fake := solana.NewFake()
		fake.Delay = 1500 * time.Millisecond
		return fake, "", nil
	}
	client, err := solana.NewRPC(cfg.SolanaRPCURL, []byte(cfg.SolanaKeypair))
	if err != nil {
		return nil, "", err
	}
	log.Printf("signing memos as %s on %s", client.Address(), cfg.SolanaCluster)
	return client, client.Address(), nil
}
