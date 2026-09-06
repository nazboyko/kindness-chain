// Command memotest sends one memo to the configured cluster and reads it
// back. It is the smoke test for the chain package: when the explorer
// link it prints opens and shows the text, the server can write links.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nazboyko/kindness-chain/internal/solana"
)

func main() {
	keypair := os.Getenv("SOLANA_KEYPAIR")
	if keypair == "" {
		fatal("SOLANA_KEYPAIR is not set; it takes the JSON byte array from solana-keygen")
	}
	rpcURL := env("SOLANA_RPC_URL", "https://api.devnet.solana.com")
	cluster := env("SOLANA_CLUSTER", "devnet")

	client, err := solana.NewRPC(rpcURL, []byte(keypair))
	if err != nil {
		fatal("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Printf("node      %s\n", rpcURL)
	fmt.Printf("signer    %s\n", client.Address())
	balance, err := client.Balance(ctx)
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("balance   %.4f SOL\n", float64(balance)/1e9)
	if balance == 0 {
		fatal("the signer has no SOL; airdrop some on devnet first")
	}

	memo := []byte("kindness-chain smoke test " + time.Now().UTC().Format(time.RFC3339))
	start := time.Now()
	sig, err := client.SendMemo(ctx, memo)
	if err != nil {
		fatal("send memo: %v", err)
	}
	fmt.Printf("signature %s\n", sig)
	fmt.Printf("confirmed in %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("explorer  %s\n", solana.ExplorerURL(cluster, sig))

	back, err := client.GetMemo(ctx, sig)
	if err != nil {
		fatal("read memo back: %v", err)
	}
	fmt.Printf("read back %q\n", back)
	if !bytes.Equal(back, memo) {
		fatal("the memo read back does not match what was sent")
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
