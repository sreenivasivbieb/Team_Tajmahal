package query

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"

	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// ---------------------------------------------------------------------------
// Subgraph Query Types
// ---------------------------------------------------------------------------

// SubgraphQueryType identifies the kind of subgraph to extract.
type SubgraphQueryType string

const (
	QueryServiceOverview  SubgraphQueryType = "service_overview"
	QueryCallChain        SubgraphQueryType = "call_chain"
	QueryFailurePath      SubgraphQueryType = "failure_path"
	QueryDataLineage      SubgraphQueryType = "data_lineage"
	QueryDependencyImpact SubgraphQueryType = "dependency_impact"
)

// ---------------------------------------------------------------------------
// SubgraphQuery — input
// ---------------------------------------------------------------------------

// SubgraphQuery describes what subgraph to extract.
type SubgraphQuery struct {
	QueryType SubgraphQueryType `json:"query_type"`
	TargetID  string            `json:"target_id"`
	MaxNodes  int               `json:"max_nodes"`
	MaxDepth  int               `json:"max_depth"`
	Question  string            `json:"question,omitempty"` // original NL
}

func (q *SubgraphQuery) defaults() {
	if q.MaxNodes <= 0 {
		q.MaxNodes = 25
	}
	if q.MaxNodes > 50 {
		q.MaxNodes = 50
	}
	if q.MaxDepth <= 0 {
		q.MaxDepth = 4
	}
}

// ---------------------------------------------------------------------------
// SubgraphResult — output
// ---------------------------------------------------------------------------

// SubgraphResult is the outcome of a subgraph extraction.
type SubgraphResult struct {
	QueryType    SubgraphQueryType `json:"query_type"`
	TargetID     string            `json:"target_id"`
	Nodes        []*graph.Node     `json:"nodes"`
	Edges        []*graph.Edge     `json:"edges"`
	EntryPoints  []string          `json:"entry_points,omitempty"`
	CriticalPath []string          `json:"critical_path,omitempty"`
	NodeRoles    map[string]string `json:"node_roles"`  // node_id → role
	Layout       *SubgraphLayout   `json:"layout"`
}

// SubgraphLayout provides 2-D positions for frontend rendering.
type SubgraphLayout struct {
	Positions map[string]Position `json:"positions"`
}

// Position is an (x, y) coordinate.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ---------------------------------------------------------------------------
// SubgraphExtractor
// ---------------------------------------------------------------------------

// SubgraphExtractor builds focused subgraphs for different query types.
type SubgraphExtractor struct {
	index  *graph.GraphIndex
	store  *storage.Storage
	scorer *NodeScorer
}

// NewSubgraphExtractor creates a ready-to-use extractor.
func NewSubgraphExtractor(
	index *graph.GraphIndex,
	store *storage.Storage,
) *SubgraphExtractor {
	return &SubgraphExtractor{
		index:  index,
		store:  store,
		scorer: NewNodeScorer(index),
	}
}

// ---------------------------------------------------------------------------
// Extract — main dispatch
// ---------------------------------------------------------------------------

// Extract produces a SubgraphResult for the given query.
func (x *SubgraphExtractor) Extract(ctx context.Context, q SubgraphQuery) (*SubgraphResult, error) {
	q.defaults()

	target, ok := x.index.GetNode(q.TargetID)
	if !ok {
		return nil, fmt.Errorf("query/subgraph: node %q not found", q.TargetID)
	}

	switch q.QueryType {
	case QueryServiceOverview:
		return x.serviceOverview(ctx, target, &q)
	case QueryCallChain:
		return x.callChain(ctx, target, &q)
	case QueryFailurePath:
		return x.failurePath(ctx, target, &q)
	case QueryDataLineage:
		return x.dataLineage(ctx, target, &q)
	case QueryDependencyImpact:
		return x.dependencyImpact(ctx, target, &q)
	default:
		return nil, fmt.Errorf("query/subgraph: unsupported query type %q", q.QueryType)
	}
}

// =========================================================================
// QueryServiceOverview
// =========================================================================

