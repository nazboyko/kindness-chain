package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter allows a number of actions per key inside a sliding window.
// It lives in memory only: nothing about an address is ever written
// down, and a restart forgets everything.
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	hits  map[string][]time.Time
	calls int
}

// NewLimiter allows limit actions per key per window.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

// Allow records an action for key when it is within the limit. When it
// is not, it also says how long until the oldest action leaves the
// window and one more is allowed.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	return l.check(key, true)
}

// Peek is Allow without recording anything: it answers whether the next
// action would be allowed.
func (l *Limiter) Peek(key string) (bool, time.Duration) {
	return l.check(key, false)
}

func (l *Limiter) check(key string, record bool) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.calls++
	if l.calls%1000 == 0 {
		l.sweep(now)
	}

	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.limit {
		l.hits[key] = recent
		return false, recent[0].Add(l.window).Sub(now)
	}
	if record {
		recent = append(recent, now)
	}
	l.hits[key] = recent
	return true, 0
}

// sweep forgets keys with nothing inside the window, so the map does
// not grow with every address that ever visited.
func (l *Limiter) sweep(now time.Time) {
	for key, times := range l.hits {
		if len(times) == 0 || now.Sub(times[len(times)-1]) >= l.window {
			delete(l.hits, key)
		}
	}
}

// clientIP is the address the limiter counts. Fly sets Fly-Client-IP
// itself, so a visitor cannot forge it. Behind another proxy the last
// X-Forwarded-For entry is the one that proxy appended. With no proxy
// at all the socket address is the client.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("Fly-Client-IP")); ip != "" {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
