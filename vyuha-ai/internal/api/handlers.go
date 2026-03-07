// HTTP handlers — thin proxies to contextplus MCP tools
// Each handler decodes the request, calls the bridge, and returns JSON

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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

// ---------------------------------------------------------------------------
// POST /api/generate-diagram — Generate architecture diagram via Groq LLM
// ---------------------------------------------------------------------------

type generateDiagramRequest struct {
	Prompt   string `json:"prompt"`
	RepoPath string `json:"repo_path,omitempty"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqChatRequest struct {
	Model          string        `json:"model"`
	Messages       []groqMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens"`
	ResponseFormat *groqFormat   `json:"response_format,omitempty"`
}

type groqFormat struct {
	Type string `json:"type"`
}

type groqChoice struct {
	Message groqMessage `json:"message"`
}

type groqChatResponse struct {
	Choices []groqChoice `json:"choices"`
}

// callGroq sends a request to the Groq API with automatic retry on transient
// failures (429 rate-limit, 503/502 server errors). Returns the raw response body.
func callGroq(ctx context.Context, groqKey string, groqReq groqChatRequest) ([]byte, error) {
	body, err := json.Marshal(groqReq)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 2 * time.Second
			slog.Info("retrying Groq API call", "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("request creation error: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+groqKey)

		client := &http.Client{Timeout: 90 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("HTTP error: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read error: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return respBody, nil
		}

		// Retry on transient errors
		if resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 {
			slog.Warn("Groq transient error, will retry", "status", resp.StatusCode, "body", string(respBody))
			lastErr = fmt.Errorf("Groq API returned %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		// Non-retryable error
		return nil, fmt.Errorf("Groq API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil, fmt.Errorf("Groq API failed after %d retries: %w", maxRetries, lastErr)
}

const diagramSystemPrompt = `You are an expert architecture diagram generator. Given a description (and optionally codebase context), generate a DETAILED and COMPREHENSIVE structured architecture diagram specification as a JSON object.

Return ONLY a valid JSON object with this exact structure:
{
  "title": "Diagram Title",
  "groups": [
    {
      "id": "unique_group_id",
      "label": "GROUP LABEL",
      "color": "#hex_background_color",
      "borderColor": "#hex_border_color"
    }
  ],
  "nodes": [
    {
      "id": "unique_node_id",
      "label": "Node Label",
      "icon": "iconify_icon_name",
      "group": "group_id_or_null"
    }
  ],
  "edges": [
    {
      "source": "source_node_id",
      "target": "target_node_id",
      "label": "optional_edge_label",
      "style": "solid or dashed",
      "animated": false
    }
  ]
}

ICON REFERENCE — use these Iconify icon names:
AWS: logos:aws-lambda, logos:aws-s3, logos:aws-dynamodb, logos:aws-api-gateway, logos:aws-cloudwatch, logos:aws-cognito, logos:aws-ec2, logos:aws-rds, logos:aws-sqs, logos:aws-sns, logos:aws-ecs, logos:aws-cloudfront, logos:aws-route53, logos:aws-elasticache
Azure: logos:azure-icon, logos:microsoft-azure
GCP: logos:google-cloud
Databases: mdi:database, simple-icons:postgresql, simple-icons:mongodb, simple-icons:redis, simple-icons:mysql
General: mdi:account-group (users), mdi:cellphone (mobile), mdi:web (web app), mdi:api (API), mdi:server (server), mdi:cloud (cloud), mdi:shield-lock (security/auth), mdi:monitor-eye (monitoring), mdi:cached (cache), mdi:tray-full (queue), mdi:robot (AI/ML), mdi:scale-balance (load balancer), mdi:earth (CDN), mdi:folder-multiple (storage), mdi:lightning-bolt (event/trigger), mdi:cog (config), mdi:lock (encryption), mdi:email (email), mdi:chart-line (analytics), mdi:magnify-scan (tracing/x-ray)
Tech: logos:docker-icon, logos:kubernetes, logos:react, logos:nodejs-icon, logos:python, logos:go, logos:java, logos:nginx, logos:graphql, logos:kafka, logos:rabbitmq, logos:elasticsearch

GROUP COLORS (translucent for dark themes):
- Monitoring/Ops: color "#78350F", borderColor "#D97706"
- AI/ML Services: color "#1E1B4B", borderColor "#6366F1"
- Storage/Data: color "#172554", borderColor "#3B82F6"
- Security: color "#052E16", borderColor "#22C55E"
- Compute/Processing: color "#431407", borderColor "#EA580C"
- Networking: color "#083344", borderColor "#06B6D4"
- General: color "#1C1917", borderColor "#78716C"

DETAIL LEVEL REQUIREMENTS — THIS IS CRITICAL:
- Generate between 12 and 30 nodes depending on architecture complexity.
- Focus on the MAIN services, modules, and data stores. Do NOT create separate nodes for controllers, repositories, models, and validators within a single service — represent each service as ONE node.
- Group related sub-components into a single node with a clear label (e.g. "User Service" instead of separate User Controller + User Repository + User Model).
- For each data flow edge, add a SHORT label (2-4 words) explaining what flows (e.g. "JWT token", "user data", "events").
- Create 3-6 groups to logically organize the architecture layers.
- If codebase context is provided, identify the KEY modules and services — skip utility files, configs, and boilerplate.
- Use clear, specific labels. "Auth Service" is better than "Middleware". "PostgreSQL" is better than "Database".

NODE NAMING RULES — CRITICAL:
- NEVER use raw file names, file paths, or file extensions as node labels. For example, do NOT use "server.go", "handler.ts", "docker-compose.yml", "Dockerfile", "config.yaml".
- Instead, derive a descriptive COMPONENT NAME from the file's purpose. Examples: "server.go" → "HTTP Server", "handler.ts" → "Request Handler", "docker-compose.yml" → "Container Orchestration", "Dockerfile" → "Container Build", "config.yaml" → "App Configuration", "schema.sql" → "Database Schema", "main.py" → "Application Entry Point".
- Node labels should describe WHAT THE COMPONENT DOES, not what the file is called.
- Group labels should be descriptive categories like "API Layer", "Data Storage", "Authentication", not file directory names.

CONNECTIVITY RULE — CRITICAL:
- EVERY node must be connected to at least one other node via an edge. There must be NO isolated/disconnected nodes.
- If a node logically connects to a flow, add an edge. If a node provides a cross-cutting concern (logging, config, monitoring), connect it to the services that use it.
- After generating all nodes and edges, double-check that every node ID appears in at least one edge (as source or target).

IMPORTANT:
- Ensure all node IDs referenced in edges exist in the nodes array
- Nodes without a group should have group set to null
- Return ONLY the JSON, no markdown, no code fences, no explanation`

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (s *Server) handleGenerateDiagram(w http.ResponseWriter, r *http.Request) {
	var req generateDiagramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PROMPT", "prompt is required")
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_start",
		Data:  map[string]string{"tool": "generate_diagram", "prompt": req.Prompt},
	})

	// Build user prompt, optionally enriched with codebase context
	userPrompt := req.Prompt
	if req.RepoPath != "" {
		if treeText, err := s.bridge.GetContextTree(req.RepoPath); err == nil && treeText != "" {
			// Truncate context to avoid overwhelming the LLM
			if len(treeText) > 5000 {
				treeText = treeText[:5000] + "\n... (truncated)"
			}
			userPrompt = fmt.Sprintf("Based on this codebase structure:\n\n%s\n\nUser request: %s\n\nIMPORTANT: Focus on the most important architectural components. Combine related sub-components into single nodes. Keep the diagram clean and readable with 12-30 nodes.", treeText, req.Prompt)
		}
	}

	llmResp, err := callLLM(r.Context(), llmRequest{
		SystemPrompt: diagramSystemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.7,
		MaxTokens:    4096,
		JSONMode:     true,
	})
	if err != nil {
		s.sse.Broadcast(SSEEvent{Event: "tool_error", Data: map[string]string{"tool": "generate_diagram", "error": err.Error()}})
		writeError(w, http.StatusBadGateway, "LLM_API_ERROR", err.Error())
		return
	}

	slog.Info("generate_diagram completed", "provider", llmResp.Provider)

	// Extract and clean JSON content
	content := strings.TrimSpace(llmResp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var diagram map[string]interface{}
	if err := json.Unmarshal([]byte(content), &diagram); err != nil {
		writeError(w, http.StatusInternalServerError, "INVALID_DIAGRAM", "AI returned invalid diagram JSON: "+err.Error())
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_done",
		Data:  map[string]string{"tool": "generate_diagram"},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": diagram,
	})
}

// ---------------------------------------------------------------------------
// POST /api/context-tree-architecture — AI Layer: context tree → architecture diagram
// Takes user prompt + optional target_path, fetches context tree, and uses
// the same Groq LLM + diagram system prompt to produce a DiagramSpec.
// ---------------------------------------------------------------------------

type contextTreeArchitectureRequest struct {
	Prompt     string `json:"prompt"`
	TargetPath string `json:"target_path,omitempty"`
}

func (s *Server) handleContextTreeArchitecture(w http.ResponseWriter, r *http.Request) {
	var req contextTreeArchitectureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PROMPT", "prompt is required")
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_start",
		Data:  map[string]string{"tool": "context_tree_architecture", "prompt": req.Prompt},
	})

	// Step 1: Fetch the context tree from the repo via bridge
	treeText, err := s.bridge.GetContextTree(req.TargetPath)
	if err != nil {
		s.sse.Broadcast(SSEEvent{
			Event: "tool_error",
			Data:  map[string]string{"tool": "context_tree_architecture", "error": err.Error()},
		})
		writeError(w, http.StatusInternalServerError, "CONTEXT_TREE_ERROR", "Failed to get context tree: "+err.Error())
		return
	}
	if treeText == "" {
		writeError(w, http.StatusBadRequest, "EMPTY_TREE", "Context tree is empty — scan a repository first")
		return
	}

	// Truncate to avoid overwhelming the LLM
	if len(treeText) > 5000 {
		treeText = treeText[:5000] + "\n... (truncated)"
	}

	// Step 2: Compose the enriched prompt with context tree + user intent
	userPrompt := fmt.Sprintf(
		"Analyze this codebase structure and generate an architecture diagram based on the user's request.\n\n"+
			"CODEBASE CONTEXT TREE:\n%s\n\n"+
			"USER REQUEST: %s\n\n"+
			"Generate the architecture diagram that best represents this codebase's structure and the user's architectural intent.\n"+
			"IMPORTANT: Focus on the most important architectural components. Combine related sub-components into single nodes. Keep the diagram clean and readable with 12-30 nodes.",
		treeText, req.Prompt,
	)

	// Step 3: Call LLM (Bedrock primary, Groq fallback) with diagram system prompt
	llmResp, err := callLLM(r.Context(), llmRequest{
		SystemPrompt: diagramSystemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.7,
		MaxTokens:    4096,
		JSONMode:     true,
	})
	if err != nil {
		s.sse.Broadcast(SSEEvent{Event: "tool_error", Data: map[string]string{"tool": "context_tree_architecture", "error": err.Error()}})
		writeError(w, http.StatusBadGateway, "LLM_API_ERROR", err.Error())
		return
	}

	slog.Info("context_tree_architecture completed", "provider", llmResp.Provider)

	content := strings.TrimSpace(llmResp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var diagram map[string]interface{}
	if err := json.Unmarshal([]byte(content), &diagram); err != nil {
		writeError(w, http.StatusInternalServerError, "INVALID_DIAGRAM", "AI returned invalid diagram JSON: "+err.Error())
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_done",
		Data:  map[string]string{"tool": "context_tree_architecture"},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": diagram,
	})
}

