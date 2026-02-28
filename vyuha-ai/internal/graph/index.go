package graph

import (
	"context"
	"log"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// storageLoader is the minimal interface the GraphIndex needs to bulk-load
// from the persistence layer. This avoids a direct dependency on the
// concrete storage package (which already imports graph).
// ---------------------------------------------------------------------------

type storageLoader interface {
	GetAllNodes(ctx context.Context) ([]*Node, error)
	GetAllEdges(ctx context.Context) ([]*Edge, error)
}

// ---------------------------------------------------------------------------
// IndexStats
// ---------------------------------------------------------------------------

// IndexStats summarises the contents of the in-memory graph index.
type IndexStats struct {
	TotalNodes    int            `json:"total_nodes"`
	TotalEdges    int            `json:"total_edges"`
	NodesByType   map[string]int `json:"nodes_by_type"`
	EdgesByType   map[string]int `json:"edges_by_type"`
	FilesIndexed  int            `json:"files_indexed"`
	StatusCounts  map[string]int `json:"status_counts"`
}

// ---------------------------------------------------------------------------
// GraphIndex
// ---------------------------------------------------------------------------

// GraphIndex is an in-memory mirror of the code-intelligence graph stored
// in SQLite.  It is loaded once on startup and kept in sync via
// write-through on every mutation.
//
// All public methods are goroutine-safe.
type GraphIndex struct {
	mu        sync.RWMutex
	nodes     map[string]*Node      // id → node
	children  map[string][]string   // parent_id → []child_ids
	outEdges  map[string][]*Edge    // source_id → edges
	inEdges   map[string][]*Edge    // target_id → edges
	byType    map[NodeType][]string // type → []node_ids
	byFile    map[string][]string   // file_path → []node_ids
	byStatus  map[string][]string   // runtime_status → []node_ids
	funcByName map[string][]string  // lowercase(name) → []node_ids
	funcByFQN  map[string]string    // "service::func" (lowercase) → node_id
}

// NewGraphIndex returns an empty, initialised GraphIndex ready for use.
func NewGraphIndex() *GraphIndex {
	return &GraphIndex{
		nodes:      make(map[string]*Node),
		children:   make(map[string][]string),
		outEdges:   make(map[string][]*Edge),
		inEdges:    make(map[string][]*Edge),
		byType:     make(map[NodeType][]string),
		byFile:     make(map[string][]string),
		byStatus:   make(map[string][]string),
		funcByName: make(map[string][]string),
		funcByFQN:  make(map[string]string),
	}
}

// ============================= LOADING ====================================

// LoadFromStorage bulk-loads every node and edge from the persistence layer
// into the in-memory index.  It accepts any value that satisfies the
// storageLoader interface (e.g. *storage.Storage).
func (g *GraphIndex) LoadFromStorage(ctx context.Context, store storageLoader) error {
	nodes, err := store.GetAllNodes(ctx)
	if err != nil {
		return err
	}
	edges, err := store.GetAllEdges(ctx)
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Reset maps.
	g.nodes = make(map[string]*Node, len(nodes))
	g.children = make(map[string][]string)
	g.outEdges = make(map[string][]*Edge)
	g.inEdges = make(map[string][]*Edge)
	g.byType = make(map[NodeType][]string)
	g.byFile = make(map[string][]string)
	g.byStatus = make(map[string][]string)
	g.funcByName = make(map[string][]string)
	g.funcByFQN = make(map[string]string)

	for _, n := range nodes {
		g.indexNodeLocked(n)
	}
	for _, e := range edges {
		g.indexEdgeLocked(e)
	}

	log.Printf("graph-index: loaded %d nodes, %d edges", len(nodes), len(edges))
	return nil
}

// indexNodeLocked inserts a node into every secondary map.
// Caller MUST hold g.mu write lock.
func (g *GraphIndex) indexNodeLocked(n *Node) {
	g.nodes[n.ID] = n
	if n.ParentID != "" {
		g.children[n.ParentID] = append(g.children[n.ParentID], n.ID)
	}
	g.byType[n.Type] = append(g.byType[n.Type], n.ID)
	if n.FilePath != "" {
		g.byFile[n.FilePath] = append(g.byFile[n.FilePath], n.ID)
	}
	if n.RuntimeStatus != "" {
		g.byStatus[n.RuntimeStatus] = append(g.byStatus[n.RuntimeStatus], n.ID)
	}

	// Populate fast function-lookup maps.
	if n.Type == NodeTypeFunction {
		nameKey := strings.ToLower(n.Name)
		g.funcByName[nameKey] = append(g.funcByName[nameKey], n.ID)

		svcName := g.findServiceNameLocked(n.ID)
		if svcName != "" {
			fqnKey := strings.ToLower(svcName) + "::" + nameKey
			g.funcByFQN[fqnKey] = n.ID
		}
	}
}

// indexEdgeLocked inserts an edge into outEdges and inEdges.
// Caller MUST hold g.mu write lock.
func (g *GraphIndex) indexEdgeLocked(e *Edge) {
	g.outEdges[e.SourceID] = append(g.outEdges[e.SourceID], e)
	g.inEdges[e.TargetID] = append(g.inEdges[e.TargetID], e)
}

// ============================ MUTATIONS ==================================

// AddNode adds (or replaces) a node in the index.
func (g *GraphIndex) AddNode(node *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// If the node already exists, remove stale secondary-index entries first.
	if old, ok := g.nodes[node.ID]; ok {
		g.deindexNodeLocked(old)
	}
	g.indexNodeLocked(node)
}

// AddEdge adds an edge to the index.
func (g *GraphIndex) AddEdge(edge *Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.indexEdgeLocked(edge)
}

// UpdateNodeStatus changes the runtime status of a node and updates the
// byStatus index.
func (g *GraphIndex) UpdateNodeStatus(nodeID string, status string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	n, ok := g.nodes[nodeID]
	if !ok {
		return
	}

	// Remove from old status bucket.
	if n.RuntimeStatus != "" {
		g.removeFromSlice(&g.byStatus, n.RuntimeStatus, nodeID)
	}

	n.RuntimeStatus = status
	if status != "" {
		g.byStatus[status] = append(g.byStatus[status], nodeID)
	}
}

// RemoveNodesByFile removes every node associated with filePath and all
// edges where the removed node is a source or target.
func (g *GraphIndex) RemoveNodesByFile(filePath string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	ids, ok := g.byFile[filePath]
	if !ok {
		return
	}

	removed := make(map[string]bool, len(ids))
	for _, id := range ids {
		removed[id] = true
	}

	// Remove nodes and their secondary-index entries.
	for _, id := range ids {
		if n, ok := g.nodes[id]; ok {
			g.deindexNodeLocked(n)
			delete(g.nodes, id)
		}
	}

	// Remove edges touching removed nodes.
	g.purgeEdgesLocked(removed)
}

// deindexNodeLocked removes a node from all secondary maps but does NOT
// delete from g.nodes.  Caller MUST hold g.mu write lock.
func (g *GraphIndex) deindexNodeLocked(n *Node) {
	if n.ParentID != "" {
		g.removeIDFromSlice(g.children, n.ParentID, n.ID)
	}
	g.removeIDFromNodeTypeSlice(g.byType, n.Type, n.ID)
	if n.FilePath != "" {
		g.removeIDFromSlice(g.byFile, n.FilePath, n.ID)
	}
	if n.RuntimeStatus != "" {
		g.removeFromSlice(&g.byStatus, n.RuntimeStatus, n.ID)
	}

	// Clean up fast function-lookup maps.
	if n.Type == NodeTypeFunction {
		nameKey := strings.ToLower(n.Name)
		g.removeIDFromSlice(g.funcByName, nameKey, n.ID)

		svcName := g.findServiceNameLocked(n.ID)
		if svcName != "" {
			fqnKey := strings.ToLower(svcName) + "::" + nameKey
			delete(g.funcByFQN, fqnKey)
		}
	}
}

// purgeEdgesLocked drops any edge whose source or target is in the
// removed set.  Caller MUST hold g.mu write lock.
func (g *GraphIndex) purgeEdgesLocked(removed map[string]bool) {
	// Filter outEdges.
	for src, edges := range g.outEdges {
		if removed[src] {
			delete(g.outEdges, src)
			continue
		}
		filtered := edges[:0]
		for _, e := range edges {
			if !removed[e.TargetID] {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(g.outEdges, src)
		} else {
			g.outEdges[src] = filtered
		}
	}
	// Filter inEdges.
	for tgt, edges := range g.inEdges {
		if removed[tgt] {
			delete(g.inEdges, tgt)
			continue
		}
		filtered := edges[:0]
		for _, e := range edges {
			if !removed[e.SourceID] {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			delete(g.inEdges, tgt)
		} else {
			g.inEdges[tgt] = filtered
		}
	}
}

// ---------------------------------------------------------------------------
// Slice-removal helpers (operate on maps of slices, no allocations)
// ---------------------------------------------------------------------------

func (g *GraphIndex) removeIDFromSlice(m map[string][]string, key, id string) {
	ids := m[key]
	for i, v := range ids {
		if v == id {
			m[key] = append(ids[:i], ids[i+1:]...)
			if len(m[key]) == 0 {
				delete(m, key)
			}
			return
		}
	}
}

func (g *GraphIndex) removeIDFromNodeTypeSlice(m map[NodeType][]string, key NodeType, id string) {
	ids := m[key]
	for i, v := range ids {
		if v == id {
			m[key] = append(ids[:i], ids[i+1:]...)
			if len(m[key]) == 0 {
				delete(m, key)
			}
			return
		}
	}
}

func (g *GraphIndex) removeFromSlice(m *map[string][]string, key, id string) {
	ids := (*m)[key]
	for i, v := range ids {
		if v == id {
			(*m)[key] = append(ids[:i], ids[i+1:]...)
			if len((*m)[key]) == 0 {
				delete(*m, key)
			}
			return
		}
	}
}

// ===================== FAST FUNCTION LOOKUP ==============================

// findServiceNameLocked walks the ParentID chain from nodeID upward through
// g.nodes and returns the Name of the first NodeTypeService ancestor.
// Returns "" when no service ancestor is found.
// Caller MUST already hold g.mu (read or write).
func (g *GraphIndex) findServiceNameLocked(nodeID string) string {
	cur := nodeID
	for i := 0; i < 20; i++ { // depth guard
		n, ok := g.nodes[cur]
		if !ok || n.ParentID == "" {
			return ""
		}
		parent, ok := g.nodes[n.ParentID]
		if !ok {
			return ""
		}
		if parent.Type == NodeTypeService {
			return parent.Name
		}
		cur = parent.ID
	}
	return ""
}

// GetFuncByName returns the IDs of all function nodes whose name matches
// (case-insensitive).  Returns nil if none found.
func (g *GraphIndex) GetFuncByName(name string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ids := g.funcByName[strings.ToLower(name)]
	if len(ids) == 0 {
		return nil
	}
	// Return a copy so the caller can't mutate the index.
	out := make([]string, len(ids))
	copy(out, ids)
	return out
}

// GetFuncByFQN returns the node ID for a fully-qualified function name
// (service + function name), or "" if not found.
func (g *GraphIndex) GetFuncByFQN(service, funcName string) string {
	key := strings.ToLower(service) + "::" + strings.ToLower(funcName)
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.funcByFQN[key]
}

// ======================== TRAVERSAL QUERIES ===============================

// GetNode returns the node with the given ID and true, or nil and false if
// not found.
func (g *GraphIndex) GetNode(id string) (*Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	return n, ok
}

// GetChildren returns the direct children of parentID.
func (g *GraphIndex) GetChildren(parentID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.resolveIDsLocked(g.children[parentID])
}

// GetDescendants returns all transitive descendants of parentID up to
// maxDepth levels using BFS.  Results are in breadth-first order.
func (g *GraphIndex) GetDescendants(parentID string, maxDepth int) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Node
	type entry struct {
		id    string
		depth int
	}
	visited := map[string]bool{parentID: true}
	queue := []entry{}
	for _, cid := range g.children[parentID] {
		if !visited[cid] {
			visited[cid] = true
			queue = append(queue, entry{cid, 1})
		}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if n, ok := g.nodes[cur.id]; ok {
			result = append(result, n)
		}
		if cur.depth < maxDepth {
			for _, cid := range g.children[cur.id] {
				if !visited[cid] {
					visited[cid] = true
					queue = append(queue, entry{cid, cur.depth + 1})
				}
			}
		}
	}
	return result
}

// GetOutEdges returns all edges originating from nodeID.
func (g *GraphIndex) GetOutEdges(nodeID string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*Edge, len(g.outEdges[nodeID]))
	copy(out, g.outEdges[nodeID])
	return out
}

// GetInEdges returns all edges targeting nodeID.
func (g *GraphIndex) GetInEdges(nodeID string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*Edge, len(g.inEdges[nodeID]))
	copy(out, g.inEdges[nodeID])
	return out
}