func (x *SubgraphExtractor) serviceOverview(ctx context.Context, target *graph.Node, q *SubgraphQuery) (*SubgraphResult, error) {
	// Resolve the lookup ID: for service: nodes, also include pkg: descendants.
	lookupID := target.ID
	var pkgID string
	if strings.HasPrefix(target.ID, "service:") {
		pkgID = "pkg:" + strings.TrimPrefix(target.ID, "service:")
	}

	// 1. Entry points.
	entryPoints := x.index.FindEntryPoints(lookupID)
	if pkgID != "" {
		pkgEntries := x.index.FindEntryPoints(pkgID)
		entryPoints = append(entryPoints, pkgEntries...)
	}
	entrySet := idSet(entryPoints)

	// 2. BFS from entry points.
	reachable := make(map[string]*graph.Node)
	reachable[target.ID] = target
	for _, ep := range entryPoints {
		reachable[ep.ID] = ep
		bfsNodes := x.index.BFS(ep.ID, q.MaxDepth, nil)
		for _, n := range bfsNodes {
			reachable[n.ID] = n
		}
	}
	// Also include direct descendants of the service for context.
	descendants := x.index.GetDescendants(target.ID, 2)
	for _, n := range descendants {
		reachable[n.ID] = n
	}
	// For service: nodes, also include descendants from the pkg: node.
	if pkgID != "" {
		pkgDescendants := x.index.GetDescendants(pkgID, 2)
		for _, n := range pkgDescendants {
			reachable[n.ID] = n
		}
	}

	// 3. Score all reachable nodes.
	allNodes := mapValues(reachable)
	scored := x.scorer.ScoreAll(allNodes, QueryServiceOverview)

	// 4. Select top-N by score.
	selected := make(map[string]*graph.Node)

	// Force includes.
	for _, sn := range scored {
		n := sn.Node
		// Force: entry points.
		if entrySet[n.ID] {
			selected[n.ID] = n
		}
		// Force: nodes with errors.
		if n.ErrorCount > 0 {
			selected[n.ID] = n
		}
	}
	// Force: cloud services reachable within 2 hops.
	for _, ep := range entryPoints {
		cloudNodes := x.index.BFS(ep.ID, 2, func(n *graph.Node) bool {
			return n.IsCloud()
		})
		for _, cn := range cloudNodes {
			selected[cn.ID] = cn
		}
	}

	// Fill remaining slots with top-scored nodes.
	for _, sn := range scored {
		if len(selected) >= q.MaxNodes {
			break
		}
		selected[sn.Node.ID] = sn.Node
	}

	// Always include the target service node.
	selected[target.ID] = target

	// 5. Get edges between selected nodes.
	nodeIDs := mapKeys(selected)
	edges := x.index.GetEdgesBetween(nodeIDs)

	// 6. Add edges to immediate external dependencies.
	extDeps := x.index.GetExternalDependencies(target.ID)
	if pkgID != "" {
		pkgDeps := x.index.GetExternalDependencies(pkgID)
		extDeps = append(extDeps, pkgDeps...)
	}
	for _, dep := range extDeps {
		if len(selected) < q.MaxNodes+10 { // small overflow budget for deps
			selected[dep.ID] = dep
		}
	}
	// Re-gather edges with the expanded set.
	nodeIDs = mapKeys(selected)
	edges = x.index.GetEdgesBetween(nodeIDs)

	// 7. Assign roles.
	roles := make(map[string]string, len(selected))
	fanInCache := make(map[string]int)
	for id := range selected {
		fanInCache[id] = x.index.ComputeFanIn(id)
	}

	for id, n := range selected {
		switch {
		case entrySet[id]:
			roles[id] = "entry_point"
		case n.ErrorCount > 0 || n.RuntimeStatus == "error":
			roles[id] = "failing"
		case n.IsCloud() || n.Type == graph.NodeTypeDataFlow:
			roles[id] = "dependency"
		case fanInCache[id] >= 3:
			roles[id] = "core"
		default:
			roles[id] = "supporting"
		}
	}

	// 8. Critical path: longest path from top entry point through high-fan-in nodes.
	criticalPath := x.computeCriticalPath(entryPoints, selected, fanInCache)

	// 9. Layout.
	layout := x.layoutServiceOverview(selected, roles)

	result := &SubgraphResult{
		QueryType:    q.QueryType,
		TargetID:     q.TargetID,
		Nodes:        mapValues(selected),
		Edges:        edges,
		EntryPoints:  nodeIDList(entryPoints),
		CriticalPath: criticalPath,
		NodeRoles:    roles,
		Layout:       layout,
	}
	return result, nil
}

