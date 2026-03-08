// Bridge MCP client — spawns contextplus as a child process and communicates via JSON-RPC 2.0 over stdio
// Provides typed Go wrappers for each contextplus tool call

package bridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// MCPClient manages a long-lived contextplus child process and sends
// JSON-RPC 2.0 tool-call requests over stdin/stdout.
type MCPClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	mu      sync.Mutex   // serialise request/response pairs
	nextID  atomic.Int64 // monotonic request IDs
	rootDir string       // codebase root passed to contextplus
}

// jsonRPCRequest is a JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolCallParams wraps the MCP tools/call request payload.
type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// mcpToolResult is the shape returned by MCP tools/call.
type mcpToolResult struct {
	Content []mcpContent `json:"content"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewMCPClient spawns contextplus with the given root directory and performs
// the MCP initialise handshake. The contextplusPath should be the path to
// the contextplus build/index.js (or "npx contextplus").
func NewMCPClient(contextplusPath, rootDir string) (*MCPClient, error) {
	// Determine spawn command — if path ends with .js, use node; otherwise run directly.
	var cmd *exec.Cmd
	if len(contextplusPath) > 3 && contextplusPath[len(contextplusPath)-3:] == ".js" {
		cmd = exec.Command("node", contextplusPath, rootDir)
	} else {
		cmd = exec.Command(contextplusPath, rootDir)
	}

	// Inherit environment so OLLAMA_*, GROQ_API_KEY, etc. propagate.
	// Also load any .env file next to contextplus (e.g. contextplus/.env).
	env := os.Environ()
	envFilePath := filepath.Join(filepath.Dir(contextplusPath), "..", ".env")
	if dotEnv, err := loadDotEnv(envFilePath); err == nil {
		slog.Info("loaded .env for contextplus", "path", envFilePath, "vars", len(dotEnv))
		// Merge: .env values only set if NOT already present in the real environment
		existing := make(map[string]bool)
		for _, e := range env {
			if k, _, ok := strings.Cut(e, "="); ok {
				existing[k] = true
			}
		}
		for k, v := range dotEnv {
			if !existing[k] {
				env = append(env, k+"="+v)
			}
		}
	} else {
		slog.Debug("no .env found for contextplus", "path", envFilePath, "err", err)
	}
	cmd.Env = env
	cmd.Stderr = os.Stderr // let contextplus logs flow through

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("bridge: stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("bridge: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("bridge: start contextplus: %w", err)
	}

	c := &MCPClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReaderSize(stdoutPipe, 1024*1024), // 1 MB buffer
		rootDir: rootDir,
	}

	// MCP initialise handshake
	if err := c.initialise(); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("bridge: MCP init: %w", err)
	}

	slog.Info("contextplus bridge connected", "root", rootDir, "pid", cmd.Process.Pid)
	return c, nil
}

// initialise performs the MCP protocol handshake.
func (c *MCPClient) initialise() error {
	// 1. Send initialize request
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "vyuha-bridge",
				"version": "1.0.0",
			},
		},
	}

	if err := c.sendRequest(initReq); err != nil {
		return fmt.Errorf("send initialize: %w", err)
	}

	// 2. Read initialize response
	resp, err := c.readResponse()
	if err != nil {
		return fmt.Errorf("read initialize response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	// 3. Send initialized notification (no ID = notification)
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if err := c.sendNotification(notif); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

// CallTool invokes a named MCP tool with the given arguments and returns
// the text content of the first result block.
func (c *MCPClient) CallTool(toolName string, args map[string]interface{}) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params:  toolCallParams{Name: toolName, Arguments: args},
	}

	if err := c.sendRequest(req); err != nil {
		return "", fmt.Errorf("bridge: send %s: %w", toolName, err)
	}

	resp, err := c.readResponse()
	if err != nil {
		return "", fmt.Errorf("bridge: read %s response: %w", toolName, err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("bridge: tool %s error (%d): %s", toolName, resp.Error.Code, resp.Error.Message)
	}

	// Parse MCP tool result
	var result mcpToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("bridge: parse %s result: %w", toolName, err)
	}

	if len(result.Content) == 0 {
		return "", nil
	}

	return result.Content[0].Text, nil
}

// sendRequest writes a JSON-RPC request as a single line to stdin.
func (c *MCPClient) sendRequest(req jsonRPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

// sendNotification writes a JSON-RPC notification (no ID) as a single line.
func (c *MCPClient) sendNotification(notif jsonRPCRequest) error {
	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

// readResponse reads the next JSON-RPC response line from stdout.
// It skips notification lines (lines without an "id" field).
func (c *MCPClient) readResponse() (*jsonRPCResponse, error) {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("read line: %w", err)
		}

		// Skip empty lines
		if len(line) <= 1 {
			continue
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Could be a notification or log line — skip it
			slog.Debug("bridge: skipping non-JSON line", "line", string(line))
			continue
		}

		// Skip notifications (no ID)
		if resp.ID == 0 && resp.Result == nil && resp.Error == nil {
			continue
		}

		return &resp, nil
	}
}

// loadDotEnv parses a simple .env file and returns key=value pairs.
// Supports blank lines, # comments, KEY=VALUE, and optionally quoted values.
func loadDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Strip surrounding quotes (single or double)
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		if k != "" {
			result[k] = v
		}
	}
	return result, scanner.Err()
}
