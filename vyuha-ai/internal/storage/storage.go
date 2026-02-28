package storage

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/vyuha/vyuha-ai/internal/graph"
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// RuntimeEvent captures a single runtime telemetry event linked to a graph
// node (e.g. a function invocation, an error, a latency sample).
type RuntimeEvent struct {
	ID           string    `json:"id"`
	NodeID       string    `json:"node_id"`
	ServiceID    string    `json:"service_id,omitempty"`
	FunctionID   string    `json:"function_id,omitempty"`
	TraceID      string    `json:"trace_id,omitempty"`
	SpanID       string    `json:"span_id,omitempty"`
	ParentSpanID string    `json:"parent_span_id,omitempty"`
	EventType    string    `json:"event_type,omitempty"`
	Status       string    `json:"status,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	LatencyMs    int       `json:"latency_ms,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Metadata     string    `json:"metadata,omitempty"` // JSON string
}

// DataFlowRecord represents a row in the data_flow table.
type DataFlowRecord struct {
	ID          string `json:"id"`
	FunctionID  string `json:"function_id"`
	Kind        string `json:"kind"`
	TypeName    string `json:"type_name"`
	Source      string `json:"source,omitempty"`
	Sink        string `json:"sink,omitempty"`
	FieldMaps   string `json:"field_maps,omitempty"` // JSON string
	IsAggregate bool   `json:"is_aggregate,omitempty"`
	FanIn       int    `json:"fan_in,omitempty"`
	Metadata    string `json:"metadata,omitempty"` // JSON string
}

// Embedding stores an AI-generated vector embedding keyed to a graph node.
type Embedding struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	Content    string    `json:"content"`
	Vector     []float32 `json:"vector"`
	Model      string    `json:"model"`
	Dimensions int       `json:"dimensions"`
	CreatedAt  time.Time `json:"created_at"`
}

// NodeFailureStat is returned by GetTopFailingNodes.
type NodeFailureStat struct {
	NodeID       string    `json:"node_id"`
	NodeName     string    `json:"node_name"`
	FailureCount int       `json:"failure_count"`
	LastFailure  time.Time `json:"last_failure"`
}

// TypeCount is a helper used inside GraphStats.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// GraphStats summarises the current state of the graph database.
type GraphStats struct {
	NodesByType      []TypeCount `json:"nodes_by_type"`
	EdgesByType      []TypeCount `json:"edges_by_type"`
	TotalNodes       int         `json:"total_nodes"`
	TotalEdges       int         `json:"total_edges"`
	TotalRuntime     int         `json:"total_runtime_events"`
	NodesWithErrors  int         `json:"nodes_with_errors"`
}

// ---------------------------------------------------------------------------
// Float32 ↔ BLOB helpers
// ---------------------------------------------------------------------------

// float32SliceToBytes serialises a []float32 as raw little-endian bytes.
func float32SliceToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// bytesToFloat32Slice deserialises raw little-endian bytes into []float32.
func bytesToFloat32Slice(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

// Storage is a thread-safe wrapper around a SQLite database that persists
// the VYUHA AI code-intelligence graph.
type Storage struct {
	db *sql.DB
	mu sync.RWMutex
}

// ============================= LIFECYCLE ==================================

// New opens (or creates) the SQLite database at dbPath, applies the
// recommended PRAGMAs, runs any pending migrations and returns a ready
// *Storage.
func New(dbPath string) (*Storage, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open db %q: %w", dbPath, err)
	}

	// Only one writer at a time for SQLite.
	conn.SetMaxOpenConns(1)

	// Apply PRAGMAs.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=MEMORY",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			conn.Close()
			return nil, fmt.Errorf("storage: set pragma %q: %w", p, err)
		}
	}

	s := &Storage{db: conn}
	if err := s.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// ============================ MIGRATIONS ==================================

