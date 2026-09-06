package solana

import (
	"context"
	"errors"
	"fmt"
	"time"

	solanago "github.com/gagliardetto/solana-go"
	memoprogram "github.com/gagliardetto/solana-go/programs/memo"
	"github.com/gagliardetto/solana-go/rpc"
)

// RPC talks to one JSON-RPC node and signs everything with one keypair.
type RPC struct {
	node    *rpc.Client
	key     solanago.PrivateKey
	timeout time.Duration
}

// NewRPC parses the keypair in the solana-keygen JSON array format and
// prepares a client for the node at url. Nothing is sent yet.
func NewRPC(url string, keypairJSON []byte) (*RPC, error) {
	key, err := solanago.PrivateKeyFromSolanaKeygenFileBytes(keypairJSON)
	if err != nil {
		return nil, fmt.Errorf("parse keypair: %w", err)
	}
	return &RPC{node: rpc.New(url), key: key, timeout: 90 * time.Second}, nil
}

// Address is the base58 public key that signs and pays for every memo.
func (c *RPC) Address() string {
	return c.key.PublicKey().String()
}

// Balance is the signer's balance in lamports.
func (c *RPC) Balance(ctx context.Context) (uint64, error) {
	out, err := c.node.GetBalance(ctx, c.key.PublicKey(), rpc.CommitmentConfirmed)
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}
	return out.Value, nil
}

// SendMemo builds a transaction with a single memo instruction, signs
// it, sends it and waits for confirmation.
func (c *RPC) SendMemo(ctx context.Context, memo []byte) (string, error) {
	if err := Validate(memo); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	signer := c.key.PublicKey()
	instruction, err := memoprogram.NewMemoInstruction(memo, signer).ValidateAndBuild()
	if err != nil {
		return "", fmt.Errorf("build memo instruction: %w", err)
	}

	recent, err := c.node.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("get latest blockhash: %w", err)
	}

	tx, err := solanago.NewTransaction(
		[]solanago.Instruction{instruction},
		recent.Value.Blockhash,
		solanago.TransactionPayer(signer),
	)
	if err != nil {
		return "", fmt.Errorf("build transaction: %w", err)
	}
	if _, err := tx.Sign(func(key solanago.PublicKey) *solanago.PrivateKey {
		if key.Equals(signer) {
			return &c.key
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}

	sig, err := c.node.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{
		PreflightCommitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return "", fmt.Errorf("send transaction: %w", err)
	}
	if err := c.waitConfirmed(ctx, sig, recent.Value.LastValidBlockHeight); err != nil {
		return "", err
	}
	return sig.String(), nil
}

// waitConfirmed polls until the cluster reports the transaction as
// confirmed or finalized. While the transaction is still unseen, the
// block height is checked too: once the chain has moved past the
// blockhash's last valid height the transaction can never land, and
// saying so early lets the caller retry with a fresh blockhash.
func (c *RPC) waitConfirmed(ctx context.Context, sig solanago.Signature, lastValidHeight uint64) error {
	delay := 500 * time.Millisecond
	for {
		seen := false
		statuses, err := c.node.GetSignatureStatuses(ctx, false, sig)
		if err == nil && len(statuses.Value) == 1 && statuses.Value[0] != nil {
			seen = true
			status := statuses.Value[0]
			if status.Err != nil {
				return fmt.Errorf("%w: %s: %v", ErrRejected, sig, status.Err)
			}
			switch status.ConfirmationStatus {
			case rpc.ConfirmationStatusConfirmed, rpc.ConfirmationStatusFinalized:
				return nil
			}
		}
		if !seen {
			height, err := c.node.GetBlockHeight(ctx, rpc.CommitmentConfirmed)
			if err == nil && height > lastValidHeight {
				return fmt.Errorf("%w: %s", ErrExpired, sig)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for confirmation of %s: %w", sig, ctx.Err())
		case <-time.After(delay):
		}
		if delay < 3*time.Second {
			delay += 500 * time.Millisecond
		}
	}
}

// GetMemo fetches the transaction and returns the data of its memo
// instruction, exactly as it was written.
func (c *RPC) GetMemo(ctx context.Context, signature string) ([]byte, error) {
	sig, err := solanago.SignatureFromBase58(signature)
	if err != nil {
		return nil, fmt.Errorf("parse signature: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var maxVersion uint64 // accept versioned transactions too
	out, err := c.node.GetTransaction(ctx, sig, &rpc.GetTransactionOpts{
		Encoding:                       solanago.EncodingBase64,
		Commitment:                     rpc.CommitmentConfirmed,
		MaxSupportedTransactionVersion: &maxVersion,
	})
	if err != nil {
		if errors.Is(err, rpc.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get transaction: %w", err)
	}
	if out == nil || out.Transaction == nil {
		return nil, ErrNotFound
	}
	tx, err := out.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("decode transaction: %w", err)
	}
	return memoFrom(tx)
}

// memoFrom returns the data of the first memo instruction in tx.
func memoFrom(tx *solanago.Transaction) ([]byte, error) {
	for _, ix := range tx.Message.Instructions {
		program, err := tx.Message.Program(ix.ProgramIDIndex)
		if err != nil {
			return nil, fmt.Errorf("resolve program: %w", err)
		}
		if program.Equals(solanago.MemoProgramID) {
			return []byte(ix.Data), nil
		}
	}
	return nil, errors.New("transaction has no memo instruction")
}
