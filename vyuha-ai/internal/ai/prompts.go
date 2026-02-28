package ai

import (
	"fmt"
	"strings"

	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// ---------------------------------------------------------------------------
// Prompt templates
// ---------------------------------------------------------------------------

// ExplainFunctionPrompt builds a prompt that asks the LLM to explain what
// a function does, including its parameters, return types, and relationships.
func ExplainFunctionPrompt(node *graph.Node, callers, callees []*graph.Node) []Message {
	var context strings.Builder
	context.WriteString(fmt.Sprintf("Function: %s\n", node.Name))
	if node.FilePath != "" {
		context.WriteString(fmt.Sprintf("File: %s (L%d–L%d)\n", node.FilePath, node.LineStart, node.LineEnd))
	}
	m := &node.Metadata
	if m.Signature != "" {
		context.WriteString(fmt.Sprintf("Signature: %s\n", m.Signature))
	}
	if m.Receiver != "" {
		context.WriteString(fmt.Sprintf("Receiver: %s\n", m.Receiver))
	}
	if m.DocComment != "" {
		context.WriteString(fmt.Sprintf("Doc: %s\n", m.DocComment))
	}
	if len(m.Parameters) > 0 {
		params := make([]string, len(m.Parameters))
		for i, p := range m.Parameters {
			params[i] = p.Name + " " + p.Type
		}
		context.WriteString(fmt.Sprintf("Parameters: %s\n", strings.Join(params, ", ")))
	}
	if len(m.ReturnTypes) > 0 {
		context.WriteString(fmt.Sprintf("Returns: %s\n", strings.Join(m.ReturnTypes, ", ")))
	}
	if m.HasErrorReturn {
		context.WriteString("Returns error: yes\n")
	}
	if m.HasContextParam {
		context.WriteString("Accepts context.Context: yes\n")
	}
	if m.CyclomaticComplexity > 0 {
		context.WriteString(fmt.Sprintf("Cyclomatic complexity: %d\n", m.CyclomaticComplexity))
	}
	if m.SourceSnippet != "" {
		context.WriteString(fmt.Sprintf("\nSource code:\n```go\n%s\n```\n", m.SourceSnippet))
	}

	if len(callers) > 0 {
		names := nodeNames(callers)
		context.WriteString(fmt.Sprintf("\nCalled by: %s\n", strings.Join(names, ", ")))
	}
	if len(callees) > 0 {
		names := nodeNames(callees)
		context.WriteString(fmt.Sprintf("Calls: %s\n", strings.Join(names, ", ")))
	}

	return BuildConversation(
		"You are an expert Go developer. Explain the function precisely "+
			"and concisely. Mention key behaviors, error handling patterns, "+
			"and how it relates to its callers and callees.",
		Message{
			Role:    RoleUser,
			Content: context.String(),
		},
	)
}

// WhyFailingPrompt builds a prompt to diagnose why a node is experiencing
// failures based on runtime events.
func WhyFailingPrompt(node *graph.Node, events []storage.RuntimeEvent) []Message {
	var context strings.Builder
	context.WriteString(fmt.Sprintf("Node: %s (type=%s, status=%s, errors=%d)\n",
		node.Name, node.Type, node.RuntimeStatus, node.ErrorCount))
	if node.FilePath != "" {
		context.WriteString(fmt.Sprintf("File: %s\n", node.FilePath))
	}

	if len(events) > 0 {
		context.WriteString("\nRecent runtime events:\n")
		limit := len(events)
		if limit > 20 {
			limit = 20
		}
		for _, e := range events[:limit] {
			context.WriteString(fmt.Sprintf("- [%s] %s status=%s",
				e.Timestamp.Format("2006-01-02 15:04:05"), e.EventType, e.Status))
			if e.ErrorMessage != "" {
				context.WriteString(fmt.Sprintf(" error=%q", e.ErrorMessage))
			}
			if e.ErrorCode != "" {
				context.WriteString(fmt.Sprintf(" code=%s", e.ErrorCode))
			}
			if e.LatencyMs > 0 {
				context.WriteString(fmt.Sprintf(" latency=%dms", e.LatencyMs))
			}
			context.WriteString("\n")
		}
	}

	return BuildConversation(
		"You are a senior SRE. Analyze the failing node and its recent "+
			"runtime events. Identify the root cause, explain the failure "+
			"pattern, and suggest actionable remediation steps.",
		Message{
			Role:    RoleUser,
			Content: context.String(),
		},
	)
}

// ServiceOverviewPrompt builds a prompt to explain a service and its architecture.
func ServiceOverviewPrompt(
	service *graph.Node,
	children []*graph.Node,
	externalDeps []string,
	stats *graph.IndexStats,
) []Message {
	var context strings.Builder
	context.WriteString(fmt.Sprintf("Service: %s\n", service.Name))
	m := &service.Metadata
	if m.EntryPoint != "" {
		context.WriteString(fmt.Sprintf("Entry point: %s\n", m.EntryPoint))
	}
	if m.PackageCount > 0 {
		context.WriteString(fmt.Sprintf("Packages: %d\n", m.PackageCount))
	}
	if m.FunctionCount > 0 {
		context.WriteString(fmt.Sprintf("Functions: %d\n", m.FunctionCount))
	}
	if len(m.CloudDependencies) > 0 {
		context.WriteString(fmt.Sprintf("Cloud deps: %s\n", strings.Join(m.CloudDependencies, ", ")))
	}

	if len(children) > 0 {
		context.WriteString("\nChild nodes:\n")
		for _, c := range children {
			context.WriteString(fmt.Sprintf("- [%s] %s", c.Type, c.Name))
			if c.RuntimeStatus != "" {
				context.WriteString(fmt.Sprintf(" (status=%s)", c.RuntimeStatus))
			}
			context.WriteString("\n")
		}
	}

	if len(externalDeps) > 0 {
		context.WriteString(fmt.Sprintf("\nExternal dependencies: %s\n",
			strings.Join(externalDeps, ", ")))
	}

	if stats != nil {
		context.WriteString(fmt.Sprintf("\nGraph stats: %d nodes, %d edges, %d files\n",
			stats.TotalNodes, stats.TotalEdges, stats.FilesIndexed))
	}

	return BuildConversation(
		"You are a software architect. Provide a concise overview of this "+
			"service: its purpose, architecture, key components, external "+
			"dependencies, and any concerns about its structure or health.",
		Message{
			Role:    RoleUser,
			Content: context.String(),
		},
	)
}

// DataLineagePrompt builds a prompt to explain the data flow through
// a function and its downstream effects.
func DataLineagePrompt(
	node *graph.Node,
	flows []storage.DataFlowRecord,
	downstream []*graph.Node,
) []Message {
	var context strings.Builder
	context.WriteString(fmt.Sprintf("Function: %s\n", node.Name))
	if node.FilePath != "" {
		context.WriteString(fmt.Sprintf("File: %s\n", node.FilePath))
	}
	if node.Metadata.Signature != "" {
		context.WriteString(fmt.Sprintf("Signature: %s\n", node.Metadata.Signature))
	}

	if len(flows) > 0 {
		context.WriteString("\nData flow records:\n")
		for _, f := range flows {
			context.WriteString(fmt.Sprintf("- kind=%s type=%s", f.Kind, f.TypeName))
			if f.Source != "" {
				context.WriteString(fmt.Sprintf(" source=%s", f.Source))
			}
			if f.Sink != "" {
				context.WriteString(fmt.Sprintf(" sink=%s", f.Sink))
			}
			if f.IsAggregate {
				context.WriteString(" (aggregate)")
			}
			if f.FanIn > 1 {
				context.WriteString(fmt.Sprintf(" fan_in=%d", f.FanIn))
			}
			context.WriteString("\n")
		}
	}

	if len(downstream) > 0 {
		names := nodeNames(downstream)
		context.WriteString(fmt.Sprintf("\nDownstream consumers: %s\n", strings.Join(names, ", ")))
	}

	return BuildConversation(
		"You are a data engineer. Explain the data lineage through this "+
			"function: what data enters, how it is transformed, where it "+
			"goes, and any potential data quality or consistency concerns.",
		Message{
			Role:    RoleUser,
			Content: context.String(),
		},
	)
}

// AgentSystemPrompt returns the system prompt for the VYUHA AI assistant
// that powers interactive questions about the codebase.
func AgentSystemPrompt() string {
	return `You are VYUHA AI, an expert code intelligence assistant.
You have deep knowledge of the user's codebase represented as a graph of
nodes (repositories, services, packages, files, functions, structs,
interfaces, cloud services, runtime instances, and data flows) connected
by typed edges (contains, imports, calls, implements, depends_on,
connects_to, runtime_calls, failed_at, produces_to, consumed_by,
transforms, field_map).

When the user asks about their code, you can:
1. Explain functions, structs, and interfaces in detail.
2. Trace call chains and data flow paths.
3. Diagnose runtime failures using event data.
4. Identify architectural patterns and anti-patterns.
5. Suggest improvements for reliability, performance, and maintainability.

Be precise, cite specific node names and file paths when available, and
keep explanations concise but thorough. If you need more context, ask
the user or use available tools to look up nodes and edges.`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nodeNames(nodes []*graph.Node) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Name
	}
	return names
}
