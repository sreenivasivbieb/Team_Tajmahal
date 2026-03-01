// ---------------------------------------------------------------------------
// hooks/useGraph.ts — Graph state management with dagre layout
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'; // CHANGED — added useEffect, useMemo, useRef
import { MarkerType, useViewport } from 'reactflow'; // EDGE STYLE — added MarkerType
import type { Node as ReactFlowNode, Edge as ReactFlowEdge } from 'reactflow';
import { api } from '../api/client';
import type { GraphEdge, GraphNode, NodeType } from '../types/graph';
import { nodeTypeToRF } from '../utils/nodeMapping';
import type {                                                                     // FILTER PIPELINE
  GraphFilters as ControlFilters,                                                 // FILTER PIPELINE
} from '../components/GraphControls';                                             // FILTER PIPELINE
import { applyDagreLayout } from './useGraphLayout';

// ---------------------------------------------------------------------------
// Re-exports — backward-compatible facade for existing importers
// ---------------------------------------------------------------------------
export { applyDagreLayout } from './useGraphLayout';
export {
  applyFilters,
  hasActiveFilters,
  countActiveFilters,
  DEFAULT_FILTERS,
  type GraphFilters,
} from './useGraphFilters';

// ---------------------------------------------------------------------------
// CHANGED — Zoom-aware filtering constants
// ---------------------------------------------------------------------------

const ZOOM_THRESHOLDS = { // CHANGED
  SERVICE:  0.35,         // CHANGED
  PACKAGE:  0.65,         // CHANGED
  FUNCTION: 0.90,         // CHANGED
  DATAFLOW: 1.20,         // CHANGED
} as const;               // CHANGED

// ---------------------------------------------------------------------------
// CHANGED — Pure zoom-filtering helpers
// ---------------------------------------------------------------------------

function getVisibleEdgeTypes(zoom: number): Set<string> { // CHANGED
  if (zoom < ZOOM_THRESHOLDS.SERVICE) {                    // CHANGED
    return new Set(['depends_on', 'connects_to',           // CHANGED
                    'failed_at']);                          // CHANGED
  }                                                        // CHANGED
  if (zoom < ZOOM_THRESHOLDS.PACKAGE) {                    // CHANGED
    return new Set(['depends_on', 'imports',               // CHANGED
                    'connects_to', 'failed_at']);           // CHANGED
  }                                                        // CHANGED
  if (zoom < ZOOM_THRESHOLDS.FUNCTION) {                   // CHANGED
    return new Set(['calls', 'implements',                  // CHANGED
                    'depends_on', 'failed_at',             // CHANGED
                    'produces_to', 'consumed_by']);         // CHANGED
  }                                                        // CHANGED
  return new Set(['calls', 'implements', 'transforms',     // CHANGED
                  'field_map', 'produces_to',              // CHANGED
                  'consumed_by', 'failed_at']);             // CHANGED
}                                                          // CHANGED

function getVisibleNodeTypes(zoom: number): Set<string> {  // CHANGED
  if (zoom < ZOOM_THRESHOLDS.SERVICE) {                    // CHANGED
    return new Set(['service', 'cloud_service',            // CHANGED
                    'runtime_instance']);                   // CHANGED
  }                                                        // CHANGED
  if (zoom < ZOOM_THRESHOLDS.PACKAGE) {                    // CHANGED
    return new Set(['service', 'package', 'file',          // CHANGED
                    'cloud_service']);                      // CHANGED
  }                                                        // CHANGED
  if (zoom < ZOOM_THRESHOLDS.FUNCTION) {                   // CHANGED
    return new Set(['file', 'function', 'struct',          // CHANGED
                    'interface', 'cloud_service']);         // CHANGED
  }                                                        // CHANGED
  return new Set(['function', 'struct', 'interface',       // CHANGED
                  'cloud_service', 'data_flow']);           // CHANGED
}                                                          // CHANGED

function getImportanceThreshold(zoom: number): number {    // CHANGED
  if (zoom < ZOOM_THRESHOLDS.SERVICE)  return 70;          // CHANGED
  if (zoom < ZOOM_THRESHOLDS.PACKAGE)  return 50;          // CHANGED
  if (zoom < ZOOM_THRESHOLDS.FUNCTION) return 20;          // CHANGED
  return 0;                                                // CHANGED
}                                                          // CHANGED

