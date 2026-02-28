// ---------------------------------------------------------------------------
// hooks/useGraph.ts — Graph state management with dagre layout
// ---------------------------------------------------------------------------

import { useCallback, useState } from 'react';
import type { Node as ReactFlowNode, Edge as ReactFlowEdge } from 'reactflow';
import dagre from 'dagre';
import { api } from '../api/client';
import type { GraphEdge, GraphNode, NodeType } from '../types/graph';

// ---------------------------------------------------------------------------
// Graph filter types
// ---------------------------------------------------------------------------

export interface GraphFilters {
  statuses: Set<string>; // 'healthy' | 'degraded' | 'error' | 'unknown'
  types: Set<string>;    // 'service' | 'function' | 'struct' | 'interface' | 'cloud_service' | 'data_flow'
  quickFilter: 'none' | 'failing' | 'cloud-deps' | 'entry-points';
}

export const DEFAULT_FILTERS: GraphFilters = {
  statuses: new Set(['healthy', 'degraded', 'error', 'unknown']),
  types: new Set(['service', 'function', 'struct', 'interface', 'cloud_service', 'data_flow']),
  quickFilter: 'none',
};

function matchesAllFilters(
  node: ReactFlowNode,
  filters: GraphFilters,
  edges: ReactFlowEdge[],
): boolean {
  const data = node.data as GraphNode;
  const status = data.runtime_status || 'unknown';
  const type = data.type || 'unknown';

  // Quick filters override normal filters
  switch (filters.quickFilter) {
    case 'failing':
      return status === 'error';
    case 'cloud-deps': {
      if (type === 'cloud_service') return true;
      // Include nodes that have an edge to a cloud_service node
      // (caller side — this node is the source, cloud node is the target)
      // We check if any edge from this node goes to a cloud node — but we
      // don't have direct access to other nodes here, so we rely on node type.
      // Instead, accept nodes that are sources of edges whose target is cloud.
      return false; // handled at the applyFilters level
    }
    case 'entry-points': {
      // Nodes with no incoming edges
      const hasIncoming = edges.some((e) => e.target === node.id);
      return !hasIncoming;
    }
    default:
      break;
  }

  // Normal filters: must match BOTH status and type
  if (!filters.statuses.has(status)) return false;
  if (!filters.types.has(type)) return false;
  return true;
}

export function applyFilters(
  nodes: ReactFlowNode[],
  edges: ReactFlowEdge[],
  filters: GraphFilters,
): { nodes: ReactFlowNode[]; edges: ReactFlowEdge[] } {
  // Build matching set
  let matchingIds: Set<string>;

  if (filters.quickFilter === 'cloud-deps') {
    // Cloud nodes + any node that has an edge to/from a cloud node
    const cloudIds = new Set(
      nodes
        .filter((n) => (n.data as GraphNode).type === 'cloud_service')
        .map((n) => n.id),
    );
    const connectedToCloud = new Set<string>();
    for (const e of edges) {
      if (cloudIds.has(e.source)) connectedToCloud.add(e.target);
      if (cloudIds.has(e.target)) connectedToCloud.add(e.source);
    }
    matchingIds = new Set([...cloudIds, ...connectedToCloud]);
  } else {
    matchingIds = new Set(
      nodes
        .filter((n) => matchesAllFilters(n, filters, edges))
        .map((n) => n.id),
    );
  }

  return {
    nodes: nodes.map((n) => ({
      ...n,
      style: {
        ...n.style,
        opacity: matchingIds.has(n.id) ? 1 : 0.15,
        transition: 'opacity 0.3s ease',
      },
    })),
    edges: edges.map((e) => ({
      ...e,
      style: {
        ...e.style,
        opacity:
          matchingIds.has(e.source) && matchingIds.has(e.target) ? 1 : 0.05,
        transition: 'opacity 0.3s ease',
      },
    })),
  };
}

export function hasActiveFilters(filters: GraphFilters): boolean {
  if (filters.quickFilter !== 'none') return true;
  if (filters.statuses.size !== 4) return true;
  if (filters.types.size !== 6) return true;
  return false;
}

