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

	// For service nodes, also include descendants from the corresponding
	// package node. The parser creates service: and pkg: nodes as siblings
	// under the repo; functions live under pkg → file → func.
	var pkgID string
	if strings.HasPrefix(parentID, "service:") {
		pkgID = "pkg:" + strings.TrimPrefix(parentID, "service:")
		if _, ok := s.index.GetNode(pkgID); ok {
			pkgDescendants := s.index.GetDescendants(pkgID, 4)
			seen := make(map[string]bool, len(nodes))
			for _, n := range nodes {
				seen[n.ID] = true
			}
			for _, n := range pkgDescendants {
				if !seen[n.ID] {
					nodes = append(nodes, n)
					seen[n.ID] = true
				}
			}
		} else {
			pkgID = "" // pkg node doesn't exist
		}
	}

	// Collect node IDs for edge lookup — include pkgID so we get
	// the contains edges from pkg → file.
	nodeIDs := make([]string, 0, len(nodes)+2)
	nodeIDs = append(nodeIDs, parentID)
	if pkgID != "" {
		nodeIDs = append(nodeIDs, pkgID)
	}
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}

	edges := s.index.GetEdgesBetween(nodeIDs)

	// Remap edges: replace pkg: source/target with service: so the
	// frontend sees edges connected to the service node it already has.
	if pkgID != "" {
		for i := range edges {
			if edges[i].SourceID == pkgID {
				edges[i].SourceID = parentID
			}
			if edges[i].TargetID == pkgID {
				edges[i].TargetID = parentID
			}
		}
	}

	// Synthesize "contains" edges from parentID to each node whose parent_id
	// matches parentID or pkgID but has no explicit incoming contains edge.
	// This ensures the service→file→function hierarchy is connected.
	incomingContains := make(map[string]bool)
	for _, e := range edges {
		if e.Type == "contains" {
			incomingContains[e.TargetID] = true
		}
	}
	for _, n := range nodes {
		if incomingContains[n.ID] {
			continue // already has a contains edge
		}
		// Check if this node's parent is the requested parent or the pkg alias.
		if n.ParentID == parentID || (pkgID != "" && n.ParentID == pkgID) {
			edges = append(edges, &graph.Edge{
				ID:       parentID + "-contains-" + n.ID,
				SourceID: parentID,
				TargetID: n.ID,
				Type:     "contains",
			})
		}
	}

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

	// For service nodes, also include descendants from the corresponding
	// package node so the detail view shows functions.
	var svcPkgID string
	if node.Type == graph.NodeTypeService {
		svcPkgID = "pkg:" + strings.TrimPrefix(nodeID, "service:")
		if _, ok := s.index.GetNode(svcPkgID); ok {
			pkgDescendants := s.index.GetDescendants(svcPkgID, 4)
			seen := make(map[string]bool, len(children))
			for _, n := range children {
				seen[n.ID] = true
			}
			for _, n := range pkgDescendants {
				if !seen[n.ID] {
					children = append(children, n)
					seen[n.ID] = true
				}
			}
		} else {
			svcPkgID = ""
		}
	}

	// In-edges (who calls/targets this).
	inEdges := s.index.GetInEdges(nodeID)
	if svcPkgID != "" {
		pkgIn := s.index.GetInEdges(svcPkgID)
		for i := range pkgIn {
			if pkgIn[i].TargetID == svcPkgID {
				pkgIn[i].TargetID = nodeID
			}
		}
		inEdges = append(inEdges, pkgIn...)
	}

	// Out-edges (what this calls/targets).
	outEdges := s.index.GetOutEdges(nodeID)
	if svcPkgID != "" {
		pkgOut := s.index.GetOutEdges(svcPkgID)
		for i := range pkgOut {
			if pkgOut[i].SourceID == svcPkgID {
				pkgOut[i].SourceID = nodeID
			}
		}
		outEdges = append(outEdges, pkgOut...)
	}

	// Data flow — only for function nodes.
	var dataFlow interface{}
	if node.Type == graph.NodeTypeFunction {
		ctx := r.Context()
		records, err := s.store.GetDataFlow(ctx, nodeID)
		if err == nil && len(records) > 0 {
			dataFlow = records
		}
	}

	// Callers (who calls this node) and callees (what this node calls).
	callers := s.index.GetCallers(nodeID)
	callees := s.index.GetCallees(nodeID)

	// Recent runtime events (last 10).
	ctx := r.Context()
	events, _ := s.store.GetRecentEvents(ctx, nodeID, 10)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"node":      node,
			"children":  children,
			"in_edges":  inEdges,
			"out_edges": outEdges,
			"callers":   callers,
			"callees":   callees,
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
