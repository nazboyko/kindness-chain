package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nazboyko/kindness-chain/internal/chain"
	"github.com/nazboyko/kindness-chain/internal/solana"
	"github.com/nazboyko/kindness-chain/internal/store"
)

type fixture struct {
	api     *API
	service *chain.Service
	ledger  *solana.Fake
	hub     *Hub
	handler http.Handler
}

func setup(t *testing.T, limit int) *fixture {
	t.Helper()
	return setupWith(t, Config{Cluster: "devnet", RateLimitPerHour: limit, GlobalPerMinute: 100})
}

func setupWith(t *testing.T, cfg Config) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ledger := solana.NewFake()
	hub := NewHub("devnet")
	service := chain.New(chain.Config{
		PerLinkCents: 10,
		CapCents:     5000,
		CharityName:  "International Institute of Minnesota",
		CharityURL:   "https://iimn.org",
		PledgerName:  "Nazar",
	}, st, ledger, hub)
	dist := fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}}
	api := New(cfg, service, ledger, hub, dist)
	api.log = log.New(io.Discard, "", 0)
	return &fixture{api: api, service: service, ledger: ledger, hub: hub, handler: api.Handler()}
}

// runWorker confirms links in the background until the test ends.
func (f *fixture) runWorker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if err := f.service.Start(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		f.service.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func (f *fixture) do(t *testing.T, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "192.0.2.1:4000"
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	var decoded map[string]any
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s: bad JSON %q", method, path, rec.Body.String())
		}
	}
	return rec, decoded
}

