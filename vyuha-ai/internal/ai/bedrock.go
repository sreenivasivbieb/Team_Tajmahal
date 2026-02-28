package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// ---------------------------------------------------------------------------
// Default model IDs
// ---------------------------------------------------------------------------

const (
	defaultBedrockModel     = "anthropic.claude-3-haiku-20240307-v1:0"
	defaultBedrockEmbedding = "amazon.titan-embed-text-v2:0"
	titanEmbedDimensions    = 1024
	anthropicVersion        = "bedrock-2023-05-31"
)

// ---------------------------------------------------------------------------
// BedrockProvider
// ---------------------------------------------------------------------------

// bedrockProvider implements Provider using InvokeModel for chat completions
// (Anthropic Messages API format) and embeddings (Titan).
type bedrockProvider struct {
	client         *bedrockruntime.Client
	defaultModel   string
	embeddingModel string
	region         string
}

// newBedrockProvider initialises an AWS Bedrock provider.
func newBedrockProvider(ctx context.Context, cfg ProviderConfig) (*bedrockProvider, error) {
	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: load aws config: %w", err)
	}

	model := cfg.Model
	if model == "" {
		model = defaultBedrockModel
	}
	embModel := cfg.EmbeddingModel
	if embModel == "" {
		embModel = defaultBedrockEmbedding
	}

	return &bedrockProvider{
		client:         bedrockruntime.NewFromConfig(awsCfg),
		defaultModel:   model,
		embeddingModel: embModel,
		region:         cfg.Region,
	}, nil
}

// Name implements Provider.
func (b *bedrockProvider) Name() string { return "bedrock" }

// Close implements Provider.
func (b *bedrockProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Anthropic Messages API types (used as InvokeModel body)
// ---------------------------------------------------------------------------

type anthropicRequest struct {
	AnthropicVersion string             `json:"anthropic_version"`
	MaxTokens        int                `json:"max_tokens"`
	Temperature      float64            `json:"temperature,omitempty"`
	TopP             float64            `json:"top_p,omitempty"`
	StopSequences    []string           `json:"stop_sequences,omitempty"`
	System           string             `json:"system,omitempty"`
	Messages         []anthropicMessage `json:"messages"`
	Tools            []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string               `json:"role"`
	Content []anthropicContent   `json:"content"`
}

type anthropicContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []anthropicContent `json:"content"`
	StopReason   string             `json:"stop_reason"`
	StopSequence *string            `json:"stop_sequence"`
}

// ---------------------------------------------------------------------------
// Generate
// ---------------------------------------------------------------------------

// Generate implements Provider using InvokeModel with the Anthropic Messages API.
func (b *bedrockProvider) Generate(ctx context.Context, messages []Message, opts GenerateOptions) (*Message, error) {
	model := b.resolveModel(opts.Model)
	req := b.buildAnthropicRequest(messages, nil, opts)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: marshal request: %w", err)
	}

	resp, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(model),
		Body:        body,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: invoke model: %w", err)
	}

	return b.parseAnthropicResponse(resp.Body)
}

// ---------------------------------------------------------------------------
// StreamGenerate
// ---------------------------------------------------------------------------

// StreamGenerate implements Provider using InvokeModelWithResponseStream.
func (b *bedrockProvider) StreamGenerate(ctx context.Context, messages []Message, opts GenerateOptions) (<-chan StreamDelta, error) {
	model := b.resolveModel(opts.Model)
	req := b.buildAnthropicRequest(messages, nil, opts)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: marshal stream request: %w", err)
	}

	resp, err := b.client.InvokeModelWithResponseStream(ctx,
		&bedrockruntime.InvokeModelWithResponseStreamInput{
			ModelId:     aws.String(model),
			Body:        body,
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
		})
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: invoke model stream: %w", err)
	}

	ch := make(chan StreamDelta, 64)
	go func() {
		defer close(ch)
		stream := resp.GetStream()
		defer stream.Close()

		for event := range stream.Events() {
			switch v := event.(type) {
			case *types.ResponseStreamMemberChunk:
				// Parse the SSE-style chunk from Anthropic.
				b.handleStreamChunk(v.Value.Bytes, ch)
			}
		}
		// Ensure we always send a done signal.
		ch <- StreamDelta{Done: true}
	}()

	return ch, nil
}

