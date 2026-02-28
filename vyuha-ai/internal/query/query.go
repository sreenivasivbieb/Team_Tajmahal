package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/vyuha/vyuha-ai/internal/ai"
	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// ---------------------------------------------------------------------------
// QueryLayer — top-level orchestrator
// ---------------------------------------------------------------------------

// QueryLayer is the single entry point for all code-intelligence queries.
// It owns the subgraph extractor, AI agent, and embedding service.
type QueryLayer struct {
	index       *graph.GraphIndex
	store       *storage.Storage
	provider    ai.Provider
	embeddings  *ai.EmbeddingService
	broadcaster ai.Broadcaster
	extractor   *SubgraphExtractor
	agent       *VyuhaAgent
	scorer      *NodeScorer
}

// NewQueryLayer wires together all query subsystems.
func NewQueryLayer(
	index *graph.GraphIndex,
	store *storage.Storage,
	provider ai.Provider,
	embeddings *ai.EmbeddingService,
	broadcaster ai.Broadcaster,
) *QueryLayer {
	extractor := NewSubgraphExtractor(index, store)
	scorer := NewNodeScorer(index)

	ql := &QueryLayer{
		index:       index,
		store:       store,
		provider:    provider,
		embeddings:  embeddings,
		broadcaster: broadcaster,
		extractor:   extractor,
		scorer:      scorer,
	}

	ql.agent = NewVyuhaAgent(provider, index, store, extractor, broadcaster)

	return ql
}

// ---------------------------------------------------------------------------
// Query modes
// ---------------------------------------------------------------------------

// QueryMode determines how a question is answered.
type QueryMode string

const (
	ModeDirectGraph QueryMode = "direct_graph"
	ModeSubgraph    QueryMode = "subgraph"
	ModeAgent       QueryMode = "agent"
	ModeSQL         QueryMode = "sql"
)

// ---------------------------------------------------------------------------
// QueryDecision — routing
// ---------------------------------------------------------------------------

// QueryDecision captures the routing decision for a question.
type QueryDecision struct {
	Mode        QueryMode         `json:"mode"`
	SubgraphType SubgraphQueryType `json:"subgraph_type,omitempty"`
	TargetID    string            `json:"target_id,omitempty"`
	Confidence  float64           `json:"confidence"`
	Reasoning   string            `json:"reasoning"`
}

// ---------------------------------------------------------------------------
// QueryResult
// ---------------------------------------------------------------------------

// QueryResult is the unified response for any query.
type QueryResult struct {
	Mode        QueryMode        `json:"mode"`
	Answer      string           `json:"answer"`
	Subgraph    *SubgraphResult  `json:"subgraph,omitempty"`
	AgentRun    *AgentRun        `json:"agent_run,omitempty"`
	GraphData   *DirectGraphData `json:"graph_data,omitempty"`
	Duration    time.Duration    `json:"duration_ms"`
	Decision    QueryDecision    `json:"decision"`
}

// DirectGraphData holds results from direct graph lookups.
type DirectGraphData struct {
	Nodes []*graph.Node `json:"nodes"`
	Edges []*graph.Edge `json:"edges"`
	Stats *graph.IndexStats `json:"stats,omitempty"`
}

// ---------------------------------------------------------------------------
// HandleQuestion — main entry
// ---------------------------------------------------------------------------

// HandleQuestion routes a natural-language question to the best answering
// strategy and returns a unified QueryResult.
func (ql *QueryLayer) HandleQuestion(ctx context.Context, question string) (*QueryResult, error) {
	start := time.Now()

	ql.broadcastProgress("query_start", map[string]string{
		"question": question,
	})

	// 1. Classify the question.
	decision := ql.classifyQuestion(question)

	ql.broadcastProgress("query_classified", map[string]interface{}{
		"mode":       decision.Mode,
		"confidence": decision.Confidence,
		"reasoning":  decision.Reasoning,
	})

	// 2. If we identified a target name but not an ID, resolve it.
	if decision.TargetID == "" && decision.Mode != ModeAgent {
		targetID := ql.resolveTarget(ctx, question)
		if targetID != "" {
			decision.TargetID = targetID
		}
	}

	// 3. Execute based on mode.
	var result *QueryResult
	var err error

	switch decision.Mode {
	case ModeDirectGraph:
		result, err = ql.executeDirectGraph(ctx, question, decision)
	case ModeSubgraph:
		result, err = ql.executeSubgraph(ctx, question, decision)
	case ModeAgent:
		result, err = ql.executeAgent(ctx, question, decision)
	case ModeSQL:
		result, err = ql.executeSQLMode(ctx, question, decision)
	default:
		// Fallback to agent for unclassified queries.
		decision.Mode = ModeAgent
		result, err = ql.executeAgent(ctx, question, decision)
	}

	if err != nil {
		ql.broadcastProgress("query_error", map[string]string{"error": err.Error()})
		return nil, err
	}

	result.Duration = time.Since(start)
	result.Decision = decision

	ql.broadcastProgress("query_done", map[string]interface{}{
		"mode":     result.Mode,
		"duration": result.Duration.Milliseconds(),
	})

	return result, nil
}