// (Filter types, DEFAULT_FILTERS, applyFilters, hasActiveFilters,
//  countActiveFilters are now in ./useGraphFilters.ts — re-exported above)

// (Layout function applyDagreLayout, getNodeWidth, getNodeHeight are now in\n//  ./useGraphLayout.ts — re-exported above)

function toReactFlowNode(n: GraphNode): ReactFlowNode {
  return {
    id: n.id,
    type: nodeTypeToRF(n.type),
    position: { x: 0, y: 0 },
    data: { ...n },
  };
}

function toReactFlowEdge(e: GraphEdge): ReactFlowEdge {                          // EDGE STYLE
  const importance = e.metadata?.importance ?? 0;                                   // EDGE STYLE
  const styling = edgeStyle(e.type, importance, false);                             // EDGE STYLE

  return {                                                                          // EDGE STYLE
    id: e.id || `${e.source_id}-${e.type}-${e.target_id}`,                          // EDGE STYLE
    source: e.source_id,                                                            // EDGE STYLE
    target: e.target_id,                                                            // EDGE STYLE
    ...styling,                                                                     // EDGE STYLE
    data: { type: e.type, metadata: e.metadata },                                   // EDGE STYLE
  };                                                                                // EDGE STYLE
}

// ---------------------------------------------------------------------------   // EDGE STYLE
// Edge styling — rich per-type visual differentiation                           // EDGE STYLE
// ---------------------------------------------------------------------------   // EDGE STYLE

