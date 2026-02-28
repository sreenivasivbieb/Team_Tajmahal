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
// Agent
// ---------------------------------------------------------------------------

const maxAgentSteps = 6

// AgentTool wraps an ai.Tool with its execution function.
type AgentTool struct {
	Definition ai.Tool
	Execute    func(ctx context.Context, args string) (string, error)
}

// AgentStep records a single iteration of the tool-use loop.
type AgentStep struct {
	StepNum   int           `json:"step"`
	ToolCalls []ai.ToolCall `json:"tool_calls,omitempty"`
	Results   []string      `json:"results,omitempty"`
	Reasoning string        `json:"reasoning,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// AgentRun holds the complete result of an agent execution.
type AgentRun struct {
	Question string      `json:"question"`
	Answer   string      `json:"answer"`
	Steps    []AgentStep `json:"steps"`
	Duration time.Duration `json:"duration_ms"`
}

// VyuhaAgent is the AI-powered agent that can answer questions about the
// codebase using tool calls backed by the graph, storage, and subgraph layers.
type VyuhaAgent struct {
	provider    ai.Provider
	index       *graph.GraphIndex
	store       *storage.Storage
	extractor   *SubgraphExtractor
	broadcaster ai.Broadcaster
	tools       []AgentTool
}

// NewVyuhaAgent creates an agent wired to all required dependencies.
func NewVyuhaAgent(
	provider ai.Provider,
	index *graph.GraphIndex,
	store *storage.Storage,
	extractor *SubgraphExtractor,
	broadcaster ai.Broadcaster,
) *VyuhaAgent {
	a := &VyuhaAgent{
		provider:    provider,
		index:       index,
		store:       store,
		extractor:   extractor,
		broadcaster: broadcaster,
	}
	a.tools = a.buildTools()
	return a
}

// ---------------------------------------------------------------------------
// Run — main agent loop
// ---------------------------------------------------------------------------

// Run executes the agent loop for the given question. It sends SSE progress
// events via the broadcaster and returns the full AgentRun.
func (a *VyuhaAgent) Run(ctx context.Context, question string) (*AgentRun, error) {
	start := time.Now()

	a.broadcast("agent_start", map[string]string{
		"question": question,
	})

	// Build initial conversation.
	messages := ai.BuildConversation(ai.AgentSystemPrompt(),
		ai.Message{Role: ai.RoleUser, Content: question},
	)

	// Convert tools to ai.Tool slice.
	aiTools := make([]ai.Tool, len(a.tools))
	for i, t := range a.tools {
		aiTools[i] = t.Definition
	}

	opts := ai.DefaultGenerateOptions()
	opts.MaxTokens = 4096
	opts.Temperature = 0.2

	var steps []AgentStep

	for step := 0; step < maxAgentSteps; step++ {
		a.broadcast("agent_step", map[string]interface{}{
			"step":  step + 1,
			"total": maxAgentSteps,
		})

		resp, err := a.provider.GenerateWithTools(ctx, messages, aiTools, opts)
		if err != nil {
			a.broadcast("agent_error", map[string]string{"error": err.Error()})
			return nil, fmt.Errorf("query/agent: step %d generate error: %w", step+1, err)
		}

		// If there are no tool calls, the model has produced a final answer.
		if len(resp.ToolCalls) == 0 {
			steps = append(steps, AgentStep{
				StepNum:   step + 1,
				Reasoning: resp.Content,
				Timestamp: time.Now(),
			})

			run := &AgentRun{
				Question: question,
				Answer:   resp.Content,
				Steps:    steps,
				Duration: time.Since(start),
			}

			a.broadcast("agent_done", map[string]interface{}{
				"answer": resp.Content,
				"steps":  len(steps),
			})

			return run, nil
		}

		// Execute each tool call.
		stepRecord := AgentStep{
			StepNum:   step + 1,
			ToolCalls: resp.ToolCalls,
			Timestamp: time.Now(),
		}

		// Append assistant message with tool calls.
		messages = append(messages, *resp)

		for _, tc := range resp.ToolCalls {
			a.broadcast("agent_tool_call", map[string]string{
				"tool": tc.Name,
				"args": tc.Arguments,
			})

			result, execErr := a.executeTool(ctx, tc)
			if execErr != nil {
				result = fmt.Sprintf("Error: %v", execErr)
			}

			stepRecord.Results = append(stepRecord.Results, result)

			// Append tool response message.
			messages = append(messages, ai.Message{
				Role:       ai.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})

			a.broadcast("agent_tool_result", map[string]interface{}{
				"tool":   tc.Name,
				"length": len(result),
			})
		}

		steps = append(steps, stepRecord)
	}

	// If we exhausted steps, ask for a final summary.
	messages = append(messages, ai.Message{
		Role:    ai.RoleUser,
		Content: "You've used all available tool steps. Please provide your best answer with the information gathered so far.",
	})

	resp, err := a.provider.Generate(ctx, messages, opts)
	if err != nil {
		return nil, fmt.Errorf("query/agent: final summary error: %w", err)
	}

	run := &AgentRun{
		Question: question,
		Answer:   resp.Content,
		Steps:    steps,
		Duration: time.Since(start),
	}

	a.broadcast("agent_done", map[string]interface{}{
		"answer": resp.Content,
		"steps":  len(steps),
	})

	return run, nil
}

// ---------------------------------------------------------------------------
// Tool execution
// ---------------------------------------------------------------------------

func (a *VyuhaAgent) executeTool(ctx context.Context, tc ai.ToolCall) (string, error) {
	for _, t := range a.tools {
		if t.Definition.Name == tc.Name {
			return t.Execute(ctx, tc.Arguments)
		}
	}
	return "", fmt.Errorf("unknown tool %q", tc.Name)
}

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

func (a *VyuhaAgent) buildTools() []AgentTool {
	return []AgentTool{
		a.toolFindService(),
		a.toolGetServiceOverview(),
		a.toolGetCallChain(),
		a.toolFindFunctions(),
		a.toolGetFailures(),
		a.toolGetDependencies(),
		a.toolGetDependencyImpact(),
	}
}

// -- find_service -----------------------------------------------------------

func (a *VyuhaAgent) toolFindService() AgentTool {
	return AgentTool{
		Definition: ai.Tool{
			Name:        "find_service",
			Description: "Search for a service node by name. Returns matching service nodes.",
			Parameters:  mustJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Name or partial name of the service to find",
					},
				},
				"required": []string{"name"},
			}),
		},
		Execute: func(ctx context.Context, args string) (string, error) {
			var p struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}

			services := a.index.GetByType(graph.NodeTypeService)
			var matches []*graph.Node
			query := strings.ToLower(p.Name)
			for _, svc := range services {
				if strings.Contains(strings.ToLower(svc.Name), query) {
					matches = append(matches, svc)
				}
			}
			if len(matches) == 0 {
				return "No services found matching: " + p.Name, nil
			}
			return formatNodes(matches), nil
		},
	}
}

// -- get_service_overview ---------------------------------------------------

func (a *VyuhaAgent) toolGetServiceOverview() AgentTool {
	return AgentTool{
		Definition: ai.Tool{
			Name:        "get_service_overview",
			Description: "Get a detailed overview of a service including its structure, entry points, and health status.",
			Parameters:  mustJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the service node",
					},
				},
				"required": []string{"service_id"},
			}),
		},
		Execute: func(ctx context.Context, args string) (string, error) {
			var p struct {
				ServiceID string `json:"service_id"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}

			result, err := a.extractor.Extract(ctx, SubgraphQuery{
				QueryType: QueryServiceOverview,
				TargetID:  p.ServiceID,
				MaxNodes:  20,
			})
			if err != nil {
				return "", err
			}
			return formatSubgraphResult(result), nil
		},
	}
}