// migrate ensures the schema_migrations table exists, then applies every
// unapplied Migration from the package-level Migrations slice.
func (s *Storage) migrate() error {
	// Guarantee the migrations tracking table is present.
	const createMigTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version     INTEGER PRIMARY KEY,
		applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		description TEXT
	)`
	if _, err := s.db.Exec(createMigTable); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, m := range Migrations {
		var exists int
		err := s.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.Version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration v%d: %w", m.Version, err)
		}
		if exists > 0 {
			continue
		}

		if _, err := s.db.Exec(m.SQL); err != nil {
			return fmt.Errorf("apply migration v%d (%s): %w", m.Version, m.Description, err)
		}
		if _, err := s.db.Exec(
			"INSERT INTO schema_migrations (version, description) VALUES (?, ?)",
			m.Version, m.Description,
		); err != nil {
			return fmt.Errorf("record migration v%d: %w", m.Version, err)
		}
	}
	return nil
}

// ========================== NODE OPERATIONS ===============================

// SaveNode upserts a single Node.
func (s *Storage) SaveNode(ctx context.Context, node *graph.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := json.Marshal(node.Metadata)
	if err != nil {
		return fmt.Errorf("storage: marshal node metadata: %w", err)
	}

	const q = `INSERT OR REPLACE INTO nodes
		(id, type, name, parent_id, file_path, line_start, line_end,
		 runtime_status, error_count, last_seen, language, is_exported, depth, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, q,
		node.ID, string(node.Type), node.Name, node.ParentID, node.FilePath,
		node.LineStart, node.LineEnd, node.RuntimeStatus, node.ErrorCount,
		node.LastSeen, node.Language, node.IsExported, node.Depth, string(meta),
	)
	if err != nil {
		return fmt.Errorf("storage: save node %q: %w", node.ID, err)
	}
	return nil
}

