// Thin HTTP server — routes, middleware, static serving
// All intelligence is delegated to contextplus via the bridge package

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
	"time"

	"github.com/vyuha/vyuha-ai/internal/bridge"
)

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

type Server struct {
	bridge *bridge.MCPClient
	sse    *SSEBroadcaster
	mux    *http.ServeMux
	server *http.Server
}

// NewServer creates a thin HTTP server backed by the contextplus MCP bridge.
func NewServer(mcp *bridge.MCPClient, sse *SSEBroadcaster) *Server {
	if sse == nil {
		sse = NewSSEBroadcaster()
	}
	return &Server{
		bridge: mcp,
		sse:    sse,
		mux:    http.NewServeMux(),
	}
}

// RegisterRoutes wires up every API endpoint.
func (s *Server) RegisterRoutes() {
	// -- Contextplus tool proxies -----------------------------------------
	s.mux.HandleFunc("POST /api/call-chain", s.handleCallChain)
	s.mux.HandleFunc("POST /api/search", s.handleSearch)
	s.mux.HandleFunc("POST /api/context-tree", s.handleContextTree)
	s.mux.HandleFunc("POST /api/skeleton", s.handleSkeleton)
	s.mux.HandleFunc("POST /api/blast-radius", s.handleBlastRadius)
	s.mux.HandleFunc("POST /api/identifier-search", s.handleIdentifierSearch)
	s.mux.HandleFunc("POST /api/static-analysis", s.handleStaticAnalysis)
	s.mux.HandleFunc("POST /api/scan-repo", s.handleScanRepo)
	s.mux.HandleFunc("POST /api/clone-repo", s.handleCloneRepo)
	s.mux.HandleFunc("POST /api/rag-query", s.handleRagQuery)
	s.mux.HandleFunc("POST /api/generate-diagram", s.handleGenerateDiagram)
	s.mux.HandleFunc("POST /api/context-tree-architecture", s.handleContextTreeArchitecture)
	s.mux.HandleFunc("POST /api/edit-diagram", s.handleEditDiagram)

	// -- Deep Research endpoints ------------------------------------------
	s.mux.HandleFunc("POST /api/deep-research/start", s.handleDeepResearchStart)
	s.mux.HandleFunc("GET /api/deep-research/status/{analysisId}", s.handleDeepResearchStatus)
	s.mux.HandleFunc("GET /api/deep-research/report/{analysisId}", s.handleDeepResearchReport)
	s.mux.HandleFunc("POST /api/deep-research/generate-diagrams", s.handleDeepResearchDiagrams)

	// -- SSE event stream -------------------------------------------------
	s.mux.HandleFunc("GET /api/events", s.handleSSE)

	// -- Health -----------------------------------------------------------
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// -- Frontend static files (if built) ---------------------------------
	tryServeFrontend(s.mux)
}

// ListenAndServe starts the HTTP server with middleware.
func (s *Server) ListenAndServe(addr string) error {
	handler := corsMiddleware(
		loggingMiddleware(
			recoveryMiddleware(s.mux),
		),
	)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	slog.Info("HTTP server listening", "addr", addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// allowedOrigins returns the set of allowed CORS origins.
// Reads CORS_ORIGINS env var (comma-separated); falls back to permissive "*".
func allowedOrigins() map[string]bool {
	raw := os.Getenv("CORS_ORIGINS")
	if raw == "" {
		return nil // nil means allow all
	}
	m := make(map[string]bool)
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			m[o] = true
		}
	}
	return m
}

func corsMiddleware(next http.Handler) http.Handler {
	origins := allowedOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origins == nil {
			// No allowlist configured — reflect origin (dev mode)
			if origin == "" {
				origin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else if origins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			// Origin not allowed — still serve the request but without CORS header
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "err", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Static frontend serving
// ---------------------------------------------------------------------------

func tryServeFrontend(mux *http.ServeMux) {
	// Look for frontend/dist directory
	candidates := []string{
		filepath.Join("frontend", "dist"),
		filepath.Join("..", "frontend", "dist"),
	}
	for _, dir := range candidates {
		abs, _ := filepath.Abs(dir)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			slog.Info("serving frontend", "dir", abs)
			fsys := os.DirFS(abs)
			fileServer := http.FileServer(http.FS(fsys))

			mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
				// For SPA: serve index.html for any path that doesn't match a real file
				path := strings.TrimPrefix(r.URL.Path, "/")
				if path == "" {
					path = "index.html"
				}
				if _, err := fs.Stat(fsys, path); err != nil {
					// File not found — serve index.html for SPA routing
					r.URL.Path = "/"
				}
				fileServer.ServeHTTP(w, r)
			})
			return
		}
	}
	slog.Warn("no frontend/dist found, skipping static serving")
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "VYUHA AI API — frontend not built")
	})
}
