package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	l := newLimiter(3, time.Hour)
	l.now = func() time.Time { return now }

	for i := range 3 {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("action %d was refused", i+1)
		}
	}
	ok, wait := l.allow("a")
	if ok || wait != time.Hour {
		t.Fatalf("fourth action: ok=%v wait=%v, want refused for 1h", ok, wait)
	}
	if ok, _ := l.allow("b"); !ok {
		t.Fatal("another key was refused")
	}

	now = now.Add(30 * time.Minute)
	if ok, wait := l.allow("a"); ok || wait != 30*time.Minute {
		t.Fatalf("after 30m: ok=%v wait=%v", ok, wait)
	}
	now = now.Add(31 * time.Minute)
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("still refused after the window passed")
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{"fly header wins", map[string]string{"Fly-Client-IP": "203.0.113.9", "X-Forwarded-For": "10.0.0.1, 198.51.100.7"}, "127.0.0.1:1234", "203.0.113.9"},
		{"last forwarded entry", map[string]string{"X-Forwarded-For": "10.0.0.1, 198.51.100.7"}, "127.0.0.1:1234", "198.51.100.7"},
		{"socket address", nil, "192.0.2.4:5555", "192.0.2.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := clientIP(r); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
