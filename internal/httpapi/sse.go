package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/nazboyko/kindness-chain/internal/chain"
)

const (
	maxStreams      = 2000
	streamBuffer    = 16
	heartbeatPeriod = 25 * time.Second
)

// Hub fans events out to every open event stream. It is the chain's
// Broadcaster: the worker calls it, the browsers hear it.
type Hub struct {
	cluster string
	mu      sync.Mutex
	streams map[chan []byte]struct{}
	closing chan struct{}
	once    sync.Once
}

// NewHub returns an empty hub.
func NewHub(cluster string) *Hub {
	return &Hub{
		cluster: cluster,
		streams: map[chan []byte]struct{}{},
		closing: make(chan struct{}),
	}
}

// LinkChanged pushes a link whose status changed.
func (h *Hub) LinkChanged(link chain.Link) {
	h.broadcast(frame("link", toLinkJSON(link, h.cluster)))
}

// StatsChanged pushes fresh stats.
func (h *Hub) StatsChanged(stats chain.Stats) {
	h.broadcast(frame("stats", toStatsJSON(stats, h.cluster)))
}

// Close ends every open stream. The server calls it on shutdown so it
// does not wait on connections that would otherwise never end.
func (h *Hub) Close() {
	h.once.Do(func() { close(h.closing) })
}

// open is how many browsers are listening.
func (h *Hub) open() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.streams)
}

func (h *Hub) broadcast(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for stream := range h.streams {
		select {
		case stream <- msg:
		default:
			// a browser that cannot keep up misses this one; the page
			// refetches on reconnect anyway
		}
	}
}

func (h *Hub) subscribe() (chan []byte, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.streams) >= maxStreams {
		return nil, nil, false
	}
	stream := make(chan []byte, streamBuffer)
	h.streams[stream] = struct{}{}
	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.streams, stream)
	}
	return stream, unsubscribe, true
}

// frame renders one server-sent event.
func frame(event string, payload any) []byte {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("null")
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

// handleEvents keeps one response open and writes events into it as
// they happen, with a comment every so often so proxies keep the
// connection alive.
func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	stream, unsubscribe, ok := a.hub.subscribe()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "too many open streams, try again in a minute")
		return
	}
	defer unsubscribe()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	// every stream opens with the current numbers, so a reconnect after
	// missed events is right again at once
	w.Write([]byte("retry: 3000\n\n"))
	if stats, err := a.chain.Stats(r.Context()); err == nil {
		w.Write(frame("stats", toStatsJSON(stats, a.cfg.Cluster)))
	}
	if err := rc.Flush(); err != nil {
		return
	}

	heartbeat := time.NewTicker(heartbeatPeriod)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-a.hub.closing:
			return
		case msg := <-stream:
			if _, err := w.Write(msg); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