// ---------------------------------------------------------------------------
// POST /api/edit-diagram — Modify an existing diagram based on an edit prompt
// Takes existing DiagramSpec + edit instructions → returns modified DiagramSpec
// ---------------------------------------------------------------------------

type editDiagramRequest struct {
	ExistingSpec json.RawMessage `json:"existing_spec"`
	EditPrompt   string          `json:"edit_prompt"`
}

const editDiagramSystemPrompt = `You are an expert architecture diagram editor. You are given an EXISTING diagram specification (JSON) and an edit instruction from the user. Your job is to MODIFY the existing diagram — NOT regenerate it from scratch.

RULES:
1. PRESERVE all existing nodes, groups, and edges unless the user explicitly asks to remove them.
2. Only ADD, REMOVE, or MODIFY elements that the edit instruction requires.
3. Keep existing node IDs, group IDs, colors, and structure intact when not changing them.
4. If the user asks to "enlarge" or "add more nodes", expand existing groups with additional sub-components while keeping all original nodes.
5. If the user asks to "simplify", you may merge or remove nodes, but keep the overall structure recognizable.
6. Maintain consistent icon usage and group color scheme from the original diagram.
7. Ensure all node IDs referenced in edges exist in the nodes array.
8. NEVER use raw file names or file paths as node labels. Use descriptive component names instead (e.g. "HTTP Server" not "server.go", "Container Build" not "Dockerfile").
9. EVERY node must be connected to at least one other node via an edge. No isolated or disconnected nodes.

Return ONLY a valid JSON object with the same structure as the input:
{
  "title": "Diagram Title",
  "groups": [ { "id": "...", "label": "...", "color": "...", "borderColor": "..." } ],
  "nodes": [ { "id": "...", "label": "...", "icon": "...", "group": "group_id_or_null" } ],
  "edges": [ { "source": "...", "target": "...", "label": "...", "style": "solid or dashed", "animated": false } ]
}

ICON REFERENCE — use these Iconify icon names:
AWS: logos:aws-lambda, logos:aws-s3, logos:aws-dynamodb, logos:aws-api-gateway, logos:aws-cloudwatch, logos:aws-cognito, logos:aws-ec2, logos:aws-rds, logos:aws-sqs, logos:aws-sns, logos:aws-ecs, logos:aws-cloudfront, logos:aws-route53, logos:aws-elasticache
Databases: mdi:database, simple-icons:postgresql, simple-icons:mongodb, simple-icons:redis, simple-icons:mysql
General: mdi:account-group, mdi:cellphone, mdi:web, mdi:api, mdi:server, mdi:cloud, mdi:shield-lock, mdi:monitor-eye, mdi:cached, mdi:tray-full, mdi:robot, mdi:scale-balance, mdi:earth, mdi:folder-multiple, mdi:lightning-bolt, mdi:cog, mdi:lock, mdi:email, mdi:chart-line, mdi:magnify-scan
Tech: logos:docker-icon, logos:kubernetes, logos:react, logos:nodejs-icon, logos:python, logos:go, logos:java, logos:nginx, logos:graphql, logos:kafka, logos:rabbitmq, logos:elasticsearch

GROUP COLORS (translucent for dark themes):
- Monitoring/Ops: color "#78350F", borderColor "#D97706"
- AI/ML Services: color "#1E1B4B", borderColor "#6366F1"
- Storage/Data: color "#172554", borderColor "#3B82F6"
- Security: color "#052E16", borderColor "#22C55E"
- Compute/Processing: color "#431407", borderColor "#EA580C"
- Networking: color "#083344", borderColor "#06B6D4"
- General: color "#1C1917", borderColor "#78716C"

IMPORTANT:
- Return ONLY the JSON, no markdown, no code fences, no explanation
- Nodes without a group should have group set to null`

