package httpapi

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

// solve finds a nonce the slow way; tests use a small difficulty.
func solve(seed, act string, bits int) string {
	for n := 0; ; n++ {
		nonce := strconv.Itoa(n)
		if solves(seed, nonce, act, bits) {
			return nonce
		}
	}
}

func TestLeadingZeroBits(t *testing.T) {
	cases := []struct {
		in   []byte
		want int
	}{
		{[]byte{0x80}, 0},
		{[]byte{0x01}, 7},
		{[]byte{0x00, 0xff}, 8},
		{[]byte{0x00, 0x00, 0x3f}, 18},
		{[]byte{0x00, 0x00, 0x00}, 24},
	}
	for _, tc := range cases {
		if got := leadingZeroBits(tc.in); got != tc.want {
			t.Errorf("leadingZeroBits(%x) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestChallenges(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	c := newChallenges(8, 10*time.Minute)
	c.now = func() time.Time { return now }
	act := "I carried groceries for a neighbour"

	ch, err := c.issue()
	if err != nil {
		t.Fatal(err)
	}
	if ch.Difficulty != 8 || len(ch.Seed) != 32 || !ch.Expires.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("challenge = %+v", ch)
	}
	if err := c.redeem(ch.Seed, "not-the-answer", act); !errors.Is(err, errChallengeWrong) {
		t.Errorf("wrong nonce: %v", err)
	}
	// the seed was spent by the wrong answer
	if err := c.redeem(ch.Seed, solve(ch.Seed, act, 8), act); !errors.Is(err, errChallengeUnknown) {
		t.Errorf("spent seed: %v", err)
	}

	ch, _ = c.issue()
	nonce := solve(ch.Seed, act, 8)
	if err := c.redeem(ch.Seed, nonce, act+" edited"); !errors.Is(err, errChallengeWrong) {
		t.Errorf("a nonce for another sentence passed: %v", err)
	}
	ch, _ = c.issue()
	if err := c.redeem(ch.Seed, solve(ch.Seed, act, 8), act); err != nil {
		t.Errorf("a right answer was refused: %v", err)
	}
	if err := c.redeem(ch.Seed, solve(ch.Seed, act, 8), act); !errors.Is(err, errChallengeUnknown) {
		t.Errorf("reuse: %v", err)
	}

	ch, _ = c.issue()
	now = now.Add(11 * time.Minute)
	if err := c.redeem(ch.Seed, solve(ch.Seed, act, 8), act); !errors.Is(err, errChallengeExpired) {
		t.Errorf("expired: %v", err)
	}
	if err := c.redeem("0000", "1", act); !errors.Is(err, errChallengeUnknown) {
		t.Errorf("never issued: %v", err)
	}
}