const edgeStyle = (                                                                // EDGE STYLE
  type: string,                                                                    // EDGE STYLE
  importance: number,                                                              // EDGE STYLE
  hasError: boolean,                                                               // EDGE STYLE
): Partial<ReactFlowEdge> => {                                                     // EDGE STYLE

  const weight = importance > 60 ? 2 :                                             // EDGE STYLE
                 importance > 30 ? 1.5 : 1;                                        // EDGE STYLE

  const styles: Record<string, Partial<ReactFlowEdge>> = {                         // EDGE STYLE

    calls: {                                                                       // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#60A5FA',                                                         // EDGE STYLE
        strokeWidth: weight,                                                       // EDGE STYLE
      },                                                                           // EDGE STYLE
      markerEnd: {                                                                 // EDGE STYLE
        type: MarkerType.ArrowClosed,                                              // EDGE STYLE
        color: '#60A5FA',                                                          // EDGE STYLE
        width: 12,                                                                 // EDGE STYLE
        height: 12,                                                                // EDGE STYLE
      },                                                                           // EDGE STYLE
      type: 'smoothstep',                                                          // EDGE STYLE
    },                                                                             // EDGE STYLE

    depends_on: {                                                                  // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#F59E0B',                                                         // EDGE STYLE
        strokeWidth: 2,                                                            // EDGE STYLE
        strokeDasharray: undefined,                                                // EDGE STYLE
      },                                                                           // EDGE STYLE
      markerEnd: {                                                                 // EDGE STYLE
        type: MarkerType.ArrowClosed,                                              // EDGE STYLE
        color: '#F59E0B',                                                          // EDGE STYLE
      },                                                                           // EDGE STYLE
      type: 'smoothstep',                                                          // EDGE STYLE
      label: importance > 70 ? 'depends' : undefined,                              // EDGE STYLE
    },                                                                             // EDGE STYLE

    implements: {                                                                  // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#A78BFA',                                                         // EDGE STYLE
        strokeWidth: 1.5,                                                          // EDGE STYLE
        strokeDasharray: '6 3',                                                    // EDGE STYLE
      },                                                                           // EDGE STYLE
      markerEnd: {                                                                 // EDGE STYLE
        type: MarkerType.Arrow,                                                    // EDGE STYLE
        color: '#A78BFA',                                                          // EDGE STYLE
      },                                                                           // EDGE STYLE
      type: 'straight',                                                            // EDGE STYLE
    },                                                                             // EDGE STYLE

    imports: {                                                                     // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#4B5563',                                                         // EDGE STYLE
        strokeWidth: 1,                                                            // EDGE STYLE
        strokeDasharray: '3 3',                                                    // EDGE STYLE
        strokeOpacity: 0.6,                                                        // EDGE STYLE
      },                                                                           // EDGE STYLE
      type: 'straight',                                                            // EDGE STYLE
    },                                                                             // EDGE STYLE

    produces_to: {                                                                 // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#34D399',                                                         // EDGE STYLE
        strokeWidth: 2,                                                            // EDGE STYLE
      },                                                                           // EDGE STYLE
      markerEnd: {                                                                 // EDGE STYLE
        type: MarkerType.ArrowClosed,                                              // EDGE STYLE
        color: '#34D399',                                                          // EDGE STYLE
      },                                                                           // EDGE STYLE
      label: '→ queue',                                                            // EDGE STYLE
      labelStyle: {                                                                // EDGE STYLE
        fontSize: 9,                                                               // EDGE STYLE
        fill: '#34D399',                                                           // EDGE STYLE
      },                                                                           // EDGE STYLE
      type: 'smoothstep',                                                          // EDGE STYLE
      animated: true,                                                              // EDGE STYLE
    },                                                                             // EDGE STYLE

    consumed_by: {                                                                 // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#F97316',                                                         // EDGE STYLE
        strokeWidth: 1.5,                                                          // EDGE STYLE
        strokeDasharray: '8 3',                                                    // EDGE STYLE
      },                                                                           // EDGE STYLE
      markerEnd: {                                                                 // EDGE STYLE
        type: MarkerType.Arrow,                                                    // EDGE STYLE
        color: '#F97316',                                                          // EDGE STYLE
      },                                                                           // EDGE STYLE
      type: 'smoothstep',                                                          // EDGE STYLE
    },                                                                             // EDGE STYLE

    failed_at: {                                                                   // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#EF4444',                                                         // EDGE STYLE
        strokeWidth: 2,                                                            // EDGE STYLE
      },                                                                           // EDGE STYLE
      animated: true,                                                              // EDGE STYLE
      markerEnd: {                                                                 // EDGE STYLE
        type: MarkerType.ArrowClosed,                                              // EDGE STYLE
        color: '#EF4444',                                                          // EDGE STYLE
      },                                                                           // EDGE STYLE
    },                                                                             // EDGE STYLE

    transforms: {                                                                  // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        stroke: '#06B6D4',                                                         // EDGE STYLE
        strokeWidth: 1,                                                            // EDGE STYLE
        strokeDasharray: '4 2',                                                    // EDGE STYLE
      },                                                                           // EDGE STYLE
      type: 'straight',                                                            // EDGE STYLE
    },                                                                             // EDGE STYLE
  };                                                                               // EDGE STYLE

  const base = styles[type] ?? {                                                   // EDGE STYLE
    style: {                                                                       // EDGE STYLE
      stroke: '#374151',                                                           // EDGE STYLE
      strokeWidth: 1,                                                              // EDGE STYLE
    },                                                                             // EDGE STYLE
  };                                                                               // EDGE STYLE

  // Error state overrides                                                         // EDGE STYLE
  if (hasError) {                                                                  // EDGE STYLE
    return {                                                                       // EDGE STYLE
      ...base,                                                                     // EDGE STYLE
      style: {                                                                     // EDGE STYLE
        ...base.style,                                                             // EDGE STYLE
        stroke: '#EF4444',                                                         // EDGE STYLE
        strokeOpacity: 0.8,                                                        // EDGE STYLE
      },                                                                           // EDGE STYLE
      animated: true,                                                              // EDGE STYLE
    };                                                                             // EDGE STYLE
  }                                                                                // EDGE STYLE

  return base;                                                                     // EDGE STYLE
};                                                                                 // EDGE STYLE

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export interface UseGraphReturn {
  nodes: ReactFlowNode[];
  edges: ReactFlowEdge[];
  focusedNodeId: string | null;                                    // FOCUS MODE
  setFocusedNodeId: React.Dispatch<React.SetStateAction<string | null>>; // FOCUS MODE
  controlFilters: ControlFilters | null;                             // FILTER PIPELINE
  setControlFilters: React.Dispatch<React.SetStateAction<ControlFilters | null>>; // FILTER PIPELINE
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
  const [rfNodes, setNodes] = useState<ReactFlowNode[]>([]);  // CHANGED — renamed to rfNodes
  const [rfEdges, setEdges] = useState<ReactFlowEdge[]>([]);  // CHANGED — renamed to rfEdges
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const needsLayout = useRef(false); // CHANGED — layout trigger ref
  const [focusedNodeId, setFocusedNodeId] = useState<string | null>(null); // FOCUS MODE
  const [controlFilters, setControlFilters] = useState<ControlFilters | null>(null); // FILTER PIPELINE

  // CHANGED — zoom-aware memos
  const { zoom } = useViewport();                                            // CHANGED
  const visibleEdgeTypes = useMemo(() => getVisibleEdgeTypes(zoom), [zoom]);  // CHANGED
  const visibleNodeTypes = useMemo(() => getVisibleNodeTypes(zoom), [zoom]);  // CHANGED
  const importanceThreshold = useMemo(() => getImportanceThreshold(zoom), [zoom]); // CHANGED

  // CHANGED — filtered nodes: only types visible at current zoom
  const filteredNodes = useMemo(() =>                          // CHANGED
    rfNodes                                                    // CHANGED
      .filter(n => visibleNodeTypes.has(n.data?.type))         // CHANGED
      .map(n => ({                                             // CHANGED
        ...n,                                                  // CHANGED
        data: {                                                // CHANGED
          ...n.data,                                           // CHANGED
          showLabel:   zoom > 0.45,                            // CHANGED
          showDetails: zoom > 0.85,                            // CHANGED
        }                                                      // CHANGED
      }))                                                      // CHANGED
  , [rfNodes, visibleNodeTypes, zoom]);                         // CHANGED

  // CHANGED — filtered edges: only types + importance visible at current zoom
  const filteredEdges = useMemo(() =>                           // CHANGED
    rfEdges.filter(e => {                                      // CHANGED
      const edgeType = e.data?.type as string;                 // CHANGED
      if (!visibleEdgeTypes.has(edgeType)) return false;       // CHANGED
      const importance = e.data?.metadata?.importance ?? 0;    // CHANGED
      return importance >= importanceThreshold;                // CHANGED
    })                                                         // CHANGED
  , [rfEdges, visibleEdgeTypes, importanceThreshold]);          // CHANGED

  // ---- FILTER PIPELINE — user filters on top of zoom filters ----          // FILTER PIPELINE
  const userFilteredNodes = useMemo(() => {                                   // FILTER PIPELINE
    if (!controlFilters) return filteredNodes                                 // FILTER PIPELINE

    return filteredNodes.filter(n => {                                        // FILTER PIPELINE
      const nodeType = n.data?.type as string                                // FILTER PIPELINE

      // Node type filter                                                    // FILTER PIPELINE
      if (!controlFilters.showNodeTypes.has(nodeType)) {                     // FILTER PIPELINE
        return false                                                         // FILTER PIPELINE
      }                                                                      // FILTER PIPELINE

      // Status filter                                                       // FILTER PIPELINE
      if (controlFilters.statusFilter !== 'all') {                           // FILTER PIPELINE
        const status = n.data?.runtime_status as string | undefined          // FILTER PIPELINE
        if (controlFilters.statusFilter === 'failing' &&                     // FILTER PIPELINE
            status !== 'error') return false                                 // FILTER PIPELINE
        if (controlFilters.statusFilter === 'healthy' &&                     // FILTER PIPELINE
            status !== 'healthy' &&                                          // FILTER PIPELINE
            status !== '') return false                                       // FILTER PIPELINE
        if (controlFilters.statusFilter === 'degraded' &&                    // FILTER PIPELINE
            status !== 'degraded') return false                              // FILTER PIPELINE
      }                                                                      // FILTER PIPELINE

      return true                                                            // FILTER PIPELINE
    })                                                                       // FILTER PIPELINE
  }, [filteredNodes, controlFilters])                                         // FILTER PIPELINE

  const userFilteredEdges = useMemo(() => {                                   // FILTER PIPELINE
    if (!controlFilters) return filteredEdges                                 // FILTER PIPELINE

    // Get IDs of visible nodes for edge filtering                           // FILTER PIPELINE
    const visibleNodeIds = new Set(                                           // FILTER PIPELINE
      userFilteredNodes.map(n => n.id))                                      // FILTER PIPELINE

    return filteredEdges.filter(e => {                                        // FILTER PIPELINE
      // Edge type filter                                                    // FILTER PIPELINE
      if (!controlFilters.showEdgeTypes.has(                                 // FILTER PIPELINE
          e.data?.type as string)) return false                              // FILTER PIPELINE

      // Importance filter                                                   // FILTER PIPELINE
      const importance = e.data?.metadata?.importance ?? 0                    // FILTER PIPELINE
      if (importance < controlFilters.minImportance) return false             // FILTER PIPELINE

      // Both endpoints must be visible                                      // FILTER PIPELINE
      if (!visibleNodeIds.has(e.source)) return false                        // FILTER PIPELINE
      if (!visibleNodeIds.has(e.target)) return false                        // FILTER PIPELINE

      return true                                                            // FILTER PIPELINE
    })                                                                       // FILTER PIPELINE
  }, [filteredEdges, userFilteredNodes, controlFilters])                      // FILTER PIPELINE

  // CHANGED — layout effect driven by needsLayout ref
  useEffect(() => {                                            // CHANGED
    if (!needsLayout.current) return;                          // CHANGED
    if (filteredNodes.length === 0) return;                    // CHANGED
    needsLayout.current = false;                                               // CHANGED
    const { nodes: ln, edges: le } = applyDagreLayout(                          // LAYOUT
        userFilteredNodes,                                                      // FILTER PIPELINE
        userFilteredEdges,                                                      // FILTER PIPELINE
        { direction: 'TB', rankByType: true }                                   // LAYOUT
    )                                                                           // LAYOUT
    setNodes(ln);                                                               // LAYOUT
    setEdges(le);                                                               // LAYOUT
  }, [userFilteredNodes.length, userFilteredEdges.length]);                      // FILTER PIPELINE

  // ---- FOCUS MODE — dim everything except focused node + neighbors --------
  const applyFocusMode = useCallback((                                     // FOCUS MODE
    nodes: ReactFlowNode[],                                                // FOCUS MODE
    edges: ReactFlowEdge[],                                                // FOCUS MODE
    focusId: string | null                                                 // FOCUS MODE
  ): { nodes: ReactFlowNode[]; edges: ReactFlowEdge[] } => {              // FOCUS MODE
    if (!focusId) {                                                        // FOCUS MODE
      // No focus — restore full opacity                                   // FOCUS MODE
      return {                                                             // FOCUS MODE
        nodes: nodes.map(n => ({                                           // FOCUS MODE
          ...n,                                                            // FOCUS MODE
          style: { ...n.style, opacity: 1 },                               // FOCUS MODE
        })),                                                               // FOCUS MODE
        edges: edges.map(e => ({                                           // FOCUS MODE
          ...e,                                                            // FOCUS MODE
          style: { ...e.style, opacity: 1 },                               // FOCUS MODE
        })),                                                               // FOCUS MODE
      };                                                                   // FOCUS MODE
    }                                                                      // FOCUS MODE

    // Build neighbor set — one hop in either direction                     // FOCUS MODE
    const neighborIds = new Set<string>([focusId]);                        // FOCUS MODE
    edges.forEach(e => {                                                   // FOCUS MODE
      if (e.source === focusId) neighborIds.add(e.target);                 // FOCUS MODE
      if (e.target === focusId) neighborIds.add(e.source);                 // FOCUS MODE
    });                                                                    // FOCUS MODE

    // Edges where BOTH endpoints are neighbors                            // FOCUS MODE
    const relevantEdgeIds = new Set(                                       // FOCUS MODE
      edges                                                                // FOCUS MODE
        .filter(e =>                                                       // FOCUS MODE
          neighborIds.has(e.source) &&                                     // FOCUS MODE
          neighborIds.has(e.target))                                       // FOCUS MODE
        .map(e => e.id)                                                    // FOCUS MODE
    );                                                                     // FOCUS MODE

    return {                                                               // FOCUS MODE
      nodes: nodes.map(n => ({                                             // FOCUS MODE
        ...n,                                                              // FOCUS MODE
        style: {                                                           // FOCUS MODE
          ...n.style,                                                      // FOCUS MODE
          opacity: neighborIds.has(n.id) ? 1 : 0.06,                       // FOCUS MODE
          transition: 'opacity 0.25s ease',                                // FOCUS MODE
        },                                                                 // FOCUS MODE
        selectable: neighborIds.has(n.id),                                 // FOCUS MODE
        focusable: neighborIds.has(n.id),                                  // FOCUS MODE
      })),                                                                 // FOCUS MODE
      edges: edges.map(e => ({                                             // FOCUS MODE
        ...e,                                                              // FOCUS MODE
        style: {                                                           // FOCUS MODE
          ...e.style,                                                      // FOCUS MODE
          opacity: relevantEdgeIds.has(e.id) ? 1 : 0.03,                   // FOCUS MODE
          transition: 'opacity 0.25s ease',                                // FOCUS MODE
        },                                                                 // FOCUS MODE
        animated: e.data?.type === 'failed_at' &&                          // FOCUS MODE
                  relevantEdgeIds.has(e.id),                               // FOCUS MODE
      })),                                                                 // FOCUS MODE
    };                                                                     // FOCUS MODE
  }, []);                                                                  // FOCUS MODE

  // Apply focus mode AFTER user filtering                                  // FILTER PIPELINE
  const { nodes: focusedNodes, edges: focusedEdges } = useMemo(            // FOCUS MODE
    () => applyFocusMode(userFilteredNodes, userFilteredEdges, focusedNodeId), // FILTER PIPELINE
    [userFilteredNodes, userFilteredEdges, focusedNodeId, applyFocusMode],  // FILTER PIPELINE
  );                                                                       // FOCUS MODE

  // ---- BUNDLE — collapse high fan-out edges into a single bundled edge ---  // BUNDLE
  const bundleHighFanoutEdges = useCallback(                                  // BUNDLE
    (edges: ReactFlowEdge[], threshold: number): ReactFlowEdge[] => {         // BUNDLE
      // Count outgoing edges per source node                                 // BUNDLE
      const outCount = new Map<string, number>();                              // BUNDLE
      edges.forEach(e => outCount.set(e.source, (outCount.get(e.source) ?? 0) + 1)); // BUNDLE

      const bundled: ReactFlowEdge[] = [];                                    // BUNDLE
      const handled = new Set<string>();                                       // BUNDLE

      edges.forEach(e => {                                                    // BUNDLE
        const count = outCount.get(e.source) ?? 0;                            // BUNDLE
        if (count <= threshold) {                                              // BUNDLE
          bundled.push(e);                                                    // BUNDLE
          return;                                                             // BUNDLE
        }                                                                     // BUNDLE
        // High fan-out — replace with one bundled edge per source             // BUNDLE
        const key = `bundle-${e.source}`;                                     // BUNDLE
        if (handled.has(key)) return;                                          // BUNDLE
        handled.add(key);                                                     // BUNDLE

        // Pick the first target so the edge has a valid destination           // BUNDLE
        const targets = edges.filter(x => x.source === e.source);             // BUNDLE
        const firstTarget = targets[0]?.target ?? e.target;                   // BUNDLE

        bundled.push({                                                        // BUNDLE
          id: key,                                                            // BUNDLE
          source: e.source,                                                   // BUNDLE
          target: firstTarget,                                                // BUNDLE
          type: 'bundled',                                                    // BUNDLE
          label: `${count} edges`,                                            // BUNDLE
          data: {                                                             // BUNDLE
            type: 'bundled',                                                  // BUNDLE
            count,                                                            // BUNDLE
            originalEdges: targets,                                           // BUNDLE
          },                                                                  // BUNDLE
          style: { stroke: '#6366f1', strokeWidth: 2.5 },                     // BUNDLE
          animated: false,                                                    // BUNDLE
        } as ReactFlowEdge);                                                  // BUNDLE
      });                                                                     // BUNDLE

      return bundled;                                                         // BUNDLE
    },                                                                        // BUNDLE
    [],                                                                       // BUNDLE
  );                                                                          // BUNDLE

  // BUNDLE — apply bundling after focus mode                                 // BUNDLE
  const finalEdges = useMemo(                                                 // BUNDLE
    () => bundleHighFanoutEdges(focusedEdges, 4),                             // BUNDLE
    [focusedEdges, bundleHighFanoutEdges],                                    // BUNDLE
  );                                                                          // BUNDLE

  // ---- Load top-level services + auto-expand children ----------------------
  const loadServices = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const services = await api.getServices();
      const serviceRfNodes = (services ?? []).map(toReactFlowNode);

      // Auto-expand: fetch children (depth 2) for every service in parallel
      const childResults = await Promise.allSettled(
        serviceRfNodes.map((svc) => api.getChildren(svc.id, 2)),
      );

      let allNodes = [...serviceRfNodes];
      let allEdges: ReactFlowEdge[] = [];
      const seenNodeIds = new Set(serviceRfNodes.map((n) => n.id));
      const seenEdgeIds = new Set<string>();

      for (let i = 0; i < childResults.length; i++) {
        const res = childResults[i];
        if (res.status !== 'fulfilled') continue;
        const { nodes: childNodes, edges: childEdges } = res.value;

        for (const cn of (childNodes ?? []).map(toReactFlowNode)) {
          if (!seenNodeIds.has(cn.id)) {
            allNodes.push(cn);
            seenNodeIds.add(cn.id);
          }
        }

        for (const ce of (childEdges ?? []).map(toReactFlowEdge)) {
          if (!seenEdgeIds.has(ce.id)) {
            allEdges.push(ce);
            seenEdgeIds.add(ce.id);
          }
        }

        // Synthesize "contains" edges from service → direct children
        const parentId = serviceRfNodes[i].id;
        const childEdgeSources = new Set(
          allEdges.filter((e) => e.data?.type === 'contains' || (e as { label?: string }).label === 'contains')
            .map((e) => e.target),
        );
        for (const cn of (childNodes ?? []).map(toReactFlowNode)) {
          if (!childEdgeSources.has(cn.id)) {
            const synId = `${parentId}-contains-${cn.id}`;
            if (!seenEdgeIds.has(synId)) {
              const containsStyling = edgeStyle('contains', 0, false);
              allEdges.push({
                id: synId,
                source: parentId,
                target: cn.id,
                label: 'contains',
                animated: false,
                ...containsStyling,
                data: { type: 'contains', metadata: {} },
              } as ReactFlowEdge);
              seenEdgeIds.add(synId);
            }
          }
        }
      }

      const { nodes: ln, edges: le } = applyDagreLayout(allNodes, allEdges, { rankByType: true }); // LAYOUT
      setNodes(ln);                                                             // LAYOUT
      setEdges(le);
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

        // Also add a synthetic "contains" edge from the parent to each
        // direct child that doesn't already have an incoming contains edge.
        const childEdgeSources = new Set(
          newRfEdges
            .filter((e) => e.label === 'contains')
            .map((e) => e.target),
        );
        const syntheticEdges = newRfNodes
          .filter((n) => !childEdgeSources.has(n.id))
          .map((n) => {
            const containsStyling = edgeStyle('contains', 0, false);              // EDGE STYLE
            return {
              id: `${nodeId}-contains-${n.id}`,
              source: nodeId,
              target: n.id,
              label: 'contains',
              animated: false,
              ...containsStyling,                                                  // EDGE STYLE
              data: { type: 'contains', metadata: {} },                            // EDGE STYLE
            } as ReactFlowEdge;                                                    // EDGE STYLE
          });

        // CHANGED — merge nodes and edges, then trigger layout via ref
        setNodes((prev) => {
          const existingNodes = new Set(prev.map((n) => n.id)); // CHANGED
          return [                                               // CHANGED
            ...prev,                                             // CHANGED
            ...newRfNodes.filter((n) => !existingNodes.has(n.id)), // CHANGED
          ];                                                     // CHANGED
        });                                                      // CHANGED
        setEdges((prevEdges) => {                                // CHANGED
          const existingEdges = new Set(prevEdges.map((e) => e.id)); // CHANGED
          const allNewEdges = [...newRfEdges, ...syntheticEdges]; // CHANGED
          return [                                               // CHANGED
            ...prevEdges,                                        // CHANGED
            ...allNewEdges.filter((e) => !existingEdges.has(e.id)), // CHANGED
          ];                                                     // CHANGED
        });                                                      // CHANGED
        needsLayout.current = true;                              // CHANGED
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
        const { nodes: ln, edges: le } = applyDagreLayout(rfNodes, rfEdges, { rankByType: true }); // LAYOUT
        setNodes(ln);                                                           // LAYOUT
        setEdges(le);                                                           // LAYOUT
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
      const { nodes: ln, edges: le } = applyDagreLayout(newNodes, newEdges, { rankByType: true }); // LAYOUT
      setNodes(ln);                                                             // LAYOUT
      setEdges(le);                                                             // LAYOUT
    },
    [],
  );

  // ---- Clear error state --------------------------------------------------
  const clearError = useCallback(() => {
    setError(null);
  }, []);

  return {
    nodes: focusedNodes,    // FOCUS MODE — return focus-aware nodes
    edges: finalEdges,      // BUNDLE — return bundled edges
    focusedNodeId,          // FOCUS MODE
    setFocusedNodeId,       // FOCUS MODE
    controlFilters,         // FILTER PIPELINE
    setControlFilters,      // FILTER PIPELINE
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