// GetEdgesBetween returns all edges where both source and target are in
// nodeIDs.  Uses a set for O(1) membership checks.
func (g *GraphIndex) GetEdgesBetween(nodeIDs []string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	set := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		set[id] = true
	}

	var result []*Edge
	for _, id := range nodeIDs {
		for _, e := range g.outEdges[id] {
			if set[e.TargetID] {
				result = append(result, e)
			}
		}
	}
	return result
}

// GetCallers returns nodes that have CALLS edges pointing to nodeID.
func (g *GraphIndex) GetCallers(nodeID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Node
	for _, e := range g.inEdges[nodeID] {
		if e.Type == EdgeTypeCalls {
			if n, ok := g.nodes[e.SourceID]; ok {
				result = append(result, n)
			}
		}
	}
	return result
}

// GetCallees returns nodes that nodeID has CALLS edges to.
func (g *GraphIndex) GetCallees(nodeID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Node
	for _, e := range g.outEdges[nodeID] {
		if e.Type == EdgeTypeCalls {
			if n, ok := g.nodes[e.TargetID]; ok {
				result = append(result, n)
			}
		}
	}
	return result
}

// GetByType returns all nodes of the specified type.
func (g *GraphIndex) GetByType(nodeType NodeType) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.resolveIDsLocked(g.byType[nodeType])
}

