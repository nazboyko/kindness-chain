package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	solanago "github.com/gagliardetto/solana-go"
	memoprogram "github.com/gagliardetto/solana-go/programs/memo"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		memo []byte
		want error
	}{
		{"short memo", []byte(`{"v":1}`), nil},
		{"exactly at the limit", bytes.Repeat([]byte("a"), maxMemoBytes), nil},
		{"one byte over", bytes.Repeat([]byte("a"), maxMemoBytes+1), ErrMemoTooLarge},
		{"invalid utf-8", []byte{0xff, 0xfe}, ErrMemoNotUTF8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.memo); !errors.Is(err, tc.want) {
				t.Errorf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestExplorerURL(t *testing.T) {
	cases := []struct {
		cluster, sig, want string
	}{
		{"devnet", "5abc", "https://explorer.solana.com/tx/5abc?cluster=devnet"},
		{"mainnet-beta", "5abc", "https://explorer.solana.com/tx/5abc"},
		{"", "5abc", "https://explorer.solana.com/tx/5abc"},
	}
	for _, tc := range cases {
		if got := ExplorerURL(tc.cluster, tc.sig); got != tc.want {
			t.Errorf("ExplorerURL(%q) = %q, want %q", tc.cluster, got, tc.want)
		}
	}
	if got := ExplorerAddressURL("devnet", "Bh1"); got != "https://explorer.solana.com/address/Bh1?cluster=devnet" {
		t.Errorf("ExplorerAddressURL = %q", got)
	}
}

func TestFake(t *testing.T) {
	ctx := context.Background()
	fake := NewFake()

	first, err := fake.SendMemo(ctx, []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := fake.SendMemo(ctx, []byte("two"))
	if err != nil {
		t.Fatal(err)
	}
	if first != "fake-000001" || second != "fake-000002" {
		t.Errorf("signatures = %q, %q", first, second)
	}

	memo, err := fake.GetMemo(ctx, second)
	if err != nil || string(memo) != "two" {
		t.Errorf("GetMemo = %q, %v", memo, err)
	}
	if _, err := fake.GetMemo(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown signature error = %v, want ErrNotFound", err)
	}

	boom := errors.New("boom")
	fake.FailNext(boom)
	if _, err := fake.SendMemo(ctx, []byte("three")); !errors.Is(err, boom) {
		t.Errorf("scripted failure = %v, want boom", err)
	}
	third, err := fake.SendMemo(ctx, []byte("three"))
	if err != nil || third != "fake-000003" {
		t.Errorf("after failure: %q, %v", third, err)
	}
	if got := fake.Sent(); len(got) != 3 {
		t.Errorf("Sent() = %v", got)
	}

	if _, err := fake.SendMemo(ctx, bytes.Repeat([]byte("x"), maxMemoBytes+1)); !errors.Is(err, ErrMemoTooLarge) {
		t.Errorf("oversize memo error = %v", err)
	}
}

func keypairJSON(t *testing.T) ([]byte, solanago.PrivateKey) {
	t.Helper()
	key, err := solanago.NewRandomPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	nums := make([]int, len(key))
	for i, b := range key {
		nums[i] = int(b)
	}
	out, err := json.Marshal(nums)
	if err != nil {
		t.Fatal(err)
	}
	return out, key
}

func TestReaderCannotSend(t *testing.T) {
	reader := NewReader("http://127.0.0.1:9")
	if reader.Address() != "" {
		t.Errorf("a reader has an address: %q", reader.Address())
	}
	if _, err := reader.SendMemo(context.Background(), []byte("hello")); !errors.Is(err, errNoKey) {
		t.Errorf("a reader sent a memo: %v", err)
	}
}

func TestNewRPC(t *testing.T) {
	raw, key := keypairJSON(t)
	client, err := NewRPC("http://127.0.0.1:9", raw)
	if err != nil {
		t.Fatalf("NewRPC: %v", err)
	}
	if client.Address() != key.PublicKey().String() {
		t.Errorf("Address() = %s, want %s", client.Address(), key.PublicKey())
	}

	if _, err := NewRPC("http://127.0.0.1:9", []byte("[1,2,3]")); err == nil {
		t.Error("a 3 byte keypair was accepted")
	}
	if _, err := NewRPC("http://127.0.0.1:9", []byte("not json")); err == nil {
		t.Error("garbage was accepted as a keypair")
	}
}

func TestRPCRejectsOversizeMemoBeforeSending(t *testing.T) {
	raw, _ := keypairJSON(t)
	client, err := NewRPC("http://127.0.0.1:9", raw)
	if err != nil {
		t.Fatal(err)
	}
	// nothing listens on that port, so reaching the network would fail
	// with a different error than the size check
	_, err = client.SendMemo(context.Background(), bytes.Repeat([]byte("x"), maxMemoBytes+1))
	if !errors.Is(err, ErrMemoTooLarge) {
		t.Errorf("error = %v, want ErrMemoTooLarge", err)
	}
}

func TestMemoFromTransaction(t *testing.T) {
	_, key := keypairJSON(t)
	memo := []byte(`{"v":1,"n":7}`)
	instruction, err := memoprogram.NewMemoInstruction(memo, key.PublicKey()).ValidateAndBuild()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := solanago.NewTransaction(
		[]solanago.Instruction{instruction},
		solanago.Hash{},
		solanago.TransactionPayer(key.PublicKey()),
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := memoFrom(tx)
	if err != nil {
		t.Fatalf("memoFrom: %v", err)
	}
	if !bytes.Equal(got, memo) {
		t.Errorf("memoFrom = %q, want %q", got, memo)
	}

	empty := &solanago.Transaction{}
	if _, err := memoFrom(empty); err == nil || !strings.Contains(err.Error(), "no memo") {
		t.Errorf("empty transaction error = %v", err)
	}
}