func (x *SubgraphExtractor) computeCriticalPath(
	entryPoints []*graph.Node,
	selected map[string]*graph.Node,
	fanInCache map[string]int,
) []string {
	if len(entryPoints) == 0 {
		return nil
	}

	// Pick the entry point with the highest fan-in as starting point.
	var bestEP *graph.Node
	bestFanIn := -1
	for _, ep := range entryPoints {
		fi := fanInCache[ep.ID]
		if fi > bestFanIn {
			bestFanIn = fi
			bestEP = ep
		}
	}
	if bestEP == nil {
		bestEP = entryPoints[0]
	}

	// DFS to find the longest simple path through selected nodes.
	var longest []string
	visited := map[string]bool{}

	var dfs func(id string, path []string)
	dfs = func(id string, path []string) {
		if len(path) > len(longest) {
			longest = make([]string, len(path))
			copy(longest, path)
		}
		for _, e := range x.index.GetOutEdges(id) {
			if e.Type != graph.EdgeTypeCalls && e.Type != graph.EdgeTypeDependsOn && e.Type != graph.EdgeTypeConnectsTo {
				continue
			}
			tid := e.TargetID
			if !visited[tid] {
				if _, inSet := selected[tid]; inSet {
					visited[tid] = true
					dfs(tid, append(path, tid))
					delete(visited, tid)
				}
			}
		}
	}

	visited[bestEP.ID] = true
	dfs(bestEP.ID, []string{bestEP.ID})

	return longest
}

func (x *SubgraphExtractor) layoutServiceOverview(
	nodes map[string]*graph.Node,
	roles map[string]string,
) *SubgraphLayout {
	layout := &SubgraphLayout{Positions: make(map[string]Position, len(nodes))}

	// Group by role.
	buckets := map[string][]string{}
	for id, role := range roles {
		buckets[role] = append(buckets[role], id)
	}

	yMap := map[string]float64{
		"entry_point": 0,
		"core":        200,
		"supporting":  300,
		"failing":     200,
		"dependency":  400,
	}

	for role, ids := range buckets {
		sort.Strings(ids) // deterministic
		y := yMap[role]
		spacing := 150.0
		startX := -float64(len(ids)-1) / 2.0 * spacing
		for i, id := range ids {
			node := nodes[id]
			yOff := y
			if node != nil && node.IsCloud() {
				yOff = 600
			}
			layout.Positions[id] = Position{X: startX + float64(i)*spacing, Y: yOff}
		}
	}

	return layout
}

// =========================================================================
// QueryCallChain
// =========================================================================

func (x *SubgraphExtractor) callChain(ctx context.Context, target *graph.Node, q *SubgraphQuery) (*SubgraphResult, error) {
	selected := map[string]*graph.Node{target.ID: target}

	// 1. BFS following CALLS edges outward.
	callees := x.bfsCallsOut(target.ID, q.MaxDepth)
	for _, n := range callees {
		selected[n.ID] = n
	}

	// 2. One level of callers.
	callers := x.index.GetCallers(target.ID)
	for _, n := range callers {
		selected[n.ID] = n
	}

	// 3. Data flow nodes for the target function.
	flows, err := x.store.GetDataFlow(ctx, target.ID)
	if err != nil {
		log.Printf("query/subgraph: data flow error: %v", err)
	}
	for _, f := range flows {
		// If the data flow has a function_id that maps to a node, include it.
		if n, ok := x.index.GetNode(f.FunctionID); ok {
			selected[n.ID] = n
		}
	}

	// 4. Cloud service endpoints reachable from selected nodes.
	for _, n := range mapValues(selected) {
		for _, e := range x.index.GetOutEdges(n.ID) {
			if tn, ok := x.index.GetNode(e.TargetID); ok && tn.IsCloud() {
				selected[tn.ID] = tn
			}
		}
	}

	// 5. Score and limit.
	scored := x.scorer.ScoreAll(mapValues(selected), QueryCallChain)
	selected = trimToLimit(scored, q.MaxNodes, target.ID)

	nodeIDs := mapKeys(selected)
	edges := x.index.GetEdgesBetween(nodeIDs)

	// 6. Assign roles.
	callerSet := idSet(callers)
	roles := make(map[string]string, len(selected))
	for id, n := range selected {
		switch {
		case id == target.ID:
			roles[id] = "target"
		case callerSet[id]:
			roles[id] = "caller"
		case n.IsCloud():
			roles[id] = "dependency"
		case n.ErrorCount > 0:
			roles[id] = "failing"
		default:
			roles[id] = "callee"
		}
	}

	// 7. Layout: top-down.
	layout := x.layoutCallChain(selected, roles, target)

	return &SubgraphResult{
		QueryType:    q.QueryType,
		TargetID:     q.TargetID,
		Nodes:        mapValues(selected),
		Edges:        edges,
		EntryPoints:  []string{target.ID},
		CriticalPath: nil,
		NodeRoles:    roles,
		Layout:       layout,
	}, nil
}

