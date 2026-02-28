// ---------------------------------------------------------------------------
// components/GraphCanvas.tsx — Main React Flow canvas
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useMemo, useState, type FC } from 'react';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  type Node as RFNode,
  type NodeMouseHandler,
  useNodesState,
  useEdgesState,
} from 'reactflow';
import 'reactflow/dist/style.css';

import ServiceNode from './nodes/ServiceNode';
import FunctionNode from './nodes/FunctionNode';
import CloudNode from './nodes/CloudNode';
import DataFlowNode from './nodes/DataFlowNode';
import DetailPanel from './panels/DetailPanel';
import type { GraphNode } from '../types/graph';
import type { UseGraphReturn } from '../hooks/useGraph';
import type { UseSSEReturn } from '../hooks/useSSE';
import {
  applyFilters,
  countActiveFilters,
  DEFAULT_FILTERS,
  hasActiveFilters,
  type GraphFilters,
} from '../hooks/useGraph';
import { api } from '../api/client';

interface GraphCanvasProps {
  graph: UseGraphReturn;
  sse: UseSSEReturn;
}

// ---------------------------------------------------------------------------
// Skeleton positions for the 6 placeholder nodes (dagre-like layout)
// ---------------------------------------------------------------------------
const SKELETON_POSITIONS = [
  { x: 200, y: 40 },
  { x: 60, y: 180 },
  { x: 340, y: 180 },
  { x: 0, y: 320 },
  { x: 200, y: 320 },
  { x: 400, y: 320 },
];

// ---------------------------------------------------------------------------
// Filter constants
// ---------------------------------------------------------------------------
const STATUS_OPTIONS = [
  { key: 'healthy', label: 'Healthy', dot: 'bg-green-400' },
  { key: 'error', label: 'Failing', dot: 'bg-red-400' },
  { key: 'degraded', label: 'Degraded', dot: 'bg-yellow-400' },
  { key: 'unknown', label: 'Unknown', dot: 'bg-gray-400' },
] as const;

const TYPE_OPTIONS = [
  { key: 'service', label: 'Service' },
  { key: 'function', label: 'Function' },
  { key: 'struct', label: 'Struct' },
  { key: 'interface', label: 'Interface' },
  { key: 'cloud_service', label: 'Cloud' },
  { key: 'data_flow', label: 'DataFlow' },
] as const;

const QUICK_FILTERS = [
  { key: 'failing' as const, label: 'Failing only', icon: '🔴' },
  { key: 'cloud-deps' as const, label: 'Cloud deps', icon: '☁' },
  { key: 'entry-points' as const, label: 'Entry points', icon: '→' },
];