// SaveNodes batch-inserts nodes in chunks of 500 inside a transaction.
func (s *Storage) SaveNodes(ctx context.Context, nodes []*graph.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	const chunkSize = 500
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		if err := s.saveNodesChunk(ctx, nodes[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) saveNodesChunk(ctx context.Context, nodes []*graph.Node) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin tx (save nodes): %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO nodes
		(id, type, name, parent_id, file_path, line_start, line_end,
		 runtime_status, error_count, last_seen, language, is_exported, depth, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("storage: prepare save-node stmt: %w", err)
	}
	defer stmt.Close()

	for _, n := range nodes {
		meta, err := json.Marshal(n.Metadata)
		if err != nil {
			return fmt.Errorf("storage: marshal node %q metadata: %w", n.ID, err)
		}
		if _, err := stmt.ExecContext(ctx,
			n.ID, string(n.Type), n.Name, n.ParentID, n.FilePath,
			n.LineStart, n.LineEnd, n.RuntimeStatus, n.ErrorCount,
			n.LastSeen, n.Language, n.IsExported, n.Depth, string(meta),
		); err != nil {
			return fmt.Errorf("storage: insert node %q: %w", n.ID, err)
		}
	}
	return tx.Commit()
}

// GetNode retrieves a single Node by ID.
func (s *Storage) GetNode(ctx context.Context, id string) (*graph.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, type, name, parent_id, file_path, line_start, line_end,
		runtime_status, error_count, last_seen, language, is_exported, depth, metadata
		FROM nodes WHERE id = ?`

	n := &graph.Node{}
	var metaStr string
	var lastSeen sql.NullTime
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&n.ID, &n.Type, &n.Name, &n.ParentID, &n.FilePath,
		&n.LineStart, &n.LineEnd, &n.RuntimeStatus, &n.ErrorCount,
		&lastSeen, &n.Language, &n.IsExported, &n.Depth, &metaStr,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: get node %q: %w", id, err)
	}
	if lastSeen.Valid {
		n.LastSeen = lastSeen.Time
	}
	if err := json.Unmarshal([]byte(metaStr), &n.Metadata); err != nil {
		return nil, fmt.Errorf("storage: unmarshal node %q metadata: %w", id, err)
	}
	return n, nil
}

// scanNodes is a shared helper that scans rows into []*graph.Node.
func scanNodes(rows *sql.Rows) ([]*graph.Node, error) {
	defer rows.Close()
	var result []*graph.Node
	for rows.Next() {
		n := &graph.Node{}
		var metaStr string
		var lastSeen sql.NullTime
		if err := rows.Scan(
			&n.ID, &n.Type, &n.Name, &n.ParentID, &n.FilePath,
			&n.LineStart, &n.LineEnd, &n.RuntimeStatus, &n.ErrorCount,
			&lastSeen, &n.Language, &n.IsExported, &n.Depth, &metaStr,
		); err != nil {
			return nil, fmt.Errorf("storage: scan node row: %w", err)
		}
		if lastSeen.Valid {
			n.LastSeen = lastSeen.Time
		}
		if err := json.Unmarshal([]byte(metaStr), &n.Metadata); err != nil {
			return nil, fmt.Errorf("storage: unmarshal node metadata: %w", err)
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// GetChildren returns all nodes whose parent_id equals parentID.
func (s *Storage) GetChildren(ctx context.Context, parentID string) ([]*graph.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, type, name, parent_id, file_path, line_start, line_end,
		runtime_status, error_count, last_seen, language, is_exported, depth, metadata
		FROM nodes WHERE parent_id = ?`
	rows, err := s.db.QueryContext(ctx, q, parentID)
	if err != nil {
		return nil, fmt.Errorf("storage: get children of %q: %w", parentID, err)
	}
	return scanNodes(rows)
}

// GetDescendants returns all transitive children of parentID up to maxDepth
// levels deep using a recursive CTE.
func (s *Storage) GetDescendants(ctx context.Context, parentID string, maxDepth int) ([]*graph.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `WITH RECURSIVE descendants AS (
		SELECT id, type, name, parent_id, file_path, line_start, line_end,
			runtime_status, error_count, last_seen, language, is_exported, depth, metadata,
			1 AS level
		FROM nodes WHERE parent_id = ?
		UNION ALL
		SELECT n.id, n.type, n.name, n.parent_id, n.file_path, n.line_start, n.line_end,
			n.runtime_status, n.error_count, n.last_seen, n.language, n.is_exported, n.depth, n.metadata,
			d.level + 1
		FROM nodes n
		JOIN descendants d ON n.parent_id = d.id
		WHERE d.level < ?
	)
	SELECT id, type, name, parent_id, file_path, line_start, line_end,
		runtime_status, error_count, last_seen, language, is_exported, depth, metadata
	FROM descendants`

	rows, err := s.db.QueryContext(ctx, q, parentID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("storage: get descendants of %q: %w", parentID, err)
	}
	return scanNodes(rows)
}

// GetNodesByType returns all nodes of the specified type.
func (s *Storage) GetNodesByType(ctx context.Context, nodeType graph.NodeType) ([]*graph.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, type, name, parent_id, file_path, line_start, line_end,
		runtime_status, error_count, last_seen, language, is_exported, depth, metadata
		FROM nodes WHERE type = ?`
	rows, err := s.db.QueryContext(ctx, q, string(nodeType))
	if err != nil {
		return nil, fmt.Errorf("storage: get nodes by type %q: %w", nodeType, err)
	}
	return scanNodes(rows)
}

// GetNodesByFile returns all nodes associated with a given file path.
func (s *Storage) GetNodesByFile(ctx context.Context, filePath string) ([]*graph.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, type, name, parent_id, file_path, line_start, line_end,
		runtime_status, error_count, last_seen, language, is_exported, depth, metadata
		FROM nodes WHERE file_path = ?`
	rows, err := s.db.QueryContext(ctx, q, filePath)
	if err != nil {
		return nil, fmt.Errorf("storage: get nodes by file %q: %w", filePath, err)
	}
	return scanNodes(rows)
}

// UpdateNodeStatus sets the runtime_status and error_count for a node.
func (s *Storage) UpdateNodeStatus(ctx context.Context, nodeID string, status string, errorCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `UPDATE nodes SET runtime_status = ?, error_count = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, status, errorCount, nodeID)
	if err != nil {
		return fmt.Errorf("storage: update node status %q: %w", nodeID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("storage: node %q not found", nodeID)
	}
	return nil
}

// DeleteNodesByFile removes all nodes for a file. Edges are cascade-deleted
// via foreign key constraints.
func (s *Storage) DeleteNodesByFile(ctx context.Context, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `DELETE FROM nodes WHERE file_path = ?`
	if _, err := s.db.ExecContext(ctx, q, filePath); err != nil {
		return fmt.Errorf("storage: delete nodes by file %q: %w", filePath, err)
	}
	return nil
}

// SearchNodes performs a LIKE search against node name and file_path.
func (s *Storage) SearchNodes(ctx context.Context, query string, limit int) ([]*graph.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, type, name, parent_id, file_path, line_start, line_end,
		runtime_status, error_count, last_seen, language, is_exported, depth, metadata
		FROM nodes WHERE name LIKE ? OR file_path LIKE ? LIMIT ?`
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, q, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("storage: search nodes %q: %w", query, err)
	}
	return scanNodes(rows)
}