func (x *SubgraphExtractor) bfsCallsOut(startID string, maxDepth int) []*graph.Node {
	type entry struct {
		id    string
		depth int
	}
	visited := map[string]bool{startID: true}
	queue := []entry{{startID, 0}}
	var result []*graph.Node

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if cur.depth >= maxDepth {
			continue
		}
		for _, e := range x.index.GetOutEdges(cur.id) {
			if e.Type != graph.EdgeTypeCalls {
				continue
			}
			if visited[e.TargetID] {
				continue
			}
			visited[e.TargetID] = true
			if n, ok := x.index.GetNode(e.TargetID); ok {
				result = append(result, n)
				queue = append(queue, entry{e.TargetID, cur.depth + 1})
			}
		}
	}
	return result
}

func (x *SubgraphExtractor) layoutCallChain(
	nodes map[string]*graph.Node,
	roles map[string]string,
	target *graph.Node,
) *SubgraphLayout {
	layout := &SubgraphLayout{Positions: make(map[string]Position, len(nodes))}

	// Place target at top-center.
	layout.Positions[target.ID] = Position{X: 0, Y: 0}

	// Callers above.
	callerIDs := nodeIDsByRole(roles, "caller")
	placeRow(layout, callerIDs, -150, 150)

	// Callees below target.
	calleeIDs := nodeIDsByRole(roles, "callee")
	placeRow(layout, calleeIDs, 200, 150)

	// Failing nodes at same level as callees but offset.
	failingIDs := nodeIDsByRole(roles, "failing")
	placeRow(layout, failingIDs, 200, 150)

	// Dependencies (cloud) at bottom.
	depIDs := nodeIDsByRole(roles, "dependency")
	placeRow(layout, depIDs, 400, 150)

	return layout
}

// =========================================================================
// QueryFailurePath
// =========================================================================

