package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/bits"
	"sync"
	"time"
)

// challenges hands out proof-of-work seeds and checks the answers. A
// browser asks for a seed, finds a nonce that makes
// sha256(seed + nonce + sentence) start with enough zero bits, and sends
// both with the sentence. That costs a phone a second or two and a
// script farm the same per link, which is the point. Seeds live in
// memory, are good for one submission, and expire; a restart forgets
// them and clients simply ask again.
type challenges struct {
	bits int
	ttl  time.Duration
	now  func() time.Time

	mu     sync.Mutex
	issued map[string]time.Time
	count  int
}

// challenge is what a browser receives.
type challenge struct {
	Seed       string    `json:"seed"`
	Difficulty int       `json:"difficulty"`
	Expires    time.Time `json:"expires"`
}

var (
	// errChallengeUnknown covers seeds that were never issued, were
	// already spent, or were forgotten by a restart.
	errChallengeUnknown = errors.New("unknown or used challenge")
	// errChallengeExpired means the seed was issued too long ago.
	errChallengeExpired = errors.New("challenge expired")
	// errChallengeWrong means the nonce does not meet the difficulty.
	errChallengeWrong = errors.New("proof of work does not meet the difficulty")
)

// newChallenges requires bits leading zero bits and forgets seeds after ttl.
func newChallenges(bits int, ttl time.Duration) *challenges {
	return &challenges{bits: bits, ttl: ttl, now: time.Now, issued: map[string]time.Time{}}
}

// issue makes a fresh seed.
func (c *challenges) issue() (challenge, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return challenge{}, err
	}
	seed := hex.EncodeToString(raw[:])
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	if c.count%256 == 0 {
		c.sweep(now)
	}
	c.issued[seed] = now
	return challenge{Seed: seed, Difficulty: c.bits, Expires: now.Add(c.ttl)}, nil
}

// redeem checks an answer. The seed is spent whether or not the answer
// is right, so every attempt costs a fresh challenge.
func (c *challenges) redeem(seed, nonce, act string) error {
	c.mu.Lock()
	issuedAt, ok := c.issued[seed]
	delete(c.issued, seed)
	c.mu.Unlock()
	if !ok {
		return errChallengeUnknown
	}
	if c.now().Sub(issuedAt) > c.ttl {
		return errChallengeExpired
	}
	if !solves(seed, nonce, act, c.bits) {
		return errChallengeWrong
	}
	return nil
}

func (c *challenges) sweep(now time.Time) {
	for seed, at := range c.issued {
		if now.Sub(at) > c.ttl {
			delete(c.issued, seed)
		}
	}
}

// solves is the check itself: the SHA-256 of seed, nonce and sentence,
// concatenated as text, must start with at least bits zero bits.
func solves(seed, nonce, act string, bits int) bool {
	sum := sha256.Sum256([]byte(seed + nonce + act))
	return leadingZeroBits(sum[:]) >= bits
}

func leadingZeroBits(b []byte) int {
	n := 0
	for _, x := range b {
		if x == 0 {
			n += 8
			continue
		}
		n += bits.LeadingZeros8(x)
		break
	}
	return n
}
