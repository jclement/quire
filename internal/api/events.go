// SSE: one /api/v1/events stream per browser tab. The indexer publishes an
// Event after every applied change; connected tabs use them to invalidate
// query caches. No polling anywhere in the product.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/jclement/quire/internal/index"
)

// Broadcaster fans index events out to connected SSE clients. Slow clients
// drop events rather than blocking the indexer — a dropped invalidation just
// means one stale list until the next interaction.
type Broadcaster struct {
	mu      sync.Mutex
	clients map[chan index.Event]struct{}
}

// NewBroadcaster returns an empty broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{clients: map[chan index.Event]struct{}{}}
}

// Publish sends ev to every connected client without blocking.
func (b *Broadcaster) Publish(ev index.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (b *Broadcaster) subscribe() chan index.Event {
	ch := make(chan index.Event, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broadcaster) unsubscribe(ch chan index.Event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // reverse proxies must not buffer

	ch := s.Events.subscribe()
	defer s.Events.unsubscribe(ch)

	// An initial comment confirms the stream is live before any event fires.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: doc\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
