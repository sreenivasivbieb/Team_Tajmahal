-- ===========================================================================
-- VYUHA AI — Complete SQLite Schema
-- ===========================================================================
--
-- Recommended PRAGMA settings (applied in Go code, not here):
--   PRAGMA journal_mode=WAL;
--   PRAGMA synchronous=NORMAL;
--   PRAGMA cache_size=-64000;
--   PRAGMA foreign_keys=ON;
--   PRAGMA temp_store=MEMORY;
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- TABLE: nodes
-- Stores every entity in the code-intelligence graph.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS nodes (
    id               TEXT PRIMARY KEY,
    type             TEXT NOT NULL,
    name             TEXT NOT NULL,
    parent_id        TEXT,
    file_path        TEXT,
    line_start       INTEGER DEFAULT 0,
    line_end         INTEGER DEFAULT 0,
    runtime_status   TEXT DEFAULT '',
    error_count      INTEGER DEFAULT 0,
    last_seen        DATETIME,
    language         TEXT DEFAULT 'go',
    is_exported      BOOLEAN DEFAULT FALSE,
    depth            INTEGER DEFAULT 0,
    metadata         TEXT NOT NULL DEFAULT '{}'
);

-- ---------------------------------------------------------------------------
-- TABLE: edges
-- Directed relationships between nodes.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS edges (
    id               TEXT PRIMARY KEY,
    source_id        TEXT NOT NULL,
    target_id        TEXT NOT NULL,
    type             TEXT NOT NULL,
    metadata         TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (source_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- TABLE: runtime_events
-- Captures live runtime telemetry linked back to graph nodes.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS runtime_events (
    id               TEXT PRIMARY KEY,
    node_id          TEXT NOT NULL,
    service_id       TEXT,
    function_id      TEXT,
    trace_id         TEXT,
    span_id          TEXT,
    parent_span_id   TEXT,
    event_type       TEXT,
    status           TEXT,
    error_message    TEXT,
    error_code       TEXT,
    latency_ms       INTEGER,
    timestamp        DATETIME NOT NULL,
    metadata         TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- TABLE: data_flow
-- Tracks data transformations between functions.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS data_flow (
    id               TEXT PRIMARY KEY,
    function_id      TEXT NOT NULL,
    kind             TEXT NOT NULL,
    type_name        TEXT NOT NULL,
    source           TEXT,
    sink             TEXT,
    field_maps       TEXT DEFAULT '[]',
    is_aggregate     BOOLEAN DEFAULT FALSE,
    fan_in           INTEGER DEFAULT 0,
    metadata         TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (function_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- TABLE: type_definitions
-- Canonical registry of types discovered across the codebase.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS type_definitions (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    package          TEXT NOT NULL,
    fields           TEXT DEFAULT '[]',
    implements       TEXT DEFAULT '[]',
    crosses_boundary BOOLEAN DEFAULT FALSE,
    language         TEXT DEFAULT 'go',
    metadata         TEXT NOT NULL DEFAULT '{}'
);

-- ---------------------------------------------------------------------------
-- TABLE: embeddings
-- Stores AI-generated vector embeddings keyed to graph nodes.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS embeddings (
    id               TEXT PRIMARY KEY,
    node_id          TEXT NOT NULL,
    content          TEXT NOT NULL,
    vector           BLOB NOT NULL,
    model            TEXT NOT NULL,
    dimensions       INTEGER NOT NULL,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- TABLE: schema_migrations
-- Tracks which migrations have been applied to this database.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS schema_migrations (
    version          INTEGER PRIMARY KEY,
    applied_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    description      TEXT
);

-- ===========================================================================
-- INDEXES
-- ===========================================================================

-- nodes indexes
CREATE INDEX IF NOT EXISTS idx_nodes_parent_id      ON nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_type           ON nodes(type);
CREATE INDEX IF NOT EXISTS idx_nodes_file_path      ON nodes(file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_language       ON nodes(language);
CREATE INDEX IF NOT EXISTS idx_nodes_depth          ON nodes(depth);
CREATE INDEX IF NOT EXISTS idx_nodes_runtime_status ON nodes(runtime_status);
CREATE INDEX IF NOT EXISTS idx_nodes_error_count    ON nodes(error_count DESC);

-- edges indexes
CREATE INDEX IF NOT EXISTS idx_edges_source         ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target         ON edges(target_id);
CREATE INDEX IF NOT EXISTS idx_edges_type           ON edges(type);
CREATE INDEX IF NOT EXISTS idx_edges_source_type    ON edges(source_id, type);
CREATE INDEX IF NOT EXISTS idx_edges_target_type    ON edges(target_id, type);

-- runtime_events indexes
CREATE INDEX IF NOT EXISTS idx_runtime_node         ON runtime_events(node_id);
CREATE INDEX IF NOT EXISTS idx_runtime_timestamp    ON runtime_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_runtime_trace        ON runtime_events(trace_id);
CREATE INDEX IF NOT EXISTS idx_runtime_status       ON runtime_events(status);
CREATE INDEX IF NOT EXISTS idx_runtime_service      ON runtime_events(service_id);

-- embeddings indexes
CREATE INDEX IF NOT EXISTS idx_embeddings_node      ON embeddings(node_id);
