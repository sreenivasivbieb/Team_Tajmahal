// Bridge typed tool wrappers — Go-friendly interfaces for contextplus MCP tools
// Each method maps to a contextplus tool and returns structured Go data

package bridge

import (
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// Call Chain types (mirrors contextplus ChainNodeJSON / CallChainJSON)
// ---------------------------------------------------------------------------

// ChainNode is a node in the contextplus call chain tree.
type ChainNode struct {
	Name       string      `json:"name"`
	File       string      `json:"file"`
	Line       int         `json:"line"`
	EndLine    int         `json:"endLine"`
	Signature  string      `json:"signature"`
	IsDbHit    bool        `json:"isDbHit"`
	IsMarked   bool        `json:"isMarked"`
	IsExternal bool        `json:"isExternal"`
	IsCycle    bool        `json:"isCycle"`
	IsLeaf     bool        `json:"isLeaf"`
	Calls      []ChainNode `json:"calls"`
}

// ChainStats are summary numbers for a call chain.
type ChainStats struct {
	Total    int `json:"total"`
	Internal int `json:"internal"`
	Leaves   int `json:"leaves"`
	DbHits   int `json:"dbHits"`
	Cycles   int `json:"cycles"`
}

// CallChainResult is the structured JSON from contextplus get_call_chain.
type CallChainResult struct {
	Root       ChainNode  `json:"root"`
	Stats      ChainStats `json:"stats"`
	SymbolName string     `json:"symbolName"`
	File       string     `json:"file"`
	Line       int        `json:"line"`
	EndLine    int        `json:"endLine"`
	Signature  string     `json:"signature"`
	MaxDepth   int        `json:"maxDepth"`
}

// ---------------------------------------------------------------------------
// Flattened graph types for frontend consumption
// ---------------------------------------------------------------------------

// FlatNode is a graph node suitable for React Flow rendering.
type FlatNode struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	FilePath    string            `json:"file_path"`
	Line        int               `json:"line,omitempty"`
	EndLine     int               `json:"end_line,omitempty"`
	Signature   string            `json:"signature,omitempty"`
	Annotations []string          `json:"annotations,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// FlatEdge is a graph edge suitable for React Flow rendering.
type FlatEdge struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Type     string `json:"type"`
}

// CallChainMeta preserves tree structure info for the frontend.
type CallChainMeta struct {
	RootID      string              `json:"root_id"`
	Depths      map[string]int      `json:"depths"`
	Annotations map[string][]string `json:"annotations"`
	Stats       CallChainFrontStats `json:"stats"`
}

// CallChainFrontStats is the frontend-friendly stats shape.
type CallChainFrontStats struct {
	TotalHops   int `json:"total_hops"`
	LeafCount   int `json:"leaf_count"`
	DBCount     int `json:"db_count"`
	CycleCount  int `json:"cycle_count"`
	CallerCount int `json:"caller_count"`
}

// CallChainResponse is the complete response sent to the frontend.
type CallChainResponse struct {
	QueryType     string         `json:"query_type"`
	TargetID      string         `json:"target_id"`
	Nodes         []FlatNode     `json:"nodes"`
	Edges         []FlatEdge     `json:"edges"`
	NodeRoles     map[string]string `json:"node_roles"`
	CallChainMeta *CallChainMeta `json:"call_chain_meta"`
}

// ---------------------------------------------------------------------------
// Tool: get_call_chain (JSON mode)
// ---------------------------------------------------------------------------

// GetCallChain invokes contextplus get_call_chain with format=json and
// returns both the raw structured result and a flattened frontend-ready response.
func (c *MCPClient) GetCallChain(symbolName string, filePath string, maxDepth int) (*CallChainResponse, error) {
	args := map[string]interface{}{
		"symbol_name": symbolName,
		"format":      "json",
	}
	if filePath != "" {
		args["file_path"] = filePath
	}
	if maxDepth > 0 {
		args["max_depth"] = maxDepth
	}

	text, err := c.CallTool("get_call_chain", args)
	if err != nil {
		return nil, err
	}

	// Try parsing as JSON; if it fails, the tool returned an error message.
	var result CallChainResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("call_chain: %s", text)
	}

	return flattenCallChain(&result), nil
}

// flattenCallChain converts a ChainNode tree into flat nodes, edges, and metadata.
func flattenCallChain(result *CallChainResult) *CallChainResponse {
	nodes := make([]FlatNode, 0)
	edges := make([]FlatEdge, 0)
	depths := make(map[string]int)
	annotations := make(map[string][]string)
	nodeRoles := make(map[string]string)
	seen := make(map[string]bool)

	rootID := nodeID(result.Root.File, result.Root.Name, result.Root.Line)

	var walk func(node *ChainNode, depth int, parentID string)
	walk = func(node *ChainNode, depth int, parentID string) {
		id := nodeID(node.File, node.Name, node.Line)
		if seen[id] {
			// Still add the edge for cycles
			if parentID != "" {
				edges = append(edges, FlatEdge{
					ID:       fmt.Sprintf("%s-calls-%s", parentID, id),
					SourceID: parentID,
					TargetID: id,
					Type:     "calls",
				})
			}
			return
		}
		seen[id] = true

		// Build annotations list
		var annots []string
		if node.IsLeaf {
			annots = append(annots, "leaf")
		}
		if node.IsDbHit {
			annots = append(annots, "db")
		}
		if node.IsCycle {
			annots = append(annots, "cycle")
		}
		if node.IsExternal {
			annots = append(annots, "external")
		}
		if node.IsMarked {
			annots = append(annots, "marked")
		}

		// Determine role
		role := "callee"
		if id == rootID {
			role = "target"
		} else if node.IsExternal {
			role = "dependency"
		}

		nodeType := "function"
		if node.IsExternal {
			nodeType = "external"
		}

		nodes = append(nodes, FlatNode{
			ID:        id,
			Name:      node.Name,
			Type:      nodeType,
			FilePath:  node.File,
			Line:      node.Line,
			EndLine:   node.EndLine,
			Signature: node.Signature,
		})

		depths[id] = depth
		annotations[id] = annots
		nodeRoles[id] = role

		if parentID != "" {
			edges = append(edges, FlatEdge{
				ID:       fmt.Sprintf("%s-calls-%s", parentID, id),
				SourceID: parentID,
				TargetID: id,
				Type:     "calls",
			})
		}

		for i := range node.Calls {
			walk(&node.Calls[i], depth+1, id)
		}
	}

	walk(&result.Root, 0, "")

	maxDepth := 0
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}

	return &CallChainResponse{
		QueryType: "call_chain",
		TargetID:  rootID,
		Nodes:     nodes,
		Edges:     edges,
		NodeRoles: nodeRoles,
		CallChainMeta: &CallChainMeta{
			RootID:      rootID,
			Depths:      depths,
			Annotations: annotations,
			Stats: CallChainFrontStats{
				TotalHops:   maxDepth,
				LeafCount:   result.Stats.Leaves,
				DBCount:     result.Stats.DbHits,
				CycleCount:  result.Stats.Cycles,
				CallerCount: 0,
			},
		},
	}
}

// nodeID generates a deterministic ID from file + name + line.
func nodeID(file, name string, line int) string {
	if file == "" {
		return fmt.Sprintf("ext:%s", name)
	}
	return fmt.Sprintf("fn:%s:%s:%d", file, name, line)
}

// ---------------------------------------------------------------------------
// Tool: semantic_code_search
// ---------------------------------------------------------------------------

// SemanticSearch calls contextplus semantic_code_search and returns raw text.
func (c *MCPClient) SemanticSearch(query string, topK int) (string, error) {
	args := map[string]interface{}{
		"query": query,
	}
	if topK > 0 {
		args["top_k"] = topK
	}
	return c.CallTool("semantic_code_search", args)
}

// ---------------------------------------------------------------------------
// Tool: get_context_tree
// ---------------------------------------------------------------------------

// GetContextTree calls contextplus get_context_tree and returns raw text.
func (c *MCPClient) GetContextTree(targetPath string) (string, error) {
	args := map[string]interface{}{}
	if targetPath != "" {
		args["target_path"] = targetPath
	}
	return c.CallTool("get_context_tree", args)
}

// ---------------------------------------------------------------------------
// Tool: get_file_skeleton
// ---------------------------------------------------------------------------

// GetFileSkeleton calls contextplus get_file_skeleton and returns raw text.
func (c *MCPClient) GetFileSkeleton(filePath string) (string, error) {
	return c.CallTool("get_file_skeleton", map[string]interface{}{
		"file_path": filePath,
	})
}

// ---------------------------------------------------------------------------
// Tool: get_blast_radius
// ---------------------------------------------------------------------------

// GetBlastRadius calls contextplus get_blast_radius and returns raw text.
func (c *MCPClient) GetBlastRadius(symbolName, fileContext string) (string, error) {
	args := map[string]interface{}{
		"symbol_name": symbolName,
	}
	if fileContext != "" {
		args["file_context"] = fileContext
	}
	return c.CallTool("get_blast_radius", args)
}

// ---------------------------------------------------------------------------
// Tool: semantic_identifier_search
// ---------------------------------------------------------------------------

// SemanticIdentifierSearch calls contextplus semantic_identifier_search.
func (c *MCPClient) SemanticIdentifierSearch(query string, topK int) (string, error) {
	args := map[string]interface{}{
		"query": query,
	}
	if topK > 0 {
		args["top_k"] = topK
	}
	return c.CallTool("semantic_identifier_search", args)
}

// ---------------------------------------------------------------------------
// Tool: run_static_analysis
// ---------------------------------------------------------------------------

// RunStaticAnalysis calls contextplus run_static_analysis.
func (c *MCPClient) RunStaticAnalysis(targetPath string) (string, error) {
	args := map[string]interface{}{}
	if targetPath != "" {
		args["target_path"] = targetPath
	}
	return c.CallTool("run_static_analysis", args)
}

// ---------------------------------------------------------------------------
// Tool: ask_question (agentic RAG via Groq)
// ---------------------------------------------------------------------------

// AskQuestion calls contextplus ask_question (agentic RAG).
// It sends a natural-language question and returns the LLM-synthesised answer.
// When rootDir is non-empty, the RAG session targets that specific repo directory
// instead of the global root, ensuring tools only access the correct codebase.
func (c *MCPClient) AskQuestion(question, rootDir string) (string, error) {
	args := map[string]interface{}{
		"question": question,
	}
	if rootDir != "" {
		args["root_dir"] = rootDir
	}
	return c.CallTool("ask_question", args)
}

// PrimeSearchIndex forces a semantic search index build for the given directory
// by issuing a cheap search query. This ensures embeddings are pre-computed
// so later RAG queries can access them instantly without a cold-start rebuild.
func (c *MCPClient) PrimeSearchIndex(rootDir string) error {
	args := map[string]interface{}{
		"query": "main entry point",
		"top_k": 1,
	}
	// Note: semantic_code_search uses the rootDir that contextplus was started with.
	// For cloned repos we rely on the embedding tracker to pick up new files,
	// or we call ask_question with root_dir override which builds its own index.
	_, err := c.CallTool("semantic_code_search", args)
	return err
}