func (x *SubgraphExtractor) failurePath(ctx context.Context, target *graph.Node, q *SubgraphQuery) (*SubgraphResult, error) {
	selected := map[string]*graph.Node{target.ID: target}

	// 1. All callers up to 3 hops (blast radius).
	callers := x.bfsCallsIn(target.ID, 3)
	for _, n := range callers {
		selected[n.ID] = n
	}

	// 2. All callees 2 hops (dependencies).
	callees := x.bfsCallsOut(target.ID, 2)
	for _, n := range callees {
		selected[n.ID] = n
	}

	// 3. Check which dependencies are also failing.
	failingDeps := make(map[string]bool)
	for _, n := range callees {
		if n.IsFailing() {
			failingDeps[n.ID] = true
		}
	}

	// 4. Fetch last 10 runtime events for context (not stored in result but used for scoring).
	events, err := x.store.GetRecentEvents(ctx, target.ID, 10)
	if err != nil {
		log.Printf("query/subgraph: recent events error: %v", err)
	}
	_ = events // avail for future AI prompt enrichment

	// 5. Score and limit.
	scored := x.scorer.ScoreAll(mapValues(selected), QueryFailurePath)
	selected = trimToLimit(scored, q.MaxNodes, target.ID)

	nodeIDs := mapKeys(selected)
	edges := x.index.GetEdgesBetween(nodeIDs)

	// 6. Assign roles.
	callerSet := idSet(callers)
	roles := make(map[string]string, len(selected))
	for id, n := range selected {
		switch {
		case id == target.ID:
			roles[id] = "failing_target"
		case n.IsFailing():
			roles[id] = "failing"
		case callerSet[id]:
			roles[id] = "caller"
		default:
			roles[id] = "dependency"
		}
	}

	// Critical path: path from deepest caller through target to deepest failing dep.
	var critPath []string
	if len(callers) > 0 {
		deepestCaller := callers[len(callers)-1]
		path := x.index.FindPath(deepestCaller.ID, target.ID)
		for _, n := range path {
			critPath = append(critPath, n.ID)
		}
	}
	if critPath == nil {
		critPath = []string{target.ID}
	}
	for id := range failingDeps {
		critPath = append(critPath, id)
	}

	// Layout: failing node center, callers above, deps below.
	layout := x.layoutFailurePath(selected, roles, target)

	return &SubgraphResult{
		QueryType:    q.QueryType,
		TargetID:     q.TargetID,
		Nodes:        mapValues(selected),
		Edges:        edges,
		EntryPoints:  []string{target.ID},
		CriticalPath: critPath,
		NodeRoles:    roles,
		Layout:       layout,
	}, nil
}

func (x *SubgraphExtractor) bfsCallsIn(startID string, maxDepth int) []*graph.Node {
	type entry struct {
		id    string
		depth int
	}
	visited := map[string]bool{startID: true}
	queue := []entry{{startID, 0}}
	var result []*graph.Node

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if cur.depth >= maxDepth {
			continue
		}
		for _, e := range x.index.GetInEdges(cur.id) {
			if e.Type != graph.EdgeTypeCalls {
				continue
			}
			if visited[e.SourceID] {
				continue
			}
			visited[e.SourceID] = true
			if n, ok := x.index.GetNode(e.SourceID); ok {
				result = append(result, n)
				queue = append(queue, entry{e.SourceID, cur.depth + 1})
			}
		}
	}
	return result
}

func (x *SubgraphExtractor) layoutFailurePath(
	nodes map[string]*graph.Node,
	roles map[string]string,
	target *graph.Node,
) *SubgraphLayout {
	layout := &SubgraphLayout{Positions: make(map[string]Position, len(nodes))}

	// Target in center.
	layout.Positions[target.ID] = Position{X: 0, Y: 0}

	// Callers above (fanning out).
	callerIDs := nodeIDsByRole(roles, "caller")
	placeRow(layout, callerIDs, -200, 150)

	// Dependencies below.
	depIDs := nodeIDsByRole(roles, "dependency")
	placeRow(layout, depIDs, 200, 150)

	// Other failing nodes beside target.
	failingIDs := nodeIDsByRole(roles, "failing")
	placeRow(layout, failingIDs, 0, 200)

	return layout
}

// =========================================================================
// QueryDataLineage
// =========================================================================

