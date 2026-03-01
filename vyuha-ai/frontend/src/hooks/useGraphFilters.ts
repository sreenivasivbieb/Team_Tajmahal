// ---------------------------------------------------------------------------
// hooks/useGraphFilters.ts — Filter state and application logic
// ---------------------------------------------------------------------------

import { useCallback, useState } from 'react';
import type { Node as ReactFlowNode, Edge as ReactFlowEdge } from 'reactflow';
import type { GraphNode } from '../types/graph';

export interface GraphFilters {
  statuses: Set<string>;
  types: Set<string>;
  quickFilter: 'none' | 'failing' | 'cloud-deps' | 'entry-points';
}

export const DEFAULT_FILTERS: GraphFilters = {
  statuses: new Set(['healthy', 'degraded', 'error', 'unknown']),
  types: new Set(['service', 'package', 'file', 'function', 'struct', 'interface', 'cloud_service', 'data_flow']),
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

  switch (filters.quickFilter) {
    case 'failing':
      return status === 'error';
    case 'cloud-deps': {
      if (type === 'cloud_service') return true;
      return false;
    }
    case 'entry-points': {
      const hasIncoming = edges.some((e) => e.target === node.id);
      return !hasIncoming;
    }
    default:
      break;
  }

  if (!filters.statuses.has(status)) return false;
  if (!filters.types.has(type)) return false;
  return true;
}

export function applyFilters(
  nodes: ReactFlowNode[],
  edges: ReactFlowEdge[],
  filters: GraphFilters,
): { nodes: ReactFlowNode[]; edges: ReactFlowEdge[] } {
  let matchingIds: Set<string>;

  if (filters.quickFilter === 'cloud-deps') {
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

export interface UseGraphFiltersReturn {
  filters: GraphFilters;
  setFilters: (f: GraphFilters) => void;
  resetFilters: () => void;
  hasActive: boolean;
  activeCount: number;
  apply: (
    nodes: ReactFlowNode[],
    edges: ReactFlowEdge[],
  ) => { nodes: ReactFlowNode[]; edges: ReactFlowEdge[] };
}

export function useGraphFilters(): UseGraphFiltersReturn {
  const [filters, setFilters] = useState<GraphFilters>(DEFAULT_FILTERS);

  const resetFilters = useCallback(() => setFilters(DEFAULT_FILTERS), []);

  const hasActive = hasActiveFilters(filters);
  const activeCount = countActiveFilters(filters);

  const apply = useCallback(
    (nodes: ReactFlowNode[], edges: ReactFlowEdge[]) =>
      applyFilters(nodes, edges, filters),
    [filters],
  );

  return { filters, setFilters, resetFilters, hasActive, activeCount, apply };
}