export function countActiveFilters(filters: GraphFilters): number {
  let count = 0;
  if (filters.quickFilter !== 'none') count++;
  if (filters.statuses.size !== 4) count += (4 - filters.statuses.size);
  if (filters.types.size !== 6) count += (6 - filters.types.size);
  return count;
}

// ---------------------------------------------------------------------------
// Dagre layout
// ---------------------------------------------------------------------------

export function applyDagreLayout(
  nodes: ReactFlowNode[],
  edges: ReactFlowEdge[],
  direction: 'TB' | 'LR' = 'TB',
): ReactFlowNode[] {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: direction, nodesep: 80, ranksep: 120, edgesep: 20 });

  for (const node of nodes) {
    g.setNode(node.id, { width: 180, height: 60 });
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target);
  }

  dagre.layout(g);

  return nodes.map((node) => {
    const pos = g.node(node.id);
    return {
      ...node,
      position: { x: pos.x - 90, y: pos.y - 30 },
    };
  });
}

// ---------------------------------------------------------------------------
// Convert backend types to React Flow types
// ---------------------------------------------------------------------------

function nodeTypeToRF(type: NodeType | string): string {
  switch (type) {
    case 'service':
      return 'serviceNode';
    case 'function':
      return 'functionNode';
    case 'cloud_service':
      return 'cloudNode';
    case 'data_flow':
      return 'dataFlowNode';
    default:
      return 'default';
  }
}

function toReactFlowNode(n: GraphNode): ReactFlowNode {
  return {
    id: n.id,
    type: nodeTypeToRF(n.type),
    position: { x: 0, y: 0 },
    data: { ...n },
  };
}

function toReactFlowEdge(e: GraphEdge): ReactFlowEdge {
  return {
    id: e.id || `${e.source_id}-${e.type}-${e.target_id}`,
    source: e.source_id,
    target: e.target_id,
    label: e.type,
    animated: e.type === 'calls' || e.type === 'runtime_calls',
    style: edgeStyle(e.type),
  };
}

