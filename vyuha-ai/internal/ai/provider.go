package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Provider kinds
// ---------------------------------------------------------------------------

// ProviderKind identifies a supported AI backend.
type ProviderKind string

const (
	ProviderBedrock ProviderKind = "bedrock"
	ProviderOllama  ProviderKind = "ollama"
)

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

// Role represents a conversation participant.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single turn in a conversation.
type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Tool calling types
// ---------------------------------------------------------------------------

// Tool describes a function the LLM may invoke.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ToolCall is the LLM's request to invoke a tool.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolResponse carries the result of a tool invocation back to the LLM.
type ToolResponse struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ---------------------------------------------------------------------------
// Completion options
// ---------------------------------------------------------------------------

// GenerateOptions configures a single completion request.
type GenerateOptions struct {
	Model       string  `json:"model,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	StopWords   []string `json:"stop_words,omitempty"`
}

// DefaultGenerateOptions returns sensible defaults.
func DefaultGenerateOptions() GenerateOptions {
	return GenerateOptions{
		MaxTokens:   2048,
		Temperature: 0.3,
		TopP:        0.9,
	}
}

// ---------------------------------------------------------------------------
// Streaming
// ---------------------------------------------------------------------------

// StreamDelta is one chunk of a streaming response.
type StreamDelta struct {
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Done       bool       `json:"done"`
	StopReason string     `json:"stop_reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Provider interface
// ---------------------------------------------------------------------------

// Provider is the contract every AI backend must satisfy.
type Provider interface {
	// Generate produces a single, complete assistant response.
	Generate(ctx context.Context, messages []Message, opts GenerateOptions) (*Message, error)

	// StreamGenerate produces a streaming response. The caller must drain the
	// returned channel. The channel is closed when the response is complete.
	StreamGenerate(ctx context.Context, messages []Message, opts GenerateOptions) (<-chan StreamDelta, error)

	// GenerateWithTools runs a tool-use loop: the LLM may request tool calls, the
	// caller supplies results, and the LLM produces a final text response.
	GenerateWithTools(ctx context.Context, messages []Message, tools []Tool, opts GenerateOptions) (*Message, error)

	// Embed produces a vector embedding for the given text.
	Embed(ctx context.Context, text string, model string) ([]float32, error)

	// Name returns a human-readable provider name, e.g. "bedrock" or "ollama".
	Name() string

	// Close releases any resources held by the provider (e.g. HTTP clients).
	Close() error
}

// ---------------------------------------------------------------------------
// Provider configuration
// ---------------------------------------------------------------------------

// ProviderConfig holds all configuration accepted by NewProvider.
type ProviderConfig struct {
	Kind   ProviderKind `json:"kind"`
	Region string       `json:"region,omitempty"` // AWS region for Bedrock
	Model  string       `json:"model,omitempty"`  // default model ID
	EmbeddingModel string `json:"embedding_model,omitempty"`

	// Ollama-specific
	OllamaURL string `json:"ollama_url,omitempty"` // e.g. "http://localhost:11434"
}

// Validate checks that required fields are set.
func (c ProviderConfig) Validate() error {
	switch c.Kind {
	case ProviderBedrock:
		if c.Region == "" {
			return fmt.Errorf("ai: bedrock provider requires region")
		}
	case ProviderOllama:
		if c.OllamaURL == "" {
			return fmt.Errorf("ai: ollama provider requires ollama_url")
		}
	default:
		return fmt.Errorf("ai: unknown provider kind %q", c.Kind)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

// NewProvider creates a concrete Provider from configuration.
func NewProvider(ctx context.Context, cfg ProviderConfig) (Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	switch cfg.Kind {
	case ProviderBedrock:
		return newBedrockProvider(ctx, cfg)
	case ProviderOllama:
		return newOllamaProvider(cfg)
	default:
		return nil, fmt.Errorf("ai: unsupported provider %q", cfg.Kind)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// BuildConversation is a convenience that prepends a system prompt to a
// sequence of user/assistant turns.
func BuildConversation(system string, turns ...Message) []Message {
	msgs := make([]Message, 0, 1+len(turns))
	if system != "" {
		msgs = append(msgs, Message{Role: RoleSystem, Content: strings.TrimSpace(system)})
	}
	msgs = append(msgs, turns...)
	return msgs
}