// GetByFile returns all nodes associated with the given file path.
func (g *GraphIndex) GetByFile(filePath string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.resolveIDsLocked(g.byFile[filePath])
}

// GetFailingNodes returns all nodes whose runtime status is "error".
func (g *GraphIndex) GetFailingNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.resolveIDsLocked(g.byStatus["error"])
}

// GetDegradedNodes returns all nodes whose runtime status is "degraded".
func (g *GraphIndex) GetDegradedNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.resolveIDsLocked(g.byStatus["degraded"])
}

// resolveIDsLocked converts a slice of IDs into a slice of *Node.
// Caller MUST hold at least g.mu.RLock.
func (g *GraphIndex) resolveIDsLocked(ids []string) []*Node {
	result := make([]*Node, 0, len(ids))
	for _, id := range ids {
		if n, ok := g.nodes[id]; ok {
			result = append(result, n)
		}
	}
	return result
}

// ======================== GRAPH ALGORITHMS ================================

// BFS performs a breadth-first search from startID up to maxDepth hops.
// An optional filter function controls which nodes are included in the
// result; pass nil to include all reachable nodes.  Results are returned
// in BFS order.
func (g *GraphIndex) BFS(startID string, maxDepth int, filter func(*Node) bool) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type entry struct {
		id    string
		depth int
	}

	visited := map[string]bool{startID: true}
	queue := []entry{{startID, 0}}
	var result []*Node

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		n, ok := g.nodes[cur.id]
		if !ok {
			continue
		}
		if cur.id != startID { // don't include start node itself
			if filter == nil || filter(n) {
				result = append(result, n)
			}
		}

		if cur.depth >= maxDepth {
			continue
		}

		// Follow outgoing edges.
		for _, e := range g.outEdges[cur.id] {
			if !visited[e.TargetID] {
				visited[e.TargetID] = true
				queue = append(queue, entry{e.TargetID, cur.depth + 1})
			}
		}
		// Also follow children (containment).
		for _, cid := range g.children[cur.id] {
			if !visited[cid] {
				visited[cid] = true
				queue = append(queue, entry{cid, cur.depth + 1})
			}
		}
	}
	return result
}

