// ---------------------------------------------------------------------------
// types/graph.ts — TypeScript equivalents of Go structs
// ---------------------------------------------------------------------------

// ---- Node Types -----------------------------------------------------------

export enum NodeType {
  Repository = 'repository',
  Service = 'service',
  Package = 'package',
  File = 'file',
  Function = 'function',
  Struct = 'struct',
  Interface = 'interface',
  CloudService = 'cloud_service',
  RuntimeInstance = 'runtime_instance',
  DataFlow = 'data_flow',
}

// ---- Edge Types -----------------------------------------------------------

export enum EdgeType {
  Contains = 'contains',
  Imports = 'imports',
  Calls = 'calls',
  Implements = 'implements',
  DependsOn = 'depends_on',
  ConnectsTo = 'connects_to',
  RuntimeCalls = 'runtime_calls',
  FailedAt = 'failed_at',
  ProducesTo = 'produces_to',
  ConsumedBy = 'consumed_by',
  Transforms = 'transforms',
  FieldMap = 'field_map',
}

// ---- Parameter / Field ----------------------------------------------------

export interface Parameter {
  name: string;
  type: string;
}

// ---- Node Metadata --------------------------------------------------------

export interface NodeMetadata {
  // AI fields
  ai_summary?: string;
  ai_embedding_id?: string;

  // Function metadata
  receiver?: string;
  signature?: string;
  parameters?: Parameter[];
  return_types?: string[];
  calls?: string[];
  called_by?: string[];
  doc_comment?: string;
  source_snippet?: string;
  cyclomatic_complexity?: number;
  has_error_return?: boolean;
  has_context_param?: boolean;

  // Struct / Interface
  fields?: Parameter[];
  methods?: string[];
  implements_interfaces?: string[];
  implemented_by?: string[];
  embedded_types?: string[];

  // File
  import_paths?: string[];
  line_count?: number;

  // Service
  entry_point?: string;
  package_count?: number;
  function_count?: number;
  total_lines?: number;
  cloud_dependencies?: string[];

  // Cloud
  cloud_provider?: string;
  cloud_service?: string;
  operations?: string[];
  detected_in_files?: string[];
  sdk_calls?: string[];

  // Data flow
  kind?: string;
  type_name?: string;
  source?: string;
  sink?: string;
  is_aggregate?: boolean;
  fan_in?: number;
}

// ---- GraphNode (matches Go graph.Node) ------------------------------------

export interface GraphNode {
  id: string;
  type: NodeType;
  name: string;
  parent_id: string;
  file_path: string;
  line_start: number;
  line_end: number;
  runtime_status: string;
  error_count: number;
  last_seen: string;
  language: string;
  is_exported: boolean;
  depth: number;
  metadata: NodeMetadata;
}

// ---- GraphEdge (matches Go graph.Edge) ------------------------------------

export interface EdgeMetadata {
  // CALLS edges
  call_type?: string;          // "method_call" | "function_call" | "interface_dispatch"
  is_resolved?: boolean;
  resolution_method?: string;  // "type_checker" | "ast_heuristic"
  is_goroutine?: boolean;
  is_deferred?: boolean;
  call_site_line?: number;
  is_cloud_call?: boolean;
  cloud_service?: string;
  cloud_operation?: string;

  // CONTAINS edges
  hierarchy_depth?: number;

  // IMPORTS edges
  alias?: string;
  is_stdlib?: boolean;
  is_third_party?: boolean;
  resolved_version?: string;

  // PRODUCES_TO / CONSUMED_BY edges
  topic_name?: string;
  queue_type?: string;         // "kafka" | "sqs" | "rabbitmq" | "nats"

  // TRANSFORMS / FIELD_MAP edges
  from_field?: string;
  to_field?: string;
  is_computed?: boolean;
  expression?: string;
}

export interface GraphEdge {
  id: string;
  source_id: string;
  target_id: string;
  type: EdgeType;
  metadata: EdgeMetadata;
}

// ---- Runtime Event --------------------------------------------------------

export interface RuntimeEvent {
  id: string;
  node_id: string;
  event_type: string;
  status: string;
  error_message: string;
  error_code: string;
  latency_ms: number;
  trace_id: string;
  span_id: string;
  parent_span_id: string;
  timestamp: string;
  metadata: Record<string, string>;
}

