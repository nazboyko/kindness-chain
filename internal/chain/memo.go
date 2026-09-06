package chain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/nazboyko/kindness-chain/internal/solana"
)

// Memo is what goes on the chain for one link. The field order is fixed
// so the bytes can be rebuilt from the stored row and compared.
type Memo struct {
	V    int    `json:"v"`
	N    int64  `json:"n"`
	Act  string `json:"act"`
	By   string `json:"by"`
	Prev string `json:"prev"`
	T    string `json:"t"`
}

const (
	memoVersion = 1
	// GenesisPrev is what link #0 points at, since nothing comes before it.
	GenesisPrev = "genesis"
)

// EncodeMemo renders the memo for a link that follows prev. An empty
// prev means the link is the first on the chain.
func EncodeMemo(link Link, prev string) ([]byte, error) {
	if prev == "" {
		prev = GenesisPrev
	}
	memo := Memo{
		V:    memoVersion,
		N:    link.N,
		Act:  link.Act,
		By:   link.By,
		Prev: prev,
		T:    link.CreatedAt.UTC().Format(time.RFC3339),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// memos are read by people on an explorer, not by browsers
	enc.SetEscapeHTML(false)
	if err := enc.Encode(memo); err != nil {
		return nil, fmt.Errorf("encode memo: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// DecodeMemo parses what a transaction carries.
func DecodeMemo(data []byte) (Memo, error) {
	var memo Memo
	if err := json.Unmarshal(data, &memo); err != nil {
		return Memo{}, fmt.Errorf("parse memo: %w", err)
	}
	if memo.V != memoVersion {
		return Memo{}, fmt.Errorf("memo version %d is not %d", memo.V, memoVersion)
	}
	return memo, nil
}

// fitsOnChain refuses text that would not fit one memo once encoded.
// Plain text never gets near the limit; 200 characters of a wide script
// with escaping can.
func fitsOnChain(act, by string, createdAt time.Time) error {
	const longestSignature = 88 // base58 of 64 bytes
	widest := Link{N: 1 << 40, Act: act, By: by, CreatedAt: createdAt}
	memo, err := EncodeMemo(widest, strings.Repeat("1", longestSignature))
	if err != nil {
		return err
	}
	if err := solana.Validate(memo); err != nil {
		return refuse(MsgTooBigOnChain)
	}
	return nil
}

// GenesisText is the pledge, worded from the configuration, exactly as
// it goes on the chain as link #0.
func GenesisText(cfg Config) string {
	return fmt.Sprintf(
		"I, %s, pledge %s for every link added to this chain, up to %s, to the %s (%s). Count the links on-chain to hold me to it.",
		cfg.PledgerName, Money(cfg.PerLinkCents), Money(cfg.CapCents), cfg.CharityName, hostOf(cfg.CharityURL),
	)
}

// Money formats cents the way the page shows them: $0.10, $50.
func Money(cents int) string {
	if cents%100 == 0 {
		return fmt.Sprintf("$%d", cents/100)
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return strings.TrimPrefix(u.Host, "www.")
}