// GetAncestors walks inEdges upward from nodeID and returns all reachable
// ancestor nodes up to maxDepth levels.
func (g *GraphIndex) GetAncestors(nodeID string, maxDepth int) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type entry struct {
		id    string
		depth int
	}

	visited := map[string]bool{nodeID: true}
	queue := []entry{{nodeID, 0}}
	var result []*Node

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if cur.depth >= maxDepth {
			continue
		}

		// Walk in-edges.
		for _, e := range g.inEdges[cur.id] {
			if !visited[e.SourceID] {
				visited[e.SourceID] = true
				if n, ok := g.nodes[e.SourceID]; ok {
					result = append(result, n)
				}
				queue = append(queue, entry{e.SourceID, cur.depth + 1})
			}
		}
		// Walk parent containment.
		if n, ok := g.nodes[cur.id]; ok && n.ParentID != "" && !visited[n.ParentID] {
			visited[n.ParentID] = true
			if p, ok := g.nodes[n.ParentID]; ok {
				result = append(result, p)
			}
			queue = append(queue, entry{n.ParentID, cur.depth + 1})
		}
	}
	return result
}

// FindPath returns the shortest path (BFS) between fromID and toID,
// following only CALLS and CONTAINS edges.  Returns nil if no path exists.
// The returned slice starts with fromID and ends with toID.
func (g *GraphIndex) FindPath(fromID, toID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if fromID == toID {
		if n, ok := g.nodes[fromID]; ok {
			return []*Node{n}
		}
		return nil
	}

	type entry struct {
		id   string
		prev string
	}

	visited := map[string]bool{fromID: true}
	queue := []entry{{fromID, ""}}
	parent := map[string]string{} // child → parent in BFS tree

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		neighbours := g.pathNeighboursLocked(cur.id)
		for _, nid := range neighbours {
			if visited[nid] {
				continue
			}
			visited[nid] = true
			parent[nid] = cur.id

			if nid == toID {
				// Reconstruct path.
				return g.reconstructPathLocked(fromID, toID, parent)
			}
			queue = append(queue, entry{nid, cur.id})
		}
	}
	return nil // no path
}

