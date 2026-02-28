// ---------------------------------------------------------------------------
// App.tsx — Main application shell
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useState, type FC } from 'react';
import GraphCanvas from './components/GraphCanvas';
import StatusBar from './components/StatusBar';
import QueryBar from './components/panels/QueryBar';
import AgentPanel from './components/panels/AgentPanel';
import LogIngestion from './components/panels/LogIngestion';
import { useGraph } from './hooks/useGraph';
import { useSSE } from './hooks/useSSE';
import { useAgent } from './hooks/useAgent';
import { api } from './api/client';
import type { QueryDecision } from './types/graph';
import { applyDagreLayout } from './hooks/useGraph';

const App: FC = () => {
  const graph = useGraph();
  const sse = useSSE();
  const agent = useAgent();

  const [showAgent, setShowAgent] = useState(false);
  const [showIngestion, setShowIngestion] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);

  // Load services on mount
  useEffect(() => {
    graph.loadServices();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Handle query result from QueryBar
  const handleQueryResult = useCallback(
    (result: QueryDecision) => {
      switch (result.mode) {
        case 'subgraph': {
          // Display the returned subgraph on the canvas
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
            const laid = applyDagreLayout(rfNodes, rfEdges);
            graph.setNodesAndEdges(laid, rfEdges);
          } else if (result.target_id && result.subgraph_type) {
            graph.loadSubgraph(result.target_id, result.subgraph_type);
          }
          break;
        }

        case 'agent': {
          // Open agent panel
          setShowAgent(true);
          break;
        }

        case 'direct_graph': {
          // Display graph_data if present
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
            const laid = applyDagreLayout(rfNodes, rfEdges);
            graph.setNodesAndEdges(laid, rfEdges);
          }
          break;
        }

        case 'sql':
        default:
          // For direct answer modes, show agent panel with the answer
          if (result.answer) {
            setShowAgent(true);
          }
          break;
      }
    },
    [graph],
  );

  // Rescan handler
  const handleRescan = useCallback(async () => {
    setScanError(null);
    try {
      await api.scan('.');
      // Reload services after scan completes
      setTimeout(() => graph.loadServices(), 1000);
    } catch (e) {
      setScanError(e instanceof Error ? e.message : String(e));
    }
  }, [graph]);

  // Navigate to node from agent panel
  const handleAgentNodeClick = useCallback(
    (nodeId: string) => {
      // Find the node on the canvas and select it
      const found = graph.nodes.find((n) => n.id === nodeId);
      if (found) {
        // close agent panel and let the canvas handle focus
        setShowAgent(false);
      }
    },
    [graph.nodes],
  );

  return (
    <div className="flex h-screen w-screen flex-col bg-gray-950 text-gray-100">
      {/* Scan error banner */}
      {scanError && (
        <div className="flex items-center gap-3 bg-red-900/90 px-4 py-2 text-sm text-red-200">
          <svg className="h-4 w-4 shrink-0 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span className="flex-1">Scan failed: {scanError}</span>
          <button
            onClick={handleRescan}
            className="shrink-0 rounded bg-red-800 px-2.5 py-1 text-xs font-medium text-red-100 transition-colors hover:bg-red-700"
          >
            Retry
          </button>
          <button
            onClick={() => setScanError(null)}
            className="shrink-0 text-red-400 transition-colors hover:text-red-200"
            aria-label="Dismiss"
          >
            ✕
          </button>
        </div>
      )}

      {/* Query bar */}
      <QueryBar onResult={handleQueryResult} isRunning={agent.isRunning} />

      {/* Main area */}
      <div className="relative flex-1 overflow-hidden">
        <GraphCanvas graph={graph} sse={sse} />

        {/* Agent panel (left) */}
        {showAgent && (
          <AgentPanel
            steps={sse.agentSteps}
            agentRun={agent.agentRun}
            isRunning={agent.isRunning}
            onClose={() => {
              setShowAgent(false);
              sse.clearAgentSteps();
            }}
            onNodeClick={handleAgentNodeClick}
          />
        )}
      </div>

      {/* Log ingestion modal */}
      {showIngestion && <LogIngestion onClose={() => setShowIngestion(false)} />}

      {/* Status bar */}
      <StatusBar
        nodeCount={graph.nodes.length}
        edgeCount={graph.edges.length}
        isConnected={sse.isConnected}
        onRescan={handleRescan}
        onOpenIngestion={() => setShowIngestion(true)}
        rfNodes={graph.nodes}
      />
    </div>
  );
};

export default App;

// ---------------------------------------------------------------------------
// Helper — duplicated from useGraph for use in App-level conversion
// ---------------------------------------------------------------------------

function nodeTypeToRF(type: string): string {
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