// -- get_call_chain ---------------------------------------------------------

func (a *VyuhaAgent) toolGetCallChain() AgentTool {
	return AgentTool{
		Definition: ai.Tool{
			Name:        "get_call_chain",
			Description: "Trace the call chain for a function, showing callers, callees, and data flow.",
			Parameters:  mustJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"function_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the function node to trace",
					},
					"depth": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum call depth to follow (default 3)",
					},
				},
				"required": []string{"function_id"},
			}),
		},
		Execute: func(ctx context.Context, args string) (string, error) {
			var p struct {
				FunctionID string `json:"function_id"`
				Depth      int    `json:"depth"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			if p.Depth <= 0 {
				p.Depth = 3
			}

			result, err := a.extractor.Extract(ctx, SubgraphQuery{
				QueryType: QueryCallChain,
				TargetID:  p.FunctionID,
				MaxDepth:  p.Depth,
			})
			if err != nil {
				return "", err
			}
			return formatSubgraphResult(result), nil
		},
	}
}

// -- find_functions ---------------------------------------------------------

func (a *VyuhaAgent) toolFindFunctions() AgentTool {
	return AgentTool{
		Definition: ai.Tool{
			Name:        "find_functions",
			Description: "Search for functions by name. Returns matching function nodes with their files and signatures.",
			Parameters:  mustJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Name or partial name of the function to find",
					},
				},
				"required": []string{"name"},
			}),
		},
		Execute: func(ctx context.Context, args string) (string, error) {
			var p struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}

			functions := a.index.GetByType(graph.NodeTypeFunction)
			var matches []*graph.Node
			query := strings.ToLower(p.Name)
			for _, fn := range functions {
				if strings.Contains(strings.ToLower(fn.Name), query) {
					matches = append(matches, fn)
				}
			}
			if len(matches) > 20 {
				matches = matches[:20]
			}
			if len(matches) == 0 {
				return "No functions found matching: " + p.Name, nil
			}
			return formatNodes(matches), nil
		},
	}
}

// -- get_failures -----------------------------------------------------------

func (a *VyuhaAgent) toolGetFailures() AgentTool {
	return AgentTool{
		Definition: ai.Tool{
			Name:        "get_failures",
			Description: "Get runtime failures for a node, or list top failing nodes across the system.",
			Parameters:  mustJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"node_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the node to check (omit for system-wide top failures)",
					},
				},
			}),
		},
		Execute: func(ctx context.Context, args string) (string, error) {
			var p struct {
				NodeID string `json:"node_id"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}

			if p.NodeID == "" {
				// System-wide top failures.
				stats, err := a.store.GetTopFailingNodes(ctx, 24*time.Hour, 10)
				if err != nil {
					return "", err
				}
				if len(stats) == 0 {
					return "No failures in the last 24 hours.", nil
				}
				var sb strings.Builder
				sb.WriteString("Top failing nodes (last 24h):\n")
				for _, s := range stats {
					sb.WriteString(fmt.Sprintf("- %s (id=%s): %d failures, last=%s\n",
						s.NodeName, s.NodeID, s.FailureCount,
						s.LastFailure.Format("2006-01-02 15:04:05")))
				}
				return sb.String(), nil
			}

			events, err := a.store.GetFailuresByNode(ctx, p.NodeID, 20)
			if err != nil {
				return "", err
			}
			if len(events) == 0 {
				return "No failures found for node: " + p.NodeID, nil
			}
			var sb strings.Builder
			node, ok := a.index.GetNode(p.NodeID)
			if ok {
				sb.WriteString(fmt.Sprintf("Failures for %s (status=%s, total_errors=%d):\n",
					node.Name, node.RuntimeStatus, node.ErrorCount))
			}
			for _, e := range events {
				sb.WriteString(fmt.Sprintf("- [%s] %s status=%s",
					e.Timestamp.Format("15:04:05"), e.EventType, e.Status))
				if e.ErrorMessage != "" {
					sb.WriteString(fmt.Sprintf(" error=%q", e.ErrorMessage))
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	}
}

// -- get_dependencies -------------------------------------------------------

func (a *VyuhaAgent) toolGetDependencies() AgentTool {
	return AgentTool{
		Definition: ai.Tool{
			Name:        "get_dependencies",
			Description: "List external dependencies (packages, cloud services) for a node.",
			Parameters:  mustJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"node_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the node to check",
					},
				},
				"required": []string{"node_id"},
			}),
		},
		Execute: func(ctx context.Context, args string) (string, error) {
			var p struct {
				NodeID string `json:"node_id"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}

			deps := a.index.GetExternalDependencies(p.NodeID)
			if len(deps) == 0 {
				return "No external dependencies found for: " + p.NodeID, nil
			}
			return formatNodes(deps), nil
		},
	}
}

// -- get_dependency_impact --------------------------------------------------

func (a *VyuhaAgent) toolGetDependencyImpact() AgentTool {
	return AgentTool{
		Definition: ai.Tool{
			Name:        "get_dependency_impact",
			Description: "Analyze the blast radius if a given node were to fail. Shows which services and functions depend on it.",
			Parameters:  mustJSON(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"node_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the node to analyze impact for",
					},
				},
				"required": []string{"node_id"},
			}),
		},
		Execute: func(ctx context.Context, args string) (string, error) {
			var p struct {
				NodeID string `json:"node_id"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}

			result, err := a.extractor.Extract(ctx, SubgraphQuery{
				QueryType: QueryDependencyImpact,
				TargetID:  p.NodeID,
				MaxNodes:  25,
			})
			if err != nil {
				return "", err
			}
			return formatSubgraphResult(result), nil
		},
	}
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

