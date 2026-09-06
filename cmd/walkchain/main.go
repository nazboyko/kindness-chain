// Command walkchain reads the chain straight from a Solana RPC node and
// prints it, so anyone can check the site's numbers without trusting
// the site. It starts at a signature, follows every memo's prev back to
// genesis, and reports what it found. It never opens the database.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	"github.com/nazboyko/kindness-chain/internal/chain"
	"github.com/nazboyko/kindness-chain/internal/solana"
)

// pace keeps the walk under the public node's rate limit.
const pace = 100 * time.Millisecond

// site is the part of /api/stats the walk needs: where the chain ends
// and what each link is worth.
type site struct {
	Count        int64 `json:"count"`
	PerLinkCents int   `json:"perLinkCents"`
	CapCents     int   `json:"capCents"`
	Head         struct {
		Signature string `json:"signature"`
	} `json:"head"`
}

func main() {
	rpcURL := flag.String("rpc", "https://api.devnet.solana.com", "the JSON-RPC node to read from")
	statsURL := flag.String("stats", "https://kindness-chain.fly.dev/api/stats", "where the chain head and the pledge terms come from when no signature is given")
	perLink := flag.Int("per-link-cents", 10, "the pledge per link, when a signature is given")
	capCents := flag.Int("cap-cents", 5000, "the pledge cap, when a signature is given")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: walkchain [flags] [signature]")
		fmt.Fprintln(os.Stderr, "Walks the chain from a signature, or from the head the site reports, back to genesis.")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()

	head := flag.Arg(0)
	var reported int64 = -1
	if head == "" {
		s, err := readSite(*statsURL)
		if err != nil {
			fatal("read the chain head from %s: %v", *statsURL, err)
		}
		if s.Head.Signature == "" {
			fatal("the site reports no confirmed link yet")
		}
		head, reported = s.Head.Signature, s.Count
		*perLink, *capCents = s.PerLinkCents, s.CapCents
	}

	links, problems := walk(solana.NewReader(*rpcURL), head)

	fmt.Println()
	after := max(links-1, 0)
	if links > 0 {
		fmt.Printf("%d links after the pledge, %s owed of %s at %s per link\n",
			after, chain.Money(chain.Pledged(int64(after), *perLink, *capCents)), chain.Money(*capCents), chain.Money(*perLink))
	}
	if len(problems) == 0 {
		fmt.Println("every prev matched: yes, the chain runs back to the pledge at #0")
	} else {
		fmt.Printf("every prev matched: no, %d problems\n", len(problems))
		for _, p := range problems {
			fmt.Println("  " + p)
		}
	}
	if reported >= 0 {
		switch {
		case reported == int64(after):
			fmt.Printf("the site reports %d confirmed links, the same\n", reported)
		default:
			fmt.Printf("the site reports %d confirmed links, the walk found %d\n", reported, after)
		}
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
}

// walk prints one line per memo from head back to genesis and returns
// how many it printed and what did not add up.
func walk(reader *solana.RPC, head string) (int, []string) {
	ctx := context.Background()
	var (
		links    int
		problems []string
		lastN    int64 = -1
		sig            = head
	)
	for {
		if links > 0 {
			time.Sleep(pace)
		}
		raw, err := reader.GetMemo(ctx, sig)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", short(sig), err))
			break
		}
		memo, err := chain.DecodeMemo(raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", short(sig), err))
			break
		}
		links++
		fmt.Printf("#%-4d %-20s %-62s %s\n", memo.N, cut(name(memo.By), 20), cut(memo.Act, 60), sig)

		if lastN >= 0 && memo.N >= lastN {
			problems = append(problems, fmt.Sprintf("#%d points at #%d, which is not older", lastN, memo.N))
		}
		lastN = memo.N
		if memo.Prev == chain.GenesisPrev {
			if memo.N != 0 {
				problems = append(problems, fmt.Sprintf("the chain ends at #%d, not at #0", memo.N))
			}
			break
		}
		sig = memo.Prev
	}
	return links, problems
}

func readSite(url string) (site, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return site{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return site{}, errors.New(resp.Status)
	}
	var s site
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return site{}, err
	}
	return s, nil
}

func name(by string) string {
	if by == "" {
		return "Anonymous"
	}
	return by
}

// cut shortens text to n characters with an ellipsis.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-1]) + "…"
}

func short(sig string) string {
	if len(sig) <= 12 {
		return sig
	}
	return sig[:4] + "…" + sig[len(sig)-4:]
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
