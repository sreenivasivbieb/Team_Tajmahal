package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/vyuha/vyuha-ai/internal/ai"
	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/query"
	rt "github.com/vyuha/vyuha-ai/internal/runtime"
	"github.com/vyuha/vyuha-ai/internal/storage"
	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

// Server is the HTTP API layer for VYUHA AI.
type Server struct {
	store         *storage.Storage
	index         *graph.GraphIndex
	sse           *SSEBroadcaster
	mux           *http.ServeMux
	server        *http.Server
	queryLayer    *query.QueryLayer
	jobQueue      *ai.JobQueue
	scanJobs      sync.Map // jobID (string) → *scanJobStatus
	ingestLimiter *rate.Limiter

	// File tail watching
	activeTailer *rt.LogTailer
	tailerMu     sync.Mutex
	tailerCancel context.CancelFunc
}

// NewServer creates a new Server wired to the given storage, graph index,
// SSE broadcaster, and (optional) AI job queue.
// Pass nil for jobQueue when no AI provider is configured.
func NewServer(store *storage.Storage, index *graph.GraphIndex, sse *SSEBroadcaster, jobQueue *ai.JobQueue) *Server {
	if sse == nil {
		sse = NewSSEBroadcaster()
	}
	s := &Server{
		store:    store,
		index:    index,
		sse:      sse,
		mux:      http.NewServeMux(),
		jobQueue: jobQueue,
	}

	// Rate limiter for ingest endpoints: 1000 events/sec, burst 5000.
	// This is per-server (not per-IP); sufficient for single-instance deployments.
	s.ingestLimiter = rate.NewLimiter(rate.Limit(1000), 5000)

	return s
}

// RegisterRoutes wires up every API endpoint.
func (s *Server) RegisterRoutes() {
	// -- Scan endpoints ---------------------------------------------------
	s.mux.HandleFunc("POST /api/scan", s.handleScan)
	s.mux.HandleFunc("GET /api/scan/status", s.handleScanStatus)

	// -- Graph endpoints --------------------------------------------------
	s.mux.HandleFunc("GET /api/graph/services", s.handleGraphServices)
	s.mux.HandleFunc("GET /api/graph/children", s.handleGraphChildren)
	s.mux.HandleFunc("GET /api/graph/subgraph", s.handleGraphSubgraph)
	s.mux.HandleFunc("GET /api/graph/node/", s.handleGraphNode)
	s.mux.HandleFunc("GET /api/graph/stats", s.handleGraphStats)
	s.mux.HandleFunc("GET /api/graph/search", s.handleGraphSearch)

	// -- Runtime / ingestion endpoints (rate-limited) --------------------
	s.mux.HandleFunc("POST /api/ingest/log",
		s.withRateLimit(s.ingestLimiter, s.handleIngestLog))
	s.mux.HandleFunc("POST /api/ingest/logs",
		s.withRateLimit(s.ingestLimiter, s.handleIngestLogs))
	s.mux.HandleFunc("POST /api/ingest/watch", s.handleWatchStart)
	s.mux.HandleFunc("DELETE /api/ingest/watch", s.handleWatchStop)
	s.mux.HandleFunc("GET /api/ingest/watch", s.handleWatchStatus)
	s.mux.HandleFunc("GET /api/runtime/failures", s.handleRuntimeFailures)
	s.mux.HandleFunc("GET /api/runtime/trace/", s.handleRuntimeTrace)
	s.mux.HandleFunc("GET /api/runtime/node/", s.handleRuntimeNodeEvents)

	// -- AI / Query endpoints ---------------------------------------------
	s.mux.HandleFunc("POST /api/ai/query", s.handleAIQuery)
	s.mux.HandleFunc("GET /api/ai/jobs/", s.handleAIJobStatus)

	// -- SSE event stream -------------------------------------------------
	s.mux.HandleFunc("GET /api/events", s.handleSSE)

	// -- Health check -----------------------------------------------------
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// -- Static frontend serving ------------------------------------------
	// Serve frontend/dist if it exists relative to the working directory.
	s.serveFrontend()
}

// serveFrontend registers a static file handler for the React frontend.
// It looks for a "frontend/dist" directory relative to the working directory
// or the executable path. If not found, static serving is silently skipped.
func (s *Server) serveFrontend() {
	candidates := []string{"frontend/dist"}

	// Also check relative to the executable (for deployed binaries).
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "frontend", "dist"))
	}

	var distDir string
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			distDir = c
			break
		}
	}

	if distDir == "" {
		slog.Warn("frontend dist not found — SPA not served (use Vite dev server)")
		return
	}

	absDir, _ := filepath.Abs(distDir)
	slog.Info("serving frontend", "dir", absDir)

	distFS := os.DirFS(distDir)
	fileServer := http.FileServerFS(distFS)

	// Serve the SPA: try the file first, fall back to index.html.
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Don't intercept API or health routes (they're already registered
		// with more specific patterns and will match first).
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// If the file exists, serve it directly.
		if f, err := fs.Stat(distFS, path); err == nil && !f.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// Handler returns the fully-wrapped http.Handler (middleware chain + mux).
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = recoveryMiddleware(h)
	h = loggingMiddleware(h)
	h = corsMiddleware(h)
	return h
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx interface{ Deadline() (time.Time, bool); Done() <-chan struct{}; Err() error; Value(any) any }) error {
	// Stop active file tailer first.
	s.stopActiveTailer()

	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "vyuha-ai",
	})
}

// ---------------------------------------------------------------------------
// JSON response helpers
// ---------------------------------------------------------------------------

// writeJSON writes an arbitrary value as JSON with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a standardised JSON error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  code,
	})
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// corsMiddleware allows requests from localhost:5173 (Vite dev server).
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "http://localhost:5173"
		}

		// Allow localhost:5173 and any localhost variant.
		if strings.HasPrefix(origin, "http://localhost:") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseRecorder captures the status code written by downstream handlers.
// It also implements http.Flusher so SSE streaming works through the
// logging middleware.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher by delegating to the underlying writer.
func (rr *responseRecorder) Flush() {
	if f, ok := rr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// loggingMiddleware logs method, path, duration and status code.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rec, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// recoveryMiddleware catches panics and returns a 500 response.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				slog.Error("panic recovered",
					"error", err,
					"stack", string(stack),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error":"internal server error"}`)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// withRateLimit wraps a handler with a token-bucket rate limiter.
// Returns 429 when the limiter is exhausted.
// NOTE: this is a per-server limiter (not per-IP).
func (s *Server) withRateLimit(limiter *rate.Limiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.Header().Set("X-RateLimit-Limit", "1000")
			w.Header().Set("X-RateLimit-Remaining",
				fmt.Sprintf("%d", int(limiter.Tokens())))
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limit exceeded","retry_after_ms":1000}`)
			slog.Warn("rate limit exceeded",
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
			)
			return
		}
		next(w, r)
	}
}
