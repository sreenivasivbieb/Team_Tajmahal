// ---------------------------------------------------------------------------
// App.tsx — Main application shell (refactored with context providers)
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useMemo, type FC } from 'react';
import GraphCanvas from './components/GraphCanvas';
import StatusBar from './components/StatusBar';
import QueryBar from './components/panels/QueryBar';
import AgentPanel from './components/panels/AgentPanel';
import LogIngestion from './components/panels/LogIngestion';
import ScanSetup from './components/ScanSetup';
import Breadcrumbs from './components/Breadcrumbs';
import ScanDiffBanner from './components/ScanDiffBanner';
import { useGraph } from './hooks/useGraph';
import { useSSE } from './hooks/useSSE';
import { useAgent } from './hooks/useAgent';
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts';
import {
  ScanProvider,
  useScan,
  PanelProvider,
  usePanels,
  NavigationProvider,
  useNavigation,
} from './context';
import { applyDagreLayout } from './hooks/useGraph';
import { nodeTypeToRF } from './utils/nodeMapping';
import type { QueryDecision } from './types/graph';
import type { UseGraphReturn } from './hooks/useGraph';
import type { UseSSEReturn } from './hooks/useSSE';

// ---------------------------------------------------------------------------
// AppInnerWrapper — consumes all contexts, holds main logic
// ---------------------------------------------------------------------------

interface AppInnerWrapperProps {
  graph: UseGraphReturn;
  sse: UseSSEReturn;
}