// ---------------------------------------------------------------------------
// Question classification
// ---------------------------------------------------------------------------

// classifyQuestion analyses the question text and picks a QueryMode.
func (ql *QueryLayer) classifyQuestion(question string) QueryDecision {
	q := strings.ToLower(question)

	// Pattern: service overview
	if matchesAny(q, "overview", "architecture", "structure of", "describe service", "what does .* service") {
		return QueryDecision{
			Mode:         ModeSubgraph,
			SubgraphType: QueryServiceOverview,
			Confidence:   0.85,
			Reasoning:    "Question asks for service overview/architecture.",
		}
	}

	// Pattern: call chain / trace
	if matchesAny(q, "call chain", "call graph", "trace", "who calls", "what calls", "callers of", "callees of") {
		return QueryDecision{
			Mode:         ModeSubgraph,
			SubgraphType: QueryCallChain,
			Confidence:   0.85,
			Reasoning:    "Question asks about call chains or tracing.",
		}
	}

	// Pattern: failure / error
	if matchesAny(q, "why.*fail", "why.*error", "failing", "failure", "broken", "root cause", "diagnose") {
		return QueryDecision{
			Mode:         ModeSubgraph,
			SubgraphType: QueryFailurePath,
			Confidence:   0.80,
			Reasoning:    "Question asks about failures or errors.",
		}
	}

	// Pattern: data flow / lineage
	if matchesAny(q, "data flow", "data lineage", "where does.*data", "produces", "consumes", "transforms") {
		return QueryDecision{
			Mode:         ModeSubgraph,
			SubgraphType: QueryDataLineage,
			Confidence:   0.80,
			Reasoning:    "Question asks about data flow or lineage.",
		}
	}

	// Pattern: dependency impact / blast radius
	if matchesAny(q, "impact", "blast radius", "what depends", "who depends", "dependency impact", "if.*fails") {
		return QueryDecision{
			Mode:         ModeSubgraph,
			SubgraphType: QueryDependencyImpact,
			Confidence:   0.80,
			Reasoning:    "Question asks about dependency impact.",
		}
	}

	// Pattern: direct graph (simple lookups)
	if matchesAny(q, "list all", "how many", "count", "show me all", "graph stats") {
		return QueryDecision{
			Mode:       ModeDirectGraph,
			Confidence: 0.70,
			Reasoning:  "Question is a simple graph lookup.",
		}
	}

	// Pattern: SQL-like queries
	if matchesAny(q, "top failing", "most errors", "recent events", "runtime stats") {
		return QueryDecision{
			Mode:       ModeSQL,
			Confidence: 0.70,
			Reasoning:  "Question requires runtime/statistical data.",
		}
	}

	// Default: agent for complex / ambiguous questions.
	return QueryDecision{
		Mode:       ModeAgent,
		Confidence: 0.50,
		Reasoning:  "Question is complex or ambiguous; routing to agent for multi-step reasoning.",
	}
}