func TestAddLink(t *testing.T) {
	f := setup(t, 3)
	rec, body := f.do(t, "POST", "/api/links", `{"act":"I carried groceries for a neighbour","by":"  Olena "}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if body["n"] != float64(1) || body["status"] != "pending" || body["by"] != "Olena" {
		t.Errorf("body = %v", body)
	}
	if _, has := body["signature"]; has {
		t.Error("a pending link has no signature")
	}
}

func TestAddRefusals(t *testing.T) {
	f := setup(t, 3)
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{"too short", `{"act":"Hi"}`, 422, chain.MsgTooShort},
		{"link inside", `{"act":"Give at https://example.com today"}`, 422, chain.MsgNoLinks},
		{"malformed json", `{"act":`, 400, "send JSON"},
		{"oversized body", `{"act":"` + strings.Repeat("a", maxBodyBytes) + `"}`, 400, "send JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, body := f.do(t, "POST", "/api/links", tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if msg, _ := body["error"].(string); !strings.Contains(msg, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", msg, tc.wantError)
			}
		})
	}
}

func TestHoneypotWritesNothing(t *testing.T) {
	f := setup(t, 3)
	rec, body := f.do(t, "POST", "/api/links", `{"act":"I carried groceries for a neighbour","website":"http://spam.example"}`)
	if rec.Code != http.StatusAccepted || body["status"] != "pending" {
		t.Fatalf("bot got status %d body %v", rec.Code, body)
	}
	_, list := f.do(t, "GET", "/api/links", "")
	if links := list["links"].([]any); len(links) != 0 {
		t.Errorf("the honeypot submission was stored: %v", links)
	}
}

func TestRateLimit(t *testing.T) {
	f := setup(t, 2)
	for i := range 2 {
		if rec, _ := f.do(t, "POST", "/api/links", `{"act":"I carried groceries for a neighbour number `+strconv.Itoa(i)+`"}`); rec.Code != 202 {
			t.Fatalf("link %d: status %d", i+1, rec.Code)
		}
	}
	rec, body := f.do(t, "POST", "/api/links", `{"act":"I carried groceries for a neighbour again"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third link: status %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After header")
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "You've added 2 links this hour. Come back in 60 minutes. Kindness keeps.") {
		t.Errorf("message = %q", msg)
	}
	// a bad sentence is refused on its own merits even when over the limit
	if rec, _ := f.do(t, "POST", "/api/links", `{"act":"Hi"}`); rec.Code != 422 {
		t.Errorf("invalid sentence over the limit: status %d, want 422", rec.Code)
	}
}

func TestListPagination(t *testing.T) {
	f := setup(t, 10)
	for _, act := range []string{"first kind sentence here", "second kind sentence here", "third kind sentence here"} {
		if _, err := f.service.Add(context.Background(), act, ""); err != nil {
			t.Fatal(err)
		}
	}
	numbers := func(body map[string]any) []float64 {
		var ns []float64
		for _, l := range body["links"].([]any) {
			ns = append(ns, l.(map[string]any)["n"].(float64))
		}
		return ns
	}

	_, page := f.do(t, "GET", "/api/links?limit=2", "")
	if ns := numbers(page); len(ns) != 2 || ns[0] != 3 || ns[1] != 2 || page["hasMore"] != true {
		t.Errorf("first page = %v hasMore=%v", ns, page["hasMore"])
	}
	_, older := f.do(t, "GET", "/api/links?limit=2&before=2", "")
	if ns := numbers(older); len(ns) != 1 || ns[0] != 1 || older["hasMore"] != false {
		t.Errorf("older page = %v hasMore=%v", ns, older["hasMore"])
	}
	if rec, _ := f.do(t, "GET", "/api/links?limit=abc", ""); rec.Code != 400 {
		t.Errorf("bad limit: status %d", rec.Code)
	}
}

func TestGetLink(t *testing.T) {
	f := setup(t, 10)
	if _, err := f.service.Add(context.Background(), "I carried groceries for a neighbour", "Olena"); err != nil {
		t.Fatal(err)
	}
	if rec, body := f.do(t, "GET", "/api/links/1", ""); rec.Code != 200 || body["by"] != "Olena" {
		t.Errorf("get: %d %v", rec.Code, body)
	}
	if rec, _ := f.do(t, "GET", "/api/links/99", ""); rec.Code != 404 {
		t.Errorf("missing: %d", rec.Code)
	}
	if rec, _ := f.do(t, "GET", "/api/links/abc", ""); rec.Code != 400 {
		t.Errorf("junk: %d", rec.Code)
	}
	if rec, body := f.do(t, "GET", "/api/nope", ""); rec.Code != 404 || body["error"] == nil {
		t.Errorf("unknown api path: %d %v", rec.Code, body)
	}
}

func TestStats(t *testing.T) {
	f := setup(t, 10)
	rec, body := f.do(t, "GET", "/api/stats", "")
	if rec.Code != 200 || body["count"] != float64(0) || body["capCents"] != float64(5000) || body["cluster"] != "devnet" {
		t.Errorf("stats: %d %v", rec.Code, body)
	}
	if body["charity"].(map[string]any)["name"] != "International Institute of Minnesota" {
		t.Errorf("charity = %v", body["charity"])
	}
	if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security headers missing")
	}
}

func TestVerify(t *testing.T) {
	f := setup(t, 10)
	stream, unsubscribe, _ := f.hub.subscribe()
	defer unsubscribe()
	f.runWorker(t)

	if _, err := f.service.Add(context.Background(), "I carried groceries for a neighbour", "Olena"); err != nil {
		t.Fatal(err)
	}
	// wait for link 1 to confirm
	deadline := time.After(5 * time.Second)
	for confirmed := false; !confirmed; {
		select {
		case msg := <-stream:
			confirmed = strings.Contains(string(msg), `"n":1,`) && strings.Contains(string(msg), `"status":"confirmed"`)
		case <-deadline:
			t.Fatal("link 1 never confirmed")
		}
	}

	rec, body := f.do(t, "GET", "/api/verify/1", "")
	if rec.Code != 200 || body["matches"] != true {
		t.Fatalf("verify: %d %v", rec.Code, body)
	}
	onChain := body["onChain"].(map[string]any)
	if onChain["n"] != float64(1) || onChain["by"] != "Olena" || onChain["prev"] == "" {
		t.Errorf("onChain = %v", onChain)
	}
	if rec, _ := f.do(t, "GET", "/api/verify/0", ""); rec.Code != 200 {
		t.Errorf("verify genesis: %d", rec.Code)
	}
}

func TestVerifyPendingLink(t *testing.T) {
	f := setup(t, 10)
	if _, err := f.service.Add(context.Background(), "I carried groceries for a neighbour", ""); err != nil {
		t.Fatal(err)
	}
	if rec, _ := f.do(t, "GET", "/api/verify/1", ""); rec.Code != http.StatusConflict {
		t.Errorf("verify pending: %d", rec.Code)
	}
}

func TestEventStream(t *testing.T) {
	f := setup(t, 10)
	server := httptest.NewServer(f.handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	readEvent := func() (string, string) {
		var event, data string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("stream ended: %v", err)
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "" && event != "":
				return event, data
			}
		}
	}

	if event, data := readEvent(); event != "stats" || !strings.Contains(data, `"capCents":5000`) {
		t.Fatalf("first event = %s %s", event, data)
	}
	for f.hub.Streams() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	f.hub.LinkChanged(chain.Link{N: 5, Act: "hello there world", Status: store.StatusConfirmed, Signature: "sig5", CreatedAt: time.Now()})
	if event, data := readEvent(); event != "link" || !strings.Contains(data, `"n":5`) || !strings.Contains(data, "explorer.solana.com/tx/sig5?cluster=devnet") {
		t.Fatalf("second event = %s %s", event, data)
	}
}