const AppInnerWrapper: FC<AppInnerWrapperProps> = ({ graph, sse }) => {
  const agent = useAgent();
  const scan = useScan();
  const panels = usePanels();
  const nav = useNavigation();

  // Load services on mount
  useEffect(() => {
    graph.loadServices();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Handle query result from QueryBar
  const handleQueryResult = useCallback(
    (result: QueryDecision, question: string) => {
      switch (result.mode) {
        case 'subgraph': {
          if (result.subgraph) {
            const { nodes: subNodes, edges: subEdges } = result.subgraph;
            const rfNodes = subNodes.map((n) => ({
              id: n.id,
              type: nodeTypeToRF(n.type),
              position: { x: 0, y: 0 },
              data: { ...n },
            }));
            const rfEdges = subEdges.map((e) => ({
              id: e.id || `${e.source_id}-${e.type}-${e.target_id}`,
              source: e.source_id,
              target: e.target_id,
              label: e.type,
              animated: e.type === 'calls' || e.type === 'runtime_calls',
            }));
            const laid = applyDagreLayout(rfNodes, rfEdges, { rankByType: true });
            graph.setNodesAndEdges(laid.nodes, laid.edges);
          } else if (result.target_id && result.subgraph_type) {
            graph.loadSubgraph(result.target_id, result.subgraph_type);
          }
          break;
        }
        case 'agent': {
          panels.openAgentPanel();
          if (!result.agent_run && !result.answer) {
            agent.ask(question);
          }
          break;
        }
        case 'direct_graph': {
          if (result.graph_data?.nodes) {
            const rfNodes = result.graph_data.nodes.map((n) => ({
              id: n.id,
              type: nodeTypeToRF(n.type),
              position: { x: 0, y: 0 },
              data: { ...n },
            }));
            const rfEdges = (result.graph_data.edges ?? []).map((e) => ({
              id: e.id || `${e.source_id}-${e.type}-${e.target_id}`,
              source: e.source_id,
              target: e.target_id,
              label: e.type,
              animated: false,
            }));
            const laid = applyDagreLayout(rfNodes, rfEdges, { rankByType: true });
            graph.setNodesAndEdges(laid.nodes, laid.edges);
          }
          break;
        }
        case 'sql':
        default:
          if (result.answer) {
            panels.openAgentPanel();
          }
          break;
      }
    },
    [graph, agent, panels],
  );

  // Agent panel node click → navigate canvas
  const handleAgentNodeClick = useCallback(
    (nodeId: string) => {
      panels.closeAgentPanel();
      nav.navigateTo(nodeId);
    },
    [panels, nav],
  );

  // Keyboard shortcuts
  const shortcutHandlers = useMemo(
    () => ({
      onFocusQueryBar: () => {
        const input = document.getElementById('vyuha-query-input');
        if (input) {
          (input as HTMLInputElement).focus();
          (input as HTMLInputElement).select();
        }
      },
      onClosePanel: () => panels.closeTopPanel(),
      onRescan: () => scan.handleRescan(),
      onFitView: () => window.dispatchEvent(new CustomEvent('vyuha-fit-view')),
    }),
    [panels, scan],
  );

  useKeyboardShortcuts(shortcutHandlers);

  return (
    <div className="flex h-screen w-screen flex-col bg-gray-950 text-gray-100">
      {/* Scan error banner */}
      {scan.scanError && (
        <div className="flex items-center gap-3 bg-red-900/90 px-4 py-2 text-sm text-red-200">
          <svg className="h-4 w-4 shrink-0 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span className="flex-1">Scan failed: {scan.scanError}</span>
          <button
            onClick={scan.handleRescan}
            className="shrink-0 rounded bg-red-800 px-2.5 py-1 text-xs font-medium text-red-100 transition-colors hover:bg-red-700"
          >
            Retry
          </button>
          <button
            onClick={scan.dismissScanError}
            className="shrink-0 text-red-400 transition-colors hover:text-red-200"
            aria-label="Dismiss"
          >
            ✕
          </button>
        </div>
      )}

      {/* Scan diff banner */}
      {scan.scanDiff && (
        <ScanDiffBanner diff={scan.scanDiff} onDismiss={scan.dismissScanDiff} />
      )}

      {/* Query bar */}
      <QueryBar onResult={handleQueryResult} isRunning={agent.isRunning} />

      {/* Breadcrumbs */}
      <Breadcrumbs currentNode={nav.breadcrumbNode} onNavigate={nav.navigateTo} />

      {/* Main area */}
      <div className="relative flex-1 overflow-hidden">
        <GraphCanvas
          graph={graph}
          sse={sse}
          navigateToNodeId={nav.navigateToNodeId}
          onNavigationComplete={nav.clearNavigation}
          onNodeSelect={nav.selectNode}
        />

        {/* Agent panel */}
        {panels.showAgent && (
          <AgentPanel
            steps={sse.agentSteps}
            agentRun={agent.agentRun}
            isRunning={agent.isRunning}
            onClose={panels.closeAgentPanel}
            onNodeClick={handleAgentNodeClick}
          />
        )}
      </div>

      {/* Log ingestion modal */}
      {panels.showIngestion && <LogIngestion onClose={panels.closeIngestionPanel} />}

      {/* Setup modal */}
      {scan.needsSetup && (
        <ScanSetup
          onScan={scan.doScan}
          onLoadDemo={scan.handleLoadDemo}
          isScanning={scan.isScanning}
          error={scan.scanError}
        />
      )}

      {/* Status bar */}
      <StatusBar
        nodeCount={graph.nodes.length}
        edgeCount={graph.edges.length}
        isConnected={sse.isConnected}
        onRescan={scan.handleRescan}
        onOpenIngestion={panels.openIngestionPanel}
        rfNodes={graph.nodes}
      />
    </div>
  );
};

// ---------------------------------------------------------------------------
// AppWithProviders — sets up context providers, default export
// ---------------------------------------------------------------------------

const AppWithProviders: FC = () => {
  const graph = useGraph();
  const sse = useSSE();

  return (
    <NavigationProvider>
      <ScanProvider graph={graph}>
        <PanelProvider sse={sse}>
          <AppInnerWrapper graph={graph} sse={sse} />
        </PanelProvider>
      </ScanProvider>
    </NavigationProvider>
  );
};

export default AppWithProviders;