// matchesAny checks if the text contains any of the given patterns.
// Simple substring matching with basic glob support.
func matchesAny(text string, patterns ...string) bool {
	for _, p := range patterns {
		if strings.Contains(p, ".*") {
			// Simple regex-like: split on .* and check both parts exist in order.
			parts := strings.SplitN(p, ".*", 2)
			idx := strings.Index(text, parts[0])
			if idx >= 0 && len(parts) > 1 {
				rest := text[idx+len(parts[0]):]
				if strings.Contains(rest, parts[1]) {
					return true
				}
			}
		} else if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Target resolution
// ---------------------------------------------------------------------------

// resolveTarget attempts to identify a target node ID from the question.
// It uses fuzzy name matching against graph nodes and embedding similarity.
func (ql *QueryLayer) resolveTarget(ctx context.Context, question string) string {
	// 1. Extract potential names from the question.
	candidates := extractCandidateNames(question)

	// 2. Try exact/fuzzy match against the graph index.
	for _, name := range candidates {
		if id := ql.fuzzyFindNode(name); id != "" {
			return id
		}
	}

	// 3. Fall back to embedding similarity search if available.
	if ql.embeddings != nil {
		results, err := ql.embeddings.SimilaritySearch(ctx, question, 1)
		if err == nil && len(results) > 0 && results[0].Score > 0.7 {
			return results[0].NodeID
		}
	}

	return ""
}

// extractCandidateNames pulls potential identifiers from the question.
func extractCandidateNames(question string) []string {
	var names []string
	words := strings.Fields(question)
	for _, w := range words {
		// Heuristic: identifiers have mixed case, dots, slashes, or underscores.
		clean := strings.Trim(w, "?.,!;:'\"()")
		if len(clean) < 2 {
			continue
		}
		if strings.ContainsAny(clean, "._/") ||
			(hasUpper(clean) && hasLower(clean)) ||
			strings.HasSuffix(clean, "()") {
			names = append(names, strings.TrimSuffix(clean, "()"))
		}
	}
	return names
}

func hasUpper(s string) bool {
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

func hasLower(s string) bool {
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			return true
		}
	}
	return false
}

// fuzzyFindNode searches the graph index for a node matching the given name.
func (ql *QueryLayer) fuzzyFindNode(name string) string {
	lower := strings.ToLower(name)

	// Try services first.
	for _, n := range ql.index.GetByType(graph.NodeTypeService) {
		if strings.ToLower(n.Name) == lower {
			return n.ID
		}
	}

	// Try functions.
	for _, n := range ql.index.GetByType(graph.NodeTypeFunction) {
		if strings.ToLower(n.Name) == lower {
			return n.ID
		}
	}

	// Try partial match on all node types.
	allTypes := []graph.NodeType{
		graph.NodeTypeService,
		graph.NodeTypeFunction,
		graph.NodeTypeStruct,
		graph.NodeTypeInterface,
		graph.NodeTypePackage,
		graph.NodeTypeCloudService,
	}
	for _, nt := range allTypes {
		for _, n := range ql.index.GetByType(nt) {
			if strings.Contains(strings.ToLower(n.Name), lower) {
				return n.ID
			}
		}
	}

	return ""
}

// ---------------------------------------------------------------------------
// Mode executors
// ---------------------------------------------------------------------------

func (ql *QueryLayer) executeDirectGraph(ctx context.Context, question string, decision QueryDecision) (*QueryResult, error) {
	q := strings.ToLower(question)

	// Stats query.
	if strings.Contains(q, "stats") || strings.Contains(q, "count") || strings.Contains(q, "how many") {
		stats := ql.index.Stats()
		answer := fmt.Sprintf(
			"Graph statistics:\n"+
				"- Total nodes: %d\n"+
				"- Total edges: %d\n"+
				"- Files indexed: %d\n",
			stats.TotalNodes, stats.TotalEdges, stats.FilesIndexed)

		if len(stats.NodesByType) > 0 {
			answer += "- Nodes by type:\n"
			for nt, cnt := range stats.NodesByType {
				answer += fmt.Sprintf("  - %s: %d\n", nt, cnt)
			}
		}

		return &QueryResult{
			Mode:   ModeDirectGraph,
			Answer: answer,
			GraphData: &DirectGraphData{
				Stats: &stats,
			},
		}, nil
	}

	// List queries.
	var nodes []*graph.Node
	for _, nt := range []struct {
		keyword  string
		nodeType graph.NodeType
	}{
		{"services", graph.NodeTypeService},
		{"functions", graph.NodeTypeFunction},
		{"packages", graph.NodeTypePackage},
		{"structs", graph.NodeTypeStruct},
		{"interfaces", graph.NodeTypeInterface},
		{"cloud", graph.NodeTypeCloudService},
	} {
		if strings.Contains(q, nt.keyword) {
			nodes = ql.index.GetByType(nt.nodeType)
			break
		}
	}

	if nodes == nil {
		// Default: return overall stats.
		stats := ql.index.Stats()
		return &QueryResult{
			Mode:   ModeDirectGraph,
			Answer: fmt.Sprintf("Found %d nodes and %d edges in the graph.", stats.TotalNodes, stats.TotalEdges),
			GraphData: &DirectGraphData{
				Stats: &stats,
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d nodes:\n", len(nodes)))
	limit := len(nodes)
	if limit > 50 {
		limit = 50
		sb.WriteString("(showing first 50)\n")
	}
	for _, n := range nodes[:limit] {
		sb.WriteString(fmt.Sprintf("- %s (id=%s", n.Name, n.ID))
		if n.FilePath != "" {
			sb.WriteString(fmt.Sprintf(", file=%s", n.FilePath))
		}
		sb.WriteString(")\n")
	}

	return &QueryResult{
		Mode:   ModeDirectGraph,
		Answer: sb.String(),
		GraphData: &DirectGraphData{
			Nodes: nodes,
		},
	}, nil
}

func (ql *QueryLayer) executeSubgraph(ctx context.Context, question string, decision QueryDecision) (*QueryResult, error) {
	if decision.TargetID == "" {
		// Couldn't resolve a target; fall back to agent.
		log.Printf("query: subgraph mode but no target found, falling back to agent")
		return ql.executeAgent(ctx, question, decision)
	}

	result, err := ql.extractor.Extract(ctx, SubgraphQuery{
		QueryType: decision.SubgraphType,
		TargetID:  decision.TargetID,
		Question:  question,
	})
	if err != nil {
		return nil, fmt.Errorf("query: subgraph extraction: %w", err)
	}

	// Generate AI explanation of the subgraph.
	var answer string
	if ql.provider != nil {
		var err2 error
		answer, err2 = ql.explainSubgraph(ctx, question, result)
		if err2 != nil {
			// Non-fatal: return subgraph without AI explanation.
			log.Printf("query: AI explanation failed: %v", err2)
			answer = ""
		}
	}
	if answer == "" {
		answer = fmt.Sprintf("Extracted %s subgraph with %d nodes and %d edges.",
			result.QueryType, len(result.Nodes), len(result.Edges))
	}

	return &QueryResult{
		Mode:     ModeSubgraph,
		Answer:   answer,
		Subgraph: result,
	}, nil
}

func (ql *QueryLayer) executeAgent(ctx context.Context, question string, decision QueryDecision) (*QueryResult, error) {
	if ql.provider == nil {
		// AI provider not configured — fall back to direct graph search.
		log.Printf("query: agent mode requested but no AI provider configured, falling back to direct_graph")
		return ql.executeDirectGraph(ctx, question, decision)
	}

	run, err := ql.agent.Run(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("query: agent run: %w", err)
	}

	return &QueryResult{
		Mode:     ModeAgent,
		Answer:   run.Answer,
		AgentRun: run,
	}, nil
}

func (ql *QueryLayer) executeSQLMode(ctx context.Context, question string, decision QueryDecision) (*QueryResult, error) {
	q := strings.ToLower(question)

	// Top failing nodes.
	if strings.Contains(q, "top failing") || strings.Contains(q, "most errors") {
		stats, err := ql.store.GetTopFailingNodes(ctx, 24*time.Hour, 10)
		if err != nil {
			return nil, fmt.Errorf("query: top failing: %w", err)
		}
		if len(stats) == 0 {
			return &QueryResult{
				Mode:   ModeSQL,
				Answer: "No failures recorded in the last 24 hours.",
			}, nil
		}

		var sb strings.Builder
		sb.WriteString("Top failing nodes (last 24h):\n")
		for i, s := range stats {
			sb.WriteString(fmt.Sprintf("%d. %s — %d failures (last: %s)\n",
				i+1, s.NodeName, s.FailureCount,
				s.LastFailure.Format("2006-01-02 15:04:05")))
		}
		return &QueryResult{
			Mode:   ModeSQL,
			Answer: sb.String(),
		}, nil
	}

	// Recent events for a target node.
	if decision.TargetID != "" && strings.Contains(q, "recent events") {
		events, err := ql.store.GetRecentEvents(ctx, decision.TargetID, 20)
		if err != nil {
			return nil, fmt.Errorf("query: recent events: %w", err)
		}
		if len(events) == 0 {
			return &QueryResult{
				Mode:   ModeSQL,
				Answer: "No recent events found for this node.",
			}, nil
		}

		var sb strings.Builder
		sb.WriteString("Recent events:\n")
		for _, e := range events {
			sb.WriteString(fmt.Sprintf("- [%s] %s status=%s",
				e.Timestamp.Format("15:04:05"), e.EventType, e.Status))
			if e.ErrorMessage != "" {
				sb.WriteString(fmt.Sprintf(" error=%q", e.ErrorMessage))
			}
			sb.WriteString("\n")
		}
		return &QueryResult{
			Mode:   ModeSQL,
			Answer: sb.String(),
		}, nil
	}

	// Fallback to agent for SQL-like queries we can't handle directly.
	return ql.executeAgent(ctx, question, decision)
}

// ---------------------------------------------------------------------------
// AI explanation for subgraphs
// ---------------------------------------------------------------------------

func (ql *QueryLayer) explainSubgraph(ctx context.Context, question string, result *SubgraphResult) (string, error) {
	if ql.provider == nil {
		return "", fmt.Errorf("no AI provider configured")
	}

	// Build context for the LLM.
	var context strings.Builder
	context.WriteString(fmt.Sprintf("User question: %s\n\n", question))
	context.WriteString(fmt.Sprintf("Subgraph type: %s\n", result.QueryType))
	context.WriteString(fmt.Sprintf("Target: %s\n", result.TargetID))
	context.WriteString(fmt.Sprintf("Nodes: %d, Edges: %d\n\n", len(result.Nodes), len(result.Edges)))

	// Include the subgraph summary.
	context.WriteString(formatSubgraphResult(result))

	messages := ai.BuildConversation(
		ai.AgentSystemPrompt(),
		ai.Message{
			Role:    ai.RoleUser,
			Content: context.String(),
		},
	)

	opts := ai.DefaultGenerateOptions()
	opts.MaxTokens = 2048

	resp, err := ql.provider.Generate(ctx, messages, opts)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ---------------------------------------------------------------------------
// ExplainFunction — convenience for single-function explanation
// ---------------------------------------------------------------------------

// ExplainFunction generates an AI-powered explanation of a function node.
func (ql *QueryLayer) ExplainFunction(ctx context.Context, nodeID string) (string, error) {
	node, ok := ql.index.GetNode(nodeID)
	if !ok {
		return "", fmt.Errorf("query: node %q not found", nodeID)
	}

	callers := ql.index.GetCallers(nodeID)
	callees := ql.index.GetCallees(nodeID)
	messages := ai.ExplainFunctionPrompt(node, callers, callees)

	opts := ai.DefaultGenerateOptions()
	resp, err := ql.provider.Generate(ctx, messages, opts)
	if err != nil {
		return "", fmt.Errorf("query: explain function: %w", err)
	}
	return resp.Content, nil
}

// ---------------------------------------------------------------------------
// WhyFailing — convenience for failure diagnosis
// ---------------------------------------------------------------------------

// WhyFailing generates an AI-powered diagnosis of why a node is failing.
func (ql *QueryLayer) WhyFailing(ctx context.Context, nodeID string) (string, *SubgraphResult, error) {
	node, ok := ql.index.GetNode(nodeID)
	if !ok {
		return "", nil, fmt.Errorf("query: node %q not found", nodeID)
	}

	// Get failure events.
	events, err := ql.store.GetFailuresByNode(ctx, nodeID, 20)
	if err != nil {
		return "", nil, fmt.Errorf("query: get failures: %w", err)
	}

	// Build failure subgraph.
	subgraph, err := ql.extractor.Extract(ctx, SubgraphQuery{
		QueryType: QueryFailurePath,
		TargetID:  nodeID,
	})
	if err != nil {
		log.Printf("query: failure subgraph: %v", err)
	}

	// Convert events to storage.RuntimeEvent for the prompt.
	storageEvents := make([]storage.RuntimeEvent, len(events))
	for i, e := range events {
		storageEvents[i] = *e
	}

	messages := ai.WhyFailingPrompt(node, storageEvents)
	opts := ai.DefaultGenerateOptions()
	opts.MaxTokens = 2048

	resp, err := ql.provider.Generate(ctx, messages, opts)
	if err != nil {
		return "", subgraph, fmt.Errorf("query: AI diagnosis: %w", err)
	}
	return resp.Content, subgraph, nil
}

// ---------------------------------------------------------------------------
// GetSubgraph — direct access
// ---------------------------------------------------------------------------

// GetSubgraph extracts a subgraph without AI explanation.
func (ql *QueryLayer) GetSubgraph(ctx context.Context, q SubgraphQuery) (*SubgraphResult, error) {
	return ql.extractor.Extract(ctx, q)
}

// ---------------------------------------------------------------------------
// SimilaritySearch — semantic code search
// ---------------------------------------------------------------------------

// SimilaritySearch finds nodes semantically similar to the query.
func (ql *QueryLayer) SimilaritySearch(ctx context.Context, query string, topK int) ([]ai.SimilarityResult, error) {
	if ql.embeddings == nil {
		return nil, fmt.Errorf("query: embedding service not configured")
	}
	return ql.embeddings.SimilaritySearch(ctx, query, topK)
}

// ---------------------------------------------------------------------------
// Broadcast helper
// ---------------------------------------------------------------------------

func (ql *QueryLayer) broadcastProgress(event string, data interface{}) {
	if ql.broadcaster == nil {
		return
	}
	ql.broadcaster.Broadcast(ai.BroadcastEvent{
		Event: event,
		Data:  data,
	})
}

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

func jsonMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
