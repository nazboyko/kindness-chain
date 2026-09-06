// Package config reads the server's settings from the environment and
// refuses to start on anything missing or malformed.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is everything the server needs, read once at startup.
type Config struct {
	Port               string
	DBPath             string
	SolanaRPCURL       string
	SolanaKeypair      string // the JSON byte array from a solana-keygen file
	SolanaCluster      string
	FakeChain          bool // confirm links in memory instead of on Solana; for frontend work only
	PledgePerLinkCents int
	PledgeCapCents     int
	CharityName        string
	CharityURL         string
	PledgerName        string
	RateLimitPerHour   int
}

// Load reads the environment. Defaults exist only for values that are
// safe to guess on a laptop. Anything that words the pledge or signs
// transactions has to be set on purpose.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	var errs []error

	optional := func(name, fallback string) string {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v
		}
		return fallback
	}
	required := func(name string) string {
		v := strings.TrimSpace(getenv(name))
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is not set", name))
		}
		return v
	}
	positive := func(name string, fallback int) int {
		raw := strings.TrimSpace(getenv(name))
		if raw == "" {
			return fallback
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive integer, got %q", name, raw))
			return fallback
		}
		return n
	}

	cfg := Config{
		Port:               optional("PORT", "8080"),
		DBPath:             optional("DB_PATH", "chain.db"),
		SolanaRPCURL:       optional("SOLANA_RPC_URL", "https://api.devnet.solana.com"),
		SolanaKeypair:      optional("SOLANA_KEYPAIR", ""),
		SolanaCluster:      optional("SOLANA_CLUSTER", "devnet"),
		FakeChain:          isTrue(getenv("FAKE_CHAIN")),
		PledgePerLinkCents: positive("PLEDGE_PER_LINK_CENTS", 10),
		PledgeCapCents:     positive("PLEDGE_CAP_CENTS", 5000),
		CharityName:        required("CHARITY_NAME"),
		CharityURL:         required("CHARITY_URL"),
		PledgerName:        required("PLEDGER_NAME"),
		RateLimitPerHour:   positive("RATE_LIMIT_PER_HOUR", 3),
	}

	if _, err := strconv.Atoi(cfg.Port); err != nil {
		errs = append(errs, fmt.Errorf("PORT must be a number, got %q", cfg.Port))
	}
	switch {
	case cfg.FakeChain:
		// the fake signs nothing
	case cfg.SolanaKeypair == "":
		errs = append(errs, errors.New("SOLANA_KEYPAIR is not set"))
	case !strings.HasPrefix(cfg.SolanaKeypair, "["):
		errs = append(errs, errors.New("SOLANA_KEYPAIR must be the JSON byte array from solana-keygen, starting with ["))
	}
	if cfg.CharityURL != "" && !absoluteURL(cfg.CharityURL) {
		errs = append(errs, fmt.Errorf("CHARITY_URL must be an absolute URL, got %q", cfg.CharityURL))
	}
	if cfg.PledgeCapCents < cfg.PledgePerLinkCents {
		errs = append(errs, errors.New("PLEDGE_CAP_CENTS must be at least PLEDGE_PER_LINK_CENTS"))
	}

	return cfg, errors.Join(errs...)
}

func absoluteURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}
