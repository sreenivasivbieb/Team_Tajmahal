// ---------------------------------------------------------------------------
// api/client.ts — Typed API client for the VYUHA AI backend
// ---------------------------------------------------------------------------

import type {
  AIJob,
  BatchIngestResponse,
  DataFlowRecord,
  GraphEdge,
  GraphNode,
  GraphStats,
  IngestResponse,
  LogEvent,
  NodeDetail,
  NodeFailureStat,
  QueryDecision,
  RuntimeEvent,
  ScanResponse,
  ScanStatus,
  SubgraphResult,
  TraceResult,
  WatchStatus,
} from '../types/graph';

const BASE = '/api';

// All backend responses follow the shape { data: <payload> }.
// requestRaw returns the full JSON; request unwraps `.data` automatically.
async function requestRaw<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(`API ${res.status}: ${body || res.statusText}`);
  }
  return res.json() as Promise<T>;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const json = await requestRaw<{ data: T }>(path, init);
  return json.data;
}

function get<T>(path: string): Promise<T> {
  return request<T>(path);
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' });
}

// ---------------------------------------------------------------------------
// Helper: convert [{type,count}] array → Record<string,number>
// ---------------------------------------------------------------------------
function arrayToRecord(arr: unknown): Record<string, number> {
  if (Array.isArray(arr)) {
    const rec: Record<string, number> = {};
    for (const item of arr) {
      if (item && typeof item === 'object' && 'type' in item && 'count' in item) {
        rec[(item as { type: string }).type] = (item as { count: number }).count;
      }
    }
    return rec;
  }
  if (arr && typeof arr === 'object') return arr as Record<string, number>;
  return {};
}

// ---------------------------------------------------------------------------
// Public API surface
// ---------------------------------------------------------------------------