// handleStreamChunk parses Anthropic streaming event data from a chunk.
func (b *bedrockProvider) handleStreamChunk(data []byte, ch chan<- StreamDelta) {
	// Anthropic streaming returns newline-delimited JSON events.
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			// Single JSON object in the chunk.
			if err2 := json.Unmarshal(data, &event); err2 != nil {
				continue
			}
		}
		switch event.Type {
		case "content_block_delta":
			if event.Delta.Text != "" {
				ch <- StreamDelta{Content: event.Delta.Text}
			}
		case "message_delta":
			if event.Delta.StopReason != "" {
				ch <- StreamDelta{Done: true, StopReason: event.Delta.StopReason}
			}
		case "message_stop":
			ch <- StreamDelta{Done: true}
		}
	}
}

// ---------------------------------------------------------------------------
// GenerateWithTools
// ---------------------------------------------------------------------------

// GenerateWithTools implements Provider with Anthropic tool_use support.
func (b *bedrockProvider) GenerateWithTools(ctx context.Context, messages []Message, tools []Tool, opts GenerateOptions) (*Message, error) {
	model := b.resolveModel(opts.Model)
	req := b.buildAnthropicRequest(messages, tools, opts)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: marshal tool request: %w", err)
	}

	resp, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(model),
		Body:        body,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: invoke model with tools: %w", err)
	}

	msg, err := b.parseAnthropicResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(msg.ToolCalls) > 0 {
		log.Printf("ai/bedrock: model requested %d tool calls", len(msg.ToolCalls))
	}
	return msg, nil
}

// ---------------------------------------------------------------------------
// Embed
// ---------------------------------------------------------------------------

// titanEmbedRequest is the JSON body for Titan Embedding V2.
type titanEmbedRequest struct {
	InputText  string `json:"inputText"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// titanEmbedResponse is the JSON response from Titan Embedding V2.
type titanEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed implements Provider. It uses InvokeModel with Titan Embedding.
func (b *bedrockProvider) Embed(ctx context.Context, text string, model string) ([]float32, error) {
	if model == "" {
		model = b.embeddingModel
	}

	body, err := json.Marshal(titanEmbedRequest{
		InputText:  text,
		Dimensions: titanEmbedDimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: marshal embed request: %w", err)
	}

	resp, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(model),
		Body:        body,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("ai/bedrock: invoke model embed: %w", err)
	}

	var result titanEmbedResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("ai/bedrock: unmarshal embed response: %w", err)
	}
	return result.Embedding, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (b *bedrockProvider) resolveModel(override string) string {
	if override != "" {
		return override
	}
	return b.defaultModel
}

func (b *bedrockProvider) buildAnthropicRequest(messages []Message, tools []Tool, opts GenerateOptions) anthropicRequest {
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	req := anthropicRequest{
		AnthropicVersion: anthropicVersion,
		MaxTokens:        maxTokens,
	}
	if opts.Temperature > 0 {
		req.Temperature = opts.Temperature
	}
	if opts.TopP > 0 {
		req.TopP = opts.TopP
	}
	if len(opts.StopWords) > 0 {
		req.StopSequences = opts.StopWords
	}

	// Convert tools.
	for _, t := range tools {
		req.Tools = append(req.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	// Split system messages from conversation messages.
	var sysParts []string
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			sysParts = append(sysParts, m.Content)
		case RoleUser:
			req.Messages = append(req.Messages, anthropicMessage{
				Role:    "user",
				Content: []anthropicContent{{Type: "text", Text: m.Content}},
			})
		case RoleAssistant:
			content := []anthropicContent{{Type: "text", Text: m.Content}}
			for _, tc := range m.ToolCalls {
				content = append(content, anthropicContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Arguments),
				})
			}
			req.Messages = append(req.Messages, anthropicMessage{
				Role:    "assistant",
				Content: content,
			})
		case RoleTool:
			req.Messages = append(req.Messages, anthropicMessage{
				Role: "user",
				Content: []anthropicContent{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		}
	}
	if len(sysParts) > 0 {
		req.System = strings.Join(sysParts, "\n\n")
	}

	return req
}

func (b *bedrockProvider) parseAnthropicResponse(body []byte) (*Message, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("ai/bedrock: unmarshal response: %w", err)
	}

	msg := &Message{Role: RoleAssistant}
	var textParts []string

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(argsJSON),
			})
		}
	}
	msg.Content = strings.Join(textParts, "")

	return msg, nil
}

// InvokeModelWithResponseStreamOutputMemberChunk is used to detect the chunk event type.
// This type assertion guard ensures the stream event handling compiles
// even though the actual SDK type is used dynamically at runtime.
var _ = (*types.ResponseStreamMemberChunk)(nil)
