// ---------------------------------------------------------------------------
// api/client.ts — Typed API client for the contextplus-backed backend
// ---------------------------------------------------------------------------

import type { CallChainResponse, NodeDetail, QueryDecision, TextResult } from '../types/graph';

const BASE = '/api';

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

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export const api = {
  /** Get call chain for a symbol → structured nodes/edges/meta. */
  callChain(symbolName: string, filePath?: string, maxDepth?: number): Promise<CallChainResponse> {
    return post<CallChainResponse>('/call-chain', {
      symbol_name: symbolName,
      file_path: filePath,
      max_depth: maxDepth ?? 5,
    });
  },

  /** Semantic code search → text result. */
  search(query: string, topK?: number): Promise<TextResult> {
    return post<TextResult>('/search', { query, top_k: topK });
  },

  /** Get context tree for the codebase or a specific path. */
  contextTree(targetPath?: string): Promise<TextResult> {
    return post<TextResult>('/context-tree', { target_path: targetPath });
  },

  /** Get file skeleton (signatures, structure). */
  skeleton(filePath: string): Promise<TextResult> {
    return post<TextResult>('/skeleton', { file_path: filePath });
  },

  /** Get blast radius for a symbol. */
  blastRadius(symbolName: string, fileContext?: string): Promise<TextResult> {
    return post<TextResult>('/blast-radius', { symbol_name: symbolName, file_context: fileContext });
  },

  /** Search identifiers (function/class/variable names). */
  identifierSearch(query: string, topK?: number): Promise<TextResult> {
    return post<TextResult>('/identifier-search', { query, top_k: topK });
  },

  /** Run static analysis on a path. */
  staticAnalysis(targetPath?: string): Promise<TextResult> {
    return post<TextResult>('/static-analysis', { target_path: targetPath });
  },

  /** Scan a repository — builds the semantic tree via contextplus. */
  scanRepo(repoPath: string): Promise<{ tree: string; symbol_count: number; ready: boolean }> {
    return post<{ tree: string; symbol_count: number; ready: boolean }>('/scan-repo', {
      repo_path: repoPath,
    });
  },

  /** Clone a GitHub repo, then build semantic tree. */
  cloneRepo(githubUrl: string): Promise<{
    name: string;
    local_path: string;
    tree: string;
    symbol_count: number;
    ready: boolean;
  }> {
    return post('/clone-repo', { github_url: githubUrl });
  },

  /** Agentic RAG — ask a natural-language question about the codebase.
   *  When repoPath is provided, the RAG session targets that specific repo
   *  directory so tools only access the already-built context tree / index. */
  ragQuery(question: string, repoPath?: string): Promise<{ answer: string }> {
    return post('/rag-query', { question, repo_path: repoPath });
  },

  /** Get expanded detail for a single node by ID. */
  getNode(nodeId: string): Promise<NodeDetail> {
    return post<NodeDetail>('/node', { node_id: nodeId });
  },

  /** Ask the agent a question about the codebase. */
  askQuestion(question: string): Promise<QueryDecision> {
    return post<QueryDecision>('/ask', { question });
  },

  /** Get the current status of a scan job. */
  getScanStatus(jobId: string): Promise<{ status: string; error?: string } | null> {
    return request<{ status: string; error?: string } | null>(`/scan-status/${jobId}`);
  },

  /** Start a full scan of the given root path. */
  scan(rootPath: string): Promise<{ job_id?: string }> {
    return post<{ job_id?: string }>('/scan', { root_path: rootPath });
  },

  /** Load built-in demo data. */
  loadDemo(): Promise<void> {
    return post<void>('/load-demo', {});
  },

  /** Get overall graph stats. */
  getStats(): Promise<{ total_nodes?: number; total_edges?: number } | null> {
    return request<{ total_nodes?: number; total_edges?: number } | null>('/stats');
  },
};
