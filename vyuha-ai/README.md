# VYUHA AI — Code Intelligence Graph

**AI-powered code intelligence that maps, monitors and explains your entire codebase as a living graph.**

VYUHA parses Go repositories into a rich dependency graph, overlays live runtime telemetry, and uses an AI agent to answer questions like *"Why is the payment service failing?"* or *"What breaks if Redis goes down?"*

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Demo Script (3 minutes)](#demo-script-3-minutes)
- [Architecture Overview](#architecture-overview)
- [API Reference](#api-reference)
- [Project Structure](#project-structure)

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.22+ | Backend, parser, scripts |
| **Node.js** | 18+ | Frontend build |
| **Git** | any | Clone demo repo |
| **AWS credentials** | Bedrock access | AI features (or use Ollama locally) |

---

## Quick Start

### 1. Clone and install

```bash
git clone https://github.com/vyuha/vyuha-ai.git
cd vyuha-ai
go mod download
```

### 2. Build the frontend

```bash
cd frontend
npm install
npm run build
cd ..
```

### 3. Configure AI provider

**Option A — AWS Bedrock (recommended for demo):**

```bash
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=<your-key>
export AWS_SECRET_ACCESS_KEY=<your-secret>
```

**Option B — Ollama (local, no cloud):**

```bash
# Install Ollama: https://ollama.ai
ollama pull llama3
export VYUHA_AI_PROVIDER=ollama
```

### 4. Generate demo data

```bash
# Clone a well-known Go repo as the demo target
git clone https://github.com/gin-gonic/gin /tmp/gin

# Run the demo data generator
go run scripts/generate_demo_data.go \
  --repo-path /tmp/gin \
  --db-path ./vyuha-demo.db
```

This parses the entire Gin framework, runs cloud detection and data-flow
extraction, then injects realistic runtime events including failures and
distributed traces.

### 5. Start the server

```bash
go run cmd/server/main.go \
  --db-path ./vyuha-demo.db \
  --port 8080
```

### 6. Open the UI

```
http://localhost:8080
```

The frontend is served from `frontend/dist/` and the Vite dev proxy
forwards API calls to the Go backend.

---

## Demo Script (3 minutes)

### Step 1 — Show the Graph (30s)

1. Open `http://localhost:8080` — the graph loads automatically.
2. **Zoom out** to show the full service topology.
3. Point out:
   - Green nodes = healthy functions
   - Yellow nodes = degraded (elevated errors)
   - Red nodes = failing
4. **Double-click** a service node to expand its internal functions.
5. **Click** any function to open the Detail Panel showing metadata,
   callers/callees, data flow and failure history.

> **Talk track:** *"VYUHA automatically parsed the Gin framework — every function,
> struct, interface and their relationships. The graph shows 500+ nodes with
> live runtime status. Let me ask it a question."*

### Step 2 — Ask a Question (45s)

1. In the **Query Bar** at the top, type:
   ```
   How does the Context.JSON function work?
   ```
2. Watch the **Agent Panel** slide in from the left showing the AI reasoning
   in real time — tool calls to the graph API, subgraph extraction, analysis.
3. The agent returns a structured answer with clickable node references.
4. Click a node reference to navigate the graph.

> **Talk track:** *"The AI agent doesn't just read code — it queries our graph
> database, traces call chains, and synthesises an answer with full context.
> Let me show what happens during a live incident."*

### Step 3 — Simulate a Failure (60s)

1. Open a **new terminal** and run:
   ```bash
   go run scripts/simulate_failures.go \
     --server http://localhost:8080 \
     --target "function:github.com/gin-gonic/gin/context.go::(*Context).JSON"
   ```
2. Watch the terminal: Phase 1 (normal), Phase 2 (incident), Phase 3 (recovery).
3. **In the UI**, watch nodes turn yellow → red in real time via SSE.
4. The Status Bar shows the SSE connection (green dot) and live node counts.

> **Talk track:** *"We're simulating a production incident. The Context.JSON
> function is timing out, and you can see the error propagating upstream
> through its callers — just like in real production."*

### Step 4 — Root Cause Analysis (45s)

1. While nodes are still red, type in the Query Bar:
   ```
   Why is Context.JSON failing and what's the blast radius?
   ```
2. The agent:
   - Queries the failure timeline
   - Traces through callers (blast radius)
   - Identifies the root cause (the timeout pattern)
   - Returns an answer with the full impact chain

3. Click the suggested failing node — the Detail Panel shows the actual error
   messages and latency spikes.

> **Talk track:** *"In 10 seconds, the AI traced the failure through the entire
> call graph, identified the blast radius, and explained the root cause. This
> is what takes SRE teams 30 minutes in a real incident."*

---

## Architecture Overview

```
┌──────────────────────────────────────────────────┐
│                  React Frontend                   │
│  React Flow graph · Detail panel · Agent panel    │
│  SSE live updates · Query bar · Log ingestion     │
└──────────────┬───────────────────┬───────────────┘
               │ HTTP/JSON         │ SSE
┌──────────────▼───────────────────▼───────────────┐
│               Go HTTP Server (net/http)           │
│  /api/scan   /api/graph/*   /api/ingest/*         │
│  /api/runtime/*   /api/events (SSE)               │
├───────────────────────────────────────────────────┤
│              Query & AI Layer                      │
│  Natural language → mode router → subgraph/agent  │
│  Scorer · Subgraph extractor · Agent with tools   │
├───────────────────────────────────────────────────┤
│            In-memory Graph Index                   │
│  Adjacency lists · Type/file/status indexes       │
│  Loaded from SQLite on startup                    │
├───────────────────────────────────────────────────┤
│               Go Parser Pipeline                   │
│  AST parser → Cloud detector → Data flow extractor│
│  Concurrent file processing · Incremental support │
├───────────────────────────────────────────────────┤
│             SQLite (WAL mode)                      │
│  nodes · edges · runtime_events · data_flow       │
│  embeddings · type_definitions                    │
└───────────────────────────────────────────────────┘
```

### Layer Details

**Parser Pipeline** — Walks Go source files using `go/ast`, extracts functions,
structs, interfaces, call edges, and computes cyclomatic complexity. Cloud
detection identifies AWS SDK, database drivers, queue clients and HTTP clients.
Data-flow extraction traces how values flow through function bodies.

**Storage** — SQLite in WAL mode with migration support. Stores the full graph
(nodes + edges), runtime telemetry events, data-flow records, type definitions,
and AI-generated vector embeddings. All operations are transactional.

**Graph Index** — In-memory mirror of the database loaded on startup. Provides
O(1) lookups by ID, parent, type, file path, and runtime status. All public
methods are goroutine-safe.

**Query & AI Layer** — Routes natural-language questions through a scorer that
picks the best strategy: direct graph lookup, subgraph extraction, or full
AI agent reasoning. The agent has tools to query the graph, search nodes,
read source code, and trace failures.

**HTTP API** — Standard `net/http` with CORS, logging, and panic recovery
middleware. Supports REST endpoints for graph queries, runtime ingestion,
and an SSE stream for real-time UI updates.

**React Frontend** — React 18 + TypeScript + React Flow for graph
visualisation. Custom node types (service, function, cloud, data-flow) with
status-aware coloring. Dagre auto-layout. Query bar with AI agent reasoning
panel. SSE-driven live status updates.

---

## API Reference

### Health

```bash
curl http://localhost:8080/health
# {"status":"ok","service":"vyuha-ai"}
```

### Scan

```bash
# Trigger a scan
curl -X POST http://localhost:8080/api/scan \
  -H "Content-Type: application/json" \
  -d '{"root_path":"/tmp/gin"}'
# {"job_id":"...","status":"scanning"}

# Check scan status
curl http://localhost:8080/api/scan/status?job_id=<id>
```

### Graph

```bash
# List all services (top-level nodes)
curl http://localhost:8080/api/graph/services
# [{"id":"service:...","type":"service","name":"gin",...}]

# Get children of a node
curl "http://localhost:8080/api/graph/children?parent_id=service:gin&depth=1"

# Get a focused subgraph
curl "http://localhost:8080/api/graph/subgraph?target_id=function:...&type=blast_radius"

# Get a single node with callers/callees
curl http://localhost:8080/api/graph/node/<node-id>
# {"node":{...},"callers":[...],"callees":[...],"data_flow":[...],"recent_events":[...]}

# Graph statistics
curl http://localhost:8080/api/graph/stats
# {"nodes_by_type":[...],"edges_by_type":[...],"total_nodes":500,...}

# Search nodes
curl "http://localhost:8080/api/graph/search?q=JSON&type=function"
```

### Runtime / Ingestion

```bash
# Ingest a single event
curl -X POST http://localhost:8080/api/ingest/log \
  -H "Content-Type: application/json" \
  -d '{
    "node_id": "function:github.com/gin-gonic/gin/context.go::(*Context).JSON",
    "event_type": "http_request",
    "status": "error",
    "error_message": "context deadline exceeded",
    "latency_ms": 5000
  }'
# {"status":"ok","node_id":"function:...","runtime_status":"error"}

# Ingest a batch
curl -X POST http://localhost:8080/api/ingest/logs \
  -H "Content-Type: application/json" \
  -d '{"events":[...]}'
# {"accepted":10,"rejected":0}

# Get top failures (last 24 hours)
curl "http://localhost:8080/api/runtime/failures?window=24h"

# Get trace events
curl http://localhost:8080/api/runtime/trace/<trace-id>

# Get recent events for a node
curl http://localhost:8080/api/runtime/node/<node-id>
```

### SSE Event Stream

```bash
curl -N http://localhost:8080/api/events
# event: node_status_update
# data: {"node_id":"function:...","status":"error"}
#
# event: scan_progress
# data: {"files_scanned":42,"total_files":100}
#
# event: agent_step
# data: {"step":1,"reasoning":"Looking up callers...","tool":"graph_query"}
```

Event types: `node_status_update`, `scan_progress`, `scan_complete`,
`agent_start`, `agent_step`, `agent_tool_call`, `agent_tool_result`,
`agent_done`, `agent_error`, `job_update`.

---

## Project Structure

```
vyuha-ai/
├── cmd/server/main.go          # Entry point — starts HTTP server
├── db/
│   ├── schema.sql              # Complete SQLite schema
│   └── embed.go                # Embeds schema.sql into Go binary
├── internal/
│   ├── ai/provider.go          # AI provider abstraction (Bedrock/Ollama)
│   ├── api/server.go           # HTTP API handlers + middleware
│   ├── graph/
│   │   ├── node.go             # Node types, metadata, constructors
│   │   ├── edge.go             # Edge types, metadata, constructors
│   │   └── index.go            # In-memory graph index
│   ├── parser/golang/
│   │   ├── parser.go           # Go AST parser (concurrent, incremental)
│   │   ├── cloud.go            # Cloud/queue/DB SDK detection
│   │   └── dataflow.go         # Intra-function data flow analysis
│   ├── query/
│   │   ├── query.go            # Query coordinator & mode router
│   │   ├── scorer.go           # Question → mode scoring
│   │   ├── subgraph.go         # Subgraph extraction algorithms
│   │   └── agent.go            # AI agent with graph-aware tools
│   ├── runtime/ingestor.go     # Runtime event processing
│   └── storage/
│       ├── storage.go          # SQLite operations (1000+ lines)
│       └── schema.go           # Schema versioning & migrations
├── frontend/
│   ├── src/
│   │   ├── App.tsx             # Main app shell
│   │   ├── main.tsx            # React entry point
│   │   ├── types/graph.ts      # TypeScript types mirroring Go structs
│   │   ├── api/client.ts       # Typed API client
│   │   ├── hooks/
│   │   │   ├── useGraph.ts     # Graph state + dagre layout
│   │   │   ├── useSSE.ts       # SSE with reconnection
│   │   │   └── useAgent.ts     # Agent interaction state
│   │   └── components/
│   │       ├── GraphCanvas.tsx  # React Flow canvas
│   │       ├── StatusBar.tsx    # Bottom status bar
│   │       ├── nodes/           # Custom node renderers
│   │       └── panels/          # Detail, Agent, Query, LogIngestion
│   ├── package.json
│   ├── vite.config.ts
│   └── tailwind.config.js
├── scripts/
│   ├── generate_demo_data.go   # Demo database generator
│   └── simulate_failures.go    # Live incident simulator
├── docker-compose.yml
├── Dockerfile
└── go.mod
```

---

## Docker

```bash
docker build -t vyuha-ai .
docker run -p 8080:8080 -v $(pwd)/vyuha-demo.db:/app/vyuha-demo.db \
  -e AWS_REGION=us-east-1 \
  -e AWS_ACCESS_KEY_ID=... \
  -e AWS_SECRET_ACCESS_KEY=... \
  vyuha-ai --db-path /app/vyuha-demo.db
```

---

## License

MIT