// ---- Subgraph Result ------------------------------------------------------

export interface Position {
  x: number;
  y: number;
}

export interface SubgraphLayout {
  positions: Record<string, Position>;
}

export interface SubgraphResult {
  query_type: string;
  target_id: string;
  nodes: GraphNode[];
  edges: GraphEdge[];
  entry_points?: string[];
  critical_path?: string[];
  node_roles: Record<string, string>;
  layout: SubgraphLayout;
}

// ---- Agent ----------------------------------------------------------------

export interface AgentStep {
  step: number;
  tool_calls?: { id: string; name: string; arguments: string }[];
  results?: string[];
  reasoning?: string;
  timestamp: string;
}

export interface AgentRun {
  question: string;
  answer: string;
  steps: AgentStep[];
  duration_ms: number;
}

// ---- Stats ----------------------------------------------------------------

export interface GraphStats {
  total_nodes: number;
  total_edges: number;
  nodes_by_type: Record<string, number>;
  edges_by_type: Record<string, number>;
  files_indexed: number;
  status_counts: Record<string, number>;
}

export interface NodeFailureStat {
  node_id: string;
  node_name: string;
  failure_count: number;
  last_failure: string;
}

// ---- Data Flow Records ----------------------------------------------------

export interface DataFlowRecord {
  id: string;
  function_id: string;
  kind: string;
  type_name: string;
  field_name: string;
  source: string;
  sink: string;
  is_aggregate: boolean;
  fan_in: number;
  transform_chain: string;
}

export interface FunctionDataFlow {
  function_id: string;
  function_name: string;
  records: DataFlowRecord[];
}

// ---- API-specific types ---------------------------------------------------

export interface ScanResponse {
  job_id: string;
  status: string;
}

export interface ScanStatus {
  job_id: string;
  status: string;
  progress?: number;
  files_scanned?: number;
  error?: string;
}

export interface NodeDetail {
  node: GraphNode;
  callers: GraphNode[];
  callees: GraphNode[];
  data_flow?: DataFlowRecord[];
  recent_events?: RuntimeEvent[];
}

export interface LogEvent {
  node_id: string;
  event_type: string;
  status: string;
  error_message?: string;
  error_code?: string;
  latency_ms?: number;
  trace_id?: string;
  span_id?: string;
  parent_span_id?: string;
  metadata?: Record<string, string>;
}

export interface IngestResponse {
  status: string;
  node_id: string;
  runtime_status: string;
}

export interface BatchIngestResponse {
  accepted: number;
  rejected: number;
  errors?: string[];
}

export interface TraceResult {
  trace_id: string;
  events: RuntimeEvent[];
  nodes: GraphNode[];
}

export interface QueryDecision {
  mode: 'direct_graph' | 'subgraph' | 'agent' | 'sql';
  subgraph_type?: string;
  target_id?: string;
  confidence: number;
  reasoning: string;
  answer?: string;
  subgraph?: SubgraphResult;
  agent_run?: AgentRun;
  graph_data?: {
    nodes?: GraphNode[];
    edges?: GraphEdge[];
    stats?: GraphStats;
  };
}

export interface AIJob {
  id: string;
  kind: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  params: unknown;
  result?: unknown;
  error?: string;
  created_at: string;
  started_at?: string;
  done_at?: string;
}

export interface SSEEvent {
  event: string;
  data: unknown;
}

export interface WatchStatus {
  active: boolean;
  file_path?: string;
  lines_read?: number;
  parse_errors?: number;
  submit_errors?: number;
  started_at?: string;
  event_count?: number;
}

export interface ScanProgress {
  job_id: string;
  phase: string;
  files_scanned: number;
  total_files: number;
  current_file: string;
}

// ---- EdgeMetadata type guards ---------------------------------------------

export function isCallEdge(metadata?: EdgeMetadata): boolean {
  return metadata?.call_type !== undefined;
}

export function isImportEdge(metadata?: EdgeMetadata): boolean {
  return metadata?.alias !== undefined || metadata?.is_stdlib !== undefined;
}

export function isQueueEdge(metadata?: EdgeMetadata): boolean {
  return metadata?.topic_name !== undefined;
}

export function isCloudCallEdge(metadata?: EdgeMetadata): boolean {
  return metadata?.is_cloud_call === true;
}
