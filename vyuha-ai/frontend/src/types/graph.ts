// ---------------------------------------------------------------------------
// types/graph.ts — TypeScript types for the contextplus-backed backend
// ---------------------------------------------------------------------------

// ---- Canvas Mode ----------------------------------------------------------

export type CanvasMode = 'call_chain' | 'text_result';

// ---- Graph Node (full node returned from API) -----------------------------

export interface GraphNode {
  id: string;
  name: string;
  type: string;
  parent_id?: string;
  runtime_status?: string;
  error_count: number;
  file_path?: string;
  line_start: number;
  line_end: number;
  is_exported?: boolean;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  metadata?: Record<string, any>;
}

// ---- Node Detail (expanded info from getNode) -----------------------------

export interface NodeDetail {
  node: GraphNode;
  callers?: GraphNode[];
  callees?: GraphNode[];
  cloud_deps?: GraphNode[];
  implements?: GraphNode[];
  produces_to?: GraphNode[];
  data_flow?: Array<{
    kind: string;
    type_name: string;
    source?: string;
    sink?: string;
  }>;
  recent_events?: RuntimeEvent[];
}

// ---- Runtime Event --------------------------------------------------------

export interface RuntimeEvent {
  timestamp: string;
  status: string;
  event_type: string;
  latency_ms: number;
  error_message?: string;
}

// ---- Agent types ----------------------------------------------------------

export interface AgentStep {
  step: number;
  reasoning?: string;
  tool_calls?: Array<{ name: string; arguments?: string }>;
  results?: string[];
}

export interface AgentRun {
  answer: string;
  steps: AgentStep[];
  duration_ms: number;
}

export interface QueryDecision {
  answer?: string;
  tool?: string;
  confidence?: number;
  agent_run?: AgentRun;
}

// ---- Call Chain types (match Go bridge.FlatNode / FlatEdge) ---------------

export interface FlatNode {
  id: string;
  name: string;
  file: string;
  line: number;
  end_line: number;
  signature: string;
  is_external: boolean;
}

export interface FlatEdge {
  id: string;
  source: string;
  target: string;
}

export interface ChainStats {
  total_hops: number;
  leaf_count: number;
  external_count: number;
  max_depth: number;
}

export interface CallChainMeta {
  root_id: string;
  depths: Record<string, number>;
  annotations: Record<string, string[]>;
  node_roles: Record<string, string>;
  stats: ChainStats;
}

export interface CallChainResponse {
  nodes: FlatNode[];
  edges: FlatEdge[];
  call_chain_meta: CallChainMeta;
  stats: ChainStats;
}

// ---- Text result (search, skeleton, blast-radius, etc.) -------------------

export interface TextResult {
  result: string;
}

// ---- SSE ------------------------------------------------------------------

export interface SSEEvent {
  event: string;
  data: unknown;
}

// ---- Tool types for the tool selector ------------------------------------

export type ToolType =
  | 'call-chain'
  | 'search'
  | 'context-tree'
  | 'skeleton'
  | 'blast-radius'
  | 'identifier-search'
  | 'static-analysis'
  | 'ask-ai';

export interface ToolInfo {
  key: ToolType;
  label: string;
  placeholder: string;
  requiresInput: boolean;
}

export const TOOLS: ToolInfo[] = [
  { key: 'ask-ai',            label: '✦ Ask AI',        placeholder: 'Ask anything about the codebase…',   requiresInput: true  },
  { key: 'call-chain',        label: 'Call Chain',       placeholder: 'Symbol name (e.g. handleRequest)',  requiresInput: true  },
  { key: 'search',            label: 'Search',           placeholder: 'Search query…',                     requiresInput: true  },
  { key: 'blast-radius',      label: 'Blast Radius',     placeholder: 'Symbol name…',                      requiresInput: true  },
  { key: 'identifier-search', label: 'Identifiers',      placeholder: 'Identifier query…',                 requiresInput: true  },
  { key: 'skeleton',          label: 'File Skeleton',    placeholder: 'File path…',                        requiresInput: true  },
  { key: 'context-tree',      label: 'Context Tree',     placeholder: 'Path (optional)…',                  requiresInput: false },
  { key: 'static-analysis',   label: 'Static Analysis',  placeholder: 'Path (optional)…',                  requiresInput: false },
];