// GetAllNodes returns every node in the database. Used for bulk-loading
// the in-memory graph index on startup.
func (s *Storage) GetAllNodes(ctx context.Context) ([]*graph.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, type, name, parent_id, file_path, line_start, line_end,
		runtime_status, error_count, last_seen, language, is_exported, depth, metadata
		FROM nodes`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("storage: get all nodes: %w", err)
	}
	return scanNodes(rows)
}

// GetAllEdges returns every edge in the database. Used for bulk-loading
// the in-memory graph index on startup.
func (s *Storage) GetAllEdges(ctx context.Context) ([]*graph.Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, source_id, target_id, type, metadata FROM edges`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("storage: get all edges: %w", err)
	}
	return scanEdges(rows)
}

// ========================== EDGE OPERATIONS ===============================

// SaveEdge upserts a single Edge.
func (s *Storage) SaveEdge(ctx context.Context, edge *graph.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := json.Marshal(edge.Metadata)
	if err != nil {
		return fmt.Errorf("storage: marshal edge metadata: %w", err)
	}

	const q = `INSERT OR REPLACE INTO edges (id, source_id, target_id, type, metadata)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q,
		edge.ID, edge.SourceID, edge.TargetID, string(edge.Type), string(meta),
	); err != nil {
		return fmt.Errorf("storage: save edge %q: %w", edge.ID, err)
	}
	return nil
}

// SaveEdges batch-inserts edges in a single transaction.
func (s *Storage) SaveEdges(ctx context.Context, edges []*graph.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build a set of node IDs that exist in the database so we can skip
	// edges whose source_id or target_id reference non-existent nodes
	// (e.g. stdlib packages, cross-module targets). Without this filter
	// foreign-key constraints cause the entire transaction to rollback.
	nodeIDs := make(map[string]struct{})
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM nodes`)
	if err != nil {
		return fmt.Errorf("storage: query node ids for edge filter: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("storage: scan node id: %w", err)
		}
		nodeIDs[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("storage: node id rows: %w", err)
	}

	// Filter edges to only those whose both endpoints exist.
	valid := make([]*graph.Edge, 0, len(edges))
	for _, e := range edges {
		if _, srcOK := nodeIDs[e.SourceID]; !srcOK {
			continue
		}
		if _, tgtOK := nodeIDs[e.TargetID]; !tgtOK {
			continue
		}
		valid = append(valid, e)
	}

	if len(valid) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin tx (save edges): %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO edges
		(id, source_id, target_id, type, metadata) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("storage: prepare save-edge stmt: %w", err)
	}
	defer stmt.Close()

	for _, e := range valid {
		meta, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("storage: marshal edge %q metadata: %w", e.ID, err)
		}
		if _, err := stmt.ExecContext(ctx,
			e.ID, e.SourceID, e.TargetID, string(e.Type), string(meta),
		); err != nil {
			return fmt.Errorf("storage: insert edge %q: %w", e.ID, err)
		}
	}
	return tx.Commit()
}

