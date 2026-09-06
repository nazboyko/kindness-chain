package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nazboyko/kindness-chain/internal/chain"
	"github.com/nazboyko/kindness-chain/internal/solana"
)

// linkJSON is a link as the page sees it.
type linkJSON struct {
	N           int64  `json:"n"`
	Act         string `json:"act"`
	By          string `json:"by"`
	CreatedAt   string `json:"createdAt"`
	Status      string `json:"status"`
	Signature   string `json:"signature,omitempty"`
	Prev        string `json:"prev,omitempty"`
	ConfirmedAt string `json:"confirmedAt,omitempty"`
	ExplorerURL string `json:"explorerUrl,omitempty"`
}

func toLinkJSON(link chain.Link, cluster string) linkJSON {
	out := linkJSON{
		N:         link.N,
		Act:       link.Act,
		By:        link.By,
		CreatedAt: link.CreatedAt.UTC().Format(time.RFC3339),
		Status:    link.Status,
		Signature: link.Signature,
		Prev:      link.PrevSignature,
	}
	if !link.ConfirmedAt.IsZero() {
		out.ConfirmedAt = link.ConfirmedAt.UTC().Format(time.RFC3339)
	}
	if link.Signature != "" {
		out.ExplorerURL = solana.ExplorerURL(cluster, link.Signature)
	}
	return out
}

// statsJSON is the pledge as the page sees it.
type statsJSON struct {
	Count        int64        `json:"count"`
	Pending      int64        `json:"pending"`
	PledgedCents int          `json:"pledgedCents"`
	CapCents     int          `json:"capCents"`
	PerLinkCents int          `json:"perLinkCents"`
	Charity      charityJSON  `json:"charity"`
	Pledger      string       `json:"pledger"`
	Cluster      string       `json:"cluster"`
	Signer       *addressJSON `json:"signer,omitempty"`
	Head         *headJSON    `json:"head,omitempty"`
	Paused       bool         `json:"paused"`
}

type charityJSON struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type addressJSON struct {
	Address     string `json:"address"`
	ExplorerURL string `json:"explorerUrl"`
}

type headJSON struct {
	N           int64  `json:"n"`
	Signature   string `json:"signature"`
	ExplorerURL string `json:"explorerUrl"`
}

func toStatsJSON(stats chain.Stats, cluster string) statsJSON {
	out := statsJSON{
		Count:        stats.Confirmed,
		Pending:      stats.Pending,
		PledgedCents: stats.PledgedCents,
		CapCents:     stats.CapCents,
		PerLinkCents: stats.PerLinkCents,
		Charity:      charityJSON{Name: stats.CharityName, URL: stats.CharityURL},
		Pledger:      stats.PledgerName,
		Cluster:      cluster,
		Paused:       stats.Paused,
	}
	if stats.SignerAddress != "" {
		out.Signer = &addressJSON{
			Address:     stats.SignerAddress,
			ExplorerURL: solana.ExplorerAddressURL(cluster, stats.SignerAddress),
		}
	}
	if stats.HeadSignature != "" {
		out.Head = &headJSON{
			N:           stats.HeadN,
			Signature:   stats.HeadSignature,
			ExplorerURL: solana.ExplorerURL(cluster, stats.HeadSignature),
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError is the one error shape: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