func TestProofOfWork(t *testing.T) {
	f := setupWith(t, Config{Cluster: "devnet", RateLimitPerHour: 10, GlobalPerMinute: 100, PowBits: 8})
	act := "I carried groceries for a neighbour"

	rec, body := f.do(t, "POST", "/api/links", `{"act":"`+act+`"}`)
	if rec.Code != 422 || body["reason"] != "challenge" {
		t.Fatalf("without a seal: %d %v", rec.Code, body)
	}

	rec, challenge := f.do(t, "GET", "/api/challenge", "")
	if rec.Code != 200 || challenge["difficulty"] != float64(8) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("challenge: %d %v", rec.Code, challenge)
	}
	seed := challenge["seed"].(string)
	nonce := solve(seed, act, 8)

	rec, body = f.do(t, "POST", "/api/links", `{"act":"`+act+`","seed":"`+seed+`","nonce":"`+nonce+`"}`)
	if rec.Code != 202 {
		t.Fatalf("with a seal: %d %v", rec.Code, body)
	}
	rec, body = f.do(t, "POST", "/api/links", `{"act":"another kind sentence here","seed":"`+seed+`","nonce":"`+nonce+`"}`)
	if rec.Code != 422 || body["reason"] != "challenge" {
		t.Fatalf("reused seal: %d %v", rec.Code, body)
	}
}

func TestProofOfWorkOff(t *testing.T) {
	f := setup(t, 10)
	rec, body := f.do(t, "GET", "/api/challenge", "")
	if rec.Code != 200 || body["difficulty"] != float64(0) {
		t.Fatalf("challenge with proof of work off: %d %v", rec.Code, body)
	}
}

func TestDuplicateSentence(t *testing.T) {
	f := setup(t, 10)
	if rec, _ := f.do(t, "POST", "/api/links", `{"act":"I called my grandmother today."}`); rec.Code != 202 {
		t.Fatalf("first: %d", rec.Code)
	}
	rec, body := f.do(t, "POST", "/api/links", `{"act":"i called my grandmother, today!"}`)
	if rec.Code != 422 || body["error"] != chain.MsgDuplicate {
		t.Fatalf("duplicate: %d %v", rec.Code, body)
	}
	// a refused duplicate must not have charged the hourly limit
	for i := range 9 {
		if rec, _ := f.do(t, "POST", "/api/links", `{"act":"a different kind sentence number `+strconv.Itoa(i)+`"}`); rec.Code != 202 {
			t.Fatalf("sentence %d after a duplicate: %d", i, rec.Code)
		}
	}
}

func TestGlobalThrottle(t *testing.T) {
	f := setupWith(t, Config{Cluster: "devnet", RateLimitPerHour: 100, GlobalPerMinute: 2})
	for i := range 2 {
		if rec, _ := f.do(t, "POST", "/api/links", `{"act":"a kind sentence number `+strconv.Itoa(i)+`"}`); rec.Code != 202 {
			t.Fatalf("link %d: %d", i, rec.Code)
		}
	}
	rec, body := f.do(t, "POST", "/api/links", `{"act":"one kind sentence too many"}`)
	if rec.Code != 429 || body["error"] != "The chain is busy right now, try again in a minute" || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over the global limit: %d %v", rec.Code, body)
	}
}
