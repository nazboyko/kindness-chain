package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/nazboyko/kindness-chain/internal/chain"
	"github.com/nazboyko/kindness-chain/internal/solana"
	"github.com/nazboyko/kindness-chain/internal/store"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
	maxBodyBytes    = 4 << 10
)

func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.chain.Stats(r.Context())
	if err != nil {
		a.serverError(w, "stats", err)
		return
	}
	writeJSON(w, http.StatusOK, toStatsJSON(stats, a.cfg.Cluster))
}

func (a *API) handleList(w http.ResponseWriter, r *http.Request) {
	before, err := optionalInt(r.URL.Query().Get("before"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "before must be a link number")
		return
	}
	limit, err := optionalInt(r.URL.Query().Get("limit"), defaultPageSize)
	if err != nil || limit < 1 {
		writeError(w, http.StatusBadRequest, "limit must be a positive number")
		return
	}
	size := min(int(limit), maxPageSize)

	// one extra row tells whether an older page exists
	links, err := a.chain.List(r.Context(), before, size+1)
	if err != nil {
		a.serverError(w, "list links", err)
		return
	}
	hasMore := len(links) > size
	if hasMore {
		links = links[:size]
	}
	out := make([]linkJSON, 0, len(links))
	for _, link := range links {
		out = append(out, toLinkJSON(link, a.cfg.Cluster))
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": out, "hasMore": hasMore})
}

func (a *API) handleGet(w http.ResponseWriter, r *http.Request) {
	link, ok := a.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toLinkJSON(link, a.cfg.Cluster))
}

type submission struct {
	Act     string `json:"act"`
	By      string `json:"by"`
	Website string `json:"website"` // the honeypot: people never see this field
	Seed    string `json:"seed"`    // the proof-of-work challenge, when it is on
	Nonce   string `json:"nonce"`
}

// handleAdd runs the gates in order of how cheap and how personal they
// are: the honeypot, the sentence itself, the proof of work, the
// duplicate check, then the two rate limits. A sentence is refused for
// its own faults before any limit is charged, so a typo never costs a
// visitor one of their links for the hour.
func (a *API) handleAdd(w http.ResponseWriter, r *http.Request) {
	var in submission
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "send JSON with act and an optional by")
		return
	}
	if in.Website != "" {
		a.pretendToAccept(w, r, in)
		return
	}
	if _, _, err := chain.Validate(in.Act, in.By); err != nil {
		a.refuse(w, err)
		return
	}
	if a.challenges != nil {
		if err := a.challenges.Redeem(in.Seed, in.Nonce, in.Act); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":  "This link's seal is missing or expired. Try again.",
				"reason": "challenge",
			})
			return
		}
	}
	if duplicate, err := a.chain.Duplicate(r.Context(), in.Act); err != nil {
		a.serverError(w, "duplicate check", err)
		return
	} else if duplicate {
		writeError(w, http.StatusUnprocessableEntity, chain.MsgDuplicate)
		return
	}

	ip := clientIP(r)
	if ok, wait := a.perIP.Peek(ip); !ok {
		minutes := int(math.Ceil(wait.Minutes()))
		a.tooMany(w, wait, fmt.Sprintf(
			"You've added %d links this hour. Come back in %d minutes. Kindness keeps.",
			a.cfg.RateLimitPerHour, minutes,
		))
		return
	}
	if ok, wait := a.global.Peek("chain"); !ok {
		a.tooMany(w, wait, "The chain is busy right now, try again in a minute")
		return
	}
	a.perIP.Allow(ip)
	a.global.Allow("chain")

	link, err := a.chain.Add(r.Context(), in.Act, in.By)
	if err != nil {
		a.refuse(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, toLinkJSON(link, a.cfg.Cluster))
}

func (a *API) tooMany(w http.ResponseWriter, wait time.Duration, message string) {
	seconds := int(math.Ceil(wait.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":             message,
		"retryAfterSeconds": seconds,
	})
}

