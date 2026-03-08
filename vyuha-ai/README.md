<div align="center">

# 🔮 Codrix.ai

### **AI-Powered Codebase Intelligence & Visualization**

*Scan any GitHub repository. Explore it as an interactive graph. Generate architecture, sequence, and E-R diagrams. Run deep research — all powered by AI.*

<br/>

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?style=for-the-badge&logo=react&logoColor=black)
![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?style=for-the-badge&logo=typescript&logoColor=white)
![Tailwind](https://img.shields.io/badge/Tailwind_CSS-3-06B6D4?style=for-the-badge&logo=tailwindcss&logoColor=white)
![AWS](https://img.shields.io/badge/AWS_Bedrock-FF9900?style=for-the-badge&logo=amazonaws&logoColor=white)

</div>

---

## ✨ Features

<table>
<tr>
<td width="50%">

### 🗺️ Interactive Code Graphs
Visualize any repository as a navigable **ReactFlow** graph — functions, structs, interfaces, packages, and their relationships rendered with ELK auto-layout.

</td>
<td width="50%">

### 🔍 Deep Research
One-click in-depth analysis that generates a **comprehensive Markdown report** plus three diagrams (Architecture, Sequence, E-R) for any GitHub repository.

</td>
</tr>
<tr>
<td>

### 🤖 AI-Powered Tools
10+ analysis tools — **call-chain tracing**, blast-radius analysis, semantic search, static analysis, file skeletons, identifier lookup — all driven by natural-language queries.

</td>
<td>

### 📊 Three Diagram Types
Auto-generated **Architecture**, **UML Sequence**, and **Entity-Relationship** diagrams with edge highlighting, zoom/pan controls, and PNG/SVG export.

</td>
</tr>
<tr>
<td>

### 💾 Workspace Persistence
All diagrams and research results are **auto-saved to localStorage** and visible from the dashboard. Re-open any previous analysis instantly.

</td>
<td>

### 🌐 Multi-Repo Support
Clone repositories from **GitHub URLs** or add local paths. Switch between repos from the sidebar. Each repo maintains its own analysis history.

</td>
</tr>
</table>

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     React Frontend (Vite)                    │
│                                                             │
│  Landing Page ─► Sidebar ─► Graph Canvas / Deep Research    │
│  ReactFlow  ·  ELKjs layout  ·  Tailwind + shadcn/ui       │
│  Sequence & E-R diagram renderers  ·  Export dialog         │
└──────────────────────┬──────────────────────────────────────┘
                       │  HTTP / JSON / SSE
┌──────────────────────▼──────────────────────────────────────┐
│                  Go HTTP Server (:8080)                      │
│                                                             │
│  net/http  ·  CORS middleware  ·  SSE broadcaster           │
│  /api/scan-repo  ·  /api/clone-repo  ·  /api/call-chain    │
│  /api/search  ·  /api/blast-radius  ·  /api/rag-query      │
│  /api/generate-diagram  ·  /api/context-tree  ·  ...        │
├─────────────────────────────────────────────────────────────┤
│              MCP Bridge (JSON-RPC 2.0 via stdin/stdout)     │
│                                                             │
│  Spawns contextplus as a child process                      │
│  Maps API endpoints → MCP tool calls                        │
├─────────────────────────────────────────────────────────────┤
│              AWS Bedrock (LLM)          Deep Research API   │
│  RAG queries · Diagram generation       EC2-hosted service  │
│  Qwen Coder via callLLM()              /api/v1/research/*   │
└─────────────────────────────────────────────────────────────┘
```

### How It Works

1. **Clone/scan** a repository → the Go server spawns `contextplus` via JSON-RPC 2.0 (MCP protocol) to parse and index the codebase
2. **Explore** the graph → the frontend fetches nodes/edges and renders them with ELK auto-layout in ReactFlow
3. **Run tools** → natural-language queries are routed through the backend to contextplus tools (call-chain, blast-radius, semantic search, etc.)
4. **Generate diagrams** → the backend calls AWS Bedrock to produce architecture/sequence/E-R specs from the indexed codebase
5. **Deep Research** → triggers the external Deep Research API for comprehensive analysis, polls for completion, fetches the report + diagrams

---

## 📁 Project Structure

```
vyuha-ai/
├── cmd/server/                    # Go entry point
│   └── main.go
│
├── internal/
│   ├── api/
│   │   ├── server.go              # HTTP server, router, middleware
│   │   ├── handlers.go            # All API endpoint handlers
│   │   ├── llm.go                 # AWS Bedrock LLM integration
│   │   └── sse.go                 # Server-Sent Events broadcaster
│   └── bridge/
│       ├── client.go              # MCP client (JSON-RPC 2.0 over stdin)
│       └── tools.go               # Typed wrappers for contextplus tools
│
├── frontend/
│   ├── src/
│   │   ├── App.tsx                # Root app — view routing & state
│   │   ├── api/client.ts          # Typed HTTP client for all endpoints
│   │   ├── types/
│   │   │   ├── graph.ts           # Node, edge, diagram, tool types
│   │   │   └── workspace.ts       # Workspace persistence types
│   │   ├── hooks/
│   │   │   ├── useGraph.ts        # Graph state management + layout
│   │   │   ├── useGraphLayout.ts  # ELK layout computation
│   │   │   ├── useSSE.ts          # SSE connection with reconnect
│   │   │   └── useWorkspace.ts    # localStorage persistence
│   │   └── components/
│   │       ├── LandingPage.tsx     # Hero landing page
│   │       ├── Sidebar.tsx         # Repo list + navigation
│   │       ├── Dashboard.tsx       # Saved diagrams table
│   │       ├── GraphCanvas.tsx     # ReactFlow graph canvas
│   │       ├── ChatCodeView.tsx    # Tool query + results view
│   │       ├── DeepResearchView.tsx        # Deep Research full-page view
│   │       ├── SequenceDiagramRenderer.tsx # UML sequence diagram
│   │       ├── ERDiagramRenderer.tsx       # Entity-relationship diagram
│   │       ├── AIDiagramView.tsx           # AI diagram generation
│   │       ├── RepoSelectDialog.tsx        # Clone/add repo dialog
│   │       ├── StatusBar.tsx               # Bottom status bar
│   │       ├── nodes/              # Custom ReactFlow node components
│   │       ├── edges/              # Custom ReactFlow edge components
│   │       ├── eraser/             # Export dialog & watermarking
│   │       └── ui/                 # shadcn/ui primitives
│   ├── package.json
│   ├── vite.config.ts
│   └── tailwind.config.js
│
├── _vyuha_repos/                  # Cloned repositories (gitignored)
├── docker-compose.yml
├── Dockerfile
└── go.mod
```

---

## 🚀 Quick Start

### Prerequisites

| Tool | Version | Purpose |
|:-----|:--------|:--------|
| **Go** | 1.22+ | Backend server |
| **Node.js** | 18+ | Frontend dev server |
| **Git** | any | Clone repositories |
| **AWS credentials** | Bedrock access | AI features (RAG, diagrams) |

### 1. Clone & install

```bash
git clone https://github.com/sreenivasivbieb/Team_Tajmahal.git
cd Team_Tajmahal/vyuha-ai
go mod download
```

### 2. Set up the frontend

```bash
cd frontend
npm install
cd ..
```

### 3. Configure environment

Set AWS credentials for Bedrock AI features:

```bash
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=<your-key>
export AWS_SECRET_ACCESS_KEY=<your-secret>
```

### 4. Start the backend

```bash
go run ./cmd/server/
# Server starts on http://localhost:8080
```

### 5. Start the frontend (dev mode)

```bash
cd frontend
npm run dev
# Vite dev server on http://localhost:5173
```

### 6. Open the app

Navigate to **http://localhost:5173** — the landing page will greet you.

---

## 🛠️ Available Analysis Tools

| Tool | Description |
|:-----|:------------|
| **Call Chain** | Trace the full caller/callee chain for any symbol |
| **Blast Radius** | See what breaks if a function/module changes |
| **Context Tree** | Hierarchical view of packages, files, and symbols |
| **Semantic Search** | Natural-language search across the indexed codebase |
| **Static Analysis** | Code quality insights and pattern detection |
| **File Skeleton** | Structural outline of any source file |
| **Identifier Search** | Find all occurrences of a symbol by name |
| **RAG Query** | Ask free-form questions answered with codebase context |
| **Generate Diagram** | AI-generated architecture / sequence / E-R diagrams |
| **Deep Research** | Comprehensive repository analysis with report + diagrams |

---

## 🔬 Deep Research

Deep Research performs an end-to-end analysis of a GitHub repository:

1. **Start** — Initiates analysis via the Deep Research API
2. **Poll** — Frontend polls for status updates with a live progress indicator
3. **Report** — A rich Markdown report is generated covering architecture, patterns, dependencies, and recommendations
4. **Diagrams** — Three diagrams are auto-generated:
   - 🏗️ **Architecture** — Service/module topology with boundaries and data flows
   - 🔄 **Sequence** — UML sequence diagram of key interactions
   - 🗄️ **ER** — Entity-relationship diagram of data models

Results are **auto-saved** and can be reopened from the dashboard at any time.

---

## 📡 API Endpoints

<details>
<summary><strong>Repository Management</strong></summary>

| Method | Endpoint | Description |
|:-------|:---------|:------------|
| `POST` | `/api/clone-repo` | Clone a GitHub repository |
| `POST` | `/api/scan-repo` | Scan/index a local repository |

</details>

<details>
<summary><strong>Code Analysis</strong></summary>

| Method | Endpoint | Description |
|:-------|:---------|:------------|
| `POST` | `/api/call-chain` | Get call chain for a symbol |
| `POST` | `/api/search` | Semantic code search |
| `POST` | `/api/context-tree` | Get context tree for a path |
| `POST` | `/api/context-tree-architecture` | Architecture-level context tree |
| `POST` | `/api/skeleton` | File skeleton / outline |
| `POST` | `/api/blast-radius` | Blast radius analysis |
| `POST` | `/api/identifier-search` | Search by identifier name |
| `POST` | `/api/static-analysis` | Run static analysis |
| `POST` | `/api/rag-query` | RAG-powered Q&A |

</details>

<details>
<summary><strong>Diagram Generation</strong></summary>

| Method | Endpoint | Description |
|:-------|:---------|:------------|
| `POST` | `/api/generate-diagram` | Generate architecture/sequence/ER diagram |
| `POST` | `/api/edit-diagram` | Edit an existing diagram with instructions |

</details>

<details>
<summary><strong>Deep Research</strong></summary>

| Method | Endpoint | Description |
|:-------|:---------|:------------|
| `POST` | `/api/deep-research/start` | Start deep research analysis |
| `GET` | `/api/deep-research/status` | Poll analysis status |
| `GET` | `/api/deep-research/report` | Fetch completed report |
| `POST` | `/api/deep-research/diagrams` | Generate diagrams from report |

</details>

<details>
<summary><strong>Real-time</strong></summary>

| Method | Endpoint | Description |
|:-------|:---------|:------------|
| `GET` | `/api/events` | SSE stream for live updates |
| `GET` | `/health` | Health check |

</details>

---

## 🐳 Docker

```bash
docker build -t codrix-ai .
docker run -p 8080:8080 \
  -e AWS_REGION=us-east-1 \
  -e AWS_ACCESS_KEY_ID=... \
  -e AWS_SECRET_ACCESS_KEY=... \
  codrix-ai
```

---

## 🧰 Tech Stack

| Layer | Technology |
|:------|:-----------|
| **Frontend** | React 18, TypeScript, Vite, ReactFlow, ELKjs, Tailwind CSS, shadcn/ui, react-markdown |
| **Backend** | Go 1.22+, net/http, AWS SDK for Go |
| **AI / LLM** | AWS Bedrock (Qwen Coder), RAG pipeline |
| **Code Intelligence** | contextplus (MCP protocol via JSON-RPC 2.0) |
| **Deep Research** | External EC2-hosted analysis service |
| **State** | localStorage (client-side workspace persistence) |

---

## 👥 Team Tajmahal

Built with ❤️ as part of the Codrix.ai project.

---

<div align="center">
<sub>Powered by <strong>contextplus</strong> · <strong>AWS Bedrock</strong> · <strong>ReactFlow</strong></sub>
</div>