func (x *SubgraphExtractor) dataLineage(ctx context.Context, target *graph.Node, q *SubgraphQuery) (*SubgraphResult, error) {
	selected := map[string]*graph.Node{target.ID: target}

	// Get data flow records.
	flows, err := x.store.GetDataFlow(ctx, target.ID)
	if err != nil {
		log.Printf("query/subgraph: data flow error: %v", err)
	}

	// Include data flow source/sink nodes.
	for _, f := range flows {
		if f.Source != "" {
			if n, ok := x.index.GetNode(f.Source); ok {
				selected[n.ID] = n
			}
		}
		if f.Sink != "" {
			if n, ok := x.index.GetNode(f.Sink); ok {
				selected[n.ID] = n
			}
		}
	}

	// Follow produces_to and consumed_by edges.
	for _, e := range x.index.GetOutEdges(target.ID) {
		if e.Type == graph.EdgeTypeProducesTo || e.Type == graph.EdgeTypeConsumedBy || e.Type == graph.EdgeTypeTransforms {
			if n, ok := x.index.GetNode(e.TargetID); ok {
				selected[n.ID] = n
			}
		}
	}
	for _, e := range x.index.GetInEdges(target.ID) {
		if e.Type == graph.EdgeTypeProducesTo || e.Type == graph.EdgeTypeConsumedBy || e.Type == graph.EdgeTypeTransforms {
			if n, ok := x.index.GetNode(e.SourceID); ok {
				selected[n.ID] = n
			}
		}
	}

	// Also follow call chain for 2 levels.
	callees := x.bfsCallsOut(target.ID, 2)
	for _, n := range callees {
		selected[n.ID] = n
	}

	// Score and limit.
	scored := x.scorer.ScoreAll(mapValues(selected), QueryDataLineage)
	selected = trimToLimit(scored, q.MaxNodes, target.ID)

	nodeIDs := mapKeys(selected)
	edges := x.index.GetEdgesBetween(nodeIDs)

	roles := make(map[string]string, len(selected))
	for id, n := range selected {
		switch {
		case id == target.ID:
			roles[id] = "source"
		case n.Type == graph.NodeTypeDataFlow:
			roles[id] = "transform"
		case n.IsCloud():
			roles[id] = "sink"
		case n.ErrorCount > 0:
			roles[id] = "failing"
		default:
			roles[id] = "pipeline"
		}
	}

	layout := x.layoutDataLineage(selected, roles, target)

	return &SubgraphResult{
		QueryType:    q.QueryType,
		TargetID:     q.TargetID,
		Nodes:        mapValues(selected),
		Edges:        edges,
		EntryPoints:  []string{target.ID},
		CriticalPath: nil,
		NodeRoles:    roles,
		Layout:       layout,
	}, nil
}

func (x *SubgraphExtractor) layoutDataLineage(
	nodes map[string]*graph.Node,
	roles map[string]string,
	target *graph.Node,
) *SubgraphLayout {
	layout := &SubgraphLayout{Positions: make(map[string]Position, len(nodes))}

	layout.Positions[target.ID] = Position{X: 0, Y: 0}

	sourceIDs := nodeIDsByRole(roles, "source")
	placeRow(layout, sourceIDs, 0, 150)

	transformIDs := nodeIDsByRole(roles, "transform")
	placeRow(layout, transformIDs, 200, 150)

	pipelineIDs := nodeIDsByRole(roles, "pipeline")
	placeRow(layout, pipelineIDs, 300, 150)

	sinkIDs := nodeIDsByRole(roles, "sink")
	placeRow(layout, sinkIDs, 400, 150)

	failingIDs := nodeIDsByRole(roles, "failing")
	placeRow(layout, failingIDs, 200, 200)

	return layout
}

// =========================================================================
// QueryDependencyImpact
// =========================================================================

func (x *SubgraphExtractor) dependencyImpact(ctx context.Context, target *graph.Node, q *SubgraphQuery) (*SubgraphResult, error) {
	selected := map[string]*graph.Node{target.ID: target}

	// 1. All nodes that DEPENDS_ON or CALLS target.
	directDependents := x.findDirectDependents(target.ID)
	for _, n := range directDependents {
		selected[n.ID] = n
	}

	// 2. For each dependent, find parent services.
	serviceSet := make(map[string]*graph.Node)
	for _, n := range directDependents {
		svc := x.findServiceAncestor(n.ID)
		if svc != nil && svc.ID != target.ID {
			serviceSet[svc.ID] = svc
			selected[svc.ID] = svc
		}
	}

	// 3. Compute blast radius.
	var blastRadius int
	for _, svc := range serviceSet {
		blastRadius += svc.Metadata.FunctionCount
	}

	// Score and limit.
	scored := x.scorer.ScoreAll(mapValues(selected), QueryDependencyImpact)
	selected = trimToLimit(scored, q.MaxNodes, target.ID)

	nodeIDs := mapKeys(selected)
	edges := x.index.GetEdgesBetween(nodeIDs)

	// Roles.
	roles := make(map[string]string, len(selected))
	for id, n := range selected {
		switch {
		case id == target.ID:
			roles[id] = "target"
		case serviceSet[id] != nil:
			roles[id] = "affected_service"
		case n.ErrorCount > 0:
			roles[id] = "failing"
		default:
			roles[id] = "dependent"
		}
	}

	// Layout: target center, direct dependents ring, services outer ring.
	layout := x.layoutDependencyImpact(selected, roles, target)

	log.Printf("query/subgraph: dependency_impact blast_radius=%d services (est. %d functions)",
		len(serviceSet), blastRadius)

	return &SubgraphResult{
		QueryType:    q.QueryType,
		TargetID:     q.TargetID,
		Nodes:        mapValues(selected),
		Edges:        edges,
		EntryPoints:  []string{target.ID},
		CriticalPath: nil,
		NodeRoles:    roles,
		Layout:       layout,
	}, nil
}

