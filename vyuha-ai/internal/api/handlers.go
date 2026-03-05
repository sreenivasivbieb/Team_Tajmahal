// HTTP handlers — thin proxies to contextplus MCP tools
// Each handler decodes the request, calls the bridge, and returns JSON

package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// POST /api/call-chain
// ---------------------------------------------------------------------------

type callChainRequest struct {
	SymbolName string `json:"symbol_name"`
	FilePath   string `json:"file_path,omitempty"`
	MaxDepth   int    `json:"max_depth,omitempty"`
}

func (s *Server) handleCallChain(w http.ResponseWriter, r *http.Request) {
	var req callChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.SymbolName == "" {
		writeError(w, http.StatusBadRequest, "MISSING_SYMBOL", "symbol_name is required")
		return
	}

	// Broadcast progress via SSE
	s.sse.Broadcast(SSEEvent{
		Event: "tool_start",
		Data:  map[string]string{"tool": "get_call_chain", "symbol": req.SymbolName},
	})

	result, err := s.bridge.GetCallChain(req.SymbolName, req.FilePath, req.MaxDepth)
	if err != nil {
		s.sse.Broadcast(SSEEvent{
			Event: "tool_error",
			Data:  map[string]string{"tool": "get_call_chain", "error": err.Error()},
		})
		writeError(w, http.StatusInternalServerError, "TOOL_ERROR", err.Error())
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_done",
		Data:  map[string]string{"tool": "get_call_chain", "symbol": req.SymbolName},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

// ---------------------------------------------------------------------------
// POST /api/search
// ---------------------------------------------------------------------------

type searchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUERY", "query is required")
		return
	}

	text, err := s.bridge.SemanticSearch(req.Query, req.TopK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TOOL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"result": text},
	})
}

// ---------------------------------------------------------------------------
// POST /api/context-tree
// ---------------------------------------------------------------------------

type contextTreeRequest struct {
	TargetPath string `json:"target_path,omitempty"`
}

func (s *Server) handleContextTree(w http.ResponseWriter, r *http.Request) {
	var req contextTreeRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // body is optional

	text, err := s.bridge.GetContextTree(req.TargetPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TOOL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"result": text},
	})
}

// ---------------------------------------------------------------------------
// POST /api/skeleton
// ---------------------------------------------------------------------------

type skeletonRequest struct {
	FilePath string `json:"file_path"`
}

func (s *Server) handleSkeleton(w http.ResponseWriter, r *http.Request) {
	var req skeletonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PATH", "file_path is required")
		return
	}

	text, err := s.bridge.GetFileSkeleton(req.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TOOL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"result": text},
	})
}

// ---------------------------------------------------------------------------
// POST /api/blast-radius
// ---------------------------------------------------------------------------

type blastRadiusRequest struct {
	SymbolName  string `json:"symbol_name"`
	FileContext string `json:"file_context,omitempty"`
}

func (s *Server) handleBlastRadius(w http.ResponseWriter, r *http.Request) {
	var req blastRadiusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.SymbolName == "" {
		writeError(w, http.StatusBadRequest, "MISSING_SYMBOL", "symbol_name is required")
		return
	}

	text, err := s.bridge.GetBlastRadius(req.SymbolName, req.FileContext)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TOOL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"result": text},
	})
}

// ---------------------------------------------------------------------------
// POST /api/identifier-search
// ---------------------------------------------------------------------------

type identifierSearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

func (s *Server) handleIdentifierSearch(w http.ResponseWriter, r *http.Request) {
	var req identifierSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUERY", "query is required")
		return
	}

	text, err := s.bridge.SemanticIdentifierSearch(req.Query, req.TopK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TOOL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"result": text},
	})
}

// ---------------------------------------------------------------------------
// POST /api/static-analysis
// ---------------------------------------------------------------------------

type staticAnalysisRequest struct {
	TargetPath string `json:"target_path,omitempty"`
}

func (s *Server) handleStaticAnalysis(w http.ResponseWriter, r *http.Request) {
	var req staticAnalysisRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	text, err := s.bridge.RunStaticAnalysis(req.TargetPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TOOL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"result": text},
	})
}

// ---------------------------------------------------------------------------
// POST /api/scan-repo — Scan a repository with contextplus context-tree
// to pre-build the semantic tree so it's ready for queries.
// ---------------------------------------------------------------------------

type scanRepoRequest struct {
	RepoPath string `json:"repo_path"`
}

func (s *Server) handleScanRepo(w http.ResponseWriter, r *http.Request) {
	var req scanRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.RepoPath == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PATH", "repo_path is required")
		return
	}

	// Broadcast scan start
	s.sse.Broadcast(SSEEvent{
		Event: "tool_start",
		Data:  map[string]string{"tool": "scan_repo", "path": req.RepoPath},
	})

	// Call context-tree to build the semantic tree for the repo
	tree, err := s.bridge.GetContextTree(req.RepoPath)
	if err != nil {
		s.sse.Broadcast(SSEEvent{
			Event: "tool_error",
			Data:  map[string]string{"tool": "scan_repo", "error": err.Error()},
		})
		writeError(w, http.StatusInternalServerError, "SCAN_ERROR", err.Error())
		return
	}

	// Count approximate symbols (lines that look like identifiers)
	symbolCount := 0
	for _, line := range splitLines(tree) {
		trimmed := trimSpace(line)
		if len(trimmed) > 0 && trimmed[0] != '#' && trimmed[0] != '-' {
			symbolCount++
		}
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_done",
		Data:  map[string]string{"tool": "scan_repo", "path": req.RepoPath},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"tree":         tree,
			"symbol_count": symbolCount,
			"ready":        true,
		},
	})
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// trimSpace trims leading/trailing whitespace from a string.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// ---------------------------------------------------------------------------
// POST /api/clone-repo — Clone a GitHub URL, then scan with contextplus
// ---------------------------------------------------------------------------

