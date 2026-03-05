// VYUHA AI — Thin HTTP layer backed by contextplus MCP server
// All code intelligence (parsing, call chains, search, RAG) is delegated to contextplus

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vyuha/vyuha-ai/internal/api"
	"github.com/vyuha/vyuha-ai/internal/bridge"
)

func initLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, opts)))
}

func envOrDefault(envKey, flagVal, defaultVal string) string {
	if flagVal != defaultVal {
		return flagVal
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return defaultVal
}

func main() {
	// ---- Flags -----------------------------------------------------------
	portFlag := flag.Int("port", 8080, "HTTP server port")
	logLevel := flag.String("log-level", "info", "Log level (debug|info|warn|error)")
	contextplusPath := flag.String("contextplus", "", "Path to contextplus index.js (default: auto-detect)")
	rootDirFlag := flag.String("root", ".", "Codebase root directory for contextplus")
	flag.Parse()

	// Resolve config via flag > env > default
	portStr := envOrDefault("VYUHA_PORT", strconv.Itoa(*portFlag), "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("invalid port %q: %v", portStr, err)
	}
	rootDir := envOrDefault("VYUHA_ROOT_DIR", *rootDirFlag, ".")
	absRoot, _ := filepath.Abs(rootDir)

	initLogger(*logLevel)

	// ---- Resolve contextplus path ----------------------------------------
	cpPath := envOrDefault("CONTEXTPLUS_PATH", *contextplusPath, "")
	if cpPath == "" {
		// Auto-detect: look for contextplus relative to this binary
		candidates := []string{
			filepath.Join("..", "contextplus", "build", "index.js"),
			filepath.Join("contextplus", "build", "index.js"),
		}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates,
				filepath.Join(filepath.Dir(exe), "..", "contextplus", "build", "index.js"),
			)
		}
		for _, c := range candidates {
			abs, _ := filepath.Abs(c)
			if _, err := os.Stat(abs); err == nil {
				cpPath = abs
				break
			}
		}
	}
	if cpPath == "" {
		log.Fatal("contextplus not found — set --contextplus or CONTEXTPLUS_PATH")
	}

	slog.Info("resolved contextplus", "path", cpPath)

	// ---- Spawn contextplus MCP bridge ------------------------------------
	mcp, err := bridge.NewMCPClient(cpPath, absRoot)
	if err != nil {
		log.Fatalf("failed to start contextplus bridge: %v", err)
	}

	// ---- SSE Broadcaster -------------------------------------------------
	sse := api.NewSSEBroadcaster()

	// ---- HTTP Server -----------------------------------------------------
	srv := api.NewServer(mcp, sse)
	srv.RegisterRoutes()

	// ---- Startup banner --------------------------------------------------
	banner := fmt.Sprintf(`
═══════════════════════════════════════════
 VYUHA AI — Code Intelligence (bridge mode)
 Root:        %s
 Port:        %d
 Contextplus: %s
═══════════════════════════════════════════`, absRoot, port, cpPath)
	fmt.Println(banner)

	addr := fmt.Sprintf(":%d", port)
	go func() {
		slog.Info("HTTP server listening", "addr", addr)
		if err := srv.ListenAndServe(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// ---- Graceful shutdown -----------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutdown signal received", "signal", sig.String())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	slog.Info("VYUHA shutdown complete")
}
