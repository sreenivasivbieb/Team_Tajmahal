package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Default Ollama settings
// ---------------------------------------------------------------------------

const (
	defaultOllamaModel     = "llama3"
	defaultOllamaEmbedding = "nomic-embed-text"
	ollamaTimeout          = 120 * time.Second
)

// ---------------------------------------------------------------------------
// OllamaProvider
// ---------------------------------------------------------------------------

// ollamaProvider implements Provider by calling the local Ollama HTTP API.
type ollamaProvider struct {
	baseURL        string
	httpClient     *http.Client
	defaultModel   string
	embeddingModel string
}

// newOllamaProvider creates an Ollama-backed provider.
func newOllamaProvider(cfg ProviderConfig) (*ollamaProvider, error) {
	base := strings.TrimRight(cfg.OllamaURL, "/")
	model := cfg.Model
	if model == "" {
		model = defaultOllamaModel
	}
	embModel := cfg.EmbeddingModel
	if embModel == "" {
		embModel = defaultOllamaEmbedding
	}

	return &ollamaProvider{
		baseURL: base,
		httpClient: &http.Client{
			Timeout: ollamaTimeout,
		},
		defaultModel:   model,
		embeddingModel: embModel,
	}, nil
}

// Name implements Provider.
func (o *ollamaProvider) Name() string { return "ollama" }