func formatNodes(nodes []*graph.Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("- %s (id=%s, type=%s", n.Name, n.ID, n.Type))
		if n.FilePath != "" {
			sb.WriteString(fmt.Sprintf(", file=%s", n.FilePath))
		}
		if n.RuntimeStatus != "" {
			sb.WriteString(fmt.Sprintf(", status=%s", n.RuntimeStatus))
		}
		if n.ErrorCount > 0 {
			sb.WriteString(fmt.Sprintf(", errors=%d", n.ErrorCount))
		}
		if n.Metadata.Signature != "" {
			sb.WriteString(fmt.Sprintf(", sig=%s", n.Metadata.Signature))
		}
		sb.WriteString(")\n")
	}
	return sb.String()
}

func formatSubgraphResult(r *SubgraphResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Subgraph [%s] for target %s:\n", r.QueryType, r.TargetID))
	sb.WriteString(fmt.Sprintf("Nodes: %d, Edges: %d\n", len(r.Nodes), len(r.Edges)))

	if len(r.EntryPoints) > 0 {
		sb.WriteString(fmt.Sprintf("Entry points: %s\n", strings.Join(r.EntryPoints, ", ")))
	}
	if len(r.CriticalPath) > 0 {
		sb.WriteString(fmt.Sprintf("Critical path: %s\n", strings.Join(r.CriticalPath, " → ")))
	}

	sb.WriteString("\nNodes:\n")
	for _, n := range r.Nodes {
		role := r.NodeRoles[n.ID]
		sb.WriteString(fmt.Sprintf("  [%s] %s (type=%s, role=%s",
			n.ID, n.Name, n.Type, role))
		if n.RuntimeStatus != "" {
			sb.WriteString(fmt.Sprintf(", status=%s", n.RuntimeStatus))
		}
		if n.ErrorCount > 0 {
			sb.WriteString(fmt.Sprintf(", errors=%d", n.ErrorCount))
		}
		sb.WriteString(")\n")
	}

	sb.WriteString("\nEdges:\n")
	for _, e := range r.Edges {
		sb.WriteString(fmt.Sprintf("  %s -[%s]-> %s\n", e.SourceID, e.Type, e.TargetID))
	}

	return sb.String()
}

func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		log.Fatalf("query/agent: mustJSON: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// broadcast helper
// ---------------------------------------------------------------------------

func (a *VyuhaAgent) broadcast(event string, data interface{}) {
	if a.broadcaster == nil {
		return
	}
	a.broadcaster.Broadcast(ai.BroadcastEvent{
		Event: event,
		Data:  data,
	})
}
