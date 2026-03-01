# VYUHA AI — Comprehensive Architectural Audit Report

**Prepared by:** Senior Architect  
**Date:** 2025  
**Module:** `github.com/vyuha/vyuha-ai`  
**Version:** Pre-release (Development)

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [System Architecture Overview](#2-system-architecture-overview)
3. [Technology Stack](#3-technology-stack)
4. [Backend Architecture (Go)](#4-backend-architecture-go)
   - 4.1 [Entry Point & Bootstrap](#41-entry-point--bootstrap)
   - 4.2 [API Layer (`internal/api`)](#42-api-layer-internalapi)
   - 4.3 [Graph Index (`internal/graph`)](#43-graph-index-internalgraph)
   - 4.4 [Storage Layer (`internal/storage`)](#44-storage-layer-internalstorage)
   - 4.5 [Parser Subsystem (`internal/parser`)](#45-parser-subsystem-internalparser)
   - 4.6 [Query Layer (`internal/query`)](#46-query-layer-internalquery)
   - 4.7 [AI Subsystem (`internal/ai`)](#47-ai-subsystem-internalai)
   - 4.8 [Runtime Monitoring (`internal/runtime`)](#48-runtime-monitoring-internalruntime)
   - 4.9 [Database Schema (`db/schema.sql`)](#49-database-schema-dbschemasql)
5. [Frontend Architecture (React/TypeScript)](#5-frontend-architecture-reacttypescript)
   - 5.1 [Application Shell (`App.tsx`)](#51-application-shell-apptsx)
   - 5.2 [Graph State Management (`useGraph`)](#52-graph-state-management-usegraph)
   - 5.3 [SSE Real-Time Layer (`useSSE`)](#53-sse-real-time-layer-usesse)
   - 5.4 [AI Agent Interaction (`useAgent`)](#54-ai-agent-interaction-useagent)
   - 5.5 [API Client (`api/client.ts`)](#55-api-client-apiclientts)
   - 5.6 [Type System (`types/graph.ts`)](#56-type-system-typesgraphts)
   - 5.7 [Component Architecture](#57-component-architecture)
6. [Data Flow Diagrams](#6-data-flow-diagrams)
7. [Deployment Architecture](#7-deployment-architecture)
8. [Architectural Improvements Required](#8-architectural-improvements-required)
9. [Advancements Required](#9-advancements-required)
10. [Critical Bugs & Failures](#10-critical-bugs--failures)
11. [Security Assessment](#11-security-assessment)
12. [Performance Assessment](#12-performance-assessment)
13. [Testing Assessment](#13-testing-assessment)
14. [Recommendations Summary](#14-recommendations-summary)

---

## 1. Executive Summary

VYUHA AI is a **code-intelligence platform** that statically analyses Go repositories, constructs a hierarchical graph of code entities (repositories, services, packages, files, functions, structs, interfaces, cloud services, data flows), and provides an AI-powered natural language query interface over that graph. It combines:

- **Static AST-level parsing** (Go `go/ast` + `go/parser`) with incremental checksums
- **In-memory graph index** with 9+ secondary indexes for O(1) lookups
- **SQLite persistence** (WAL mode) with recursive CTEs
- **Dual AI provider** support (AWS Bedrock + Ollama) for embeddings & NL queries
- **Real-time SSE** push for scan progress, runtime events, and agent step broadcasting
- **React Flow** canvas with dagre layout, zoom-aware filtering, focus mode, edge bundling
- **shadcn/ui** component library for a polished control panel

The system is **architecturally sound for a prototype** but requires significant hardening for production: authentication, testing, multi-language support, production database, observability, and CI/CD are all absent.

---

## 2. System Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                        VYUHA AI Platform                             │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────┐                    ┌──────────────────────────────┐ │
│  │  React SPA   │◄──── SSE ────────│     SSE Broadcaster          │ │
│  │  (Vite 5)    │                   │  (Server-Sent Events)        │ │
│  │              │── REST/JSON ──►   │                              │ │
│  │  React Flow  │                   └────────────┬─────────────────┘ │
│  │  + dagre     │                                │                   │
│  │  + shadcn/ui │                    ┌───────────▼─────────────────┐ │
│  └─────────────┘                    │      HTTP Server (Mux)       │ │
│                                      │   Middleware: CORS,Logging,  │ │
│                                      │   Recovery, Rate Limiting    │ │
│                                      └──┬────┬────┬────┬────┬──────┘ │
│                                         │    │    │    │    │        │
│  ┌──────────────┐  ┌──────────────┐  ┌──▼──┐ │  ┌▼──┐ │  ┌▼──────┐ │
│  │   GoParser   │  │  Cloud       │  │Scan │ │  │AI │ │  │Runtime│ │
│  │  (AST Walk)  │  │  Detector    │  │API  │ │  │API│ │  │API    │ │
│  │  + Workers   │  │  + DataFlow  │  └──┬──┘ │  └┬──┘ │  └┬──────┘ │
│  └──────┬───────┘  └──────────────┘     │    │   │    │   │        │
│         │                                │  ┌▼───▼┐   │   │        │
│         │                                │  │Graph│   │   │        │
│         ▼                                │  │API  │   │   │        │
│  ┌──────────────┐                       │  └──┬──┘   │   │        │
│  │  ParseResult │                       │     │      │   │        │
│  │ Nodes+Edges  │─────────────────┐     │     │      │   │        │
│  └──────────────┘                 │     │     │      │   │        │
│                                    ▼     ▼     ▼      ▼   ▼        │
│                          ┌─────────────────────────────────────┐   │
│                          │         Graph Index (In-Memory)      │   │
│                          │  RWMutex — 9 Secondary Indexes       │   │
│                          │  nodes, children, outEdges, inEdges  │   │
│                          │  byType, byFile, byStatus            │   │
│                          │  funcByName, funcByFQN               │   │
│                          └──────────────┬──────────────────────┘   │
│                                         │                          │
│                          ┌──────────────▼──────────────────────┐   │
│                          │       Query Layer (Orchestrator)     │   │
│                          │  4 Modes: direct_graph, subgraph,   │   │
│                          │           agent, sql                 │   │
│                          │  SubgraphExtractor + NodeScorer      │   │
│                          │  VyuhaAgent (6-step tool loop)       │   │
│                          └──────────────┬──────────────────────┘   │
│                                         │                          │
│  ┌──────────────────┐    ┌──────────────▼──────────────────────┐   │
│  │  AI Provider      │◄──│     Embedding Service                │   │
│  │  (Bedrock/Ollama) │   │  In-memory cache, 5 workers          │   │
│  │                    │   │  Cosine similarity search             │   │
│  │  + Job Queue       │   └────────────────────────────────────┘   │
│  │  (2 workers,8 jobs)│                                            │
│  └──────────────────┘                                              │
│                                                                      │
│                          ┌─────────────────────────────────────┐   │
│                          │       SQLite Storage (WAL Mode)      │   │
│                          │  6 Tables: nodes, edges,             │   │
│                          │  runtime_events, data_flow,          │   │
│                          │  type_definitions, embeddings        │   │
│                          │  + schema_migrations                 │   │
│                          └─────────────────────────────────────┘   │
│                                                                      │
│  ┌──────────────────┐                                              │
│  │ Runtime Monitor   │                                              │
│  │ LogTailer(100ms)  │                                              │
│  │ Ingestor          │                                              │
│  └──────────────────┘                                              │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. Technology Stack

### Backend
| Component       | Technology                              | Version  |
|-----------------|-----------------------------------------|----------|
| Language        | Go                                      | 1.22     |
| HTTP Server     | `net/http` (stdlib `http.ServeMux`)      | stdlib   |
| Database        | SQLite via `modernc.org/sqlite`          | pure Go  |
| AI Provider     | AWS Bedrock SDK v2 + Ollama REST         | latest   |
| UUID Generation | `github.com/google/uuid`                | v1.6.0   |
| Rate Limiting   | `golang.org/x/time/rate`                | latest   |
| Parser          | `go/ast`, `go/parser`, `go/token`       | stdlib   |

### Frontend
| Component       | Technology                              | Version  |
|-----------------|-----------------------------------------|----------|
| Framework       | React                                   | 18.2.0   |
| Language        | TypeScript                              | 5.4.5    |
| Build Tool      | Vite                                    | 5.2.8    |
| Graph Canvas    | React Flow (`@xyflow/react`)            | 11.11.3  |
| Layout Engine   | dagre                                   | 0.8.5    |
| Styling         | Tailwind CSS                            | 3.4.3    |
| UI Components   | shadcn/ui (14 components)               | latest   |

### Infrastructure
| Component       | Technology                              | Version  |
|-----------------|-----------------------------------------|----------|
| Container       | Docker (multi-stage)                    | —        |
| Frontend Build  | `node:20-alpine`                        | 20       |
| Backend Build   | `golang:1.22-alpine` (CGO_ENABLED=1)   | 1.22     |
| Runtime         | `alpine:3.19`                           | 3.19     |
| Orchestration   | docker-compose                          | v3.8     |

### Database Schema (6 Tables)
| Table              | Purpose                            |
|--------------------|------------------------------------|
| `nodes`            | Code entities (functions, structs, packages, services, etc.) |
| `edges`            | Relationships (calls, contains, imports, etc.)               |
| `runtime_events`   | Live execution events with trace/span IDs                   |
| `data_flow`        | Data transformation lineage                                  |
| `type_definitions` | Type registry across the codebase                            |
| `embeddings`       | AI embedding vectors (BLOB storage)                          |
| `schema_migrations`| Migration version tracking                                   |

---

## 4. Backend Architecture (Go)

### 4.1 Entry Point & Bootstrap

**File:** `cmd/server/main.go` (207 lines)

The bootstrap sequence follows a strict dependency-injection order:

```
1. Parse CLI flags + environment variables
2. Initialize SQLite Storage (WAL mode, auto-migrate)
3. Create Graph Index → LoadFromStorage()
4. Create SSE Broadcaster
5. Create AI Provider (optional, factory-based)
6. Create Embedding Service (if AI provider available)
7. Create Job Queue (if AI provider available, 2 workers default)
8. Create HTTP Server (wires all dependencies)
9. Create Query Layer (depends on index, storage, AI, SSE)
10. Register signal handler → graceful shutdown with 10s timeout
11. ListenAndServe on configured port (default 8080)
```

**Configuration Sources:**
- CLI flags: `-port`, `-db`, `-ai-provider`, `-ai-model`, `-ai-region`, `-ollama-url`, `-root`
- Environment variables: `PORT`, `DB_PATH`, `AI_PROVIDER`, `AI_MODEL`, `AI_REGION`, `OLLAMA_URL`, `ROOT_PATH`
- Defaults: port=8080, db=`./vyuha.db`, ai-provider="" (disabled), root="" (no auto-scan)

**Graceful Shutdown:**
- Listens for `SIGINT`/`SIGTERM`
- Calls `server.Shutdown(ctx)` with 10-second timeout
- Job queue and embedding service stopped sequentially

**Observation:** The AI provider is optional — the system degrades gracefully if no AI credentials are configured. Only graph browsing and scan features work without AI.

---

### 4.2 API Layer (`internal/api`)

**Files:** `server.go` (335 lines), `handlers_graph.go` (438 lines), `handlers_scan.go` (387 lines), `sse.go` (~170 lines)

#### HTTP Server Structure

```go
type Server struct {
    store    *storage.Storage
    index    *graph.GraphIndex
    sse      *SSEBroadcaster
    ai       ai.Provider      // may be nil
    embedSvc *ai.EmbeddingService
    jobQueue *ai.JobQueue
    query    *query.QueryLayer // set after construction
    scanJobs sync.Map          // job_id → *scanJobStatus
    mux      *http.ServeMux
    server   *http.Server
}
```

#### Route Table (20+ endpoints)

| Method | Path                          | Handler                   | Purpose                          |
|--------|-------------------------------|---------------------------|----------------------------------|
| POST   | `/api/scan`                   | `handleScan`              | Trigger repository scan          |
| GET    | `/api/scan/status`            | `handleScanStatus`        | Poll scan job progress           |
| GET    | `/api/graph/services`         | `handleGetServices`       | List top-level service nodes     |
| GET    | `/api/graph/children`         | `handleGetChildren`       | Get node children at depth       |
| GET    | `/api/graph/subgraph`         | `handleGetSubgraph`       | Get descendant subgraph          |
| GET    | `/api/graph/node`             | `handleGetNodeDetail`     | Get single node detail           |
| GET    | `/api/graph/stats`            | `handleGetStats`          | Graph statistics                 |
| GET    | `/api/graph/search`           | `handleSearch`            | Keyword search over nodes        |
| POST   | `/api/query`                  | `handleQuery`             | Natural language query            |
| POST   | `/api/runtime/events`         | `handleIngestEvent`       | Ingest single runtime event      |
| POST   | `/api/runtime/events/batch`   | `handleIngestBatch`       | Ingest batch runtime events      |
| GET    | `/api/runtime/events`         | `handleGetRuntimeEvents`  | Retrieve runtime events          |
| POST   | `/api/runtime/watch/start`    | `handleWatchStart`        | Start log file tailing           |
| POST   | `/api/runtime/watch/stop`     | `handleWatchStop`         | Stop log file tailing            |
| GET    | `/api/runtime/watch/status`   | `handleWatchStatus`       | Tailer status                    |
| POST   | `/api/ai/jobs`                | `handleSubmitJob`         | Submit AI job                    |
| GET    | `/api/ai/jobs`                | `handleListJobs`          | List AI jobs                     |
| GET    | `/api/ai/jobs/{id}`           | `handleGetJob`            | Get job status                   |
| GET    | `/api/events`                 | `handleSSE`               | SSE stream endpoint              |
| GET    | `/api/health`                 | `handleHealth`            | Health check                     |
| GET    | `/*`                          | SPA fallback              | Serve React frontend             |

#### Middleware Chain
```
Request → CORS → Logging → Recovery → Rate Limiter → Handler
```
- **CORS**: Allows `*` origin, all standard methods, 12-hour preflight cache
- **Logging**: Elapsed time, status code, method, path
- **Recovery**: Catches panics, returns 500, logs stack trace
- **Rate Limiter**: 1000 events/sec burst with `golang.org/x/time/rate`

#### SSE Broadcaster (`sse.go`)

Fan-out architecture with per-client buffered channels:

```go
type SSEBroadcaster struct {
    mu      sync.RWMutex
    clients map[string]chan SSEEvent  // clientID → channel (buffer=64)
}
```

- **Subscribe/Unsubscribe** with UUID-based client IDs
- **Non-blocking broadcast** — drops events for slow clients
- **30-second heartbeat** keeps connections alive
- **Nginx-aware** headers (`X-Accel-Buffering: no`)
- **Targeted broadcast** support (`BroadcastToClient()`)

#### Scan Pipeline (`handlers_scan.go`)

The scan is a **10-step background pipeline** triggered via POST and tracked via polling:

```
POST /api/scan → Create Job → Background Goroutine:
  1. Load existing checksums from file nodes
  2. Create GoParser (reads go.mod)
  3. Parse with progress callback (SSE broadcasts per file)
  4. Run CloudDetector
  5. Run DataFlowExtractor
  6. SaveNodes to SQLite (batch 500)
  7. SaveEdges to SQLite (batch 500)
  8. Save DataFlow records
  9. Reload Graph Index from Storage
  10. Finalize job → SSE scan_complete event
```

- Uses `context.WithoutCancel()` so scan survives HTTP request disconnect
- Panic recovery with job failure tracking
- Prunes old completed/failed jobs (keeps last 10)

#### Children Endpoint — Package Alias Remapping

The `handleGetChildren` endpoint includes a critical remapping layer:

```
Service node "service:github.com/vyuha/vyuha-ai/cmd/server"
  → Children requested with prefix "service:" remapped to "pkg:" 
  → Actually queries children of "pkg:github.com/vyuha/vyuha-ai/cmd/server"
  → Synthesizes missing "contains" edges if not in index
```

This allows the frontend to navigate the hierarchy seamlessly without knowing about the `service:` ↔ `pkg:` ID convention.

---

### 4.3 Graph Index (`internal/graph`)

**Files:** `index.go` (1003 lines), `node.go` (223 lines), `edge.go` (~120 lines)

#### In-Memory Index Architecture

The `GraphIndex` is the **central data structure** — a fully-indexed in-memory mirror of the database:

```go
type GraphIndex struct {
    mu        sync.RWMutex
    nodes     map[string]*Node         // node_id → Node
    children  map[string][]*Node       // parent_id → children
    outEdges  map[string][]*Edge       // source_id → outbound edges
    inEdges   map[string][]*Edge       // target_id → inbound edges
    byType    map[NodeType][]*Node     // type → nodes of that type
    byFile    map[string][]*Node       // file_path → nodes in file
    byStatus  map[string][]*Node       // runtime_status → nodes
    funcByName map[string][]*Node      // function_name → nodes
    funcByFQN  map[string]*Node        // fully_qualified_name → node
}
```

**Key Operations:**
- `LoadFromStorage(ctx, store)` — Bulk load all nodes + edges, rebuild all indexes
- `AddNode(node)` — Insert + update all secondary indexes
- `AddEdge(edge)` — Insert + update outEdges/inEdges adjacency
- `UpdateNodeStatus(id, status, errorCount)` — Runtime status updates
- `RemoveNodesByFile(filePath)` — Incremental re-parse cleanup

**Query Methods:**
- `GetNode(id)` — O(1) by primary key
- `GetChildren(parentID)` — O(1) by parent index
- `GetByType(nodeType)` — O(1) by type index
- `GetByFile(filePath)` — O(1) by file index
- `GetOutEdges(sourceID)` / `GetInEdges(targetID)` — O(1) adjacency
- `ComputeFanIn(id)` / `ComputeFanOut(id)` — Degree calculation
- `GetCallees(id)` / `GetCallers(id)` — Call graph traversal
- `GetDescendants(rootID, maxDepth)` — BFS with configurable depth
- `FindEntryPoints()` — Nodes with zero in-edges of type "calls"
- `SearchNodes(query, limit)` — String matching across names

**Concurrency Model:**
- `sync.RWMutex` — Read-heavy workload optimisation
- All reads take `RLock`, all writes take full `Lock`
- Write operations update all affected indexes atomically

#### Node Type Hierarchy (10 types)

```
NodeTypeRepository       // Root: one per scanned repository
  └─ NodeTypeService     // Directories with package main
  └─ NodeTypePackage     // Every Go package directory
       └─ NodeTypeFile   // Individual .go files
            └─ NodeTypeFunction   // Functions & methods
            └─ NodeTypeStruct     // Struct declarations
            └─ NodeTypeInterface  // Interface declarations
NodeTypeCloudService     // Detected AWS/GCP/Azure services
NodeTypeRuntimeInstance  // Live process instances
NodeTypeDataFlow         // Data transformation nodes
```

Each node carries rich `NodeMetadata`:
- **Function**: signature, parameters, return types, receiver, cyclomatic complexity, has_error_return, has_context_param, source snippet, doc comment
- **Struct**: fields (name, type, tag, exported), embedded types
- **Interface**: methods with parameters and return types
- **File**: imports, checksum, size_bytes, function_count, package_name, is_generated
- **Service**: entry point, port, build tags 
- **Cloud**: cloud_provider, cloud_service, cloud_operation, cloud_resource

#### Edge Type Taxonomy (12 types)

```
EdgeTypeContains        // Hierarchical parent → child
EdgeTypeCalls           // Function → function invocation
EdgeTypeImports         // File → imported package
EdgeTypeImplements      // Struct → interface
EdgeTypeEmbeds          // Struct → embedded struct
EdgeTypeProduces        // Function → data output
EdgeTypeConsumes        // Function → data input
EdgeTypeConnectsTo      // Cloud service → cloud service
EdgeTypeQueuePublish    // Producer → message queue
EdgeTypeQueueSubscribe  // Message queue → consumer
EdgeTypeHTTPCall        // HTTP client → service
EdgeTypeDataTransform   // Data transformation link
```

Each edge carries `EdgeMetadata`:
- **Importance score** (0–100): computed weight for filtering
- **Cross-package** / **cross-service** flags
- **Call metadata**: call_type, call_frequency, is_async, is_deferred, is_cloud_call
- **Import metadata**: alias, is_stdlib, is_third_party
- **Queue metadata**: queue_name, message_type

---

### 4.4 Storage Layer (`internal/storage`)

**Files:** `storage.go` (1078 lines), `schema.go`

#### SQLite Configuration

```go
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
db.Exec("PRAGMA foreign_keys=ON")
db.Exec("PRAGMA busy_timeout=5000")
db.SetMaxOpenConns(1)      // Single writer
db.SetMaxIdleConns(1)
db.SetConnMaxLifetime(0)   // Infinite
```

**Design Decision:** Single-writer (`MaxOpenConns=1`) with mutex-protected writes avoids SQLite lock contention. WAL mode allows concurrent reads while one write is in progress.

#### Batch Save Strategy

```go
func (s *Storage) SaveNodes(ctx context.Context, nodes []*graph.Node) error {
    const batchSize = 500
    for i := 0; i < len(nodes); i += batchSize {
        end := min(i+batchSize, len(nodes))
        batch := nodes[i:end]
        tx, _ := s.db.BeginTx(ctx, nil)
        stmt, _ := tx.Prepare(upsertNodeSQL)
        for _, node := range batch {
            stmt.Exec(/* 20+ columns */)
        }
        tx.Commit()
    }
}
```

- Batches of 500 within explicit transactions
- UPSERT (`INSERT OR REPLACE`) semantics for idempotent saves
- Node metadata serialised as JSON blob in `metadata` column
- Same pattern for `SaveEdges`

#### Recursive CTE for Descendants

```sql
WITH RECURSIVE descendants AS (
    SELECT id FROM nodes WHERE id = ?
    UNION ALL
    SELECT n.id FROM nodes n
    INNER JOIN edges e ON n.id = e.target_id
    INNER JOIN descendants d ON e.source_id = d.id
    WHERE e.edge_type = 'contains'
    LIMIT ?
)
SELECT * FROM nodes WHERE id IN (SELECT id FROM descendants)
```

Used by `GetDescendants(rootID, maxDepth)` to recursively walk the containment hierarchy.

#### Embedding Vector Storage

Embeddings are stored as `BLOB` columns:
```go
func float32sToBytes(v []float32) []byte     // Native float32 → BLOB
func bytesToFloat32s(b []byte) []float32     // BLOB → float32 slice
```

No vector index — similarity search is brute-force over all cached embeddings.

#### Migration System

`schema.go` implements a version-tracked migration system:
- Reads `db/schema.sql` via Go `embed.FS`
- Tracks applied migrations in `schema_migrations` table
- Auto-applies on Storage initialization

---

### 4.5 Parser Subsystem (`internal/parser`)

**File:** `internal/parser/golang/parser.go` (1458 lines)

#### Architecture

The parser is the **data acquisition engine** — it converts source code into the graph model:

```
Repository Root
  │
  ├── discoverGoFiles()     ← Production-grade recursive walk
  │   ├── Skips 11 directory types (vendor, node_modules, .git, etc.)
  │   ├── Skips hidden directories, test files, generated files
  │   ├── Resolves symlinks safely
  │   └── Returns []string of absolute paths
  │
  ├── detectServices()      ← Finds directories with package main
  │
  ├── Incremental checksums ← SHA-256 per file, skips unchanged
  │
  ├── Parallel worker pool  ← runtime.NumCPU() workers
  │   └── parseFile()       ← Per-file AST extraction
  │       ├── File node + metadata
  │       ├── Import edges + stdlib/third-party classification
  │       ├── Function nodes (extractFunction)
  │       │   ├── Parameters, return types, receiver
  │       │   ├── Cyclomatic complexity calculation
  │       │   ├── Source snippet capture
  │       │   ├── Doc comments
  │       │   └── Call edge extraction (extractCalls)
  │       │       ├── Direct function calls
  │       │       ├── Method calls (receiver.Method)
  │       │       ├── go/defer detection
  │       │       └── Call frequency tracking
  │       ├── Struct nodes (extractStruct)
  │       │   ├── Fields with names, types, JSON tags
  │       │   └── Embedded type detection
  │       └── Interface nodes (extractInterface)
  │           └── Method signatures
  │
  ├── CloudDetector         ← Detects AWS/GCP/Azure SDK usage
  │   ├── Scans function calls for cloud SDK patterns
  │   ├── Creates cloud_service nodes
  │   ├── Creates connects_to edges
  │   └── Annotates functions with cloud metadata
  │
  └── DataFlowExtractor    ← Tracks data transformations
      ├── Identifies data sources/sinks
      ├── Creates data_flow nodes
      └── Creates produces/consumes/data_transform edges
```

#### Node ID Convention

```
repo:github.com/vyuha/vyuha-ai
service:github.com/vyuha/vyuha-ai/cmd/server
pkg:github.com/vyuha/vyuha-ai/internal/api
file:github.com/vyuha/vyuha-ai/internal/api/server.go
func:github.com/vyuha/vyuha-ai/internal/api:Server.Start
func:github.com/vyuha/vyuha-ai/internal/api:NewServer
```

Convention: `{type}:{module_path}/{rel_path}` for hierarchical nodes, `{type}:{import_path}:{name}` for leaf nodes.

#### Incremental Parsing

Files are checksummed with SHA-256. The scan pipeline passes existing checksums from file-type nodes in the graph. Unchanged files are skipped entirely, enabling fast re-scans of large repositories.

#### Parallel Worker Pool

```go
numWorkers := runtime.NumCPU()
jobCh := make(chan fileJob, len(jobs))
resultCh := make(chan *parseResult, len(jobs))
// N workers consume from jobCh, produce to resultCh
```

Workers process files independently. The `fset` (token.FileSet) is thread-safe for . A mutex protects the progress callback.

#### Call Edge Resolutionreads

Calls are extracted at AST level and resolved within the same package:
```go
targetID := fmt.Sprintf("func:%s:%s", job.importPath, ci.name)
```

**Limitation:** Cross-package call resolution is best-effort — selector expressions like `pkg.Func()` generate edges to `func:{current_pkg}:pkg.Func` which may not resolve. Full type-checked resolution would require `go/types`.

---

### 4.6 Query Layer (`internal/query`)

**Files:** `query.go` (738 lines), `agent.go` (631 lines), `subgraph.go` (985 lines), `scorer.go` (261 lines)

#### Query Layer Orchestrator

The `QueryLayer` is the **brain of the NL interface**:

```go
type QueryLayer struct {
    index    *graph.GraphIndex
    store    *storage.Storage
    ai       ai.Provider          // may be nil
    embedSvc *ai.EmbeddingService // may be nil
    sse      *SSEBroadcaster
}
```

#### 4 Query Modes

```
Natural Language Question
    │
    ├── classifyQuestion()    ← Keyword/regex pattern matching
    │   Returns: QueryDecision {mode, queryType, targets}
    │
    ├── Mode: direct_graph    ← Simple lookups (stats, find node)
    │   └── resolveTargets() → graph.GetNode/Search
    │
    ├── Mode: subgraph        ← Complex graph queries
    │   └── SubgraphExtractor.Extract()
    │       5 query types: service_overview, call_chain,
    │       failure_path, data_lineage, dependency_impact
    │
    ├── Mode: agent           ← AI-powered multi-step reasoning
    │   └── VyuhaAgent.Run() (max 6 tool-use steps)
    │       Tools: graph queries, storage lookups, subgraph extraction
    │
    └── Mode: sql             ← Direct SQL queries (future)
        └── Not yet implemented
```

#### Question Classification

```go
func classifyQuestion(question string) QueryDecision {
    // Pattern-based classification using keyword matching:
    // "why.*fail|error|crash"        → failure_path
    // "call.*chain|trace|flow"       → call_chain
    // "data.*lineage|transform"      → data_lineage
    // "depend|impact|blast.*radius"  → dependency_impact
    // "overview|architecture"        → service_overview
    // "explain|how.*work|what.*do"   → agent mode
    // Default                        → direct_graph
}
```

**Limitation:** This is keyword-based, not semantic. Complex or ambiguous questions may be misclassified.

#### SubgraphExtractor (5 Query Types)

Each query type follows the same pattern:

```
1. Resolve entry nodes from target names
2. BFS traversal from entry nodes (configurable depth per type)
3. Collect connected edges (type-filtered per query type)
4. Score all nodes using NodeScorer
5. Apply layout positions
6. Detect critical paths
7. Return SubgraphResult with nodes, edges, scores, layout
```

| Query Type          | Max Depth | Edge Types           | Primary Scoring Factor  |
|---------------------|-----------|----------------------|-------------------------|
| service_overview    | 4         | contains, calls      | Fan-in + errors + cloud |
| call_chain          | 8         | calls                | Depth + errors          |
| failure_path        | 6         | calls, contains      | Error status + callers  |
| data_lineage        | 6         | produces, consumes, transforms | DataFlow type + errors |
| dependency_impact   | 5         | calls, imports       | Fan-in (blast radius)   |

#### NodeScorer

Produces `ScoredNode` with score (0.0–1.0) and human-readable reasons:

```go
type ScoredNode struct {
    Node    *graph.Node
    Score   float64
    Reasons []string    // e.g., "fan_in=8 (0.24)", "entry_point (0.15)"
}
```

Scoring factors per query type:
- **ServiceOverview**: fan_in (0.30), fan_out (0.15), errors (0.25), cloud (0.15), entry_point (0.15)
- **CallChain**: depth (0.40), errors (0.30), cloud (0.20)
- **FailurePath**: error_status (0.50), caller_of_failing (0.30), dependency_of_failing (0.20)
- **DataLineage**: data_flow_type (0.40), transformer (0.25), errors (0.20), cloud (0.15)
- **DependencyImpact**: blast_fan_in (0.40), service_level (0.30), already_failing (0.30)

#### VyuhaAgent (AI-Powered Multi-Step Reasoning)

```go
type VyuhaAgent struct {
    ai       ai.Provider
    index    *graph.GraphIndex
    store    *storage.Storage
    subgraph *SubgraphExtractor
    sse      *SSEBroadcaster
}
```

Execution flow:
```
1. Build system prompt with available tools
2. Loop (max 6 steps):
   a. Send user question + conversation history to AI
   b. AI returns either:
      - Final answer → return
      - Tool call → execute tool, append result, continue
3. If 6 steps exhausted → generate fallback summary
4. Each step broadcasts SSE events for real-time UI updates
```

Available agent tools:
- `search_nodes(query)` — Keyword search over graph
- `get_node(id)` — Retrieve node detail
- `get_children(id, depth)` — Walk hierarchy
- `get_subgraph(root_id)` — Get descendant graph
- `get_edges(node_id, direction)` — Adjacency query
- `run_subgraph_query(type, targets)` — Execute SubgraphExtractor

---

### 4.7 AI Subsystem (`internal/ai`)

**Files:** `provider.go` (~200 lines), `jobs.go` (490 lines), `embeddings.go` (438 lines)

#### Provider Interface

```go
type Provider interface {
    Generate(ctx context.Context, prompt string) (string, error)
    StreamGenerate(ctx context.Context, prompt string, callback func(chunk string)) error
    GenerateWithTools(ctx context.Context, prompt string, tools []ToolDef) (*ToolResponse, error)
    Embed(ctx context.Context, text string) ([]float32, error)
}
```

- **AWS Bedrock**: Uses `aws-sdk-go-v2` with `bedrockruntime` client, supports Claude models
- **Ollama**: REST API calls to local Ollama server, configurable URL

Factory pattern:
```go
func NewProvider(config ProviderConfig) (Provider, error) {
    switch config.Provider {
    case "bedrock": return NewBedrockProvider(config)
    case "ollama":  return NewOllamaProvider(config)
    default:        return nil, errors.New("unknown provider")
    }
}
```

#### Job Queue

```go
type JobQueue struct {
    jobs     sync.Map          // job_id → *Job
    workCh   chan *Job          // buffered channel (100)
    provider Provider
    store    *storage.Storage
    index    *graph.GraphIndex
    sse      *SSEBroadcaster
    workers  int               // default 2
}
```

**8 Job Kinds:**
1. `explain_function` — AI-generated function explanation
2. `why_failing` — Root cause analysis for failing nodes
3. `service_overview` — AI-generated service architecture summary
4. `data_lineage` — Data flow analysis
5. `embed_node` — Generate embedding for single node
6. `embed_all` — Bulk embed all nodes
7. `similarity_search` — Find similar nodes via cosine similarity
8. `freeform_query` — Arbitrary NL question to AI

Features:
- Async execution with SSE progress broadcasting per job
- Expired job eviction (>1 hour old completed jobs)
- Job status tracking: `pending` → `running` → `completed`/`failed`

#### Embedding Service

```go
type EmbeddingService struct {
    provider   Provider
    store      *storage.Storage
    mu         sync.RWMutex
    cache      map[string][]float32  // node_id → embedding vector
    dimensions int                    // learned from first embedding
}
```

**Key Operations:**
- `EmbedNode(nodeID)` — Build text content from node metadata → embed → cache + persist
- `EmbedAll(force)` — Parallel embedding with 5 workers, progress tracking
- `SimilaritySearch(query, topK)` — Embed query → cosine similarity against all cached vectors
- `buildNodeContent(node)` — Generates embedding text:
  ```
  Type: function
  Name: handleScan
  File: internal/api/handlers_scan.go
  Signature: func handleScan(w http.ResponseWriter, r *http.Request)
  Source: [first 500 chars of source snippet]
  ```

**Dimension Validation:** The service learns the embedding dimension from the first vector and rejects mismatched dimensions. This guards against provider changes mid-session.

---

### 4.8 Runtime Monitoring (`internal/runtime`)

**Files:** `ingestor.go` (~70 lines), `tailer.go` (212 lines)

#### Log Tailer

```go
type LogTailer struct {
    filePath string
    ingestor *Ingestor
    done     chan struct{}
    // Atomic counters: linesRead, parseErrs, submitErrs
}
```

- Seeks to EOF on start — only processes new lines
- **100ms polling interval** via `time.Ticker`
- Handles partial lines across ticks (accumulator pattern)
- Expects **JSON-encoded log events** (`LogEvent` struct)
- Filters out lines with no `service` or `function` fields
- Rate-limits parse error logging (every 100th error)

#### Ingestor

```go
type Ingestor struct {
    mu       sync.Mutex
    handler  func(ctx context.Context, event LogEvent) error
    count    atomic.Int64
}
```

Simple delegation pattern — the handler is set by the API layer to persist events to storage and update the graph index's runtime status.

---

### 4.9 Database Schema (`db/schema.sql`)

```sql
-- Core entity tables
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    parent_id TEXT,
    file_path TEXT,
    language TEXT,
    depth INTEGER DEFAULT 0,
    line_start INTEGER,
    line_end INTEGER,
    is_exported BOOLEAN DEFAULT FALSE,
    is_entry_point BOOLEAN DEFAULT FALSE,
    error_count INTEGER DEFAULT 0,
    runtime_status TEXT DEFAULT '',
    metadata TEXT,           -- JSON blob
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS edges (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    edge_type TEXT NOT NULL,
    metadata TEXT,           -- JSON blob
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_id) REFERENCES nodes(id),
    FOREIGN KEY (target_id) REFERENCES nodes(id)
);

CREATE TABLE IF NOT EXISTS runtime_events (
    id TEXT PRIMARY KEY,
    node_id TEXT,
    event_type TEXT NOT NULL,
    service TEXT,
    function_name TEXT,
    status TEXT,
    duration_ms REAL,
    error_message TEXT,
    trace_id TEXT,
    span_id TEXT,
    parent_span_id TEXT,
    metadata TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id)
);

CREATE TABLE IF NOT EXISTS data_flow (
    id TEXT PRIMARY KEY,
    source_node_id TEXT,
    target_node_id TEXT,
    data_type TEXT,
    transform_type TEXT,
    description TEXT,
    metadata TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS type_definitions (
    id TEXT PRIMARY KEY,
    node_id TEXT,
    type_name TEXT NOT NULL,
    type_kind TEXT NOT NULL,
    package_path TEXT,
    definition TEXT,
    metadata TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id)
);

CREATE TABLE IF NOT EXISTS embeddings (
    node_id TEXT PRIMARY KEY,
    vector BLOB NOT NULL,
    dimensions INTEGER NOT NULL,
    model TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id)
);

-- Comprehensive indexes
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(runtime_status);
CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type);
CREATE INDEX IF NOT EXISTS idx_runtime_node ON runtime_events(node_id);
CREATE INDEX IF NOT EXISTS idx_runtime_trace ON runtime_events(trace_id);
CREATE INDEX IF NOT EXISTS idx_runtime_service ON runtime_events(service);
CREATE INDEX IF NOT EXISTS idx_runtime_timestamp ON runtime_events(timestamp);
```

---

## 5. Frontend Architecture (React/TypeScript)

### 5.1 Application Shell (`App.tsx`)

**File:** `frontend/src/App.tsx` (296 lines)

The `App` component is the **top-level orchestrator**:

```
App
├── State: scanState, queryResult, agentRun, graphRef
├── doScan(rootPath)        → POST /api/scan → poll status → loadServices
├── doQuery(question)       → POST /api/query → route by mode:
│   ├── subgraph → graphRef.loadSubgraph()
│   ├── agent    → open AgentPanel
│   ├── direct_graph → show node detail
│   └── sql      → show data table
├── useSSE(handlers)        → Real-time event stream
├── ScanSetup               → Repository path input
├── StatusBar               → Stats, query bar
├── GraphCanvas             → React Flow canvas
├── AgentPanel              → AI agent chat
└── LogIngestion            → Runtime log setup
```

#### Query Result Routing

```typescript
const handleQueryResult = (result: QueryResult) => {
    switch (result.mode) {
        case 'subgraph':
            graphRef.current?.loadSubgraph(result.subgraph);
            break;
        case 'agent':
            setAgentRun(result.agentRun);
            break;
        case 'direct_graph':
            // Navigate to node in canvas
            break;
        case 'sql':
            // Display tabular results
            break;
    }
};
```

---

### 5.2 Graph State Management (`useGraph`)

**File:** `frontend/src/hooks/useGraph.ts` (~982 lines)

This is the **most complex frontend module** — a custom React hook managing all graph state:

```typescript
interface UseGraphReturn {
    nodes: Node[];
    edges: Edge[];
    loading: boolean;
    error: string | null;
    // Actions
    loadServices: () => Promise<void>;
    expandNode: (nodeId: string) => Promise<void>;
    loadSubgraph: (result: SubgraphResult) => void;
    setFocusNode: (nodeId: string | null) => void;
    // Filters
    controls: GraphControls;
    setControls: (controls: GraphControls) => void;
}
```

#### Data Pipeline

```
Raw Nodes/Edges (from API)
    │
    ├── Zoom-Aware Filtering
    │   ├── zoom < 0.4  → services only
    │   ├── zoom < 0.7  → services + packages
    │   ├── zoom < 1.0  → + files
    │   └── zoom >= 1.0 → everything (functions, structs, interfaces)
    │
    ├── Control Panel Filters
    │   ├── Node type toggles (service, package, file, function, etc.)
    │   ├── Edge type toggles (calls, contains, imports, etc.)
    │   ├── Show/hide stdlib imports
    │   ├── Show/hide test nodes
    │   └── Min importance threshold for edges
    │
    ├── Focus Mode
    │   └── If focusNode set → BFS to depth 2 → keep only reachable nodes
    │
    ├── Edge Bundling
    │   └── Groups parallel edges between same node pair → control points
    │
    └── Dagre Layout
        ├── Direction: TB (top-to-bottom)
        ├── Node separation: 80px
        ├── Rank separation: 120px
        └── Assigns x, y positions to all nodes
```

#### Auto-Expand on Load (Fixed Bug)

```typescript
const loadServices = async () => {
    const services = await api.getServices();
    // Auto-expand ALL services to depth 2
    const expandPromises = services.map(svc => api.getChildren(svc.id, 2));
    const childrenResults = await Promise.all(expandPromises);
    // De-duplicate nodes, synthesize contains edges, apply layout
    // Result: 35+ nodes, 134+ edges visible immediately
};
```

---

### 5.3 SSE Real-Time Layer (`useSSE`)

**File:** `frontend/src/hooks/useSSE.ts` (190 lines)

```typescript
const useSSE = (handlers: SSEHandlers) => {
    // EventSource with exponential backoff reconnection
    // Reconnect delays: 1s, 2s, 4s, 8s, 16s, 30s (max)
    // Handles 10+ event types:
    //   scan_progress, scan_complete, scan_failed,
    //   node_status_update, runtime_event,
    //   agent_step, agent_tool_call, agent_result,
    //   job_update, heartbeat
};
```

Features:
- Automatic reconnection with exponential backoff (max 30s)
- Typed event handlers for type-safe event processing
- Cleanup on component unmount
- Connection status tracking

---

### 5.4 AI Agent Interaction (`useAgent`)

**File:** `frontend/src/hooks/useAgent.ts` (~60 lines)

Manages the multi-step agent conversation state:
```typescript
interface AgentState {
    isRunning: boolean;
    steps: AgentStep[];       // Tool calls + results
    finalAnswer: string | null;
    error: string | null;
}
```

---

### 5.5 API Client (`api/client.ts`)

**File:** `frontend/src/api/client.ts` (279 lines)

Fully typed API client wrapping `fetch`:

```typescript
// Scan
scanRepository(rootPath: string): Promise<ScanJob>
getScanStatus(jobId: string): Promise<ScanStatus>

// Graph
getServices(): Promise<GraphNode[]>
getChildren(nodeId: string, depth?: number): Promise<ChildrenResponse>
getSubgraph(nodeId: string): Promise<SubgraphResult>
getNode(nodeId: string): Promise<GraphNode>
getStats(): Promise<GraphStats>
searchNodes(query: string): Promise<GraphNode[]>

// Query
askQuestion(question: string): Promise<QueryResult>

// Runtime
ingestLog(event: RuntimeEvent): Promise<void>
ingestLogs(events: RuntimeEvent[]): Promise<void>
watchStart(filePath: string): Promise<void>
watchStop(filePath: string): Promise<void>
watchStatus(): Promise<WatchStatus>

// AI
submitJob(job: JobRequest): Promise<Job>
listJobs(): Promise<Job[]>
getJob(jobId: string): Promise<Job>
```

Features:
- Path normalisation for Windows (`\` → `/`)
- Base URL configuration via import.meta.env
- Error response parsing with structured error objects
- JSON header injection

---

### 5.6 Type System (`types/graph.ts`)

**File:** `frontend/src/types/graph.ts` (382 lines)

Complete TypeScript mirror of Go structs:

```typescript
// 10 Node Types
type NodeType = 'repository' | 'service' | 'package' | 'file' | 
                'function' | 'struct' | 'interface' | 
                'cloud_service' | 'runtime_instance' | 'data_flow';

// 12 Edge Types
type EdgeType = 'contains' | 'calls' | 'imports' | 'implements' | 
                'embeds' | 'produces' | 'consumes' | 'connects_to' | 
                'queue_publish' | 'queue_subscribe' | 'http_call' | 
                'data_transform';

// Full metadata types for all node/edge types
interface NodeMetadata { /* 30+ fields */ }
interface EdgeMetadata { /* 15+ fields */ }

// Query system types
interface QueryDecision { mode, queryType, targets, confidence }
interface SubgraphResult { nodes, edges, scores, layout, criticalPath }
interface AgentRun { steps, finalAnswer }
```

---

### 5.7 Component Architecture

```
App.tsx
├── ScanSetup.tsx              → Repository path input + scan trigger
├── StatusBar.tsx (468 lines)  → Stats display + QueryBar integration
│   └── QueryBar.tsx           → NL query input with shadcn
├── GraphCanvas.tsx (644 lines)→ React Flow canvas
│   ├── Node types:
│   │   ├── ServiceNode.tsx    → Purple gradient, icon
│   │   ├── FunctionNode.tsx   → Blue/green, signature preview
│   │   ├── CloudNode.tsx      → Orange, cloud provider icon
│   │   └── DataFlowNode.tsx   → Teal, direction indicators
│   ├── Edge types:
│   │   └── BundledEdge.tsx    → Animated, styled, bundled paths
│   ├── GraphControls.tsx      → shadcn-based filter panel
│   └── DetailPanel.tsx (676 lines) → Node detail side panel
│       ├── Function details: signature, params, returns, complexity, source
│       ├── Struct details: fields, embeds, implements
│       ├── File details: imports, metrics
│       ├── Service details: entry points, ports
│       ├── Cloud details: provider, service, operations
│       └── Interconnections: incoming/outgoing edges
├── AgentPanel.tsx             → AI agent multi-step chat
└── LogIngestion.tsx           → Runtime log file configuration

UI Library (14 shadcn components):
├── accordion, badge, button, card
├── checkbox, collapsible, dialog
├── input, label, popover
├── scroll-area, separator
├── slider, switch
```

---

## 6. Data Flow Diagrams

### Scan Flow

```
User enters repo path
    │
    ▼
POST /api/scan {root_path}
    │
    ▼
Validate path + go.mod existence
    │
    ▼
Create job (UUID) → Store in sync.Map
    │
    ▼
Return 202 {job_id} immediately
    │
    ▼                               ┌──────────────────────┐
Background goroutine:               │  Frontend polls       │
    │                               │  GET /api/scan/status │
    ├── Load checksums              │  every 1 second       │
    ├── Create GoParser             └──────────┬───────────┘
    ├── discoverGoFiles()                      │
    ├── Checksum filter (skip unchanged)       │
    ├── Parallel parse (N workers) ───── SSE scan_progress ──► Frontend
    ├── CloudDetector.Detect()                 │
    ├── DataFlowExtractor.ExtractAll()         │
    ├── Storage.SaveNodes() (batch 500)        │
    ├── Storage.SaveEdges() (batch 500)        │
    ├── Save DataFlow records                  │
    ├── GraphIndex.LoadFromStorage()           │
    └── Finalize ─────────────── SSE scan_complete ──► Frontend
                                               │
                                               ▼
                                    loadServices() → auto-expand
                                    → 35 nodes, 134 edges render
```

### Query Flow

```
User types natural language question
    │
    ▼
POST /api/query {question}
    │
    ▼
classifyQuestion() → {mode, queryType, targets, confidence}
    │
    ├── mode=direct_graph ──► resolveTargets() → graph lookups → return nodes
    │
    ├── mode=subgraph ──► SubgraphExtractor.Extract()
    │   ├── Resolve entry nodes
    │   ├── BFS traversal (depth-limited)
    │   ├── Collect typed edges
    │   ├── Score nodes (NodeScorer)
    │   ├── Compute layout
    │   └── Return SubgraphResult → Frontend renders focused view
    │
    ├── mode=agent ──► VyuhaAgent.Run()
    │   ├── Step 1: AI generates plan or tool call
    │   │   └── SSE agent_step → Frontend AgentPanel
    │   ├── Step 2-6: Execute tools, append results
    │   │   └── SSE agent_tool_call → Frontend updates
    │   └── Final: AI synthesizes answer
    │       └── SSE agent_result → Frontend displays
    │
    └── mode=sql ──► (Not implemented)
```

### Runtime Monitoring Flow

```
POST /api/runtime/watch/start {file_path}
    │
    ▼
LogTailer.Start()
    ├── Open file, seek to EOF
    ├── Start 100ms poll loop
    │
    │   New line appended to log file
    │       │
    │       ▼
    │   Read line → JSON parse → LogEvent
    │       │
    │       ▼
    │   Ingestor.Submit()
    │       │
    │       ▼
    │   Handler:
    │   ├── storage.SaveRuntimeEvent()
    │   ├── index.UpdateNodeStatus()
    │   └── sse.Broadcast(node_status_update)
    │                │
    │                ▼
    │       Frontend useSSE handler
    │       → Update node visual state (green/red/yellow)
    │
    └── POST /api/runtime/watch/stop → LogTailer.Stop()
```

---

## 7. Deployment Architecture

### Docker Multi-Stage Build

```dockerfile
# Stage 1: Frontend (node:20-alpine)
FROM node:20-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build
# Output: /app/frontend/dist/

# Stage 2: Backend (golang:1.22-alpine)
FROM golang:1.22-alpine AS backend
RUN apk add --no-cache gcc musl-dev  # Required for CGO (SQLite)
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=1 go build -o vyuha ./cmd/server

# Stage 3: Runtime (alpine:3.19)
FROM alpine:3.19
RUN apk add --no-cache ca-certificates git
COPY --from=backend /app/vyuha /usr/local/bin/vyuha
COPY --from=backend /app/db /app/db
EXPOSE 8080
ENTRYPOINT ["vyuha"]
```

### Docker Compose

```yaml
version: "3.8"
services:
  vyuha:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - vyuha-data:/data          # Persistent SQLite storage
    environment:
      - DB_PATH=/data/vyuha.db
      - AI_PROVIDER=              # Optional
      - AI_MODEL=
      - AI_REGION=
    healthcheck:
      test: wget -qO- http://localhost:8080/api/health
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped

volumes:
  vyuha-data:
```

---

## 8. Architectural Improvements Required

### 8.1 [CRITICAL] Authentication & Authorisation

**Current State:** Zero authentication. All endpoints are publicly accessible.

**Required:**
- JWT or session-based authentication middleware
- Role-based access control (viewer, editor, admin)
- API key support for programmatic access
- CORS origin restriction (currently `*`)

### 8.2 [CRITICAL] Testing Infrastructure

**Current State:** Zero test files exist in the entire codebase — no unit tests, integration tests, or end-to-end tests.

**Required:**
- Unit tests for all packages (`_test.go` files)
- Table-driven tests for parser edge cases
- Integration tests for storage layer (SQLite in-memory)
- API handler tests with `httptest` 
- Frontend component tests (React Testing Library)
- End-to-end tests (Playwright or Cypress)
- Target: >80% code coverage

### 8.3 [HIGH] Error Handling Standardisation

**Current State:** Mixed error handling patterns:
- Some handlers return structured `{code, message}` errors
- Some log and silently continue
- Scan pipeline partial failures (data flow extraction) are logged but not tracked
- No error codes catalogue

**Required:**
- Unified error type with error codes, HTTP status, and user-friendly messages
- Error classification: retriable vs. fatal
- Error propagation strategy (wrap with context using `fmt.Errorf` + `%w`)
- Client-facing error contract documented in OpenAPI

### 8.4 [HIGH] Configuration Management

**Current State:** Flat flag/env parsing in `main.go`. No validation, no config struct.

**Required:**
- Structured config type with validation
- YAML/TOML config file support
- Environment-specific profiles (dev, staging, prod)
- Secret management (AI credentials should not be env vars in production)

### 8.5 [HIGH] Observability Stack

**Current State:** `log.Printf` and `slog.Info` throughout. No metrics, no tracing.

**Required:**
- Structured logging with consistent fields (request_id, correlation_id)
- OpenTelemetry tracing for request flow
- Prometheus metrics (scan duration, query latency, cache hit rates, SSE client count)
- Health check should report dependency status (SQLite writable, AI provider reachable)

### 8.6 [MEDIUM] API Versioning

**Current State:** All routes under `/api/` with no version prefix.

**Required:**
- Version prefix: `/api/v1/`
- Deprecation headers for breaking changes
- OpenAPI/Swagger specification

### 8.7 [MEDIUM] Cross-Package Call Resolution

**Current State:** Call edges are resolved within the same package only. Cross-package calls (`pkg.Func()`) generate edges like `func:{current_pkg}:pkg.Func` that do not resolve to actual target nodes.

**Required:**
- Use `go/types` for type-checked resolution
- Resolve import alias → package path → function target
- This will significantly improve call chain and dependency impact query accuracy

### 8.8 [MEDIUM] Graph Index ↔ Storage Consistency

**Current State:** The in-memory index is loaded from storage on startup and after scans. There is no mechanism to detect or recover from drift between the two.

**Required:**
- Periodic consistency checks (background goroutine)
- Write-through or write-behind pattern formalised
- Recovery mechanism: if index diverges from storage, re-load

### 8.9 [MEDIUM] Frontend State Management

**Current State:** All state lives in React hooks (`useState`, `useRef`) with prop drilling. The `useGraph` hook alone is ~982 lines.

**Required:**
- Extract into a proper state management layer (Zustand, Jotai, or React Context + reducers)
- Separate graph data store from UI state
- Memoisation audit (expensive dagre layout recomputation)

### 8.10 [LOW] SPA Routing

**Current State:** Single-page app with no client-side routing. All views are conditional renders within `App.tsx`.

**Required:**
- React Router for proper URL-based navigation
- Deep-linkable views (scan setup, graph view, agent chat, settings)
- Browser back/forward support

---

## 9. Advancements Required

### 9.1 Multi-Language Parser Support

**Current State:** Only Go is supported.

**Required:**
- Parser interface: `Parser { Parse(ctx, checksums, progress) (*ParseResult, error) }`
- Language-specific implementations:
  - **TypeScript/JavaScript**: Use TypeScript Compiler API or Babel AST
  - **Python**: Use `ast` module or Tree-sitter
  - **Java**: Use Eclipse JDT or Tree-sitter
  - **Rust**: Use `syn` crate or Tree-sitter
- Unified node/edge model already supports this (language-agnostic types)
- Language detection from file extensions + tool configuration

### 9.2 Production Database Migration

**Current State:** SQLite (single-file, single-writer).

**Required for Scale:**
- PostgreSQL backend for multi-user concurrent access
- Connection pooling (`pgxpool`)
- For vector search: pgvector extension instead of brute-force cosine similarity
- For graph queries: Consider Neo4j or Apache AGE for PostgreSQL
- Migration path: abstract `Storage` interface, swap implementation

### 9.3 Semantic Query Classification

**Current State:** Keyword/regex-based question classifier.

**Required:**
- Use the AI provider itself to classify questions (few-shot prompt)
- Confidence scoring with fallback to agent mode on low confidence
- Intent extraction: what entity? what relationship? what timeframe?
- Support follow-up questions with conversation context

### 9.4 Incremental Graph Updates

**Current State:** Full reload from storage after every scan (`LoadFromStorage()`).

**Required:**
- Diff-based updates: track which nodes/edges changed during scan
- Apply only the delta to the in-memory index
- Notify connected SSE clients of specific node additions/removals/updates
- Frontend receives granular updates instead of full reload

### 9.5 Git Integration

**Current State:** Git is installed in the Docker image but not used programmatically.

**Required:**
- Git blame integration: map code regions to authors and dates
- Branch/commit-aware scanning: track graph changes across commits
- PR analysis: "what nodes/edges changed in this PR?"
- Historical change heatmap on graph canvas

### 9.6 CI/CD Pipeline

**Current State:** No CI/CD configuration.

**Required:**
- GitHub Actions workflow:
  - Lint: `golangci-lint`, `eslint`
  - Test: `go test ./...`, `vitest`
  - Build: Docker build
  - Security: `govulncheck`, `npm audit`
  - Deploy: Container registry push + Kubernetes/ECS deployment

### 9.7 Multi-Tenancy & Team Features

**Current State:** Single-user, single-repository.

**Required:**
- Organisation/team model
- Multiple repository support
- Shared graph views and saved queries
- Annotation/comment system on graph nodes
- Activity feed

### 9.8 Advanced Visualisation

**Current State:** dagre top-to-bottom layout with zoom filtering.

**Required:**
- Multiple layout algorithms: force-directed, circular, hierarchical
- Clustering by package/service
- Timeline view for runtime events
- Diff view: compare graph at two points in time
- Minimap with region navigation
- Export: SVG, PNG, Mermaid diagram

### 9.9 Plugin/Extension System

**Current State:** Monolithic architecture.

**Required:**
- Plugin interface for custom parsers, detectors, and analysers
- Webhook system for external integrations (Slack, PagerDuty)
- Custom node/edge type registration
- Custom scoring functions

### 9.10 Caching Layer

**Current State:** Embedding cache is in-memory only. No HTTP caching.

**Required:**
- Redis/Memcached for frequently accessed graph queries
- HTTP ETag/Last-Modified headers for static graph data
- Query result caching with TTL
- Cache invalidation on scan completion

---

## 10. Critical Bugs & Failures

### 10.1 [CRITICAL] SQLite Single-Writer Bottleneck

**File:** `internal/storage/storage.go`  
**Issue:** `MaxOpenConns(1)` combined with `sync.Mutex` on write operations means:
- All writes are serialised — acceptable for single-user
- Under concurrent scan + runtime event ingestion + AI job execution, writers queue up
- Log tailer submitting events at high frequency can block scan saves

**Impact:** Under load, write latency degrades linearly. No back-pressure mechanism.

**Fix:** 
- Implement write coalescing (batch runtime events)
- Add write queue with max depth and back-pressure
- For production: migrate to PostgreSQL

### 10.2 [CRITICAL] Unbounded In-Memory Caches

**Files:** `internal/graph/index.go`, `internal/ai/embeddings.go`

**Issue 1: Graph Index** — The entire graph (all nodes + edges + 9 secondary indexes) lives in memory. For a large monorepo with 100K+ nodes, this could consume multiple GB of RAM.

**Issue 2: Embedding Cache** — All embedding vectors are cached in `map[string][]float32`. With 1536-dimension vectors and 50K nodes, that's ~300MB of vectors alone.

**Impact:** OOM kills on memory-constrained containers.

**Fix:**
- Implement LRU eviction for embedding cache
- Consider memory-mapped index for large graphs
- Add memory usage metrics and alerting
- Set maximum cache sizes with configurable limits

### 10.3 [CRITICAL] No Input Validation on SQL-Adjacent Operations

**File:** `internal/storage/storage.go`

**Issue:** While parameterised queries prevent SQL injection, there is no validation on:
- Node ID format (arbitrary strings accepted)
- Edge metadata JSON structure
- Maximum field lengths
- Deeply nested graph structures (unbounded recursion in CTE)

**Impact:** Malformed data can cause unexpected behaviour or storage bloat.

**Fix:**
- Validate node ID format against `{type}:{path}` pattern
- Set maximum metadata JSON size
- Add `LIMIT` to recursive CTEs (already partially done, but verify all paths)

### 10.4 [HIGH] Cross-Package Call Edges Are Phantom Edges

**File:** `internal/parser/golang/parser.go`

**Issue:** When function `A` in package `x` calls `y.Foo()`, the parser creates:
```
Edge: func:x:A → func:x:y.Foo
```
But the actual target is `func:y:Foo`. The generated edge points to a node that **does not exist**.

**Impact:** 
- Call chain queries miss cross-package calls entirely
- Dependency impact analysis underestimates blast radius
- Edge count is inflated with phantom edges

**Fix:** Implement cross-package call resolution using import map + function index lookup.

### 10.5 [HIGH] Scan Job Memory Leak (Mitigated)

**File:** `internal/api/handlers_scan.go`

**Issue:** Scan jobs are stored in `sync.Map`. While `pruneOldScanJobs(10)` runs after each scan completes or fails, a rapid succession of scans that all panic before reaching the pruning code could leak.

**Impact:** Minor memory leak over long-running instances. Mitigated by the pruning mechanism but not eliminated.

**Fix:** Add a periodic background pruner (e.g., every 5 minutes).

### 10.6 [HIGH] SSE Client Channel Leak Potential

**File:** `internal/api/sse.go`

**Issue:** If a client disconnects without the context being cancelled (e.g., network interruption without TCP RST), the subscriber channel remains in the map indefinitely. The heartbeat will detect this only if the write errors.

**Impact:** Leaked channels accumulate, each consuming 64-event buffer memory.

**Fix:**
- Add client timeout: unsubscribe clients that haven't received an event in N minutes
- Track last event delivery time per client
- Periodic stale client sweeper

### 10.7 [HIGH] Race Condition in Embedding Dimension Validation

**File:** `internal/ai/embeddings.go`

**Issue:** The dimension is learned from the first embedding. If two `EmbedNode` calls execute concurrently and the first one hasn't set `dimensions` yet, the second may also try to set it, or validation may fail spuriously.

**Impact:** Potential for embeddings with mismatched dimensions in cache.

**Fix:** Use `sync.Once` for dimension initialisation, or atomic compare-and-swap.

### 10.8 [MEDIUM] Frontend: Dagre Layout Recomputation on Every Filter Change

**File:** `frontend/src/hooks/useGraph.ts`

**Issue:** The full dagre layout (O(V + E) algorithm) runs every time any filter, zoom level, or focus mode changes. With 134+ edges, this is noticeable but manageable. At 1000+ edges, it will cause frame drops.

**Impact:** UI jank on large graphs when adjusting filters.

**Fix:**
- Debounce filter changes (300ms)
- Cache layout results for identical node/edge sets
- Consider incremental layout algorithms (e.g., D3-force with warm start)

### 10.9 [MEDIUM] No Graceful Degradation When AI Provider Fails Mid-Query

**File:** `internal/query/agent.go`

**Issue:** If the AI provider returns an error during an agent's multi-step execution (e.g., rate limit, network failure), the agent loop catches the error and returns it. There is no retry logic, circuit breaker, or fallback to non-AI mode.

**Impact:** AI queries fail completely on transient AI provider errors.

**Fix:**
- Retry with exponential backoff (max 3 retries) for transient errors
- Circuit breaker pattern for sustained failures
- Fallback: if AI fails, try `subgraph` mode classification as backup

### 10.10 [MEDIUM] Log Tailer 100ms Polling is Inefficient

**File:** `internal/runtime/tailer.go`

**Issue:** The tailer polls the file every 100ms using `bufio.ReadBytes`. On Linux, `inotify`/`fsnotify` would be more efficient. On Windows, `ReadDirectoryChangesW`.

**Impact:** 10 polls/second per tailed file. CPU waste when logs are idle.

**Fix:** Use `fsnotify` package for file system event-driven reading. Fall back to polling only on platforms that don't support it.

### 10.11 [LOW] Frontend: No Loading/Error States for Agent Steps

**File:** `frontend/src/components/panels/AgentPanel.tsx`

**Issue:** While the SSE events for agent steps are received, the UI may not clearly indicate when the agent is "thinking" vs. "executing a tool" vs. "done".

**Fix:** Add visual state machine: Thinking → Tool Call (animated) → Tool Result → Thinking → ...

### 10.12 [LOW] Missing Database Foreign Key Enforcement

**File:** `db/schema.sql`

**Issue:** While `FOREIGN KEY` constraints are declared and `PRAGMA foreign_keys=ON` is set, there are edge cases:
- `data_flow` table references `source_node_id` and `target_node_id` but these aren't declared as foreign keys
- Orphaned edges can exist if nodes are deleted without cascading

**Fix:**
- Add `ON DELETE CASCADE` to edge foreign keys
- Add foreign keys to `data_flow` table
- Add periodic orphan cleanup job

---

## 11. Security Assessment

| Area                    | Status    | Risk  | Notes                                         |
|-------------------------|-----------|-------|-----------------------------------------------|
| Authentication          | Missing | CRITICAL | No auth on any endpoint                     |
| Authorisation           | Missing | CRITICAL | No role-based access                        |
| Input Validation        | Partial | HIGH | Parameterised SQL, but no schema validation  |
| CORS                    | Open   | HIGH | Allows `*` origin                            |
| Rate Limiting           | Present | LOW  | 1000/sec burst                               |
| SQL Injection           | Safe    | LOW  | All queries parameterised                    |
| Path Traversal          | Risk   | MEDIUM | `root_path` validated for existence but not sandboxed |
| Secret Management       | Env    | MEDIUM | AI credentials in env vars                  |
| HTTPS/TLS              | Missing | HIGH | HTTP only, no TLS termination configured     |
| Dependency Audit        | Missing | MEDIUM | No `govulncheck` or `npm audit` in CI       |
| Content Security Policy | Missing | LOW  | No CSP headers on SPA                       |

---

## 12. Performance Assessment

| Component               | Current Performance              | Bottleneck                      | Recommendation                          |
|-------------------------|----------------------------------|---------------------------------|-----------------------------------------|
| Parse (31 files)        | <2s                              | N/A                             | Adequate for small repos                |
| Parse (1000+ files)     | ~30-60s estimated                | File I/O + AST parsing          | Already parallelised (NumCPU workers)   |
| Storage SaveNodes       | ~100ms (51 nodes)                | Transaction per batch of 500    | Adequate; batch size tunable            |
| Graph Index Load        | ~50ms (51 nodes, 292 edges)      | Linear scan + map builds        | O(N+E), adequate to ~100K entities      |
| SubgraphExtractor       | ~5-10ms                          | BFS depth limit                 | Adequate                                |
| Embedding (per node)    | ~200-500ms                       | AI provider API latency         | Parallelised (5 workers)               |
| Embedding (50K nodes)   | ~2.5-7 hours                     | AI provider rate limits         | Consider batch embedding API            |
| Cosine Similarity       | O(N) linear scan                 | No vector index                 | Use pgvector or FAISS for >10K vectors  |
| Frontend Layout (dagre) | ~50ms (35 nodes, 134 edges)      | O(V+E) per invocation           | Debounce + cache for >500 nodes         |
| SSE Broadcast           | O(clients) per event             | Non-blocking drop for slow clients | Adequate                             |
| SQLite Writes           | Serialised (single writer)       | Mutex contention under load     | Migrate to PostgreSQL for production    |

---

## 13. Testing Assessment

| Test Type               | Status    | Coverage | Notes                                       |
|-------------------------|-----------|----------|---------------------------------------------|
| Unit Tests (Go)         | None   | 0%       | No `_test.go` files exist                   |
| Unit Tests (TypeScript) | None   | 0%       | No test files exist                         |
| Integration Tests       | None   | 0%       | No database or API integration tests        |
| End-to-End Tests        | None   | 0%       | No browser automation tests                 |
| Load Tests              | None   | —        | No performance benchmarks                   |
| Security Tests          | None   | —        | No penetration testing or SAST              |
| Linting                 | Unknown | —       | No linting configuration found              |
| CI/CD                   | None   | —        | No GitHub Actions, Jenkins, etc.            |

**Assessment:** The codebase has **zero automated tests**. This is the single largest risk factor for ongoing development. Any refactoring or feature addition has no safety net.

---

## 14. Recommendations Summary

### Priority 1 — Immediate (Before Any Production Use)

| # | Action                          | Effort   | Impact   |
|---|----------------------------------|----------|----------|
| 1 | Add authentication middleware    | 2 days   | CRITICAL |
| 2 | Write unit tests (>60% coverage) | 2 weeks  | CRITICAL |
| 3 | HTTPS/TLS configuration          | 1 day    | CRITICAL |
| 4 | Input validation layer           | 2 days   | HIGH     |
| 5 | CORS origin restriction          | 1 hour   | HIGH     |

### Priority 2 — Short Term (Next Sprint)

| # | Action                          | Effort   | Impact   |
|---|----------------------------------|----------|----------|
| 6 | CI/CD pipeline (GitHub Actions)  | 2 days   | HIGH     |
| 7 | Structured logging + metrics     | 3 days   | HIGH     |
| 8 | Cross-package call resolution    | 1 week   | HIGH     |
| 9 | Embedding cache LRU eviction     | 1 day    | MEDIUM   |
| 10| SSE stale client cleanup         | 1 day    | MEDIUM   |

### Priority 3 — Medium Term (Next Quarter)

| # | Action                          | Effort   | Impact   |
|---|----------------------------------|----------|----------|
| 11| PostgreSQL migration             | 2 weeks  | HIGH     |
| 12| Multi-language parser support     | 1 month  | HIGH     |
| 13| Semantic query classification     | 1 week   | MEDIUM   |
| 14| Frontend state management refactor| 1 week  | MEDIUM   |
| 15| Git integration                  | 2 weeks  | MEDIUM   |

### Priority 4 — Long Term (Next 6 Months)

| # | Action                          | Effort   | Impact   |
|---|----------------------------------|----------|----------|
| 16| Multi-tenancy                    | 1 month  | HIGH     |
| 17| Plugin/extension system          | 1 month  | MEDIUM   |
| 18| Advanced visualisation modes     | 2 weeks  | MEDIUM   |
| 19| Caching layer (Redis)            | 1 week   | MEDIUM   |
| 20| API versioning + OpenAPI spec    | 1 week   | LOW      |

---

## Appendix A: File Inventory

| File                                    | Lines | Purpose                            |
|-----------------------------------------|-------|------------------------------------|
| `cmd/server/main.go`                   | 207   | Entry point, bootstrap, shutdown   |
| `internal/api/server.go`               | 335   | HTTP server, routes, middleware    |
| `internal/api/handlers_graph.go`       | 438   | Graph query endpoints              |
| `internal/api/handlers_scan.go`        | 387   | Scan trigger + status polling      |
| `internal/api/sse.go`                  | ~170  | SSE broadcaster + HTTP handler     |
| `internal/graph/index.go`              | 1003  | In-memory graph index              |
| `internal/graph/node.go`               | 223   | Node types + metadata              |
| `internal/graph/edge.go`               | ~120  | Edge types + metadata              |
| `internal/storage/storage.go`          | 1078  | SQLite persistence layer           |
| `internal/storage/schema.go`           | ~50   | Migration system                   |
| `internal/parser/golang/parser.go`     | 1458  | Go AST parser + workers            |
| `internal/query/query.go`              | 738   | Query orchestrator                 |
| `internal/query/agent.go`              | 631   | AI agent with tool-use loop        |
| `internal/query/subgraph.go`           | 985   | Subgraph extraction (5 types)      |
| `internal/query/scorer.go`             | 261   | Node relevance scoring             |
| `internal/ai/provider.go`             | ~200  | AI provider interface + factory    |
| `internal/ai/jobs.go`                 | 490   | Job queue (8 kinds, 2 workers)     |
| `internal/ai/embeddings.go`           | 438   | Embedding service + similarity     |
| `internal/runtime/ingestor.go`        | ~70   | Event ingestion handler            |
| `internal/runtime/tailer.go`          | 212   | Log file tailer (100ms poll)       |
| `db/schema.sql`                        | ~170  | Database schema (6 tables)         |
| `frontend/src/App.tsx`                 | 296   | Application shell                  |
| `frontend/src/hooks/useGraph.ts`       | ~982  | Graph state management (dagre)     |
| `frontend/src/hooks/useSSE.ts`         | 190   | SSE client with reconnection       |
| `frontend/src/hooks/useAgent.ts`       | ~60   | Agent interaction state            |
| `frontend/src/api/client.ts`           | 279   | Typed API client                   |
| `frontend/src/types/graph.ts`          | 382   | TypeScript type system             |
| `frontend/src/components/GraphCanvas.tsx`| 644  | React Flow canvas                  |
| `frontend/src/components/GraphControls.tsx`| ~269| Filter control panel              |
| `frontend/src/components/StatusBar.tsx` | 468   | Status + query bar                 |
| `frontend/src/components/panels/DetailPanel.tsx`| ~676| Node detail side panel       |
| **Total Backend (Go)**                  | **~7,245** | —                             |
| **Total Frontend (TS/TSX)**             | **~4,246** | —                             |
| **Grand Total**                         | **~11,500** | —                            |

---

## Appendix B: Dependency Graph

```
cmd/server/main.go
    ├── internal/api          (Server, SSEBroadcaster)
    ├── internal/graph        (GraphIndex)
    ├── internal/storage      (Storage)
    ├── internal/ai           (Provider, EmbeddingService, JobQueue)
    └── internal/query        (QueryLayer)

internal/api
    ├── internal/graph        (read/write index)
    ├── internal/storage      (persistence)
    ├── internal/ai           (job submission)
    ├── internal/query        (NL queries)
    └── internal/parser/golang (scan execution)

internal/query
    ├── internal/graph        (index queries)
    ├── internal/storage      (persistence queries)
    └── internal/ai           (AI generation, embeddings)

internal/ai
    ├── internal/graph        (node context for embeddings)
    └── internal/storage      (embedding persistence)

internal/parser/golang
    └── internal/graph        (node/edge types only)

internal/storage
    └── internal/graph        (node/edge types only)

internal/graph
    └── (no internal dependencies — leaf package)
```

---

*End of Audit Report*