export const api = {
  // ---- Scan ---------------------------------------------------------------

  /** Trigger a codebase scan. */
  scan(rootPath: string): Promise<ScanResponse> {
    return post<ScanResponse>('/scan', { root_path: rootPath });
  },

  /** Poll scan job status. */
  getScanStatus(jobId: string): Promise<ScanStatus> {
    return get<ScanStatus>(`/scan/status?job_id=${encodeURIComponent(jobId)}`);
  },

  // ---- Graph --------------------------------------------------------------

  /** List all service-level nodes. */
  async getServices(): Promise<GraphNode[]> {
    const res = await get<{ services: GraphNode[] }>('/graph/services');
    return res.services ?? [];
  },

  /** Get children subgraph for a parent node. */
  getChildren(parentId: string, depth: number): Promise<SubgraphResult> {
    return get<SubgraphResult>(
      `/graph/children?parent_id=${encodeURIComponent(parentId)}&depth=${depth}`,
    );
  },

  /** Extract a focused subgraph. */
  getSubgraph(targetId: string, queryType: string): Promise<SubgraphResult> {
    return get<SubgraphResult>(
      `/graph/subgraph?target_id=${encodeURIComponent(targetId)}&type=${encodeURIComponent(queryType)}`,
    );
  },

  /** Get a single node with callers, callees, events. */
  async getNode(id: string): Promise<NodeDetail> {
    // Backend returns { node, children, in_edges, out_edges, data_flow, events }
    // but NodeDetail expects { node, callers, callees, data_flow, recent_events }
    const raw = await get<{
      node: GraphNode;
      children: GraphNode[];
      in_edges: GraphEdge[];
      out_edges: GraphEdge[];
      data_flow?: DataFlowRecord[];
      events?: RuntimeEvent[];
    }>(`/graph/node/${encodeURIComponent(id)}`);
    return {
      node: raw.node,
      callers: raw.children ?? [],
      callees: raw.children ?? [],
      data_flow: raw.data_flow,
      recent_events: raw.events,
    };
  },

  /** Get aggregate graph statistics. */
  async getStats(): Promise<GraphStats> {
    const raw = await get<Record<string, unknown>>('/graph/stats');
    return {
      total_nodes: (raw.total_nodes as number) ?? 0,
      total_edges: (raw.total_edges as number) ?? 0,
      files_indexed: (raw.files_indexed as number) ?? 0,
      nodes_by_type: arrayToRecord(raw.nodes_by_type),
      edges_by_type: arrayToRecord(raw.edges_by_type),
      status_counts: arrayToRecord(raw.status_counts),
    };
  },

  /** Search nodes by name (optionally filtered by type). */
  async searchNodes(q: string, type?: string): Promise<GraphNode[]> {
    if (!q.trim()) return [];   // backend requires non-empty q
    let path = `/graph/search?q=${encodeURIComponent(q)}`;
    if (type) path += `&type=${encodeURIComponent(type)}`;
    const res = await get<{ nodes: GraphNode[]; total: number }>(path);
    return res.nodes ?? [];
  },

  // ---- Runtime / Ingestion ------------------------------------------------

  /** Ingest a single runtime event. */
  ingestLog(event: LogEvent): Promise<IngestResponse> {
    return post<IngestResponse>('/ingest/log', event);
  },

  /** Ingest a batch of runtime events. */
  ingestLogs(events: LogEvent[]): Promise<BatchIngestResponse> {
    return post<BatchIngestResponse>('/ingest/logs', { events });
  },

  /** Get top failing nodes in the given window (e.g. "24h"). */
  async getFailures(window: string): Promise<NodeFailureStat[]> {
    const res = await get<{ failures: NodeFailureStat[]; window: string }>(
      `/runtime/failures?window=${encodeURIComponent(window)}`,
    );
    return res.failures ?? [];
  },

  /** Get all events for a trace. */
  async getTrace(traceId: string): Promise<TraceResult> {
    // Backend returns { trace_id, events, duration_ms, status } — no nodes array.
    const raw = await get<{
      trace_id: string;
      events: RuntimeEvent[];
      duration_ms: number;
      status: string;
    }>(`/runtime/trace/${encodeURIComponent(traceId)}`);
    return {
      trace_id: raw.trace_id,
      events: raw.events ?? [],
      nodes: [],
    };
  },

  // ---- AI / Query ---------------------------------------------------------

  /** Ask a natural-language question about the codebase. */
  askQuestion(question: string): Promise<QueryDecision> {
    return post<QueryDecision>('/ai/query', { question });
  },

  /** Get the status / result of an AI job. */
  getJob(jobId: string): Promise<AIJob> {
    return get<AIJob>(`/ai/jobs/${encodeURIComponent(jobId)}`);
  },

  // ---- Watch / File Tailing -----------------------------------------------

  /** Start watching a log file for new events. */
  startWatch(filePath: string): Promise<{ status: string; file: string }> {
    return post<{ status: string; file: string }>('/ingest/watch', { file_path: filePath });
  },

  /** Stop watching the current log file. */
  stopWatch(): Promise<{ status: string }> {
    return del<{ status: string }>('/ingest/watch');
  },

  /** Get the current watch/tailer status. */
  getWatchStatus(): Promise<WatchStatus> {
    return get<WatchStatus>('/ingest/watch');
  },

  // ---- Quick-inject helpers -----------------------------------------------

  /**
   * Inject a synthetic error event for a node.
   * Uses the raw backend shape (service/function/file), not the LogEvent type.
   */
  injectError(node: {
    service: string;
    name: string;
    file_path: string;
  }): Promise<IngestResponse> {
    return post<IngestResponse>('/ingest/log', {
      service: node.service,
      function: node.name,
      file: node.file_path,
      trace_id: crypto.randomUUID(),
      status: 'error',
      event_type: 'ERROR',
      error: 'connection refused: dial tcp :5432',
      latency_ms: 5000,
      timestamp: new Date().toISOString(),
    });
  },

  /**
   * Send a success/recovery event for each given node.
   * Returns the count of successfully recovered nodes.
   */
  async recoverAll(
    nodes: Array<{ service: string; name: string; file_path: string }>,
  ): Promise<number> {
    let recovered = 0;
    for (const n of nodes) {
      try {
        await post<IngestResponse>('/ingest/log', {
          service: n.service,
          function: n.name,
          file: n.file_path,
          trace_id: crypto.randomUUID(),
          status: 'success',
          event_type: 'RECOVERY',
          latency_ms: 25,
          timestamp: new Date().toISOString(),
        });
        recovered++;
      } catch {
        // best-effort — continue to next node
      }
    }
    return recovered;
  },
};
