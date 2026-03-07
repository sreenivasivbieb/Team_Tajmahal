// Unified LLM client — AWS Bedrock (primary) + Groq (fallback)
// Credential-driven: checks env vars to determine which provider(s) to use.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// ---------------------------------------------------------------------------
// LLM request/response types (provider-agnostic)
// ---------------------------------------------------------------------------

// llmRequest is the provider-agnostic chat completion request.
type llmRequest struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float64
	MaxTokens    int
	JSONMode     bool // request JSON output format
}

// llmResponse is the provider-agnostic result — just the content string.
type llmResponse struct {
	Content  string
	Provider string // "bedrock" or "groq"
}

// ---------------------------------------------------------------------------
// Provider availability helpers
// ---------------------------------------------------------------------------

func hasAWSCredentials() bool {
	return os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
}

func hasGroqKey() bool {
	return os.Getenv("GROQ_API_KEY") != ""
}

// ---------------------------------------------------------------------------
// callLLM — unified entrypoint with Bedrock-first, Groq-fallback logic
// ---------------------------------------------------------------------------

func callLLM(ctx context.Context, req llmRequest) (*llmResponse, error) {
	awsOK := hasAWSCredentials()
	groqOK := hasGroqKey()

	// Neither provider configured
	if !awsOK && !groqOK {
		return nil, fmt.Errorf("Configuration Error: Neither AWS Bedrock nor GROQ API credentials were found.")
	}

	// --- Primary: AWS Bedrock ---
	if awsOK {
		resp, err := callBedrock(ctx, req)
		if err == nil {
			return resp, nil
		}
		slog.Warn("AWS Bedrock call failed, checking fallback", "error", err)

		// Bedrock failed — try Groq fallback
		if groqOK {
			slog.Info("falling back to Groq API")
			return callGroqLLM(ctx, req)
		}

		return nil, fmt.Errorf("AWS Bedrock is not working and no fallback API key is available.")
	}

	// --- Only Groq available ---
	return callGroqLLM(ctx, req)
}

// ---------------------------------------------------------------------------
// AWS Bedrock — Converse API
// ---------------------------------------------------------------------------

func callBedrock(ctx context.Context, req llmRequest) (*llmResponse, error) {
	region := getEnvDefault("AWS_REGION", "us-east-1")
	modelID := getEnvDefault("BEDROCK_MODEL_ID", "us.anthropic.claude-sonnet-4-20250514-v1:0")

	cfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(region),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			os.Getenv("AWS_ACCESS_KEY_ID"),
			os.Getenv("AWS_SECRET_ACCESS_KEY"),
			os.Getenv("AWS_SESSION_TOKEN"),
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("AWS config error: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	// Build messages for Converse API
	messages := []brtypes.Message{
		{
			Role: brtypes.ConversationRoleUser,
			Content: []brtypes.ContentBlock{
				&brtypes.ContentBlockMemberText{Value: req.UserPrompt},
			},
		},
	}

	// Build system prompt
	system := []brtypes.SystemContentBlock{
		&brtypes.SystemContentBlockMemberText{Value: req.SystemPrompt},
	}

	maxTokens := int32(req.MaxTokens)
	temp := float32(req.Temperature)

	input := &bedrockruntime.ConverseInput{
		ModelId:  &modelID,
		Messages: messages,
		System:   system,
		InferenceConfig: &brtypes.InferenceConfiguration{
			MaxTokens:   &maxTokens,
			Temperature: &temp,
		},
	}

	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 2 * time.Second
			slog.Info("retrying Bedrock API call", "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
		}

		output, err := client.Converse(ctx, input)
		if err != nil {
			lastErr = fmt.Errorf("Bedrock Converse error: %w", err)
			errMsg := err.Error()
			if isTransientBedrockError(errMsg) {
				slog.Warn("Bedrock transient error, will retry", "attempt", attempt+1, "error", errMsg)
				continue
			}
			return nil, lastErr
		}

		// Extract text from the response
		if output.Output == nil {
			lastErr = fmt.Errorf("Bedrock returned nil output")
			continue
		}

		msgOutput, ok := output.Output.(*brtypes.ConverseOutputMemberMessage)
		if !ok {
			lastErr = fmt.Errorf("Bedrock returned unexpected output type")
			continue
		}

		var textParts []string
		for _, block := range msgOutput.Value.Content {
			if tb, ok := block.(*brtypes.ContentBlockMemberText); ok {
				textParts = append(textParts, tb.Value)
			}
		}

		if len(textParts) == 0 {
			lastErr = fmt.Errorf("Bedrock returned no text content")
			continue
		}

		content := strings.Join(textParts, "")
		return &llmResponse{Content: content, Provider: "bedrock"}, nil
	}

	return nil, fmt.Errorf("Bedrock API failed after %d retries: %w", maxRetries, lastErr)
}

func isTransientBedrockError(errMsg string) bool {
	transientPatterns := []string{
		"ThrottlingException",
		"TooManyRequestsException",
		"ServiceUnavailableException",
		"InternalServerException",
		"ModelTimeoutException",
		"timeout",
		"connection reset",
	}
	lower := strings.ToLower(errMsg)
	for _, p := range transientPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Groq wrapper — reuses callGroq from handlers.go
// ---------------------------------------------------------------------------

func callGroqLLM(ctx context.Context, req llmRequest) (*llmResponse, error) {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY not set")
	}

	groqReq := groqChatRequest{
		Model: getEnvDefault("GROQ_MODEL", "llama-3.3-70b-versatile"),
		Messages: []groqMessage{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if req.JSONMode {
		groqReq.ResponseFormat = &groqFormat{Type: "json_object"}
	}

	respBody, err := callGroq(ctx, groqKey, groqReq)
	if err != nil {
		return nil, err
	}

	var groqResp groqChatResponse
	if err := json.Unmarshal(respBody, &groqResp); err != nil {
		return nil, fmt.Errorf("parse Groq response: %w", err)
	}
	if len(groqResp.Choices) == 0 {
		return nil, fmt.Errorf("Groq returned no choices")
	}

	return &llmResponse{
		Content:  groqResp.Choices[0].Message.Content,
		Provider: "groq",
	}, nil
}
