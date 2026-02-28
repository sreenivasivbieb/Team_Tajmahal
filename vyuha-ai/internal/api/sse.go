package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// SSE Types
// ---------------------------------------------------------------------------

// SSEEvent is a single server-sent event.
type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// ---------------------------------------------------------------------------
// SSEBroadcaster
// ---------------------------------------------------------------------------

// SSEBroadcaster fans out SSE events to all connected HTTP clients.
// Each client is identified by a unique string ID and receives events
// through a buffered channel.
type SSEBroadcaster struct {
	mu      sync.RWMutex
	clients map[string]chan SSEEvent
}

// NewSSEBroadcaster creates a ready-to-use broadcaster.
func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{
		clients: make(map[string]chan SSEEvent),
	}
}

// Subscribe registers a new client and returns its event channel.
// The channel is buffered (64) so slow consumers don't block the
// broadcaster.
func (b *SSEBroadcaster) Subscribe(clientID string) chan SSEEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan SSEEvent, 64)
	b.clients[clientID] = ch
	log.Printf("sse: client %s subscribed (%d total)", clientID, len(b.clients))
	return ch
}

// Unsubscribe removes a client and closes its channel.
func (b *SSEBroadcaster) Unsubscribe(clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.clients[clientID]; ok {
		close(ch)
		delete(b.clients, clientID)
		log.Printf("sse: client %s unsubscribed (%d remaining)", clientID, len(b.clients))
	}
}

// Broadcast sends an event to every connected client. If a client's channel
// is full the event is dropped for that client (non-blocking send).
func (b *SSEBroadcaster) Broadcast(event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, ch := range b.clients {
		select {
		case ch <- event:
		default:
			log.Printf("sse: dropping event %q for slow client %s", event.Event, id)
		}
	}
}

// BroadcastToClient sends an event to a single client.
func (b *SSEBroadcaster) BroadcastToClient(clientID string, event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if ch, ok := b.clients[clientID]; ok {
		select {
		case ch <- event:
		default:
			log.Printf("sse: dropping targeted event %q for slow client %s", event.Event, clientID)
		}
	}
}

// ClientCount returns the number of connected clients.
func (b *SSEBroadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

// ---------------------------------------------------------------------------
// HTTP handler — GET /api/events
// ---------------------------------------------------------------------------

// handleSSE is the HTTP handler for the Server-Sent Events stream endpoint.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Verify that the ResponseWriter supports flushing.
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE_NOT_SUPPORTED",
			"streaming unsupported")
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	clientID := uuid.New().String()
	ch := s.sse.Subscribe(clientID)
	defer s.sse.Unsubscribe(clientID)

	// Heartbeat ticker.
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return

		case evt, ok := <-ch:
			if !ok {
				return // channel closed
			}
			if err := writeSSEEvent(w, flusher, evt); err != nil {
				return
			}

		case t := <-heartbeat.C:
			hb := SSEEvent{
				Event: "heartbeat",
				Data:  map[string]int64{"t": t.Unix()},
			}
			if err := writeSSEEvent(w, flusher, hb); err != nil {
				return
			}
		}
	}
}

// writeSSEEvent formats and writes a single SSE frame.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, evt SSEEvent) error {
	data, err := json.Marshal(evt.Data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, data)
	if err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
