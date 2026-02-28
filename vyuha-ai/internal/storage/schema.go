package storage

import (
	"github.com/vyuha/vyuha-ai/db"
)

// ---------------------------------------------------------------------------
// Schema version
// ---------------------------------------------------------------------------

// SchemaVersion is the current database schema version.
const SchemaVersion = 2

// GetSchema returns the full embedded SQL schema as a string.
// The returned SQL can be executed directly against a SQLite database.
func GetSchema() string {
	return db.SchemaSQL
}

// ---------------------------------------------------------------------------
// Migration support
// ---------------------------------------------------------------------------

// Migration describes a single schema migration that can be applied to the
// database. Migrations are ordered by Version and are idempotent.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// Migrations is the ordered list of all schema migrations.
// Apply them sequentially; skip any whose Version is already recorded
// in the schema_migrations table.
var Migrations = []Migration{
	{
		Version:     1,
		Description: "Initial schema — nodes, edges, runtime_events, data_flow, type_definitions, embeddings, schema_migrations",
		SQL:         db.SchemaSQL,
	},
	{
		Version:     2,
		Description: "Add ON DELETE CASCADE to runtime_events.node_id and data_flow.function_id foreign keys",
		SQL: `
-- Recreate runtime_events with ON DELETE CASCADE
CREATE TABLE IF NOT EXISTS runtime_events_new (
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
INSERT OR IGNORE INTO runtime_events_new SELECT * FROM runtime_events;
DROP TABLE IF EXISTS runtime_events;
ALTER TABLE runtime_events_new RENAME TO runtime_events;

-- Recreate data_flow with ON DELETE CASCADE
CREATE TABLE IF NOT EXISTS data_flow_new (
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
INSERT OR IGNORE INTO data_flow_new SELECT * FROM data_flow;
DROP TABLE IF EXISTS data_flow;
ALTER TABLE data_flow_new RENAME TO data_flow;

-- Recreate indexes on the renamed tables
CREATE INDEX IF NOT EXISTS idx_runtime_node      ON runtime_events(node_id);
CREATE INDEX IF NOT EXISTS idx_runtime_timestamp  ON runtime_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_runtime_trace      ON runtime_events(trace_id);
CREATE INDEX IF NOT EXISTS idx_runtime_status     ON runtime_events(status);
CREATE INDEX IF NOT EXISTS idx_runtime_service    ON runtime_events(service_id);
`,
	},
}
