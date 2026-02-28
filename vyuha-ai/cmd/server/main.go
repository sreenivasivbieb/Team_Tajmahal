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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vyuha/vyuha-ai/internal/ai"
	"github.com/vyuha/vyuha-ai/internal/api"
	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/query"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// initLogger configures the global slog default with JSON output.
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
	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
	}
	h := slog.NewJSONHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(h))
}

// envOrDefault resolves a configuration value with the priority:
//   flag (if explicitly set, i.e. differs from defaultVal) > env var > default.
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
	dbPathFlag := flag.String("db-path", "./vyuha.db", "Path to SQLite database file")
	portFlag := flag.Int("port", 8080, "HTTP server port")
	logLevel := flag.String("log-level", "info", "Log level (debug|info|warn|error)")
	aiProviderFlag := flag.String("ai-provider", "", "AI provider: bedrock or ollama (empty = disabled)")
	aiRegionFlag := flag.String("ai-region", "us-east-1", "AWS region for Bedrock provider")
	aiModelFlag := flag.String("ai-model", "", "LLM model ID (provider-specific)")
	aiEmbedModel := flag.String("ai-embed-model", "", "Embedding model ID (provider-specific)")
	ollamaURLFlag := flag.String("ollama-url", "http://localhost:11434", "Ollama API URL")
	flag.Parse()

	// Resolve config: flag > env var > default.
	dbPath := envOrDefault("VYUHA_DB_PATH", *dbPathFlag, "./vyuha.db")
	portStr := envOrDefault("VYUHA_PORT", strconv.Itoa(*portFlag), "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("invalid port value %q: %v", portStr, err)
	}
	aiProvider := envOrDefault("VYUHA_AI_PROVIDER", *aiProviderFlag, "")
	aiRegion := envOrDefault("VYUHA_AI_REGION", *aiRegionFlag, "us-east-1")
	aiModel := envOrDefault("VYUHA_AI_MODEL", *aiModelFlag, "")
	ollamaURL := envOrDefault("VYUHA_OLLAMA_URL", *ollamaURLFlag, "http://localhost:11434")

	initLogger(*logLevel)

	// ---- Storage ---------------------------------------------------------
	store, err := storage.New(dbPath)
	if err != nil {
		log.Fatalf("failed to initialise storage: %v", err)
	}

	// ---- Graph Index -----------------------------------------------------
	ctx := context.Background()
	index := graph.NewGraphIndex()
	if err := index.LoadFromStorage(ctx, store); err != nil {
		log.Fatalf("failed to load graph index: %v", err)
	}
	nodeCount := index.NodeCount()
	edgeCount := index.EdgeCount()

	// ---- SSE Broadcaster -------------------------------------------------
	sse := api.NewSSEBroadcaster()

	// ---- AI Provider (optional) ------------------------------------------
	var provider ai.Provider
	var embedSvc *ai.EmbeddingService
	var jobQueue *ai.JobQueue

	if aiProvider != "" {
		cfg := ai.ProviderConfig{
			Kind:           ai.ProviderKind(aiProvider),
			Region:         aiRegion,
			Model:          aiModel,
			EmbeddingModel: *aiEmbedModel,
			OllamaURL:      ollamaURL,
		}
		provider, err = ai.NewProvider(ctx, cfg)
		if err != nil {
			slog.Warn("AI provider init failed — AI features disabled", "error", err)
		} else {
			slog.Info("AI provider ready", "provider", provider.Name())

			// Embedding service (best-effort — non-fatal on failure).
			embedSvc, err = ai.NewEmbeddingService(ctx, provider, store, index)
			if err != nil {
				slog.Warn("embedding service init failed", "error", err)
				embedSvc = nil
			}

			// Job queue — created before Server so it can be injected via constructor.
			broadcaster := api.NewAIBroadcaster(sse)
			jobQueue = ai.NewJobQueue(provider, embedSvc, broadcaster, 2)
		}
	}

	// ---- HTTP Server -----------------------------------------------------
	srv := api.NewServer(store, index, sse, jobQueue)

	// ---- Query Layer -----------------------------------------------------
	{
		var broadcaster ai.Broadcaster
		if provider != nil {
			broadcaster = api.NewAIBroadcaster(sse)
		}
		ql := query.NewQueryLayer(index, store, provider, embedSvc, broadcaster)
		srv.SetQueryLayer(ql)
	}

	// ---- Startup banner --------------------------------------------------
	aiStatus := "disabled"
	if provider != nil {
		aiStatus = provider.Name()
	}
	banner := fmt.Sprintf(`
═══════════════════════════════
 VYUHA AI — Code Intelligence
 DB:   %s
 Port: %d
 Nodes loaded: %d
 Edges loaded: %d
 AI:   %s
═══════════════════════════════`, dbPath, port, nodeCount, edgeCount, aiStatus)
	fmt.Println(banner)

	slog.Info("vyuha starting",
		"db_path", dbPath,
		"port", port,
		"nodes", nodeCount,
		"edges", edgeCount,
		"ai_provider", aiStatus,
	)

	srv.RegisterRoutes()

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
		slog.Error("HTTP server shutdown error", "error", err)
	}

	if jobQueue != nil {
		jobQueue.Close()
	}
	if provider != nil {
		provider.Close()
	}

	if err := store.Close(); err != nil {
		slog.Error("storage close error", "error", err)
	}

	slog.Info("VYUHA shutdown complete")
}