// Close implements Provider.
func (o *ollamaProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Generate  — POST /api/chat  (non-streaming)
// ---------------------------------------------------------------------------

// ollamaChatRequest is the JSON body for /api/chat.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Options  *ollamaOptions      `json:"options,omitempty"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float64  `json:"temperature,omitempty"`
	TopP        float64  `json:"top_p,omitempty"`
	NumPredict  int      `json:"num_predict,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// ollamaChatResponse is the JSON response from /api/chat (stream=false).
type ollamaChatResponse struct {
	Message ollamaChatMessage `json:"message"`
	Done    bool              `json:"done"`
}

// Generate implements Provider.
func (o *ollamaProvider) Generate(ctx context.Context, messages []Message, opts GenerateOptions) (*Message, error) {
	model := o.resolveModel(opts.Model)

	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: o.convertMessages(messages),
		Stream:   false,
		Options:  o.buildOptions(opts),
	}

	var resp ollamaChatResponse
	if err := o.doJSON(ctx, "/api/chat", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("ai/ollama: chat: %w", err)
	}

	return &Message{
		Role:    RoleAssistant,
		Content: resp.Message.Content,
	}, nil
}

// ---------------------------------------------------------------------------
// StreamGenerate — POST /api/chat  (stream=true, NDJSON)
// ---------------------------------------------------------------------------

// StreamGenerate implements Provider.
func (o *ollamaProvider) StreamGenerate(ctx context.Context, messages []Message, opts GenerateOptions) (<-chan StreamDelta, error) {
	model := o.resolveModel(opts.Model)

	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: o.convertMessages(messages),
		Stream:   true,
		Options:  o.buildOptions(opts),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ai/ollama: marshal stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai/ollama: build stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai/ollama: stream request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ai/ollama: stream status %d", resp.StatusCode)
	}

	ch := make(chan StreamDelta, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk ollamaChatResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				log.Printf("ai/ollama: unmarshal stream chunk: %v", err)
				continue
			}
			delta := StreamDelta{
				Content: chunk.Message.Content,
				Done:    chunk.Done,
			}
			ch <- delta
			if chunk.Done {
				return
			}
		}
		// Ensure we always send a done signal.
		ch <- StreamDelta{Done: true}
	}()

	return ch, nil
}

// ---------------------------------------------------------------------------
// GenerateWithTools — text-based tool simulation
// ---------------------------------------------------------------------------

// GenerateWithTools implements Provider. Since Ollama does not natively
// support tool_use blocks, we simulate it by injecting a text prompt that
// instructs the model to output structured JSON when it wants to call a tool.
func (o *ollamaProvider) GenerateWithTools(ctx context.Context, messages []Message, tools []Tool, opts GenerateOptions) (*Message, error) {
	// Build a tool description block to inject into the system prompt.
	toolDesc := o.buildToolPrompt(tools)

	// Prepend the tool description to the conversation.
	enhanced := make([]Message, 0, len(messages)+1)
	enhanced = append(enhanced, Message{
		Role: RoleSystem,
		Content: toolDesc,
	})
	for _, m := range messages {
		// Skip existing system messages — merge them.
		if m.Role == RoleSystem {
			enhanced[0].Content += "\n\n" + m.Content
			continue
		}
		enhanced = append(enhanced, m)
	}

	result, err := o.Generate(ctx, enhanced, opts)
	if err != nil {
		return nil, err
	}

	// Try to parse tool calls from the output.
	calls := o.parseToolCalls(result.Content)
	if len(calls) > 0 {
		result.ToolCalls = calls
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Embed — POST /api/embeddings
// ---------------------------------------------------------------------------

// ollamaEmbedRequest is the body for POST /api/embeddings.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaEmbedResponse is the response from POST /api/embeddings.
type ollamaEmbedResponse struct {
	Embedding []float64 `json:"embedding"` // Ollama returns float64
}

// Embed implements Provider.
func (o *ollamaProvider) Embed(ctx context.Context, text string, model string) ([]float32, error) {
	if model == "" {
		model = o.embeddingModel
	}

	var resp ollamaEmbedResponse
	if err := o.doJSON(ctx, "/api/embeddings", ollamaEmbedRequest{
		Model:  model,
		Prompt: text,
	}, &resp); err != nil {
		return nil, fmt.Errorf("ai/ollama: embeddings: %w", err)
	}

	// Convert float64 → float32.
	vec := make([]float32, len(resp.Embedding))
	for i, v := range resp.Embedding {
		vec[i] = float32(v)
	}
	return vec, nil
}

// ---------------------------------------------------------------------------
// HTTP helper
// ---------------------------------------------------------------------------

func (o *ollamaProvider) doJSON(ctx context.Context, path string, reqBody interface{}, out interface{}) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(errBody))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

func (o *ollamaProvider) resolveModel(override string) string {
	if override != "" {
		return override
	}
	return o.defaultModel
}

func (o *ollamaProvider) convertMessages(msgs []Message) []ollamaChatMessage {
	out := make([]ollamaChatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, ollamaChatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}
	return out
}

func (o *ollamaProvider) buildOptions(opts GenerateOptions) *ollamaOptions {
	oo := &ollamaOptions{}
	if opts.Temperature > 0 {
		oo.Temperature = opts.Temperature
	}
	if opts.TopP > 0 {
		oo.TopP = opts.TopP
	}
	if opts.MaxTokens > 0 {
		oo.NumPredict = opts.MaxTokens
	}
	if len(opts.StopWords) > 0 {
		oo.Stop = opts.StopWords
	}
	return oo
}

// buildToolPrompt creates a text description of available tools for the LLM.
func (o *ollamaProvider) buildToolPrompt(tools []Tool) string {
	var b strings.Builder
	b.WriteString("You have access to the following tools. To use a tool, respond with a JSON block like:\n")
	b.WriteString("```json\n{\"tool_call\": {\"id\": \"<uuid>\", \"name\": \"<tool_name>\", \"arguments\": {<args>}}}\n```\n\n")
	b.WriteString("Available tools:\n")
	for _, t := range tools {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n  Parameters: %s\n", t.Name, t.Description, string(t.Parameters)))
	}
	b.WriteString("\nIf you do NOT need a tool, respond normally with plain text.\n")
	return b.String()
}

// parseToolCalls attempts to extract tool call JSON from the model's text output.
func (o *ollamaProvider) parseToolCalls(text string) []ToolCall {
	// Look for ```json ... ``` blocks containing tool_call.
	var calls []ToolCall

	remaining := text
	for {
		start := strings.Index(remaining, "```json")
		if start == -1 {
			break
		}
		remaining = remaining[start+7:]
		end := strings.Index(remaining, "```")
		if end == -1 {
			break
		}
		block := strings.TrimSpace(remaining[:end])
		remaining = remaining[end+3:]

		var parsed struct {
			ToolCall *struct {
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"tool_call"`
		}
		if err := json.Unmarshal([]byte(block), &parsed); err != nil {
			continue
		}
		if parsed.ToolCall == nil {
			continue
		}

		id := parsed.ToolCall.ID
		if id == "" {
			id = uuid.New().String()
		}
		calls = append(calls, ToolCall{
			ID:        id,
			Name:      parsed.ToolCall.Name,
			Arguments: string(parsed.ToolCall.Arguments),
		})
	}

	return calls
}
