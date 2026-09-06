// Package solana writes memos to the Solana ledger and reads them back.
//
// Everything the server needs from the chain goes through the Client
// interface, so the rest of the code and all of the tests can run
// against the Fake without a network.
package solana

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"unicode/utf8"
)

// MaxMemoBytes is the largest memo the SPL Memo program takes in one
// instruction, per its documentation.
const MaxMemoBytes = 566

var (
	// ErrMemoTooLarge is returned before anything is sent when a memo
	// would not fit in a single instruction.
	ErrMemoTooLarge = errors.New("memo too large")
	// ErrMemoNotUTF8 is returned for bytes the memo program would reject.
	ErrMemoNotUTF8 = errors.New("memo is not valid UTF-8")
	// ErrNotFound is returned by GetMemo when the cluster does not know
	// the signature.
	ErrNotFound = errors.New("transaction not found")
	// ErrRejected means the cluster ran the transaction and refused it.
	// Sending the same memo again would only be refused again.
	ErrRejected = errors.New("transaction rejected on-chain")
	// ErrExpired means the transaction was never seen before its
	// blockhash stopped being valid. It can never land, so a retry with
	// a fresh blockhash is safe.
	ErrExpired = errors.New("transaction expired")
)

// Client is the chain as the rest of the server sees it.
type Client interface {
	// SendMemo writes memo as one SPL Memo transaction, waits until the
	// cluster confirms it, and returns the base58 signature.
	SendMemo(ctx context.Context, memo []byte) (string, error)
	// GetMemo reads the memo bytes back out of a confirmed transaction.
	GetMemo(ctx context.Context, signature string) ([]byte, error)
}

// Validate is the check every client runs before sending.
func Validate(memo []byte) error {
	if len(memo) > MaxMemoBytes {
		return fmt.Errorf("%w: %d bytes, the limit is %d", ErrMemoTooLarge, len(memo), MaxMemoBytes)
	}
	if !utf8.Valid(memo) {
		return ErrMemoNotUTF8
	}
	return nil
}

// ExplorerURL is the Solana Explorer page for a transaction.
func ExplorerURL(cluster, signature string) string {
	return explorer("tx", cluster, signature)
}

// ExplorerAddressURL is the Solana Explorer page for an account, which
// lists every transaction it signed.
func ExplorerAddressURL(cluster, address string) string {
	return explorer("address", cluster, address)
}

func explorer(kind, cluster, id string) string {
	u := "https://explorer.solana.com/" + kind + "/" + url.PathEscape(id)
	if cluster != "" && cluster != "mainnet-beta" {
		u += "?cluster=" + url.QueryEscape(cluster)
	}
	return u
}
