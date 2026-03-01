package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vyuha/vyuha-ai/internal/ai"
)

// ---------------------------------------------------------------------------
// Semantic question classification — AI-powered with keyword fallback
// ---------------------------------------------------------------------------

// classifyQuestion tries semanticclassification first; if the AI provider is
// nil or the call fails, it falls back to the deterministic keyword matcher.
func (ql *QueryLayer) classifyQuestion(ctx context.Context, question string) QueryDecision {
	if ql.provider != nil {
		decision, err := ql.classifyQuestionSemantic(ctx, question)
		if err == nil {
			return decision
		}
		slog.Warn("semantic classification failed, falling back to keyword",
			"question", question, "error", err)
	}
	return classifyQuestionKeyword(question)
}

// classifyQuestionSemantic asks the AI provider to classify a question into
// a QueryMode and optional SubgraphQueryType.
func (ql *QueryLayer) classifyQuestionSemantic(ctx context.Context, question string) (QueryDecision, error) {
	prompt := buildClassificationPrompt(question)

	messages := ai.BuildConversation(
		classificationSystemPrompt,
		ai.Message{Role: ai.RoleUser, Content: prompt},
	)

	opts := ai.GenerateOptions{
		MaxTokens:   256,
		Temperature: 0.0,
	}

	resp, err := ql.provider.Generate(ctx, messages, opts)
	if err != nil {
		return QueryDecision{}, fmt.Errorf("classify: provider.Generate: %w", err)
	}

	return parseClassificationResponse(resp.Content, question)
}

// ---------------------------------------------------------------------------
// Prompt construction
// ---------------------------------------------------------------------------

const classificationSystemPrompt = `You are a query classifier for a code-intelligence graph system.
Classify user questions into exactly one mode and, when applicable, a subgraph type.

Modes:
- "direct_graph"   — simple lookups: counts, listings, stats
- "subgraph"       — questions about relationships, flows, impact, call chains
- "agent"          — complex or open-ended analysis needing multi-step reasoning
- "sql"            — runtime statistics, failure counts, recent events

Subgraph types (only when mode is "subgraph"):
- "service_overview"    — architecture, structure, overview of a service
- "call_chain"          — call graphs, traces, callers/callees
- "failure_path"        — failures, errors, root cause diagnosis
- "data_lineage"        — data flow, producers, consumers
- "dependency_impact"   — blast radius, dependency impact, what-if analysis

Respond ONLY with a JSON object. No markdown, no extra text.`

// buildClassificationPrompt creates the user-turn content sent to the LLM.
func buildClassificationPrompt(question string) string {
	return fmt.Sprintf(`Classify this question:

"%s"

Respond with JSON:
{
  "mode": "<direct_graph|subgraph|agent|sql>",
  "subgraph_type": "<service_overview|call_chain|failure_path|data_lineage|dependency_impact|empty string>",
  "confidence": <0.0-1.0>,
  "reasoning": "<one sentence>"
}`, question)
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

// classificationJSON is the shape we expect from the LLM.
type classificationJSON struct {
	Mode         string  `json:"mode"`
	SubgraphType string  `json:"subgraph_type"`
	Confidence   float64 `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
}

// parseClassificationResponse extracts a QueryDecision from the LLM's
// JSON output. It returns an error if parsing fails or the mode is unknown.
func parseClassificationResponse(raw string, question string) (QueryDecision, error) {
	// The LLM sometimes wraps JSON in markdown code fences — strip them.
	cleaned := strings.TrimSpace(raw)
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}

	var parsed classificationJSON
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return QueryDecision{}, fmt.Errorf("classify: invalid JSON: %w (raw: %q)", err, raw)
	}

	// Map mode string → QueryMode.
	mode, ok := modeFromString(parsed.Mode)
	if !ok {
		return QueryDecision{}, fmt.Errorf("classify: unknown mode %q", parsed.Mode)
	}

	decision := QueryDecision{
		Mode:       mode,
		Confidence: parsed.Confidence,
		Reasoning:  parsed.Reasoning,
		Question:   question,
	}

	// Map subgraph type when applicable.
	if mode == ModeSubgraph && parsed.SubgraphType != "" {
		sgType, ok := subgraphTypeFromString(parsed.SubgraphType)
		if ok {
			decision.SubgraphType = sgType
		}
	}

	// Sanity: clamp confidence.
	if decision.Confidence <= 0 || decision.Confidence > 1 {
		decision.Confidence = 0.75
	}

	return decision, nil
}

// ---------------------------------------------------------------------------
// String ↔ enum helpers
// ---------------------------------------------------------------------------

func modeFromString(s string) (QueryMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "direct_graph":
		return ModeDirectGraph, true
	case "subgraph":
		return ModeSubgraph, true
	case "agent":
		return ModeAgent, true
	case "sql":
		return ModeSQL, true
	default:
		return "", false
	}
}

func subgraphTypeFromString(s string) (SubgraphQueryType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "service_overview":
		return QueryServiceOverview, true
	case "call_chain":
		return QueryCallChain, true
	case "failure_path":
		return QueryFailurePath, true
	case "data_lineage":
		return QueryDataLineage, true
	case "dependency_impact":
		return QueryDependencyImpact, true
	default:
		return "", false
	}
}