function edgeStyle(type: string): React.CSSProperties {
  switch (type) {
    case 'calls':
      return { stroke: '#60a5fa' };
    case 'contains':
      return { stroke: '#6b7280', strokeDasharray: '5 5' };
    case 'depends_on':
      return { stroke: '#f59e0b' };
    case 'connects_to':
      return { stroke: '#8b5cf6' };
    case 'runtime_calls':
      return { stroke: '#34d399' };
    case 'failed_at':
      return { stroke: '#ef4444' };
    case 'produces_to':
      return { stroke: '#a78bfa' };
    case 'consumed_by':
      return { stroke: '#38bdf8' };
    case 'transforms':
      return { stroke: '#2dd4bf' };
    default:
      return { stroke: '#6b7280' };
  }
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export interface UseGraphReturn {
  nodes: ReactFlowNode[];
  edges: ReactFlowEdge[];
  loadServices: () => Promise<void>;
  expandNode: (nodeId: string) => Promise<void>;
  loadSubgraph: (targetId: string, queryType: string) => Promise<void>;
  updateNodeStatus: (nodeId: string, status: string) => void;
  setNodesAndEdges: (nodes: ReactFlowNode[], edges: ReactFlowEdge[]) => void;
  clearError: () => void;
  isLoading: boolean;
  error: string | null;
}

export function useGraph(): UseGraphReturn {
  const [nodes, setNodes] = useState<ReactFlowNode[]>([]);
  const [edges, setEdges] = useState<ReactFlowEdge[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // ---- Load top-level services --------------------------------------------
  const loadServices = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const services = await api.getServices();
      const rfNodes = (services ?? []).map(toReactFlowNode);
      const laid = applyDagreLayout(rfNodes, []);
      setNodes(laid);
      setEdges([]);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, []);

  // ---- Expand a node (load children) --------------------------------------
  const expandNode = useCallback(
    async (nodeId: string) => {
      setIsLoading(true);
      setError(null);
      try {
        const result = await api.getChildren(nodeId, 1);
        const newRfNodes = (result.nodes ?? []).map(toReactFlowNode);
        const newRfEdges = (result.edges ?? []).map(toReactFlowEdge);

        // Merge: add new nodes/edges that don't already exist.
        setNodes((prev) => {
          const existing = new Set(prev.map((n) => n.id));
          const merged = [
            ...prev,
            ...newRfNodes.filter((n) => !existing.has(n.id)),
          ];
          // Merge edges too.
          setEdges((prevEdges) => {
            const existingEdges = new Set(prevEdges.map((e) => e.id));
            const mergedEdges = [
              ...prevEdges,
              ...newRfEdges.filter((e) => !existingEdges.has(e.id)),
            ];
            // Re-layout everything.
            const laid = applyDagreLayout(merged, mergedEdges);
            setEdges(mergedEdges);
            return prevEdges; // setEdges already called above
          });
          return merged;
        });

        // Do a proper layout pass after state settles.
        setTimeout(() => {
          setNodes((cur) => {
            setEdges((curEdges) => {
              const laid = applyDagreLayout(cur, curEdges);
              setNodes(laid);
              return curEdges;
            });
            return cur;
          });
        }, 50);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setIsLoading(false);
      }
    },
    [],
  );

  // ---- Load a focused subgraph --------------------------------------------
  const loadSubgraph = useCallback(
    async (targetId: string, queryType: string) => {
      setIsLoading(true);
      setError(null);
      try {
        const result = await api.getSubgraph(targetId, queryType);
        const rfNodes = (result.nodes ?? []).map(toReactFlowNode);
        const rfEdges = (result.edges ?? []).map(toReactFlowEdge);
        const laid = applyDagreLayout(rfNodes, rfEdges);
        setNodes(laid);
        setEdges(rfEdges);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setIsLoading(false);
      }
    },
    [],
  );

  // ---- Update a single node's status --------------------------------------
  const updateNodeStatus = useCallback(
    (nodeId: string, status: string) => {
      setNodes((prev) =>
        prev.map((n) =>
          n.id === nodeId
            ? {
                ...n,
                data: { ...n.data, runtime_status: status, _statusChanged: Date.now() },
              }
            : n,
        ),
      );

      // Animate edges connected to failing nodes
      if (status === 'error' || status === 'degraded') {
        setEdges((prev) =>
          prev.map((e) =>
            e.source === nodeId || e.target === nodeId
              ? { ...e, animated: true }
              : e,
          ),
        );
      } else if (status === 'healthy') {
        // When a node recovers, stop animating its edges — but only if the
        // *other* endpoint is also healthy.
        setNodes((curNodes) => {
          const failingIds = new Set(
            curNodes
              .filter((n) => {
                const s = (n.data as Record<string, unknown>).runtime_status as string | undefined;
                return s === 'error' || s === 'degraded';
              })
              .map((n) => n.id),
          );
          // The node we just updated is now healthy, so remove it from the set.
          failingIds.delete(nodeId);

          setEdges((prev) =>
            prev.map((e) => {
              if (e.source !== nodeId && e.target !== nodeId) return e;
              const otherId = e.source === nodeId ? e.target : e.source;
              return { ...e, animated: failingIds.has(otherId) };
            }),
          );

          return curNodes; // no mutation needed
        });
      }
    },
    [],
  );

  // ---- Direct setter for external subgraph loads --------------------------
  const setNodesAndEdges = useCallback(
    (newNodes: ReactFlowNode[], newEdges: ReactFlowEdge[]) => {
      const laid = applyDagreLayout(newNodes, newEdges);
      setNodes(laid);
      setEdges(newEdges);
    },
    [],
  );

  // ---- Clear error state --------------------------------------------------
  const clearError = useCallback(() => {
    setError(null);
  }, []);

  return {
    nodes,
    edges,
    loadServices,
    expandNode,
    loadSubgraph,
    updateNodeStatus,
    setNodesAndEdges,
    clearError,
    isLoading,
    error,
  };
}
