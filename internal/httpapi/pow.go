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

// Challenges hands out proof-of-work seeds and checks the answers. A
// browser asks for a seed, finds a nonce that makes
// sha256(seed + nonce + sentence) start with enough zero bits, and sends
// both with the sentence. That costs a phone a second or two and a
// script farm the same per link, which is the point. Seeds live in
// memory, are good for one submission, and expire; a restart forgets
// them and clients simply ask again.
type Challenges struct {
	bits int
	ttl  time.Duration
	now  func() time.Time

	mu     sync.Mutex
	issued map[string]time.Time
	count  int
}

// Challenge is what a browser receives.
type Challenge struct {
	Seed       string    `json:"seed"`
	Difficulty int       `json:"difficulty"`
	Expires    time.Time `json:"expires"`
}

var (
	// ErrChallengeUnknown covers seeds that were never issued, were
	// already spent, or were forgotten by a restart.
	ErrChallengeUnknown = errors.New("unknown or used challenge")
	// ErrChallengeExpired means the seed was issued too long ago.
	ErrChallengeExpired = errors.New("challenge expired")
	// ErrChallengeWrong means the nonce does not meet the difficulty.
	ErrChallengeWrong = errors.New("proof of work does not meet the difficulty")
)

// NewChallenges requires bits leading zero bits and forgets seeds after ttl.
func NewChallenges(bits int, ttl time.Duration) *Challenges {
	return &Challenges{bits: bits, ttl: ttl, now: time.Now, issued: map[string]time.Time{}}
}

// Issue makes a fresh seed.
func (c *Challenges) Issue() (Challenge, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Challenge{}, err
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
	return Challenge{Seed: seed, Difficulty: c.bits, Expires: now.Add(c.ttl)}, nil
}

// Redeem checks an answer. The seed is spent whether or not the answer
// is right, so every attempt costs a fresh challenge.
func (c *Challenges) Redeem(seed, nonce, act string) error {
	c.mu.Lock()
	issuedAt, ok := c.issued[seed]
	delete(c.issued, seed)
	c.mu.Unlock()
	if !ok {
		return ErrChallengeUnknown
	}
	if c.now().Sub(issuedAt) > c.ttl {
		return ErrChallengeExpired
	}
	if !Solves(seed, nonce, act, c.bits) {
		return ErrChallengeWrong
	}
	return nil
}

func (c *Challenges) sweep(now time.Time) {
	for seed, at := range c.issued {
		if now.Sub(at) > c.ttl {
			delete(c.issued, seed)
		}
	}
}

// Solves is the check itself, shared with tests and tools: the SHA-256
// of seed, nonce and sentence, concatenated as text, must start with at
// least bits zero bits.
func Solves(seed, nonce, act string, bits int) bool {
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
