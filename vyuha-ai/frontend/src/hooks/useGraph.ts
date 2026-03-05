// ---------------------------------------------------------------------------
// hooks/useGraph.ts — Graph state management (contextplus-backed)
// ---------------------------------------------------------------------------

import { useCallback, useMemo, useState } from 'react';
import type { Node as ReactFlowNode, Edge as ReactFlowEdge } from 'reactflow';
import type { CallChainMeta, CanvasMode, FlatNode } from '../types/graph';
import { applyTreeLayout } from './useGraphLayout';

// Re-export layout for external consumers
export { applyDagreLayout, applyTreeLayout } from './useGraphLayout';

// ---------------------------------------------------------------------------
// Convert FlatNode to a CallChainNode ReactFlow node
// ---------------------------------------------------------------------------

function flatNodeToRF(n: FlatNode, meta: CallChainMeta): ReactFlowNode {
  const annotations = meta.annotations[n.id] ?? [];
  const depth = meta.depths[n.id] ?? -1;
  const role = meta.node_roles[n.id] ?? 'callee';

  return {
    id: n.id,
    type: 'callChainNode',
    position: { x: 0, y: 0 },
    data: {
      ...n,
      // CallChainNode expects these fields in data:
      name: n.name,
      file_path: n.file,
      line_start: n.line,
      line_end: n.end_line,
      signature: n.signature,
      is_external: n.is_external,
      annotations,
      chainDepth: depth,
      role,
    },
  };
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export interface UseGraphReturn {
  nodes: ReactFlowNode[];
  edges: ReactFlowEdge[];
  canvasMode: CanvasMode;
  callChainMeta: CallChainMeta | null;
  textResult: string | null;
  textTool: string | null;
  setCallChainData: (nodes: ReactFlowNode[], edges: ReactFlowEdge[], meta: CallChainMeta) => void;
  setCallChainFromResponse: (
    flatNodes: FlatNode[],
    flatEdges: { id: string; source: string; target: string }[],
    meta: CallChainMeta,
  ) => void;
  setTextResult: (tool: string, text: string) => void;
  exitCallChain: () => void;
  clearAll: () => void;
  /** Placeholder — load services graph from the backend. Currently a no-op. */
  loadServices: () => void;
  isLoading: boolean;
  error: string | null;
  setError: (e: string | null) => void;
}

export function useGraph(): UseGraphReturn {
  const [rfNodes, setNodes] = useState<ReactFlowNode[]>([]);
  const [rfEdges, setEdges] = useState<ReactFlowEdge[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [canvasMode, setCanvasMode] = useState<CanvasMode>('call_chain');
  const [callChainMeta, setCallChainMeta] = useState<CallChainMeta | null>(null);
  const [textResult, setTextResultState] = useState<string | null>(null);
  const [textTool, setTextTool] = useState<string | null>(null);

  // ---- Set call chain from already-built ReactFlow nodes -------------------
  const setCallChainData = useCallback(
    (newNodes: ReactFlowNode[], newEdges: ReactFlowEdge[], meta: CallChainMeta) => {
      setCanvasMode('call_chain');
      setCallChainMeta(meta);
      setTextResultState(null);
      setTextTool(null);
      const { nodes: ln, edges: le } = applyTreeLayout(newNodes, newEdges, meta);
      setNodes(ln);
      setEdges(le);
    },
    [],
  );

  // ---- Set call chain from raw API response --------------------------------
  const setCallChainFromResponse = useCallback(
    (
      flatNodes: FlatNode[],
      flatEdges: { id: string; source: string; target: string }[],
      meta: CallChainMeta,
    ) => {
      const rfN = flatNodes.map((n) => flatNodeToRF(n, meta));
      const rfE: ReactFlowEdge[] = flatEdges.map((e) => ({
        id: e.id,
        source: e.source,
        target: e.target,
        type: 'callChain' as const,
        data: { type: 'calls', isCycle: false },
      }));
      setCanvasMode('call_chain');
      setCallChainMeta(meta);
      setTextResultState(null);
      setTextTool(null);
      const { nodes: ln, edges: le } = applyTreeLayout(rfN, rfE, meta);
      setNodes(ln);
      setEdges(le);
    },
    [],
  );

  // ---- Set text result (search, skeleton, etc.) ----------------------------
  const setTextResult = useCallback((tool: string, text: string) => {
    setCanvasMode('text_result');
    setCallChainMeta(null);
    setTextResultState(text);
    setTextTool(tool);
    setNodes([]);
    setEdges([]);
  }, []);

  // ---- Exit call chain mode ------------------------------------------------
  const exitCallChain = useCallback(() => {
    setCanvasMode('call_chain');
    setCallChainMeta(null);
    setNodes([]);
    setEdges([]);
  }, []);

  // ---- Clear all -----------------------------------------------------------
  const clearAll = useCallback(() => {
    setNodes([]);
    setEdges([]);
    setCanvasMode('call_chain');
    setCallChainMeta(null);
    setTextResultState(null);
    setTextTool(null);
    setError(null);
  }, []);

  // ---- Load services (placeholder for service-graph mode) ------------------
  const loadServices = useCallback(() => {
    // No-op: service graph loading is not yet implemented.
    // ScanContext calls this after a successful scan to refresh the view.
  }, []);

  return {
    nodes: rfNodes,
    edges: rfEdges,
    canvasMode,
    callChainMeta,
    textResult,
    textTool,
    setCallChainData,
    setCallChainFromResponse,
    setTextResult,
    exitCallChain,
    clearAll,
    loadServices,
    isLoading,
    error,
    setError,
  };
}
