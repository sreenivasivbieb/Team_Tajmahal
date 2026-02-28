# VYUHA AI — Comprehensive Codebase Audit Report

**Generated:** 2025  
**Scope:** 40+ source files across Go backend, TypeScript/React frontend, Docker, SQL schema  
**Module:** `github.com/vyuha/vyuha-ai` (Go 1.22)

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Architecture Overview](#architecture-overview)
3. [Backend — Go](#backend--go)
   - [cmd/server](#cmdserver)
   - [internal/api](#internalapi)
   - [internal/storage](#internalstorage)
   - [internal/graph](#internalgraph)
   - [internal/parser/golang](#internalparsergolang)
   - [internal/ai](#internalai)
   - [internal/query](#internalquery)
   - [internal/runtime](#internalruntime)
   - [db](#db)
   - [scripts](#scripts)
4. [Frontend — TypeScript/React](#frontend--typescriptreact)
   - [Entry & Config](#entry--config)
   - [api/client.ts](#apiclientts)
   - [types/graph.ts](#typests)
   - [hooks](#hooks)
   - [components](#components)
5. [Infrastructure](#infrastructure)
6. [Cross-Cutting Issues](#cross-cutting-issues)
7. [Bug Summary](#bug-summary)
8. [Recommendations](#recommendations)

---

## Executive Summary

VYUHA AI is a **code-intelligence graph platform** that parses Go repositories into a node/edge graph, enriches it with cloud-service detection and data-flow analysis, ingests runtime events, and provides an AI-powered query layer (Bedrock or Ollama). The React frontend renders the graph with React Flow and offers real-time SSE updates.

### Key Statistics

| Metric | Value |
|--------|-------|
| Go source files | ~25 |
| TypeScript source files | ~15 |
| Total lines of Go | ~9,500 |
| Total lines of TypeScript | ~3,000 |
| Test files | **0** |
| TODO/FIXME comments | 1 (ingestor.go) |
| Critical bugs found | 3 |
| Medium bugs found | 10 |
| Architectural concerns | 8 |

---

## Architecture Overview

```
┌─────────────┐     ┌──────────────┐     ┌───────────────┐
│  React SPA  │────▶│  Go HTTP API │────▶│  SQLite (WAL) │
│  (Vite)     │◀─sse│  :8080       │     │  (embedded)   │
└─────────────┘     └──────┬───────┘     └───────────────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │ GoParser │ │ AI Layer │ │ GraphIdx │
        │ (AST)    │ │ Bedrock/ │ │ (in-mem) │
        │          │ │ Ollama   │ │          │
        └──────────┘ └──────────┘ └──────────┘
```

---

## Backend — Go

---

### cmd/server

#### `cmd/server/main.go` — 139 lines

**Purpose:** Application entry point. Parses flags, wires Storage → GraphIndex → Server → AI → QueryLayer, handles graceful shutdown.

**Exported symbols:** None (package main)

**Flags declared:**
- `--db-path` (string, default `vyuha.db`)
- `--port` (string, default `8080`)
- `--log-level` (string, default `info`)
- `--ai-provider` (string, `bedrock|ollama`)
- `--ai-region`, `--ai-model`, `--ai-embed-model`, `--ollama-url`

**TODO/FIXME:** None

**Bugs & Issues:**

| Severity | Line | Description |
|----------|------|-------------|
| **CRITICAL** | ~97 | `_ = jobQueue` — `JobQueue` is created but **never passed to Server**. The `Server.jobQueue` field is never assigned. This means `GET /api/ai/jobs/*` always returns 503 ("AI jobs not available"). |
| Medium | ~45 | `logLevel` flag is parsed but never wired to a logging framework. No structured logging anywhere in the codebase. |
| Low | — | No environment-variable fallback for flags (see Docker issue below). |

---

### internal/api

#### `internal/api/server.go` — ~230 lines

**Purpose:** HTTP server skeleton with middleware chain and route registration.

**Exported types & functions:**
- `Server` struct
- `NewServer(store, index, sse) *Server`
- `RegisterRoutes()`
- `Handler() http.Handler`
- `ListenAndServe(addr string) error`
- `Shutdown(ctx interface{ Deadline() (time.Time, bool) }) error`
- `SetQueryLayer(ql *query.QueryLayer)`

**Route map (17 endpoints):**
```
POST   /api/scan
GET    /api/scan/status
GET    /api/graph/services
GET    /api/graph/children/{id}
GET    /api/graph/subgraph/{id}
GET    /api/graph/node/{id}
GET    /api/graph/stats
GET    /api/graph/search
POST   /api/runtime/ingest
POST   /api/runtime/ingest/batch
GET    /api/runtime/failures
GET    /api/runtime/trace/{id}
GET    /api/runtime/node/{id}/events
POST   /api/ai/query
GET    /api/ai/jobs/{id}
GET    /events (SSE)
GET    /* (SPA fallback)
```

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `Shutdown()` parameter typed as `interface{ Deadline() (time.Time, bool) }` instead of `context.Context` — unconventional, reduces composability. |
| Low | CORS allows any `localhost:*` origin — fine for dev, not production. |
| Low | `serveFrontend()` uses `http.FS(os.DirFS("frontend/dist"))` — requires the binary to run from the repo root. Not embedded in binary. |

---

#### `internal/api/handlers_ai.go` — ~110 lines

**Exported types & functions:**
- `SSEBroadcasterAsAI(sse *SSEBroadcaster) ai.Broadcaster` (adapter)
- `sseBroadcasterAdapter` struct

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| **CRITICAL** | `handleAIJobStatus` checks `s.jobQueue` which is **always nil** (see main.go bug). Every request to `GET /api/ai/jobs/:id` returns 503. |

---

#### `internal/api/handlers_scan.go` — 336 lines

**Exported types & functions:** None exported (handler functions are lowercase)

**Internal types:**
- `scanJobStatus` (with `sync.Mutex`)
- `scanJobSnapshot`
- `scanJobs` (package-level `sync.Map`)

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `scanJobs` is a **package-level global** `sync.Map` — not on `Server`. This means old scan status entries are never pruned (memory leak over time). |
| Low | Scan always reads checksums from storage but there's no way to force a full re-scan without changing files. |

---

#### `internal/api/handlers_graph.go` — 319 lines

**Exported types & functions:** None exported

**Functions:** `handleGraphServices`, `handleGraphChildren`, `handleGraphSubgraph`, `handleGraphNode`, `handleGraphStats`, `handleGraphSearch`, `extractPathParam`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `handleGraphSubgraph` is a **partial placeholder** — it calls `index.GetDescendants(id, 3)` with a comment "*until SubgraphExtractor is implemented*", but `SubgraphExtractor` **is fully implemented** in `internal/query/subgraph.go`. The handler should use it. |

---

#### `internal/api/handlers_runtime.go` — 463 lines

**Exported types & functions:** None exported

**Internal types:** `logEventRequest`

**Functions:** `handleIngestLog`, `handleIngestLogs`, `processEvent`, `resolveNodeID`, `computeNodeStatus`, `propagateError`, `handleRuntimeFailures`, `handleRuntimeTrace`, `handleRuntimeNodeEvents`, `parseWindow`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| **Medium-High** | `resolveNodeID` iterates **ALL function nodes twice** (O(n)) per event ingestion call. At scale with hundreds of events/sec and thousands of nodes, this is a performance bottleneck. |
| Medium | `context.Background()` used inside HTTP handlers instead of `r.Context()` — request cancellation won't propagate to DB queries. |
| Medium | Ghost node creation (`resolveNodeID`) doesn't set `ParentID`, so ghost nodes are **orphaned** in the hierarchy. |
| Low | `propagateError` only propagates **one hop** (callers). Deep cascade failures won't be reflected. |

---

#### `internal/api/sse.go` — ~170 lines

**Exported types & functions:**
- `SSEEvent` struct
- `SSEBroadcaster` struct
- `NewSSEBroadcaster() *SSEBroadcaster`
- `Subscribe/Unsubscribe/Broadcast/BroadcastToClient/ClientCount`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | No maximum client limit — could exhaust memory with many SSE connections. |
| Low | Channel buffer of 64 per client; dropped events are logged but not tracked/counted. |

---

### internal/storage

#### `internal/storage/storage.go` — 1,078 lines

**Purpose:** SQLite persistence layer with full CRUD for nodes, edges, runtime events, data flow, and embeddings.

**Exported types:**
- `Storage` struct
- `RuntimeEvent`, `DataFlowRecord`, `Embedding`, `NodeFailureStat`, `TypeCount`, `GraphStats`

**Exported functions (selected):**
- `New(dbPath) (*Storage, error)` / `Close()`
- `SaveNode/SaveNodes/GetNode/GetChildren/GetDescendants/GetNodesByType/GetNodesByFile/UpdateNodeStatus/DeleteNodesByFile/SearchNodes/GetAllNodes`
- `SaveEdge/SaveEdges/GetEdgesBySource/Target/Type/GetEdgesBetweenNodes/DeleteEdgesBySource/GetAllEdges`
- `SaveRuntimeEvent/SaveRuntimeEvents/GetLatestNodeStatus/GetFailuresInWindow/GetFailuresByNode/GetTopFailingNodes/GetTraceEvents/GetRecentEvents`
- `SaveDataFlow/GetDataFlow`
- `SaveEmbedding/GetEmbedding/GetAllEmbeddings`
- `GetGraphStats`

**Helper functions:**
- `Float32ToBlob/BlobToFloat32` — raw float32↔BLOB encoding for embeddings

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `SetMaxOpenConns(1)` + `sync.RWMutex` — all reads are serialized even though WAL mode supports concurrent readers. The RWMutex is unnecessary overhead with single-connection SQLite; alternatively, allow multiple readers. |
| Medium | `SaveEdges` queries **ALL node IDs** into a Go map before filtering edges — won't scale for graphs with 100K+ nodes. |
| Low | Embedding storage as raw `BLOB` is functional but not searchable. No vector index (acceptable for MVP). |
| Low | `GetDescendants` uses recursive CTE which is efficient, but the depth parameter isn't validated (no upper bound). |

---

#### `internal/storage/schema.go` — ~100 lines

**Exported:**
- `SchemaVersion` = 2
- `GetSchema() string`
- `Migration` type, `Migrations` slice (v1: initial schema, v2: add CASCADE)

No issues found.

---

### internal/graph

#### `internal/graph/node.go` — ~220 lines

**Exported types:**
- `NodeType` (10 values: `repository`, `service`, `package`, `file`, `function`, `struct`, `interface`, `cloud_service`, `runtime_instance`, `data_flow`)
- `Parameter`, `StructField`, `InterfaceMethod`, `ImportSpec`, `FieldMap`
- `NodeMetadata` (large flat struct covering all node type metadata)
- `Node` struct

**Exported functions:**
- `NewNode(id, name, nodeType) *Node`
- `Node.IsHealthy()`, `Node.IsFailing()`, `Node.IsCloud()`, `Node.FullyQualifiedName()`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | `NodeMetadata` is a flat "god struct" covering all node types. All fields are always serialized/present even when irrelevant (e.g. every function node carries empty cloud_provider, operations fields). Consider an interface or union-type pattern. |

---

#### `internal/graph/edge.go` — ~100 lines

**Exported types:**
- `EdgeType` (12 values: `contains`, `imports`, `calls`, `implements`, `depends_on`, `connects_to`, `runtime_calls`, `failed_at`, `produces_to`, `consumed_by`, `transforms`, `field_map`)
- `EdgeMetadata` (flat struct)
- `Edge` struct

**Exported functions:**
- `NewEdge(id, source, target, edgeType) *Edge`

No bugs found.

---

#### `internal/graph/index.go` — 923 lines

**Purpose:** In-memory graph index with 7 maps (nodes, children, outEdges, inEdges, byType, byFile, byStatus), thread-safe via `sync.RWMutex`.

**Exported types:**
- `IndexStats` struct
- `GraphIndex` struct

**Exported functions:**
- `NewGraphIndex() *GraphIndex`
- `LoadFromStorage(loader storageLoader) error`
- **Mutations:** `AddNode`, `AddEdge`, `UpdateNodeStatus`, `RemoveNodesByFile`
- **Traversal:** `GetNode`, `GetChildren`, `GetDescendants`, `GetOutEdges`, `GetInEdges`, `GetEdgesBetween`, `GetCallers`, `GetCallees`, `GetByType`, `GetByFile`, `GetFailingNodes`, `GetDegradedNodes`
- **Algorithms:** `BFS`, `GetAncestors`, `FindPath` (shortest via BFS), `GetConnectedComponent`, `GetExternalDependencies`, `ComputeFanIn`, `ComputeFanOut`, `FindEntryPoints`, `ExtractSubgraph`
- **Stats:** `NodeCount`, `EdgeCount`, `Stats()`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | All traversal methods (BFS, GetDescendants, etc.) allocate fresh slices per call. For very large graphs, pre-allocated pools would help. |
| Low | `FindPath` is BFS-only (unweighted shortest path). No option for weighted or longest path. |

---

### internal/parser/golang

#### `internal/parser/golang/parser.go` — 1,238 lines

**Purpose:** Full Go AST parser. Detects services, parses packages/files/functions/structs/interfaces, extracts call edges, computes checksums for incremental parsing.

**Exported types:**
- `GoParser` struct
- `ProgressFn` callback type
- `ParseResult` struct (Nodes, Edges, FileCount, ParseErrors, ASTFiles, Checksums)
- `ParseError` struct

**Exported functions:**
- `New(rootPath) (*GoParser, error)` — reads go.mod for module path
- `Parse(ctx, checksums, progress) (*ParseResult, error)` — full pipeline
- `ModulePath() string`
- `Fset() *token.FileSet`

**Internal highlights:**
- Parallel file parsing via worker pool (`runtime.NumCPU()` workers)
- Incremental parsing via file checksum comparison
- Cyclomatic complexity calculation
- Call extraction from AST (go/defer/regular calls)
- Source snippet extraction (first 50 lines of function body)
- Generated file detection (`// Code generated` header)

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | Call resolution is **AST-only** (unresolved) — cross-package call targets use `modulePath + packagePath + funcName` heuristic that may not match actual node IDs for methods on unexported types or renamed imports. |
| Medium | `go/types` is imported but only `types.ExprString` is used — no full type checking. Method set resolution is approximate. |
| Low | `isBuiltinCall` hardcodes a `builtinFuncs` map but doesn't include all Go builtins (e.g., `clear`, `min`, `max` added in Go 1.21). |
| Info | Only Go parser exists. The type system suggests multi-language support but no other parsers are implemented. |

---

#### `internal/parser/golang/cloud.go` — 880 lines

**Purpose:** Cloud/queue/DB/HTTP import and SDK-call detection. Enriches ParseResult with cloud_service nodes and DEPENDS_ON/CONNECTS_TO edges.

**Exported types:**
- `CloudDetector` struct
- `NewCloudDetector(modulePath, fset) *CloudDetector`

**Exported functions:**
- `Detect(ctx, result, astFiles) error`

**Import mapping tables:**
- `awsImportMap` — AWS SDK v1/v2 (s3, dynamodb, sqs, sns, lambda, secretsmanager, ses, kinesis, bedrock)
- `queueImportMap` — Kafka (multiple client libs), RabbitMQ, NATS, GCP PubSub
- `dbImportMap` — database/sql, sqlx, GORM, MongoDB, Redis, etcd
- `httpImportMap` — net/http, resty, gRPC

**SDK operation patterns:** `sdkOps` (per-service method name sets), `kafkaProducerOps`, `kafkaConsumerOps`, `httpCallOps`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | No GCP or Azure **service-specific** import mappings (only GCP PubSub). AWS has comprehensive coverage. |
| Low | SDK operation detection is by method name only — could false-positive on user methods named `GetObject`, `PutItem`, etc. |

---

#### `internal/parser/golang/dataflow.go` — 910 lines

**Purpose:** Intra-function data-flow extraction. Classifies parameters, assignments, return statements, bare calls, defer/go statements into data-flow nodes (input, fetched, computed, constructed, output, published, side_effect).

**Exported types:**
- `FunctionDataFlow` struct
- `DataFlowExtractor` struct
- `NewDataFlowExtractor(modulePath, fset) *DataFlowExtractor`

**Exported functions:**
- `ExtractAll(ctx, result, astFiles) (map[string]*FunctionDataFlow, error)`
- `ExtractFunction(funcNode, funcDecl, fileImports) (*FunctionDataFlow, error)`

**Heuristic tables:**
- `fetchPrefixes` — Get, Fetch, Find, Load, Read, Query, etc.
- `repoSuffixes` — repo, store, client, service, gateway, db, cache, etc.
- `publishNames` — Publish, SendMessage, Produce, etc.
- `sideEffectPrefixes` — Save, Insert, Update, Delete, Log, Close, etc.

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | Classification is purely heuristic-based (name prefixes/suffixes). No type-based reasoning. |
| Low | Risk score computation not validated against real-world data. |

---

### internal/ai

#### `internal/ai/provider.go` — ~230 lines

**Exported types:**
- `ProviderKind` (`bedrock`, `ollama`)
- `Role` (`system`, `user`, `assistant`, `tool`)
- `Message`, `Tool`, `ToolCall`, `ToolResponse`
- `GenerateOptions`, `StreamDelta`
- `Provider` interface (`Generate`, `StreamGenerate`, `GenerateWithTools`, `Embed`, `Name`, `Close`)
- `ProviderConfig` struct

**Exported functions:**
- `DefaultGenerateOptions() GenerateOptions`
- `NewProvider(ctx, config) (Provider, error)` — factory dispatching to bedrock or ollama
- `Validate() error` (on ProviderConfig)
- `BuildConversation(systemPrompt string, msgs ...Message) []Message`

No bugs found. Clean interface design.

---

#### `internal/ai/bedrock.go` — 424 lines

**Purpose:** AWS Bedrock provider implementing the `Provider` interface. Uses Anthropic Messages API via `InvokeModel` / `InvokeModelWithResponseStream`.

**Constants:** `defaultBedrockModel` = `anthropic.claude-3-haiku-20240307-v1:0`, `defaultBedrockEmbedding` = `amazon.titan-embed-text-v2:0`, `titanEmbedDimensions` = 1024

**Implements:** `Generate`, `StreamGenerate`, `GenerateWithTools`, `Embed`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | `handleStreamChunk` fallback to parsing the entire data chunk as a single JSON object may silently swallow errors for partially valid chunks. |
| Low | No retry logic on transient AWS errors (throttling, network timeouts). |

---

#### `internal/ai/ollama.go` — 393 lines

**Purpose:** Ollama provider implementing the `Provider` interface via local HTTP API (`/api/chat`, `/api/embeddings`).

**Constants:** `defaultOllamaModel` = `llama3`, `defaultOllamaEmbedding` = `nomic-embed-text`, `ollamaTimeout` = 120s

**Implements:** `Generate`, `StreamGenerate`, `GenerateWithTools` (simulated via text prompt), `Embed`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `GenerateWithTools` is a **text-based simulation** — it injects tool schemas into the prompt and parses JSON blocks from output. This is brittle and may fail with models that don't follow the format. |
| Low | Ollama returns `float64` embeddings, converted to `float32` — potential precision loss (acceptable for similarity search). |
| Low | `parseToolCalls` regex approach may miss tool calls that don't follow the exact ```json format, or parse non-tool-call JSON as tool calls. |

---

#### `internal/ai/embeddings.go` — 377 lines

**Exported types:**
- `EmbeddingService` struct

**Exported functions:**
- `NewEmbeddingService(provider, store, index) *EmbeddingService`
- `EmbedNode(ctx, nodeID) error`
- `EmbedAll(ctx) error` (5-worker pool)
- `SimilaritySearch(ctx, query, topK) ([]*graph.Node, error)`
- `ReloadCache()`, `CacheSize() int`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `SimilaritySearch` uses **O(n) brute-force** cosine similarity over in-memory cache. No ANN index. Acceptable for <10K nodes but problematic beyond that. |
| Low | No dimension validation — if query vector and stored vectors have different dimensions, cosine similarity silently returns incorrect results. |

---

#### `internal/ai/jobs.go` — 439 lines

**Exported types:**
- `Broadcaster` interface
- `BroadcastEvent` struct
- `JobKind` (8 values), `JobStatus` (4 values)
- `AIJob` struct
- `JobQueue` struct

**Exported functions:**
- `NewJobQueue(provider, embeddings, index, store, broadcaster) *JobQueue`
- `Enqueue(kind, params) string`
- `GetJob(id) *AIJob`
- `ListJobs() []*AIJob`
- `Close()`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `ListJobs()` uses **O(n²) insertion sort** instead of `sort.Slice`. |
| Medium | Jobs map grows **unbounded** — no eviction/TTL. Long-running servers will accumulate stale jobs. |
| Low | Queue buffer is 256 — if AI jobs back up, `Enqueue` blocks. No backpressure feedback to callers. |

---

#### `internal/ai/prompts.go` — ~250 lines

**Exported functions:**
- `ExplainFunctionPrompt(node, callers, callees) []Message`
- `WhyFailingPrompt(node, events) []Message`
- `ServiceOverviewPrompt(service, children, externalDeps, stats) []Message`
- `DataLineagePrompt(node, flows, downstream) []Message`
- `AgentSystemPrompt() string`

No bugs found. Well-structured prompt-building functions.

---

### internal/query

#### `internal/query/query.go` — 738 lines

**Purpose:** Top-level query layer that classifies natural-language questions, resolves target nodes, and dispatches to execution strategies.

**Exported types:**
- `QueryLayer` struct
- `QueryMode` (4 values: `direct_graph`, `subgraph`, `agent`, `sql`)
- `QueryDecision`, `QueryResult`, `DirectGraphData`

**Exported functions:**
- `NewQueryLayer(index, store, provider, embeddings, broadcaster) *QueryLayer`
- `HandleQuestion(ctx, question) (*QueryDecision, error)`
- `ExplainFunction(ctx, nodeID) (string, error)`
- `WhyFailing(ctx, nodeID) (string, error)`
- `GetSubgraph(ctx, query) (*SubgraphResult, error)`
- `SimilaritySearch(ctx, query, topK) ([]*graph.Node, error)`

**Question classification patterns:**
- service overview → `subgraph` (service_overview)
- call chain → `subgraph` (call_chain)
- failure → `subgraph` (failure_path)
- data flow → `subgraph` (data_lineage)
- dependency impact → `subgraph` (dependency_impact)
- direct graph queries → `direct_graph`
- SQL queries → `sql`
- fallback → `agent`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `classifyQuestion` uses **simple substring matching** with patterns like `"what.*calls"`. Could misclassify questions (e.g., "why is the callback failing?" matches "call" and could be routed incorrectly). |
| Medium | `fuzzyFindNode` does **O(n) scans** of function, service, struct, and interface node lists for each candidate name — iterates entire graph multiple times per query. |
| Low | `resolveTarget` falls back to embedding similarity search, which is good, but there's no confidence threshold — even very poor matches are used. |

---

#### `internal/query/agent.go` — 631 lines

**Purpose:** AI agent with tool-use loop (max 6 steps). Sends SSE progress events and returns a full `AgentRun`.

**Exported types:**
- `AgentTool`, `AgentStep`, `AgentRun`
- `VyuhaAgent` struct

**Exported functions:**
- `NewVyuhaAgent(provider, index, store, extractor, broadcaster) *VyuhaAgent`
- `Run(ctx, question) (*AgentRun, error)`

**Agent tools (7):**
1. `find_service` — fuzzy search services
2. `get_service_overview` — calls SubgraphExtractor
3. `get_call_chain` — calls SubgraphExtractor
4. `find_functions` — fuzzy search functions (max 20 results)
5. `get_failures` — node failures or system-wide top failures
6. `get_dependencies` — external dependencies for a node
7. `get_dependency_impact` — blast radius analysis

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | `maxAgentSteps = 6` is hardcoded with no configuration option. |
| Low | Tool execution is linear search (`for range tools`) — O(n) per tool call. Negligible with 7 tools but should be a map for extensibility. |

---

#### `internal/query/subgraph.go` — 962 lines

**Purpose:** Focused subgraph extraction for 5 query types: service_overview, call_chain, failure_path, data_lineage, dependency_impact. Includes 2D layout computation for frontend rendering.

**Exported types:**
- `SubgraphQueryType` (5 values)
- `SubgraphQuery`, `SubgraphResult`
- `SubgraphLayout`, `Position`
- `SubgraphExtractor` struct

**Exported functions:**
- `NewSubgraphExtractor(index, store) *SubgraphExtractor`
- `Extract(ctx, query) (*SubgraphResult, error)`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `computeCriticalPath` uses **DFS for longest simple path** — this is NP-hard in general, though constrained to selected nodes (typically ≤50). Potential timeout on dense subgraphs. |
| Low | `failurePath` fetches recent events but assigns to `_ = events` — intended for "future AI prompt enrichment" but currently wasted work. |
| Low | Layout positions use simple row/ring placement. Overlapping can occur when many nodes share the same role. |

---

#### `internal/query/scorer.go` — ~250 lines

**Purpose:** Relevance scoring for graph nodes per query type. Combines fan-in/out, error counts, cloud status, entry-point detection.

**Exported types:**
- `ScoredNode` struct
- `NodeScorer` struct

**Exported functions:**
- `NewNodeScorer(index) *NodeScorer`
- `Score(node, queryType) ScoredNode`
- `ScoreAll(nodes, queryType) []ScoredNode`

No bugs found. Clean scoring implementation with per-query-type strategies.

---

### internal/runtime

#### `internal/runtime/ingestor.go` — 3 lines

**Content:**
```go
package runtime
// TODO: Implement code ingestion pipeline orchestrator.
```

**This is a STUB.** The runtime event pipeline orchestrator is not implemented. Log ingestion is handled directly by `handlers_runtime.go` instead.

---

### db

#### `db/schema.sql` — ~160 lines

**Tables:** `nodes`, `edges`, `runtime_events`, `data_flow`, `type_definitions`, `embeddings`, `schema_migrations`

**Indexes:** Comprehensive — includes indexes on all foreign keys, `(node_id, timestamp)` for runtime queries, `(type)` for node filtering.

No issues found.

---

#### `db/embed.go` — 8 lines

Embeds `schema.sql` via `//go:embed`. Exports `SchemaSQL string`.

No issues found.

---

### scripts

#### `scripts/generate_demo_data/main.go` — 574 lines

**Purpose:** Complete demo data generator. Parses a real Go repo, runs cloud detection + data flow extraction, saves to SQLite, injects synthetic runtime events with realistic error scenarios.

**6-phase pipeline:**
1. Parse repository via GoParser
2. Run CloudDetector
3. Run DataFlowExtractor
4. Save to database
5. Inject synthetic runtime events (healthy/failing distributions)
6. Print summary stats

No significant issues. Well-documented usage instructions.

---

#### `scripts/simulate_failures/main.go` — 293 lines

**Purpose:** Live incident simulator that sends runtime events to a running VYUHA server in three phases: normal → incident → recovery.

**Features:** Configurable rate, phase durations, automatic caller discovery via API, gradually improving recovery phase.

No significant issues. Good demo tooling.

---

## Frontend — TypeScript/React

---

### Entry & Config

#### `frontend/src/main.tsx` — 19 lines
React 18 entry point with `ReactFlowProvider` wrapper. No issues.

#### `frontend/src/index.css` — 11 lines
Tailwind base/components/utilities plus a custom `.input` component class. No issues.

#### `frontend/vite.config.ts` — ~25 lines
Dev proxy for `/api` and `/events` to `localhost:8080`. Build output to `dist/` with sourcemaps. No issues.

#### `frontend/package.json` — 32 lines
**Dependencies:** react 18.2, reactflow 11.11.3, dagre 0.8.5, tailwindcss 3.4, vite 5.2, typescript 5.4. No issues.

---

### api/client.ts

#### `frontend/src/api/client.ts` — ~150 lines

**Exported:** `api` object with typed fetch wrappers.

**Functions:**
- `scan`, `getScanStatus`, `getServices`, `getChildren`, `getSubgraph`, `getNode`, `getStats`, `searchNodes`, `ingestLog`, `ingestLogs`, `getFailures`, `getTrace`, `askQuestion`, `getJob`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Low | No centralized error handling — each caller must `.catch()` individually. |
| Low | No request cancellation support (AbortController). |

---

### types/graph.ts

#### `frontend/src/types/graph.ts` — 326 lines

**Exported types:**
- `NodeType`, `EdgeType` (enum-like string unions mirroring Go)
- `Parameter`, `StructField`, `InterfaceMethod`, `ImportSpec`, `FieldMap`
- `NodeMetadata`, `GraphNode`, `GraphEdge`, `EdgeMetadata`
- `RuntimeEvent`, `SubgraphResult`, `AgentStep`, `AgentRun`, `GraphStats`
- `NodeFailureStat`, `DataFlowRecord`, `FunctionDataFlow`
- `ScanResponse`, `ScanStatus`, `NodeDetail`, `LogEvent`, `IngestResponse`, `BatchIngestResponse`, `TraceResult`, `QueryDecision`, `AIJob`, `SSEEvent`, `ScanProgress`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| **Medium** | `EdgeMetadata` in TypeScript has **different fields** than Go's `EdgeMetadata`. TS has `weight`, `label`, `is_external`, `call_count`, `error_rate`, `avg_latency_ms`, `field_mappings`. Go has a different set. This mismatch means the frontend may access undefined fields or miss data. |

---

### hooks

#### `frontend/src/hooks/useGraph.ts` — ~230 lines

**Exported:**
- `applyDagreLayout(nodes, edges, direction?) → { nodes, edges }`
- `UseGraphReturn` type
- `useGraph()` hook — manages React Flow nodes/edges state, layout, expansion

**Functions:** `nodeTypeToRF`, `toReactFlowNode`, `toReactFlowEdge`, `edgeStyle`

**Bugs & Issues:**

| Severity | Description |
|----------|-------------|
| Medium | `expandNode` uses nested `setState` + `setTimeout(50ms)` for layout — **fragile timing** that can cause layout glitches on slow machines. |
| Low | `nodeTypeToRF` mapping function duplicated between `useGraph.ts` and `App.tsx`. |

---

#### `frontend/src/hooks/useSSE.ts` — ~165 lines

**Exported:** `UseSSEReturn` type, `useSSE()` hook

**Features:** EventSource with auto-reconnect (exponential backoff, max 30s), tracks connection status, node status updates, agent steps, scan progress.

No significant issues. Well-implemented reconnection logic.

---

#### `frontend/src/hooks/useAgent.ts` — ~65 lines

**Exported:** `useAgent()` hook

**State:** `isRunning`, `decision`, `agentRun`, `error`

No issues. Clean simple state management.

---

### components

#### `frontend/src/components/GraphCanvas.tsx` — ~135 lines

**Exported:** `GraphCanvas` component (default export)

**Features:** React Flow canvas with custom node types, MiniMap with status-based coloring, detail panel on click, expand on double-click, SSE status propagation.

No significant issues.

---

#### `frontend/src/components/StatusBar.tsx` — ~95 lines

**Exported:** `StatusBar` component (default export)

**Features:** Bottom bar showing node/edge counts, service count, error count, SSE connection indicator, rescan button, ingest-logs button.

No issues.

---

#### `frontend/src/components/nodes/ServiceNode.tsx` — ~75 lines

Custom React Flow node for services. Shows name, package/function counts, status dot, error count badge, status-change pulse animation.

No issues.

---

#### `frontend/src/components/nodes/FunctionNode.tsx` — ~70 lines

Custom React Flow node for functions. Shows receiver.name, complexity dot (green/yellow/red), return types, cloud icon.

No issues.

---

#### `frontend/src/components/nodes/CloudNode.tsx` — ~50 lines

Custom React Flow node for cloud services. Shows provider badge (AWS/GCP/Azure), operation count.

No issues.

---

#### `frontend/src/components/nodes/DataFlowNode.tsx` — ~60 lines

Custom React Flow node for data flow records. Color-coded by kind (input/fetched/computed/constructed/output/published), fan_in indicator for aggregates.

No issues.

---

#### `frontend/src/components/panels/DetailPanel.tsx` — 394 lines

**Purpose:** Slide-in detail panel with collapsible sections: Metadata, Connections, Data Flow, Failure History, AI (Explain/WhyFailing).

**Sub-components:** `TypeBadge`, `StatusBadge`, `Section`, `KV`, `ConnList`, `KindDot`, `EventRow`, `Spinner`

No significant issues. Well-structured component.

---

#### `frontend/src/components/panels/AgentPanel.tsx` — ~200 lines

**Purpose:** Real-time agent reasoning panel. Shows steps with tool calls/results, auto-scrolls, renders final answer with clickable node references.

**Sub-components:** `StepRow`, `ClickableNodeText`

No issues.

---

#### `frontend/src/components/panels/QueryBar.tsx` — ~120 lines

**Purpose:** Top query bar with dynamic suggestion chips loaded from graph data.

No issues.

---

#### `frontend/src/components/panels/LogIngestion.tsx` — 327 lines

**Purpose:** Modal dialog for testing log ingestion. Supports single-event form and batch JSON input.

**Sub-components:** `SingleForm`, `BatchForm`, `Tab`, `Field`

No issues.

---

## Infrastructure

### Dockerfile — 28 lines

Multi-stage build: `golang:1.22-alpine` → `alpine:3.19`. Uses `CGO_ENABLED=1` for SQLite.

| Severity | Description |
|----------|-------------|
| **CRITICAL** | `frontend/dist/` is **NOT copied** into the Docker image. The SPA will not be served. The build needs a Node.js stage or pre-built frontend assets. |
| Low | `COPY db/schema.sql /app/db/schema.sql` is unnecessary — it's already embedded via `//go:embed` in the Go binary. |

### docker-compose.yml — 18 lines

Single service, port 8080, named volume `vyuha-data`.

| Severity | Description |
|----------|-------------|
| **Medium** | `VYUHA_DB_PATH=/data/vyuha.db` environment variable is set, but the Go binary uses `--db-path` **flag**, not env vars. The env var is **silently ignored**. The DB will default to `vyuha.db` in the container's CWD. |

---

## Cross-Cutting Issues

### 1. Zero Test Coverage
**No `_test.go` files exist anywhere in the codebase.** This is the single largest risk factor. Critical paths like graph indexing, SQL queries, parser output, and query classification are all untested.

### 2. No Configuration File Support
All configuration is via CLI flags. No `.env`, YAML, or TOML config files. No environment variable fallbacks (breaks standard Docker/K8s patterns).

### 3. No Structured Logging
Uses `log.Printf` throughout. The `--log-level` flag is parsed but never applied. No JSON/structured logging for production observability.

### 4. Only Go Parser
Despite the architecture supporting multi-language codebases (the node type system, import specs, etc.), only a Go parser is implemented. This limits the product to Go-only repositories.

### 5. Frontend Type Drift
The TypeScript `EdgeMetadata` type has different fields than the Go `EdgeMetadata` struct. This creates a silent data contract mismatch between frontend and backend.

### 6. No Authentication/Authorization
All API endpoints are open. No auth middleware, API keys, or RBAC.

### 7. No Rate Limiting
`/api/runtime/ingest` and `/api/runtime/ingest/batch` accept unlimited requests. Combined with the O(n) `resolveNodeID`, this is a DoS vector.

### 8. In-Memory Graph State
The `GraphIndex` mirrors all data in-memory. This works for moderate codebases but won't scale to very large monorepos (100K+ nodes). There's no pagination in graph queries.

---

## Bug Summary

### Critical (3)

| # | Location | Description |
|---|----------|-------------|
| 1 | `cmd/server/main.go:~97` | `jobQueue` never assigned to Server — `GET /api/ai/jobs/*` always returns 503 |
| 2 | `Dockerfile` | Frontend `dist/` not copied — SPA won't serve in Docker |
| 3 | `docker-compose.yml` | `VYUHA_DB_PATH` env var is ignored; Go uses `--db-path` flag |

### Medium (10)

| # | Location | Description |
|---|----------|-------------|
| 4 | `handlers_runtime.go` | `resolveNodeID` is O(n) per event — performance bottleneck |
| 5 | `handlers_runtime.go` | `context.Background()` used instead of `r.Context()` |
| 6 | `handlers_runtime.go` | Ghost nodes created without `ParentID` — orphaned |
| 7 | `handlers_graph.go` | `handleGraphSubgraph` doesn't use existing SubgraphExtractor |
| 8 | `handlers_scan.go` | `scanJobs` map never pruned — memory leak |
| 9 | `storage.go` | `SaveEdges` loads all node IDs into memory |
| 10 | `embeddings.go` | O(n) brute-force similarity search |
| 11 | `jobs.go` | O(n²) insertion sort in ListJobs; unbounded jobs map |
| 12 | `types/graph.ts` | `EdgeMetadata` type mismatch with Go backend |
| 13 | `useGraph.ts` | `setTimeout(50ms)` layout hack — fragile timing |

### Low (15+)

Including: missing structured logging, no auth, no rate limiting, CORS permissive, flat NodeMetadata struct, no dimension validation on embeddings, Ollama tool-use simulation brittleness, duplicated `nodeTypeToRF`, incomplete Go builtins list, no other language parsers, no retry logic on AWS calls, etc.

---

## Recommendations

### Immediate (P0)

1. **Fix jobQueue wiring** — pass `jobQueue` to `Server` in `main.go`
2. **Fix Dockerfile** — add Node.js build stage or copy pre-built frontend assets
3. **Fix docker-compose** — use `command: ["/app/vyuha-ai", "--db-path", "/data/vyuha.db"]` instead of env var
4. **Add tests** — start with `storage_test.go`, `index_test.go`, `parser_test.go` covering critical paths

### Short-term (P1)

5. Wire `r.Context()` into all handlers instead of `context.Background()`
6. Build a node lookup index in `resolveNodeID` (map by name) instead of O(n) scan
7. Wire `handleGraphSubgraph` to use `SubgraphExtractor`
8. Add job eviction/TTL to `JobQueue`
9. Add scan job pruning
10. Align TypeScript `EdgeMetadata` with Go `EdgeMetadata`

### Medium-term (P2)

11. Add structured logging (slog or zerolog)
12. Add authentication middleware
13. Add rate limiting on ingest endpoints
14. Replace O(n²) ListJobs sort with `sort.Slice`
15. Add env-var fallbacks for all CLI flags
16. Consider ANN index for embeddings (e.g., hnswlib) if graph exceeds 10K nodes

### Long-term (P3)

17. Additional language parsers (TypeScript, Python, Java)
18. Graph pagination for large codebases
19. WebSocket upgrade option alongside SSE
20. OpenTelemetry integration for self-observability
