package config

import (
	"strings"
	"testing"
)

func full() map[string]string {
	return map[string]string{
		"SOLANA_KEYPAIR": "[1,2,3]",
		"CHARITY_NAME":   "International Institute of Minnesota",
		"CHARITY_URL":    "https://iimn.org",
		"PLEDGER_NAME":   "Nazar",
	}
}

func TestLoad(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T, c Config)
	}{
		{
			name: "defaults fill the optional values",
			env:  full(),
			check: func(t *testing.T, c Config) {
				if c.Port != "8080" || c.PledgePerLinkCents != 10 || c.PledgeCapCents != 5000 || c.RateLimitPerHour != 3 {
					t.Errorf("defaults not applied: %+v", c)
				}
				if c.SolanaCluster != "devnet" || c.SolanaRPCURL != "https://api.devnet.solana.com" {
					t.Errorf("solana defaults not applied: %+v", c)
				}
				if c.GlobalPerMinute != 10 || c.PowBits != 18 {
					t.Errorf("abuse defaults not applied: %+v", c)
				}
			},
		},
		{
			name: "explicit values win",
			env: func() map[string]string {
				m := full()
				m["PORT"] = "9090"
				m["PLEDGE_PER_LINK_CENTS"] = "25"
				m["PLEDGE_CAP_CENTS"] = "100"
				return m
			}(),
			check: func(t *testing.T, c Config) {
				if c.Port != "9090" || c.PledgePerLinkCents != 25 || c.PledgeCapCents != 100 {
					t.Errorf("explicit values lost: %+v", c)
				}
			},
		},
		{
			name: "missing keypair",
			env: func() map[string]string {
				m := full()
				delete(m, "SOLANA_KEYPAIR")
				return m
			}(),
			wantErr: "SOLANA_KEYPAIR is not set",
		},
		{
			name: "keypair that is not a byte array",
			env: func() map[string]string {
				m := full()
				m["SOLANA_KEYPAIR"] = "/home/me/key.json"
				return m
			}(),
			wantErr: "SOLANA_KEYPAIR must be the JSON byte array",
		},
		{
			name: "non numeric cents",
			env: func() map[string]string {
				m := full()
				m["PLEDGE_PER_LINK_CENTS"] = "ten"
				return m
			}(),
			wantErr: "PLEDGE_PER_LINK_CENTS must be a positive integer",
		},
		{
			name: "cap below the per link amount",
			env: func() map[string]string {
				m := full()
				m["PLEDGE_PER_LINK_CENTS"] = "500"
				m["PLEDGE_CAP_CENTS"] = "100"
				return m
			}(),
			wantErr: "PLEDGE_CAP_CENTS must be at least",
		},
		{
			name: "relative charity url",
			env: func() map[string]string {
				m := full()
				m["CHARITY_URL"] = "iimn.org"
				return m
			}(),
			wantErr: "CHARITY_URL must be an absolute URL",
		},
		{
			name: "proof of work can be turned off",
			env: func() map[string]string {
				m := full()
				m["POW_BITS"] = "0"
				return m
			}(),
			check: func(t *testing.T, c Config) {
				if c.PowBits != 0 {
					t.Errorf("PowBits = %d, want 0", c.PowBits)
				}
			},
		},
		{
			name: "proof of work bits out of range",
			env: func() map[string]string {
				m := full()
				m["POW_BITS"] = "40"
				return m
			}(),
			wantErr: "POW_BITS must be a whole number from 0 to 32",
		},
		{
			name: "fake chain needs no keypair",
			env: func() map[string]string {
				m := full()
				delete(m, "SOLANA_KEYPAIR")
				m["FAKE_CHAIN"] = "1"
				return m
			}(),
			check: func(t *testing.T, c Config) {
				if !c.FakeChain || c.SolanaKeypair != "" {
					t.Errorf("fake chain config = %+v", c)
				}
			},
		},
		{
			name:    "every missing value is reported at once",
			env:     map[string]string{},
			wantErr: "PLEDGER_NAME is not set",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := load(func(k string) string { return tc.env[k] })
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, cfg)
		})
	}
}