// pathNeighboursLocked returns IDs reachable via CALLS or CONTAINS edges.
// Caller MUST hold g.mu.RLock.
func (g *GraphIndex) pathNeighboursLocked(id string) []string {
	var out []string
	for _, e := range g.outEdges[id] {
		if e.Type == EdgeTypeCalls || e.Type == EdgeTypeContains {
			out = append(out, e.TargetID)
		}
	}
	for _, e := range g.inEdges[id] {
		if e.Type == EdgeTypeCalls || e.Type == EdgeTypeContains {
			out = append(out, e.SourceID)
		}
	}
	return out
}

// reconstructPathLocked walks the BFS parent map back from toID to fromID.
func (g *GraphIndex) reconstructPathLocked(fromID, toID string, parentMap map[string]string) []*Node {
	var ids []string
	for cur := toID; cur != ""; cur = parentMap[cur] {
		ids = append(ids, cur)
		if cur == fromID {
			break
		}
	}
	// Reverse to get fromID → … → toID order.
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	result := make([]*Node, 0, len(ids))
	for _, id := range ids {
		if n, ok := g.nodes[id]; ok {
			result = append(result, n)
		}
	}
	return result
}

// GetConnectedComponent returns all nodes reachable from nodeID in either
// direction (out-edges and in-edges), i.e. the weakly connected component.
func (g *GraphIndex) GetConnectedComponent(nodeID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := map[string]bool{nodeID: true}
	queue := []string{nodeID}
	var result []*Node

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if n, ok := g.nodes[cur]; ok {
			result = append(result, n)
		}

		// Outgoing.
		for _, e := range g.outEdges[cur] {
			if !visited[e.TargetID] {
				visited[e.TargetID] = true
				queue = append(queue, e.TargetID)
			}
		}
		// Incoming.
		for _, e := range g.inEdges[cur] {
			if !visited[e.SourceID] {
				visited[e.SourceID] = true
				queue = append(queue, e.SourceID)
			}
		}
		// Children.
		for _, cid := range g.children[cur] {
			if !visited[cid] {
				visited[cid] = true
				queue = append(queue, cid)
			}
		}
		// Parent.
		if n, ok := g.nodes[cur]; ok && n.ParentID != "" && !visited[n.ParentID] {
			visited[n.ParentID] = true
			queue = append(queue, n.ParentID)
		}
	}
	return result
}

// GetExternalDependencies returns all CloudService nodes and nodes belonging
// to a different service parent that are reachable from nodeID.
func (g *GraphIndex) GetExternalDependencies(nodeID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Find the service ancestor of the starting node.
	serviceID := g.findServiceAncestorLocked(nodeID)

	visited := map[string]bool{nodeID: true}
	queue := []string{nodeID}
	var result []*Node

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, e := range g.outEdges[cur] {
			tid := e.TargetID
			if visited[tid] {
				continue
			}
			visited[tid] = true

			tn, ok := g.nodes[tid]
			if !ok {
				continue
			}

			if tn.Type == NodeTypeCloudService {
				result = append(result, tn)
				continue // don't traverse further from cloud services
			}

			targetService := g.findServiceAncestorLocked(tid)
			if serviceID != "" && targetService != "" && targetService != serviceID {
				result = append(result, tn)
				continue
			}

			queue = append(queue, tid)
		}
	}
	return result
}