func (s *Server) handleEditDiagram(w http.ResponseWriter, r *http.Request) {
	var req editDiagramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	if req.EditPrompt == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PROMPT", "edit_prompt is required")
		return
	}
	if len(req.ExistingSpec) == 0 {
		writeError(w, http.StatusBadRequest, "MISSING_SPEC", "existing_spec is required")
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_start",
		Data:  map[string]string{"tool": "edit_diagram", "prompt": req.EditPrompt},
	})

	userPrompt := fmt.Sprintf(
		"EXISTING DIAGRAM (JSON):\n%s\n\nEDIT INSTRUCTION: %s\n\nModify the existing diagram according to the instruction. Preserve all elements not affected by the edit.",
		string(req.ExistingSpec), req.EditPrompt,
	)

	llmResp, err := callLLM(r.Context(), llmRequest{
		SystemPrompt: editDiagramSystemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.4,
		MaxTokens:    4096,
		JSONMode:     true,
	})
	if err != nil {
		s.sse.Broadcast(SSEEvent{Event: "tool_error", Data: map[string]string{"tool": "edit_diagram", "error": err.Error()}})
		writeError(w, http.StatusBadGateway, "LLM_API_ERROR", err.Error())
		return
	}

	slog.Info("edit_diagram completed", "provider", llmResp.Provider)

	content := strings.TrimSpace(llmResp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var diagram map[string]interface{}
	if err := json.Unmarshal([]byte(content), &diagram); err != nil {
		writeError(w, http.StatusInternalServerError, "INVALID_DIAGRAM", "AI returned invalid diagram JSON: "+err.Error())
		return
	}

	s.sse.Broadcast(SSEEvent{
		Event: "tool_done",
		Data:  map[string]string{"tool": "edit_diagram"},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": diagram,
	})
}