// scanEdges is a shared helper that scans rows into []*graph.Edge.
func scanEdges(rows *sql.Rows) ([]*graph.Edge, error) {
	defer rows.Close()
	var result []*graph.Edge
	for rows.Next() {
		e := &graph.Edge{}
		var metaStr string
		if err := rows.Scan(&e.ID, &e.SourceID, &e.TargetID, &e.Type, &metaStr); err != nil {
			return nil, fmt.Errorf("storage: scan edge row: %w", err)
		}
		if err := json.Unmarshal([]byte(metaStr), &e.Metadata); err != nil {
			return nil, fmt.Errorf("storage: unmarshal edge metadata: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// GetEdgesBySource returns all edges originating from sourceID.
func (s *Storage) GetEdgesBySource(ctx context.Context, sourceID string) ([]*graph.Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, source_id, target_id, type, metadata FROM edges WHERE source_id = ?`
	rows, err := s.db.QueryContext(ctx, q, sourceID)
	if err != nil {
		return nil, fmt.Errorf("storage: get edges by source %q: %w", sourceID, err)
	}
	return scanEdges(rows)
}

// GetEdgesByTarget returns all edges pointing to targetID.
func (s *Storage) GetEdgesByTarget(ctx context.Context, targetID string) ([]*graph.Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, source_id, target_id, type, metadata FROM edges WHERE target_id = ?`
	rows, err := s.db.QueryContext(ctx, q, targetID)
	if err != nil {
		return nil, fmt.Errorf("storage: get edges by target %q: %w", targetID, err)
	}
	return scanEdges(rows)
}

// GetEdgesByType returns all edges of the specified type.
func (s *Storage) GetEdgesByType(ctx context.Context, edgeType graph.EdgeType) ([]*graph.Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, source_id, target_id, type, metadata FROM edges WHERE type = ?`
	rows, err := s.db.QueryContext(ctx, q, string(edgeType))
	if err != nil {
		return nil, fmt.Errorf("storage: get edges by type %q: %w", edgeType, err)
	}
	return scanEdges(rows)
}

// GetEdgesBetweenNodes returns all edges where both source_id AND target_id
// are in the provided nodeIDs set.
func (s *Storage) GetEdgesBetweenNodes(ctx context.Context, nodeIDs []string) ([]*graph.Edge, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	placeholders := strings.Repeat("?,", len(nodeIDs))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma

	q := fmt.Sprintf(
		`SELECT id, source_id, target_id, type, metadata FROM edges
		 WHERE source_id IN (%s) AND target_id IN (%s)`,
		placeholders, placeholders,
	)

	// Build args: nodeIDs twice (once for source_id IN, once for target_id IN).
	args := make([]any, 0, len(nodeIDs)*2)
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	for _, id := range nodeIDs {
		args = append(args, id)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: get edges between nodes: %w", err)
	}
	return scanEdges(rows)
}

// DeleteEdgesBySource removes all edges originating from sourceID.
func (s *Storage) DeleteEdgesBySource(ctx context.Context, sourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `DELETE FROM edges WHERE source_id = ?`
	if _, err := s.db.ExecContext(ctx, q, sourceID); err != nil {
		return fmt.Errorf("storage: delete edges by source %q: %w", sourceID, err)
	}
	return nil
}

// ====================== RUNTIME EVENT OPERATIONS =========================

// SaveRuntimeEvent inserts a single runtime event.
func (s *Storage) SaveRuntimeEvent(ctx context.Context, event *RuntimeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Metadata == "" {
		event.Metadata = "{}"
	}

	const q = `INSERT OR REPLACE INTO runtime_events
		(id, node_id, service_id, function_id, trace_id, span_id, parent_span_id,
		 event_type, status, error_message, error_code, latency_ms, timestamp, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := s.db.ExecContext(ctx, q,
		event.ID, event.NodeID, event.ServiceID, event.FunctionID,
		event.TraceID, event.SpanID, event.ParentSpanID,
		event.EventType, event.Status, event.ErrorMessage, event.ErrorCode,
		event.LatencyMs, event.Timestamp, event.Metadata,
	); err != nil {
		return fmt.Errorf("storage: save runtime event %q: %w", event.ID, err)
	}
	return nil
}

// SaveRuntimeEvents batch-inserts runtime events in a single transaction.
func (s *Storage) SaveRuntimeEvents(ctx context.Context, events []*RuntimeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin tx (save runtime events): %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO runtime_events
		(id, node_id, service_id, function_id, trace_id, span_id, parent_span_id,
		 event_type, status, error_message, error_code, latency_ms, timestamp, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("storage: prepare save-runtime-event stmt: %w", err)
	}
	defer stmt.Close()

	for _, ev := range events {
		if ev.ID == "" {
			ev.ID = uuid.New().String()
		}
		if ev.Metadata == "" {
			ev.Metadata = "{}"
		}
		if _, err := stmt.ExecContext(ctx,
			ev.ID, ev.NodeID, ev.ServiceID, ev.FunctionID,
			ev.TraceID, ev.SpanID, ev.ParentSpanID,
			ev.EventType, ev.Status, ev.ErrorMessage, ev.ErrorCode,
			ev.LatencyMs, ev.Timestamp, ev.Metadata,
		); err != nil {
			return fmt.Errorf("storage: insert runtime event %q: %w", ev.ID, err)
		}
	}
	return tx.Commit()
}

// scanRuntimeEvents is a shared helper that scans rows into []*RuntimeEvent.
func scanRuntimeEvents(rows *sql.Rows) ([]*RuntimeEvent, error) {
	defer rows.Close()
	var result []*RuntimeEvent
	for rows.Next() {
		ev := &RuntimeEvent{}
		if err := rows.Scan(
			&ev.ID, &ev.NodeID, &ev.ServiceID, &ev.FunctionID,
			&ev.TraceID, &ev.SpanID, &ev.ParentSpanID,
			&ev.EventType, &ev.Status, &ev.ErrorMessage, &ev.ErrorCode,
			&ev.LatencyMs, &ev.Timestamp, &ev.Metadata,
		); err != nil {
			return nil, fmt.Errorf("storage: scan runtime event row: %w", err)
		}
		result = append(result, ev)
	}
	return result, rows.Err()
}

// GetLatestNodeStatus returns the most recent status string for a node.
func (s *Storage) GetLatestNodeStatus(ctx context.Context, nodeID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT status FROM runtime_events WHERE node_id = ? ORDER BY timestamp DESC LIMIT 1`
	var status string
	if err := s.db.QueryRowContext(ctx, q, nodeID).Scan(&status); err != nil {
		return "", fmt.Errorf("storage: get latest status for node %q: %w", nodeID, err)
	}
	return status, nil
}

// GetFailuresInWindow returns all runtime events with status 'error' within
// the specified duration from now.
func (s *Storage) GetFailuresInWindow(ctx context.Context, window time.Duration) ([]*RuntimeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-window)
	const q = `SELECT id, node_id, service_id, function_id, trace_id, span_id, parent_span_id,
		event_type, status, error_message, error_code, latency_ms, timestamp, metadata
		FROM runtime_events WHERE status = 'error' AND timestamp >= ? ORDER BY timestamp DESC`
	rows, err := s.db.QueryContext(ctx, q, cutoff)
	if err != nil {
		return nil, fmt.Errorf("storage: get failures in window: %w", err)
	}
	return scanRuntimeEvents(rows)
}

// GetFailuresByNode returns the most recent failures for a specific node.
func (s *Storage) GetFailuresByNode(ctx context.Context, nodeID string, limit int) ([]*RuntimeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, node_id, service_id, function_id, trace_id, span_id, parent_span_id,
		event_type, status, error_message, error_code, latency_ms, timestamp, metadata
		FROM runtime_events WHERE node_id = ? AND status = 'error' ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, nodeID, limit)
	if err != nil {
		return nil, fmt.Errorf("storage: get failures for node %q: %w", nodeID, err)
	}
	return scanRuntimeEvents(rows)
}

// GetTopFailingNodes returns the nodes with the most failures in the given
// window, ordered by failure count descending.
func (s *Storage) GetTopFailingNodes(ctx context.Context, window time.Duration, limit int) ([]NodeFailureStat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-window)
	const q = `SELECT re.node_id, COALESCE(n.name, ''), COUNT(*) AS failure_count, MAX(re.timestamp) AS last_failure
		FROM runtime_events re
		LEFT JOIN nodes n ON n.id = re.node_id
		WHERE re.status = 'error' AND re.timestamp >= ?
		GROUP BY re.node_id
		ORDER BY failure_count DESC
		LIMIT ?`

	rows, err := s.db.QueryContext(ctx, q, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("storage: get top failing nodes: %w", err)
	}
	defer rows.Close()

	var result []NodeFailureStat
	for rows.Next() {
		var stat NodeFailureStat
		if err := rows.Scan(&stat.NodeID, &stat.NodeName, &stat.FailureCount, &stat.LastFailure); err != nil {
			return nil, fmt.Errorf("storage: scan failure stat row: %w", err)
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

// GetTraceEvents returns all runtime events for a given trace, ordered by
// timestamp ascending (suitable for replay).
func (s *Storage) GetTraceEvents(ctx context.Context, traceID string) ([]*RuntimeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, node_id, service_id, function_id, trace_id, span_id, parent_span_id,
		event_type, status, error_message, error_code, latency_ms, timestamp, metadata
		FROM runtime_events WHERE trace_id = ? ORDER BY timestamp ASC`
	rows, err := s.db.QueryContext(ctx, q, traceID)
	if err != nil {
		return nil, fmt.Errorf("storage: get trace events %q: %w", traceID, err)
	}
	return scanRuntimeEvents(rows)
}

// GetRecentEvents returns the most recent runtime events for a node.
func (s *Storage) GetRecentEvents(ctx context.Context, nodeID string, limit int) ([]*RuntimeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, node_id, service_id, function_id, trace_id, span_id, parent_span_id,
		event_type, status, error_message, error_code, latency_ms, timestamp, metadata
		FROM runtime_events WHERE node_id = ? ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, nodeID, limit)
	if err != nil {
		return nil, fmt.Errorf("storage: get recent events for node %q: %w", nodeID, err)
	}
	return scanRuntimeEvents(rows)
}

// ====================== DATA FLOW OPERATIONS =============================

// SaveDataFlow upserts a single data flow record.
func (s *Storage) SaveDataFlow(ctx context.Context, flow *DataFlowRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if flow.ID == "" {
		flow.ID = uuid.New().String()
	}
	if flow.FieldMaps == "" {
		flow.FieldMaps = "[]"
	}
	if flow.Metadata == "" {
		flow.Metadata = "{}"
	}

	const q = `INSERT OR REPLACE INTO data_flow
		(id, function_id, kind, type_name, source, sink, field_maps, is_aggregate, fan_in, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := s.db.ExecContext(ctx, q,
		flow.ID, flow.FunctionID, flow.Kind, flow.TypeName,
		flow.Source, flow.Sink, flow.FieldMaps, flow.IsAggregate,
		flow.FanIn, flow.Metadata,
	); err != nil {
		return fmt.Errorf("storage: save data flow %q: %w", flow.ID, err)
	}
	return nil
}

// GetDataFlow returns all data-flow records for a given function node.
func (s *Storage) GetDataFlow(ctx context.Context, functionID string) ([]*DataFlowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, function_id, kind, type_name, source, sink, field_maps,
		is_aggregate, fan_in, metadata FROM data_flow WHERE function_id = ?`
	rows, err := s.db.QueryContext(ctx, q, functionID)
	if err != nil {
		return nil, fmt.Errorf("storage: get data flow for function %q: %w", functionID, err)
	}
	defer rows.Close()

	var result []*DataFlowRecord
	for rows.Next() {
		f := &DataFlowRecord{}
		if err := rows.Scan(
			&f.ID, &f.FunctionID, &f.Kind, &f.TypeName,
			&f.Source, &f.Sink, &f.FieldMaps, &f.IsAggregate,
			&f.FanIn, &f.Metadata,
		); err != nil {
			return nil, fmt.Errorf("storage: scan data flow row: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

// ====================== EMBEDDING OPERATIONS =============================

// SaveEmbedding upserts a single embedding record.
func (s *Storage) SaveEmbedding(ctx context.Context, emb *Embedding) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if emb.ID == "" {
		emb.ID = uuid.New().String()
	}

	const q = `INSERT OR REPLACE INTO embeddings
		(id, node_id, content, vector, model, dimensions, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	if _, err := s.db.ExecContext(ctx, q,
		emb.ID, emb.NodeID, emb.Content, float32SliceToBytes(emb.Vector),
		emb.Model, emb.Dimensions, emb.CreatedAt,
	); err != nil {
		return fmt.Errorf("storage: save embedding %q: %w", emb.ID, err)
	}
	return nil
}

// GetEmbedding returns the embedding for a specific node.
func (s *Storage) GetEmbedding(ctx context.Context, nodeID string) (*Embedding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, node_id, content, vector, model, dimensions, created_at
		FROM embeddings WHERE node_id = ? LIMIT 1`

	emb := &Embedding{}
	var vectorBlob []byte
	if err := s.db.QueryRowContext(ctx, q, nodeID).Scan(
		&emb.ID, &emb.NodeID, &emb.Content, &vectorBlob,
		&emb.Model, &emb.Dimensions, &emb.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("storage: get embedding for node %q: %w", nodeID, err)
	}
	emb.Vector = bytesToFloat32Slice(vectorBlob)
	return emb, nil
}

// GetAllEmbeddings returns every embedding in the database. This is intended
// for loading all vectors into memory for similarity search.
func (s *Storage) GetAllEmbeddings(ctx context.Context) ([]*Embedding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const q = `SELECT id, node_id, content, vector, model, dimensions, created_at FROM embeddings`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("storage: get all embeddings: %w", err)
	}
	defer rows.Close()

	var result []*Embedding
	for rows.Next() {
		emb := &Embedding{}
		var vectorBlob []byte
		if err := rows.Scan(
			&emb.ID, &emb.NodeID, &emb.Content, &vectorBlob,
			&emb.Model, &emb.Dimensions, &emb.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("storage: scan embedding row: %w", err)
		}
		emb.Vector = bytesToFloat32Slice(vectorBlob)
		result = append(result, emb)
	}
	return result, rows.Err()
}

// ============================== STATS ====================================

// GetGraphStats returns aggregate counts summarising the graph database.
func (s *Storage) GetGraphStats(ctx context.Context) (*GraphStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &GraphStats{}

	// Nodes by type.
	{
		rows, err := s.db.QueryContext(ctx, `SELECT type, COUNT(*) FROM nodes GROUP BY type`)
		if err != nil {
			return nil, fmt.Errorf("storage: stats nodes by type: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var tc TypeCount
			if err := rows.Scan(&tc.Type, &tc.Count); err != nil {
				return nil, fmt.Errorf("storage: scan node type count: %w", err)
			}
			stats.TotalNodes += tc.Count
			stats.NodesByType = append(stats.NodesByType, tc)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	// Edges by type.
	{
		rows, err := s.db.QueryContext(ctx, `SELECT type, COUNT(*) FROM edges GROUP BY type`)
		if err != nil {
			return nil, fmt.Errorf("storage: stats edges by type: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var tc TypeCount
			if err := rows.Scan(&tc.Type, &tc.Count); err != nil {
				return nil, fmt.Errorf("storage: scan edge type count: %w", err)
			}
			stats.TotalEdges += tc.Count
			stats.EdgesByType = append(stats.EdgesByType, tc)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	// Total runtime events.
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_events`).Scan(&stats.TotalRuntime); err != nil {
		return nil, fmt.Errorf("storage: stats runtime events: %w", err)
	}

	// Nodes with errors.
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE error_count > 0`).Scan(&stats.NodesWithErrors); err != nil {
		return nil, fmt.Errorf("storage: stats nodes with errors: %w", err)
	}

	return stats, nil
}