// handleChallenge hands out a proof-of-work seed. With proof of work
// off it says so with a difficulty of zero, and browsers send nothing.
func (a *API) handleChallenge(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.challenges == nil {
		writeJSON(w, http.StatusOK, Challenge{Difficulty: 0, Expires: time.Now().Add(challengeTTL)})
		return
	}
	challenge, err := a.challenges.Issue()
	if err != nil {
		a.serverError(w, "issue challenge", err)
		return
	}
	writeJSON(w, http.StatusOK, challenge)
}

// pretendToAccept answers a bot that filled the hidden field the way a
// real submission is answered, and writes nothing.
func (a *API) pretendToAccept(w http.ResponseWriter, r *http.Request, in submission) {
	var next int64 = 1
	if stats, err := a.chain.Stats(r.Context()); err == nil {
		next = stats.Confirmed + stats.Pending + 1
	}
	act, by, _ := chain.Validate(in.Act, in.By)
	writeJSON(w, http.StatusAccepted, toLinkJSON(chain.Link{
		N:         next,
		Act:       act,
		By:        by,
		CreatedAt: time.Now(),
		Status:    store.StatusPending,
	}, a.cfg.Cluster))
}

// refuse maps what the chain said no to onto a status code.
func (a *API) refuse(w http.ResponseWriter, err error) {
	var invalid *chain.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusUnprocessableEntity, invalid.Message)
	case errors.Is(err, chain.ErrBusy):
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		a.serverError(w, "add link", err)
	}
}

type verification struct {
	N           int64       `json:"n"`
	Signature   string      `json:"signature"`
	ExplorerURL string      `json:"explorerUrl"`
	Matches     bool        `json:"matches"`
	OnChain     *chain.Memo `json:"onChain,omitempty"`
	Stored      chain.Memo  `json:"stored"`
	Raw         string      `json:"raw,omitempty"`
	Problem     string      `json:"problem,omitempty"`
}

// handleVerify reads the memo back from the cluster and compares it
// with the stored row, field by field.
func (a *API) handleVerify(w http.ResponseWriter, r *http.Request) {
	link, ok := a.lookup(w, r)
	if !ok {
		return
	}
	if link.Status != store.StatusConfirmed {
		writeError(w, http.StatusConflict, "this link is not on the chain yet")
		return
	}
	stored, err := chain.DecodeMemo([]byte(link.Memo))
	if err != nil {
		a.serverError(w, "stored memo", err)
		return
	}
	out := verification{
		N:           link.N,
		Signature:   link.Signature,
		ExplorerURL: solana.ExplorerURL(a.cfg.Cluster, link.Signature),
		Stored:      stored,
	}

	raw, err := a.ledger.GetMemo(r.Context(), link.Signature)
	switch {
	case errors.Is(err, solana.ErrNotFound):
		out.Problem = "the cluster does not know this transaction; on devnet that happens after a reset"
		writeJSON(w, http.StatusOK, out)
		return
	case err != nil:
		a.serverError(w, "read memo from the chain", err)
		return
	}
	out.Raw = string(raw)
	onChain, err := chain.DecodeMemo(raw)
	if err != nil {
		out.Problem = "the memo on the chain does not parse: " + err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.OnChain = &onChain
	out.Matches = onChain == stored
	if !out.Matches {
		out.Problem = "the memo on the chain differs from the stored link"
	}
	writeJSON(w, http.StatusOK, out)
}

// lookup reads {n} and loads the link, answering the request itself
// when that fails.
func (a *API) lookup(w http.ResponseWriter, r *http.Request) (chain.Link, bool) {
	n, err := strconv.ParseInt(r.PathValue("n"), 10, 64)
	if err != nil || n < 0 {
		writeError(w, http.StatusBadRequest, "link numbers are whole numbers")
		return chain.Link{}, false
	}
	link, err := a.chain.Get(r.Context(), n)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such link")
		return chain.Link{}, false
	}
	if err != nil {
		a.serverError(w, "get link", err)
		return chain.Link{}, false
	}
	return link, true
}

func (a *API) serverError(w http.ResponseWriter, what string, err error) {
	a.log.Printf("%s: %v", what, err)
	writeError(w, http.StatusInternalServerError, "something went wrong on our side")
}

func optionalInt(raw string, fallback int64) (int64, error) {
	if raw == "" {
		return fallback, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
