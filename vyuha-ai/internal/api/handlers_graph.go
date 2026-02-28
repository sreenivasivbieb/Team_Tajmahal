package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/query"
)

// ---------------------------------------------------------------------------
// GET /api/graph/services
// ---------------------------------------------------------------------------

func (s *Server) handleGraphServices(w http.ResponseWriter, r *http.Request) {
	services := s.index.GetByType(graph.NodeTypeService)

	type serviceView struct {
		*graph.Node
		ErrorCount    int    `json:"error_count"`
		RuntimeStatus string `json:"runtime_status"`
	}

	views := make([]serviceView, 0, len(services))
	for _, svc := range services {
		views = append(views, serviceView{
			Node:          svc,
			ErrorCount:    svc.ErrorCount,
			RuntimeStatus: svc.RuntimeStatus,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"services": views,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/graph/children?parent_id=X&depth=N
// ---------------------------------------------------------------------------

func (s *Server) handleGraphChildren(w http.ResponseWriter, r *http.Request) {
	parentID := r.URL.Query().Get("parent_id")
	if parentID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_PARENT_ID",
			"parent_id query parameter is required")
		return
	}

	// Validate parent exists.
	if _, ok := s.index.GetNode(parentID); !ok {
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND",
			"parent node not found")
		return
	}

	depth := 1
	if d := r.URL.Query().Get("depth"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v >= 1 {
			depth = v
		}
	}
	if depth > 5 {
		depth = 5
	}

	var nodes []*graph.Node
	if depth == 1 {
		nodes = s.index.GetChildren(parentID)
	} else {
		nodes = s.index.GetDescendants(parentID, depth)
	}

	// Collect node IDs for edge lookup.
	nodeIDs := make([]string, 0, len(nodes)+1)
	nodeIDs = append(nodeIDs, parentID)
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}

	edges := s.index.GetEdgesBetween(nodeIDs)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"nodes": nodes,
			"edges": edges,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/graph/subgraph?query=X&target_id=Y&type=Z
// ---------------------------------------------------------------------------

func (s *Server) handleGraphSubgraph(w http.ResponseWriter, r *http.Request) {
	// Query type — default service_overview, validate against known values.
	qt := query.SubgraphQueryType(r.URL.Query().Get("type"))
	if qt == "" {
		qt = query.QueryServiceOverview
	}
	if !isValidSubgraphType(qt) {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY_TYPE",
			"type must be one of: service_overview, call_chain, failure_path, data_lineage, dependency_impact")
		return
	}

	// Target node ID from query parameter.
	targetID := r.URL.Query().Get("target_id")
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_TARGET_ID",
			"target_id query parameter is required")
		return
	}

	// Max nodes — default 30, clamped to [5, 100].
	maxNodes := 30
	if v := r.URL.Query().Get("max_nodes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxNodes = n
		}
	}
	maxNodes = clampInt(maxNodes, 5, 100)

	// Max depth — default 4, clamped to [1, 8].
	maxDepth := 4
	if v := r.URL.Query().Get("max_depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxDepth = n
		}
	}
	maxDepth = clampInt(maxDepth, 1, 8)

	// QueryLayer must be available.
	if s.queryLayer == nil {
		writeError(w, http.StatusServiceUnavailable, "QUERY_LAYER_UNAVAILABLE",
			"query layer not initialized")
		return
	}

	result, err := s.queryLayer.GetSubgraph(r.Context(), query.SubgraphQuery{
		QueryType: qt,
		TargetID:  targetID,
		MaxNodes:  maxNodes,
		MaxDepth:  maxDepth,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SUBGRAPH_ERROR",
			"subgraph extraction failed: "+err.Error())
		return
	}
	if result == nil {
		writeError(w, http.StatusNotFound, "SUBGRAPH_NOT_FOUND",
			"subgraph not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": result,
	})
}

// ---------------------------------------------------------------------------
// GET /api/graph/node/:id
// ---------------------------------------------------------------------------

func (s *Server) handleGraphNode(w http.ResponseWriter, r *http.Request) {
	// Extract node ID from path: /api/graph/node/{id}
	nodeID := extractPathParam(r.URL.Path, "/api/graph/node/")
	if nodeID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_NODE_ID",
			"node ID is required in the URL path")
		return
	}

	node, ok := s.index.GetNode(nodeID)
	if !ok {
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND",
			"node not found")
		return
	}

	// Direct children.
	children := s.index.GetChildren(nodeID)

	// In-edges (who calls/targets this).
	inEdges := s.index.GetInEdges(nodeID)

	// Out-edges (what this calls/targets).
	outEdges := s.index.GetOutEdges(nodeID)

	// Data flow — only for function nodes.
	var dataFlow interface{}
	if node.Type == graph.NodeTypeFunction {
		ctx := r.Context()
		records, err := s.store.GetDataFlow(ctx, nodeID)
		if err == nil && len(records) > 0 {
			dataFlow = records
		}
	}

	// Recent runtime events (last 10).
	ctx := r.Context()
	events, _ := s.store.GetRecentEvents(ctx, nodeID, 10)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"node":      node,
			"children":  children,
			"in_edges":  inEdges,
			"out_edges": outEdges,
			"data_flow": dataFlow,
			"events":    events,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/graph/stats
// ---------------------------------------------------------------------------

func (s *Server) handleGraphStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := s.store.GetGraphStats(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATS_ERROR",
			"failed to get graph stats: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"nodes_by_type":  stats.NodesByType,
			"edges_by_type":  stats.EdgesByType,
			"total_nodes":    stats.TotalNodes,
			"total_edges":    stats.TotalEdges,
			"failing_nodes":  stats.NodesWithErrors,
			"total_events":   stats.TotalRuntime,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/graph/search?q=X&type=Y&limit=N
// ---------------------------------------------------------------------------

func (s *Server) handleGraphSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUERY",
			"q query parameter is required")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v >= 1 {
			limit = v
		}
	}
	if limit > 100 {
		limit = 100
	}

	nodeType := r.URL.Query().Get("type")

	ctx := r.Context()
	nodes, err := s.store.SearchNodes(ctx, q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SEARCH_ERROR",
			"search failed: "+err.Error())
		return
	}

	// Apply optional type filter.
	if nodeType != "" {
		filtered := make([]*graph.Node, 0, len(nodes))
		for _, n := range nodes {
			if string(n.Type) == nodeType {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"nodes": nodes,
			"total": len(nodes),
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// clampInt restricts val to the range [min, max].
func clampInt(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// isValidSubgraphType returns true if t is one of the 5 known query types.
func isValidSubgraphType(t query.SubgraphQueryType) bool {
	switch t {
	case query.QueryServiceOverview,
		query.QueryCallChain,
		query.QueryFailurePath,
		query.QueryDataLineage,
		query.QueryDependencyImpact:
		return true
	}
	return false
}

// extractPathParam extracts the path suffix after a given prefix.
// For example, extractPathParam("/api/graph/node/func:pkg:Foo", "/api/graph/node/")
// returns "func:pkg:Foo".
func extractPathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}