func (x *SubgraphExtractor) findDirectDependents(nodeID string) []*graph.Node {
	seen := map[string]bool{}
	var result []*graph.Node

	for _, e := range x.index.GetInEdges(nodeID) {
		if e.Type == graph.EdgeTypeDependsOn || e.Type == graph.EdgeTypeCalls {
			if !seen[e.SourceID] {
				seen[e.SourceID] = true
				if n, ok := x.index.GetNode(e.SourceID); ok {
					result = append(result, n)
				}
			}
		}
	}
	return result
}

func (x *SubgraphExtractor) findServiceAncestor(nodeID string) *graph.Node {
	cur := nodeID
	for {
		n, ok := x.index.GetNode(cur)
		if !ok {
			return nil
		}
		if n.Type == graph.NodeTypeService {
			return n
		}
		if n.ParentID == "" || n.ParentID == cur {
			return nil
		}
		cur = n.ParentID
	}
}

func (x *SubgraphExtractor) layoutDependencyImpact(
	nodes map[string]*graph.Node,
	roles map[string]string,
	target *graph.Node,
) *SubgraphLayout {
	layout := &SubgraphLayout{Positions: make(map[string]Position, len(nodes))}

	// Target at center.
	layout.Positions[target.ID] = Position{X: 0, Y: 0}

	// Direct dependents in inner ring.
	depIDs := nodeIDsByRole(roles, "dependent")
	placeRing(layout, depIDs, 200)

	// Failing in ring too.
	failIDs := nodeIDsByRole(roles, "failing")
	placeRing(layout, failIDs, 250)

	// Services in outer ring.
	svcIDs := nodeIDsByRole(roles, "affected_service")
	placeRing(layout, svcIDs, 400)

	return layout
}

// =========================================================================
// Shared helpers
// =========================================================================

func idSet(nodes []*graph.Node) map[string]bool {
	s := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		s[n.ID] = true
	}
	return s
}

func nodeIDList(nodes []*graph.Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

func mapKeys(m map[string]*graph.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func mapValues(m map[string]*graph.Node) []*graph.Node {
	vals := make([]*graph.Node, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	return vals
}

// trimToLimit keeps the top-N scored nodes plus the anchor node.
func trimToLimit(scored []ScoredNode, limit int, anchorID string) map[string]*graph.Node {
	result := make(map[string]*graph.Node, limit)
	for _, sn := range scored {
		if len(result) >= limit && sn.Node.ID != anchorID {
			continue
		}
		result[sn.Node.ID] = sn.Node
	}
	return result
}

func nodeIDsByRole(roles map[string]string, role string) []string {
	var ids []string
	for id, r := range roles {
		if r == role {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func placeRow(layout *SubgraphLayout, ids []string, y, spacing float64) {
	if len(ids) == 0 {
		return
	}
	startX := -float64(len(ids)-1) / 2.0 * spacing
	for i, id := range ids {
		if _, exists := layout.Positions[id]; exists {
			continue // don't overwrite earlier placements
		}
		layout.Positions[id] = Position{X: startX + float64(i)*spacing, Y: y}
	}
}

func placeRing(layout *SubgraphLayout, ids []string, radius float64) {
	if len(ids) == 0 {
		return
	}
	for i, id := range ids {
		if _, exists := layout.Positions[id]; exists {
			continue
		}
		angle := 2.0 * math.Pi * float64(i) / float64(len(ids))
		layout.Positions[id] = Position{
			X: radius * math.Cos(angle),
			Y: radius * math.Sin(angle),
		}
	}
}
