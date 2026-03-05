// ---------------------------------------------------------------------------
// hooks/useGraphLayout.ts — dagre layout computation + tree layout for call chains
// ---------------------------------------------------------------------------

import { useCallback, useRef } from 'react';
import type { Node as ReactFlowNode, Edge as ReactFlowEdge } from 'reactflow';
import dagre from 'dagre';
import type { CallChainMeta } from '../types/graph';

// ---------------------------------------------------------------------------
// Node size helpers — consistent sizes by type
// ---------------------------------------------------------------------------

function getNodeWidth(type: string): number {
  const widths: Record<string, number> = {
    service:          200,
    package:          180,
    file:             180,
    function:         200,
    struct:           180,
    interface:        180,
    cloud_service:    160,
    data_flow:        160,
    repository:       220,
    runtime_instance: 180,
  };
  return widths[type] ?? 180;
}

function getNodeHeight(type: string): number {
  const heights: Record<string, number> = {
    service:          60,
    package:          50,
    file:             50,
    function:         60,
    struct:           55,
    interface:        55,
    cloud_service:    55,
    data_flow:        50,
    repository:       65,
    runtime_instance: 55,
  };
  return heights[type] ?? 50;
}

// ---------------------------------------------------------------------------
// Pure function: applies dagre layout to ReactFlow nodes and edges
// ---------------------------------------------------------------------------

export const applyDagreLayout = (
  nodes: ReactFlowNode[],
  edges: ReactFlowEdge[],
  options: {
    direction?: 'TB' | 'LR';
    nodeSep?: number;
    rankSep?: number;
    rankByType?: boolean;
  } = {},
): { nodes: ReactFlowNode[]; edges: ReactFlowEdge[] } => {
  if (nodes.length === 0) {
    return { nodes, edges };
  }

  const g = new dagre.graphlib.Graph({ compound: false });
  g.setDefaultEdgeLabel(() => ({}));

  g.setGraph({
    rankdir:  options.direction ?? 'TB',
    nodesep:  options.nodeSep   ?? 70,
    ranksep:  options.rankSep   ?? 90,
    marginx:  50,
    marginy:  50,
    ranker:   'network-simplex',
  });

  // Assign rank hints by node type for cleaner hierarchy
  const typeRankHint: Record<string, number> = {
    repository:       0,
    service:          1,
    package:          2,
    file:             3,
    function:         4,
    struct:           4,
    interface:        4,
    data_flow:        5,
    cloud_service:    6,
    runtime_instance: 6,
  };

  nodes.forEach((n) => {
    const nodeType = n.data?.type as string;
    const w = n.width  ?? getNodeWidth(nodeType);
    const h = n.height ?? getNodeHeight(nodeType);

    g.setNode(n.id, {
      width:  w,
      height: h,
      rank: options.rankByType
        ? (typeRankHint[nodeType] ?? 4)
        : undefined,
    });
  });

  // Only add edges that exist between visible nodes
  const nodeIds = new Set(nodes.map((n) => n.id));

  edges.forEach((e) => {
    if (nodeIds.has(e.source) && nodeIds.has(e.target)) {
      if (e.source !== e.target) {
        g.setEdge(e.source, e.target);
      }
    }
  });

  dagre.layout(g);

  const layoutedNodes = nodes.map((n) => {
    const pos = g.node(n.id);
    if (!pos) return n;

    const nodeType = n.data?.type as string;
    return {
      ...n,
      position: {
        x: pos.x - (n.width  ?? getNodeWidth(nodeType))  / 2,
        y: pos.y - (n.height ?? getNodeHeight(nodeType)) / 2,
      },
    };
  });

  return { nodes: layoutedNodes, edges };
};

// ---------------------------------------------------------------------------
// Tree layout for call-chain mode — positions nodes in a top-down tree
// using depths from CallChainMeta
// ---------------------------------------------------------------------------

export const applyTreeLayout = (
  nodes: ReactFlowNode[],
  edges: ReactFlowEdge[],
  meta: CallChainMeta,
): { nodes: ReactFlowNode[]; edges: ReactFlowEdge[] } => {
  if (nodes.length === 0) return { nodes, edges };

  // Group nodes by depth level
  const depthBuckets = new Map<number, ReactFlowNode[]>();
  let maxDepth = 0;

  // Callers (depth -1) vs callees (depth 0+)
  for (const n of nodes) {
    const depth = meta.depths[n.id] ?? -1; // callers default to -1
    const existing = depthBuckets.get(depth) ?? [];
    existing.push(n);
    depthBuckets.set(depth, existing);
    if (depth > maxDepth) maxDepth = depth;
  }

  const NODE_W = 240;
  const NODE_H = 120;
  const H_GAP = 50;
  const V_GAP = 60;

  const layoutedNodes = nodes.map((n) => {
    const depth = meta.depths[n.id] ?? -1;
    const bucket = depthBuckets.get(depth) ?? [n];
    const idx = bucket.indexOf(n);
    const count = bucket.length;

    // Center each row horizontally
    const totalWidth = count * NODE_W + (count - 1) * H_GAP;
    const startX = -totalWidth / 2;

    return {
      ...n,
      position: {
        x: startX + idx * (NODE_W + H_GAP),
        y: (depth + 1) * (NODE_H + V_GAP), // +1 so callers at depth -1 appear at y=0
      },
    };
  });

  return { nodes: layoutedNodes, edges };
};

// ---------------------------------------------------------------------------
// Hook: provides memoized layout computation with caching
// ---------------------------------------------------------------------------

export function useGraphLayout() {
  const cacheRef = useRef<{
    nodeIds: string;
    edgeIds: string;
    result: ReactFlowNode[];
  } | null>(null);

  const computeLayout = useCallback(
    (
      nodes: ReactFlowNode[],
      edges: ReactFlowEdge[],
      direction: 'TB' | 'LR' = 'TB',
    ): ReactFlowNode[] => {
      const nodeIds = nodes.map((n) => n.id).sort().join(',');
      const edgeIds = edges.map((e) => `${e.source}-${e.target}`).sort().join(',');

      if (
        cacheRef.current &&
        cacheRef.current.nodeIds === nodeIds &&
        cacheRef.current.edgeIds === edgeIds
      ) {
        return cacheRef.current.result;
      }

      const { nodes: result } = applyDagreLayout(nodes, edges, { direction });
      cacheRef.current = { nodeIds, edgeIds, result };
      return result;
    },
    [],
  );

  return { computeLayout, applyDagreLayout };
}