// findServiceAncestorLocked walks ParentID links to find the enclosing
// service node. Caller MUST hold g.mu.RLock.
func (g *GraphIndex) findServiceAncestorLocked(id string) string {
	cur := id
	for {
		n, ok := g.nodes[cur]
		if !ok {
			return ""
		}
		if n.Type == NodeTypeService {
			return n.ID
		}
		if n.ParentID == "" || n.ParentID == cur {
			return ""
		}
		cur = n.ParentID
	}
}

// ComputeFanIn returns the number of CALLS edges targeting nodeID.
func (g *GraphIndex) ComputeFanIn(nodeID string) int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	count := 0
	for _, e := range g.inEdges[nodeID] {
		if e.Type == EdgeTypeCalls {
			count++
		}
	}
	return count
}

// ComputeFanOut returns the number of CALLS edges originating from nodeID.
func (g *GraphIndex) ComputeFanOut(nodeID string) int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	count := 0
	for _, e := range g.outEdges[nodeID] {
		if e.Type == EdgeTypeCalls {
			count++
		}
	}
	return count
}

// FindEntryPoints returns descendants of serviceID that receive zero
// in-CALLS edges from within the same service.  These are the natural
// entry points — HTTP handlers, queue consumers, etc.
func (g *GraphIndex) FindEntryPoints(serviceID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Collect all descendant IDs of the service using BFS over children.
	members := map[string]bool{serviceID: true}
	queue := []string{serviceID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, cid := range g.children[cur] {
			if !members[cid] {
				members[cid] = true
				queue = append(queue, cid)
			}
		}
	}

	// For each member, check if any in-CALLS edge comes from another member.
	var result []*Node
	for id := range members {
		n, ok := g.nodes[id]
		if !ok {
			continue
		}
		// Only functions/methods are meaningful entry points.
		if n.Type != NodeTypeFunction {
			continue
		}

		calledInternally := false
		for _, e := range g.inEdges[id] {
			if e.Type == EdgeTypeCalls && members[e.SourceID] {
				calledInternally = true
				break
			}
		}
		if !calledInternally {
			result = append(result, n)
		}
	}
	return result
}

// ===================== SUBGRAPH EXTRACTION ================================

// ExtractSubgraph returns the nodes identified by nodeIDs together with
// every edge whose source and target are both in the set.
func (g *GraphIndex) ExtractSubgraph(nodeIDs []string) ([]*Node, []*Edge) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	set := make(map[string]bool, len(nodeIDs))
	nodes := make([]*Node, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		set[id] = true
		if n, ok := g.nodes[id]; ok {
			nodes = append(nodes, n)
		}
	}

	var edges []*Edge
	for _, id := range nodeIDs {
		for _, e := range g.outEdges[id] {
			if set[e.TargetID] {
				edges = append(edges, e)
			}
		}
	}
	return nodes, edges
}

// ============================== STATS ====================================

// NodeCount returns the total number of nodes in the index.
func (g *GraphIndex) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// EdgeCount returns the total number of unique edges in the index
// (counted via outEdges to avoid double-counting).
func (g *GraphIndex) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	count := 0
	for _, edges := range g.outEdges {
		count += len(edges)
	}
	return count
}

// Stats returns a full IndexStats snapshot.
func (g *GraphIndex) Stats() IndexStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodesByType := make(map[string]int, len(g.byType))
	for t, ids := range g.byType {
		nodesByType[string(t)] = len(ids)
	}

	edgesByType := make(map[string]int)
	for _, edges := range g.outEdges {
		for _, e := range edges {
			edgesByType[string(e.Type)]++
		}
	}

	statusCounts := make(map[string]int, len(g.byStatus))
	for s, ids := range g.byStatus {
		statusCounts[s] = len(ids)
	}

	totalEdges := 0
	for _, v := range edgesByType {
		totalEdges += v
	}

	return IndexStats{
		TotalNodes:   len(g.nodes),
		TotalEdges:   totalEdges,
		NodesByType:  nodesByType,
		EdgesByType:  edgesByType,
		FilesIndexed: len(g.byFile),
		StatusCounts: statusCounts,
	}
}
