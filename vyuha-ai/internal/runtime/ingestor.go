package runtime

import (
	"context"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// LogEvent is a structured runtime log event parsed from JSON log lines.
// It mirrors the shape emitted by instrumented services.
// ---------------------------------------------------------------------------

type LogEvent struct {
	Service      string `json:"service"`
	File         string `json:"file"`
	Function     string `json:"function"`
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id"`
	Status       string `json:"status"`
	EventType    string `json:"event_type"`
	Error        string `json:"error"`
	ErrorCode    string `json:"error_code"`
	LatencyMs    int    `json:"latency_ms"`
	Timestamp    string `json:"timestamp"` // RFC 3339
}

// EventHandler processes a single runtime event. The server supplies an
// implementation that routes through the same pipeline as the HTTP ingest
// endpoints.
type EventHandler func(ctx context.Context, event LogEvent) error

// ---------------------------------------------------------------------------
// Ingestor receives events from one or more sources (HTTP handlers, file
// tailers, etc.) and forwards them through the configured EventHandler.
// ---------------------------------------------------------------------------

type Ingestor struct {
	handler EventHandler
	mu      sync.Mutex
	count   atomic.Int64
}

// NewIngestor creates an Ingestor that delegates to handler for every event.
func NewIngestor(handler EventHandler) *Ingestor {
	return &Ingestor{handler: handler}
}

// Submit processes a single event through the handler and increments the
// event counter. It is safe for concurrent use.
func (ing *Ingestor) Submit(ctx context.Context, event LogEvent) error {
	ing.mu.Lock()
	defer ing.mu.Unlock()

	if err := ing.handler(ctx, event); err != nil {
		return err
	}
	ing.count.Add(1)
	return nil
}

// EventCount returns the total number of events successfully processed.
func (ing *Ingestor) EventCount() int64 {
	return ing.count.Load()
}