// ---------------------------------------------------------------------------
// FilterPanel sub-component
// ---------------------------------------------------------------------------
const FilterPanel: FC<{
  filters: GraphFilters;
  onChange: (f: GraphFilters) => void;
  onReset: () => void;
}> = ({ filters, onChange, onReset }) => {
  const toggleStatus = (key: string) => {
    const next = new Set(filters.statuses);
    next.has(key) ? next.delete(key) : next.add(key);
    onChange({ ...filters, statuses: next, quickFilter: 'none' });
  };

  const toggleType = (key: string) => {
    const next = new Set(filters.types);
    next.has(key) ? next.delete(key) : next.add(key);
    onChange({ ...filters, types: next, quickFilter: 'none' });
  };

  const toggleAllStatuses = () => {
    const allSelected = filters.statuses.size === STATUS_OPTIONS.length;
    onChange({
      ...filters,
      statuses: allSelected
        ? new Set<string>()
        : new Set(STATUS_OPTIONS.map((s) => s.key)),
      quickFilter: 'none',
    });
  };

  const toggleAllTypes = () => {
    const allSelected = filters.types.size === TYPE_OPTIONS.length;
    onChange({
      ...filters,
      types: allSelected
        ? new Set<string>()
        : new Set(TYPE_OPTIONS.map((t) => t.key)),
      quickFilter: 'none',
    });
  };

  const setQuick = (key: GraphFilters['quickFilter']) => {
    onChange({
      ...filters,
      quickFilter: filters.quickFilter === key ? 'none' : key,
    });
  };

  return (
    <div className="w-56 rounded-lg border border-gray-700 bg-gray-900/95 p-3 shadow-xl backdrop-blur text-[11px]">
      {/* Status filter */}
      <div className="mb-2.5">
        <div className="mb-1.5 flex items-center justify-between">
          <span className="font-semibold text-gray-300">Status</span>
          <button
            onClick={toggleAllStatuses}
            className="text-[10px] text-gray-500 hover:text-gray-300"
          >
            {filters.statuses.size === STATUS_OPTIONS.length ? 'None' : 'All'}
          </button>
        </div>
        <div className="space-y-1">
          {STATUS_OPTIONS.map((s) => (
            <label key={s.key} className="flex cursor-pointer items-center gap-1.5 text-gray-400 hover:text-gray-200">
              <input
                type="checkbox"
                checked={filters.statuses.has(s.key)}
                onChange={() => toggleStatus(s.key)}
                className="h-3 w-3 rounded border-gray-600 bg-gray-800 text-blue-500 focus:ring-0 focus:ring-offset-0"
              />
              <span className={`inline-block h-2 w-2 rounded-full ${s.dot}`} />
              {s.label}
            </label>
          ))}
        </div>
      </div>

      {/* Divider */}
      <div className="mb-2.5 border-t border-gray-800" />

      {/* Type filter */}
      <div className="mb-2.5">
        <div className="mb-1.5 flex items-center justify-between">
          <span className="font-semibold text-gray-300">Type</span>
          <button
            onClick={toggleAllTypes}
            className="text-[10px] text-gray-500 hover:text-gray-300"
          >
            {filters.types.size === TYPE_OPTIONS.length ? 'None' : 'All'}
          </button>
        </div>
        <div className="grid grid-cols-2 gap-x-2 gap-y-1">
          {TYPE_OPTIONS.map((t) => (
            <label key={t.key} className="flex cursor-pointer items-center gap-1.5 text-gray-400 hover:text-gray-200">
              <input
                type="checkbox"
                checked={filters.types.has(t.key)}
                onChange={() => toggleType(t.key)}
                className="h-3 w-3 rounded border-gray-600 bg-gray-800 text-blue-500 focus:ring-0 focus:ring-offset-0"
              />
              {t.label}
            </label>
          ))}
        </div>
      </div>

      {/* Divider */}
      <div className="mb-2.5 border-t border-gray-800" />

      {/* Quick filters */}
      <div className="mb-2">
        <span className="mb-1.5 block font-semibold text-gray-300">Quick filters</span>
        <div className="flex flex-wrap gap-1.5">
          {QUICK_FILTERS.map((q) => (
            <button
              key={q.key}
              onClick={() => setQuick(q.key)}
              className={`rounded-full border px-2 py-0.5 transition-colors ${
                filters.quickFilter === q.key
                  ? 'border-blue-500 bg-blue-600/20 text-blue-300'
                  : 'border-gray-700 text-gray-400 hover:border-gray-600 hover:text-gray-300'
              }`}
            >
              {q.icon} {q.label}
            </button>
          ))}
        </div>
      </div>

      {/* Reset */}
      {hasActiveFilters(filters) && (
        <button
          onClick={onReset}
          className="mt-1 w-full rounded bg-gray-800 py-1 text-center text-[10px] text-gray-400 transition-colors hover:bg-gray-700 hover:text-gray-200"
        >
          Reset all filters
        </button>
      )}
    </div>
  );
};

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------
const GraphCanvas: FC<GraphCanvasProps> = ({ graph, sse }) => {
  // Memoised so React Flow never sees a fresh object reference (fixes #002 warning)
  const nodeTypes = useMemo(
    () => ({
      serviceNode: ServiceNode,
      functionNode: FunctionNode,
      cloudNode: CloudNode,
      dataFlowNode: DataFlowNode,
    }),
    [],
  );
  const [rfNodes, setRfNodes, onNodesChange] = useNodesState(graph.nodes);
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState(graph.edges);
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
  const [scanInput, setScanInput] = useState('.');
  const [scanError, setScanError] = useState<string | null>(null);
  const [isScanning, setIsScanning] = useState(false);
  const [filters, setFilters] = useState<GraphFilters>(() => ({ ...DEFAULT_FILTERS }));
  const [filterOpen, setFilterOpen] = useState(false);

  // Sync external graph state → React Flow state (with filters applied)
  useEffect(() => {
    if (hasActiveFilters(filters)) {
      const filtered = applyFilters(graph.nodes, graph.edges, filters);
      setRfNodes(filtered.nodes);
      setRfEdges(filtered.edges);
    } else {
      setRfNodes(graph.nodes);
      setRfEdges(graph.edges);
    }
  }, [graph.nodes, graph.edges, filters, setRfNodes, setRfEdges]);

  // Apply SSE node status updates
  useEffect(() => {
    if (sse.nodeStatusUpdates.size === 0) return;

    sse.nodeStatusUpdates.forEach((status, nodeId) => {
      graph.updateNodeStatus(nodeId, status);
    });
  }, [sse.nodeStatusUpdates, graph]);

  // Node click → open detail panel
  const onNodeClick: NodeMouseHandler = useCallback((_event, node: RFNode) => {
    setSelectedNode(node.data as GraphNode);
  }, []);

  // Background click → close panel
  const onPaneClick = useCallback(() => {
    setSelectedNode(null);
  }, []);

  // Navigate from detail panel to another node
  const handleNavigate = useCallback(
    (nodeId: string) => {
      const found = rfNodes.find((n) => n.id === nodeId);
      if (found) {
        setSelectedNode(found.data as GraphNode);
      }
    },
    [rfNodes],
  );

  // Double-click to expand
  const onNodeDoubleClick: NodeMouseHandler = useCallback(
    (_event, node: RFNode) => {
      graph.expandNode(node.id);
    },
    [graph],
  );

  // MiniMap node color
  const minimapColor = useCallback((node: RFNode) => {
    const status = (node.data as GraphNode)?.runtime_status;
    if (status === 'error') return '#ef4444';
    if (status === 'degraded') return '#f59e0b';
    if (status === 'healthy') return '#22c55e';
    return '#6b7280';
  }, []);

  // Scan handler for empty state
  const handleScan = useCallback(async () => {
    if (!scanInput.trim()) return;
    setIsScanning(true);
    setScanError(null);
    try {
      await api.scan(scanInput.trim());
      // Reload services after scan completes
      setTimeout(() => {
        graph.loadServices();
        setIsScanning(false);
      }, 1000);
    } catch (e) {
      setScanError(e instanceof Error ? e.message : String(e));
      setIsScanning(false);
    }
  }, [scanInput, graph]);

  const handleResetFilters = useCallback(() => {
    setFilters({ ...DEFAULT_FILTERS });
  }, []);

  const activeFilterCount = countActiveFilters(filters);

  // ---- Empty state: no nodes, not loading, no error -----------------------
  const isEmpty = graph.nodes.length === 0 && !graph.isLoading && !graph.error && !isScanning;

  // ---- Scanning / loading skeleton state ----------------------------------
  const showSkeleton = graph.isLoading || isScanning;

  if (isEmpty) {
    return (
      <div className="relative flex h-full w-full items-center justify-center bg-gray-950">
        <div className="flex max-w-md flex-col items-center gap-6 text-center">
          {/* VYUHA Logo */}
          <div className="flex h-20 w-20 items-center justify-center rounded-2xl bg-gradient-to-br from-blue-600 to-purple-600 shadow-lg shadow-blue-500/20">
            <span className="text-3xl font-bold tracking-tight text-white">V</span>
          </div>

          <div>
            <h2 className="text-xl font-semibold text-gray-100">VYUHA AI</h2>
            <p className="mt-1 text-sm text-gray-400">
              Scan a repository to get started
            </p>
          </div>

          {/* Scan input + button */}
          <div className="flex w-full max-w-sm items-center gap-2">
            <input
              type="text"
              value={scanInput}
              onChange={(e) => setScanInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleScan()}
              placeholder="Path to repository…"
              className="flex-1 rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-500 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            />
            <button
              onClick={handleScan}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-500"
            >
              Scan
            </button>
          </div>

          {scanError && (
            <div className="rounded-lg bg-red-900/60 px-4 py-2 text-xs text-red-300">
              {scanError}
            </div>
          )}

          <p className="text-xs text-gray-600">
            Example: <code className="text-gray-500">.</code> for the current directory
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="relative h-full w-full">
      {/* Skeleton loading overlay */}
      {showSkeleton && (
        <div className="absolute inset-0 z-30 bg-gray-950/80">
          {/* Progress bar */}
          <div className="absolute left-0 right-0 top-0 h-1 overflow-hidden bg-gray-800">
            <div className="h-full w-1/3 animate-[shimmer_1.5s_ease-in-out_infinite] bg-gradient-to-r from-transparent via-blue-500 to-transparent" />
          </div>

          {/* Skeleton nodes */}
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="relative" style={{ width: 580, height: 420 }}>
              {SKELETON_POSITIONS.map((pos, i) => (
                <div
                  key={i}
                  className="absolute h-[50px] w-[160px] animate-pulse rounded-lg bg-gray-800 border border-gray-700"
                  style={{
                    left: pos.x,
                    top: pos.y,
                    animationDelay: `${i * 150}ms`,
                  }}
                >
                  <div className="mt-3 mx-3 h-2.5 w-20 rounded bg-gray-700" />
                  <div className="mt-2 mx-3 h-2 w-12 rounded bg-gray-700/60" />
                </div>
              ))}
              {/* Skeleton edges (simple lines connecting rows) */}
              <svg className="absolute inset-0 h-full w-full" style={{ pointerEvents: 'none' }}>
                <line x1={280} y1={90} x2={140} y2={180} className="stroke-gray-700 opacity-40" strokeWidth={2} />
                <line x1={280} y1={90} x2={420} y2={180} className="stroke-gray-700 opacity-40" strokeWidth={2} />
                <line x1={140} y1={230} x2={80} y2={320} className="stroke-gray-700 opacity-40" strokeWidth={2} />
                <line x1={140} y1={230} x2={280} y2={320} className="stroke-gray-700 opacity-40" strokeWidth={2} />
                <line x1={420} y1={230} x2={480} y2={320} className="stroke-gray-700 opacity-40" strokeWidth={2} />
              </svg>
            </div>
          </div>

          {/* Scanning text */}
          <div className="absolute bottom-16 left-1/2 -translate-x-1/2 text-center">
            <p className="text-sm font-medium text-gray-300">Scanning repository…</p>
            <p className="mt-1 text-xs text-gray-500">Analyzing code structure and dependencies</p>
          </div>
        </div>
      )}

      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        onNodeDoubleClick={onNodeDoubleClick}
        onPaneClick={onPaneClick}
        fitView
        minZoom={0.1}
        maxZoom={2}
        defaultEdgeOptions={{ animated: false }}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#374151" gap={20} size={1} />
        <Controls
          position="bottom-left"
          className="!border-gray-700 !bg-gray-800 [&>button]:!border-gray-700 [&>button]:!bg-gray-800 [&>button]:!text-gray-300 [&>button:hover]:!bg-gray-700"
        />
        <MiniMap
          nodeColor={minimapColor}
          maskColor="rgba(0, 0, 0, 0.6)"
          className="!border-gray-700 !bg-gray-900"
          position="bottom-right"
        />
      </ReactFlow>

      {/* ---- Filter toggle button (top-right) ---- */}
      <div className="absolute right-4 top-4 z-40">
        <button
          onClick={() => setFilterOpen((p) => !p)}
          className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium shadow-lg backdrop-blur transition-colors ${
            filterOpen || activeFilterCount > 0
              ? 'border-blue-500 bg-blue-600/20 text-blue-300'
              : 'border-gray-700 bg-gray-900/90 text-gray-400 hover:border-gray-600 hover:text-gray-300'
          }`}
        >
          {/* Funnel icon */}
          <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
          </svg>
          Filters
          {activeFilterCount > 0 && (
            <span className="flex h-4 min-w-[16px] items-center justify-center rounded-full bg-blue-500 px-1 text-[10px] font-bold text-white">
              {activeFilterCount}
            </span>
          )}
        </button>

        {/* Filter dropdown panel */}
        {filterOpen && (
          <div className="absolute right-0 top-10 animate-fade-in">
            <FilterPanel
              filters={filters}
              onChange={setFilters}
              onReset={handleResetFilters}
            />
          </div>
        )}
      </div>

      {/* Error banner (dismissible + retry) */}
      {graph.error && (
        <div className="absolute left-4 right-64 top-4 z-40 flex items-center gap-3 rounded-lg bg-red-900/90 px-4 py-2.5 text-sm text-red-200 shadow-lg backdrop-blur">
          <svg className="h-4 w-4 shrink-0 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span className="flex-1 truncate">{graph.error}</span>
          <button
            onClick={() => graph.loadServices()}
            className="shrink-0 rounded bg-red-800 px-2.5 py-1 text-xs font-medium text-red-100 transition-colors hover:bg-red-700"
          >
            Retry
          </button>
          <button
            onClick={() => graph.clearError()}
            className="shrink-0 text-red-400 transition-colors hover:text-red-200"
            aria-label="Dismiss error"
          >
            ✕
          </button>
        </div>
      )}

      {/* Detail panel */}
      {selectedNode && (
        <DetailPanel
          node={selectedNode}
          onClose={() => setSelectedNode(null)}
          onNavigate={handleNavigate}
          onNodeHighlight={(nodeId) => {
            // Briefly pulse the referenced node on the canvas
            window.dispatchEvent(
              new CustomEvent('vyuha:node_error', {
                detail: { nodeId, status: 'degraded' },
              }),
            );
          }}
        />
      )}
    </div>
  );
};

export default GraphCanvas;