// githubURLPattern matches GitHub HTTPS URLs.
var githubURLPattern = regexp.MustCompile(`^https?://github\.com/[\w.\-]+/[\w.\-]+(?:\.git)?/?$`)

type cloneRepoRequest struct {
	GithubURL string `json:"github_url"`
}

func (s *Server) handleCloneRepo(w http.ResponseWriter, r *http.Request) {
	var req cloneRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	url := strings.TrimSpace(req.GithubURL)
	if url == "" {
		writeError(w, http.StatusBadRequest, "MISSING_URL", "github_url is required")
		return
	}
	if !githubURLPattern.MatchString(url) {
		writeError(w, http.StatusBadRequest, "INVALID_URL", "must be a valid GitHub HTTPS URL")
		return
	}

	// Derive repo name from URL: github.com/owner/name → name
	parts := strings.Split(strings.TrimSuffix(strings.TrimSuffix(url, "/"), ".git"), "/")
	repoName := parts[len(parts)-1]

	// Clone into a managed directory
	reposDir := filepath.Join(".", "_vyuha_repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "FS_ERROR", fmt.Sprintf("failed to create repos dir: %v", err))
		return
	}
	destPath, _ := filepath.Abs(filepath.Join(reposDir, repoName))

	// Broadcast progress
	s.sse.Broadcast(SSEEvent{
		Event: "clone_start",
		Data:  map[string]string{"url": url, "name": repoName},
	})

	// If dest already exists, skip cloning
	if _, err := os.Stat(destPath); err == nil {
		slog.Info("repo already cloned, skipping git clone", "path", destPath)
	} else {
		cmd := exec.Command("git", "clone", "--depth", "1", url, destPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			s.sse.Broadcast(SSEEvent{
				Event: "clone_error",
				Data:  map[string]string{"url": url, "error": err.Error()},
			})
			writeError(w, http.StatusInternalServerError, "CLONE_ERROR", fmt.Sprintf("git clone failed: %v", err))
			return
		}
	}

	s.sse.Broadcast(SSEEvent{
		Event: "clone_done",
		Data:  map[string]string{"url": url, "name": repoName, "path": destPath},
	})

	// Now scan the cloned repo with contextplus context-tree
	s.sse.Broadcast(SSEEvent{
		Event: "tool_start",
		Data:  map[string]string{"tool": "scan_repo", "path": destPath},
	})

	tree, err := s.bridge.GetContextTree(destPath)
	if err != nil {
		s.sse.Broadcast(SSEEvent{
			Event: "tool_error",
			Data:  map[string]string{"tool": "scan_repo", "error": err.Error()},
		})
		writeError(w, http.StatusInternalServerError, "SCAN_ERROR", err.Error())
		return
	}

	symbolCount := 0
	for _, line := range splitLines(tree) {
		trimmed := trimSpace(line)
		if len(trimmed) > 0 && trimmed[0] != '#' && trimmed[0] != '-' {
			symbolCount++
		}
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_done",
		Data:  map[string]string{"tool": "scan_repo", "path": destPath},
	})

	// Prime the semantic search index in the background so that
	// later RAG queries against this repo don't hit a cold-start.
	// We fire-and-forget; if it fails the index will be built lazily.
	go func() {
		s.sse.Broadcast(SSEEvent{
			Event: "tool_start",
			Data:  map[string]string{"tool": "prime_index", "path": destPath},
		})
		if err := s.bridge.PrimeSearchIndex(destPath); err != nil {
			slog.Warn("failed to prime search index", "path", destPath, "error", err)
		}
		s.sse.Broadcast(SSEEvent{
			Event: "tool_done",
			Data:  map[string]string{"tool": "prime_index", "path": destPath},
		})
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"name":         repoName,
			"local_path":   destPath,
			"tree":         tree,
			"symbol_count": symbolCount,
			"ready":        true,
		},
	})
}

// ---------------------------------------------------------------------------
// POST /api/rag-query — Agentic RAG: ask a natural-language question
// ---------------------------------------------------------------------------

type ragQueryRequest struct {
	Question string `json:"question"`
	RepoPath string `json:"repo_path,omitempty"`
}

func (s *Server) handleRagQuery(w http.ResponseWriter, r *http.Request) {
	var req ragQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUESTION", "question is required")
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_start",
		Data:  map[string]string{"tool": "ask_question", "question": req.Question},
	})

	answer, err := s.bridge.AskQuestion(req.Question, req.RepoPath)
	if err != nil {
		s.sse.Broadcast(SSEEvent{
			Event: "tool_error",
			Data:  map[string]string{"tool": "ask_question", "error": err.Error()},
		})
		writeError(w, http.StatusInternalServerError, "RAG_ERROR", err.Error())
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_done",
		Data:  map[string]string{"tool": "ask_question"},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"answer": answer,
		},
	})
}
