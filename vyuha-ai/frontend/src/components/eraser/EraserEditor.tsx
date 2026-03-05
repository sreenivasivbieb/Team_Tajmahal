// ---------------------------------------------------------------------------
// eraser/EraserEditor.tsx — Eraser.io-style split editor page
//   Top:   File name + "Document | Both | Canvas" tabs + share actions
//   Left:  Document panel (markdown notes + AI answer)
//   Right: Canvas (React Flow diagram + toolbar + AI Diagram card)
// ---------------------------------------------------------------------------

import {
  memo,
  useCallback,
  useEffect,
  useRef,
  useState,
  type FC,
} from 'react';
import {
  Sparkles,
  Loader2,
  ChevronLeft,
} from 'lucide-react';
import { ReactFlowProvider } from 'reactflow';

import GraphCanvas from '../GraphCanvas';
import DocumentPanel from './DocumentPanel';
import CanvasToolbar, { type CanvasTool } from './CanvasToolbar';
import AIDiagramCard from './AIDiagramCard';
import DrawingOverlay from './DrawingOverlay';
import QueryBar, { type QueryResult } from '../panels/QueryBar';
import { api } from '../../api/client';

import type { UseGraphReturn } from '../../hooks/useGraph';
import type { UseSSEReturn } from '../../hooks/useSSE';

// ---------------------------------------------------------------------------
// Panel mode
// ---------------------------------------------------------------------------

type PanelMode = 'document' | 'both' | 'canvas';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface EraserEditorProps {
  repoName: string;
  repoPath: string;
  graph: UseGraphReturn;
  sse: UseSSEReturn;
  onBack: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const EraserEditor: FC<EraserEditorProps> = ({ repoName, repoPath, graph, sse, onBack }) => {
  const [panelMode, setPanelMode] = useState<PanelMode>('both');
  const [docContent, setDocContent] = useState('');
  const [aiAnswer, setAiAnswer] = useState<string | null>(null);
  const [canvasTool, setCanvasTool] = useState<CanvasTool>('pointer');
  const [isGenerating, setIsGenerating] = useState(false);
  const [showQueryBar, setShowQueryBar] = useState(false);
  const [splitPercent, setSplitPercent] = useState(50);

  // ---- Resizable splitter state ──────────────────────────────────
  const isDragging = useRef(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const handleSplitterMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    isDragging.current = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }, []);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current || !containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      const pct = ((e.clientX - rect.left) / rect.width) * 100;
      setSplitPercent(Math.min(80, Math.max(20, pct)));
    };
    const handleMouseUp = () => {
      if (isDragging.current) {
        isDragging.current = false;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
      }
    };
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);
    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
    };
  }, []);

  // ---- AI Diagram generation (from card or toolbar AI button) -----
  const handleAIDiagramGenerate = useCallback(
    async (prompt: string) => {
      setIsGenerating(true);
      setAiAnswer(null);
      try {
        // First try to detect if prompt is asking for a call chain
        const isCallChainRequest = /call\s*chain|flow\s*(of|for)|trace|entry\s*point/i.test(prompt);

        if (isCallChainRequest) {
          // Extract symbol name from prompt or use a default
          const symbolMatch = prompt.match(/(?:for|of|trace)\s+[`"]?(\w+)[`"]?/i);
          const symbol = symbolMatch?.[1] ?? 'main';
          const data = await api.callChain(symbol);
          graph.setCallChainFromResponse(data.nodes, data.edges, data.call_chain_meta);
          setAiAnswer(`Generated call chain diagram for \`${symbol}\`.\n\nFound ${data.nodes.length} functions across ${data.call_chain_meta.stats.total_hops} call depth levels.`);
        } else {
          // Use RAG for architecture / general questions — target the specific repo
          const result = await api.ragQuery(
            `Based on analyzing this codebase, ${prompt}. Provide a structured analysis listing the main components, their relationships, and data flow. Format your response with clear sections.`,
            repoPath || undefined,
          );
          setAiAnswer(result.answer);

          // Also try to generate a visual: get context tree and use it to build nodes
          try {
            const tree = await api.contextTree(repoPath || undefined);
            // Parse the tree output to build architecture nodes
            const archNodes = parseTreeToArchitectureNodes(tree.result);
            if (archNodes.nodes.length > 0) {
              graph.setCallChainFromResponse(
                archNodes.nodes,
                archNodes.edges,
                archNodes.meta,
              );
            }
          } catch {
            // If context tree fails, the AI answer is still shown
          }
        }
      } catch (err) {
        console.error('AI diagram error:', err);
        setAiAnswer(`Error generating diagram: ${err instanceof Error ? err.message : 'Unknown error'}`);
      } finally {
        setIsGenerating(false);
      }
    },
    [graph, repoPath],
  );

  // ---- Handle query bar results ──────────────────────────────────
  const handleQueryResult = useCallback(
    (result: QueryResult) => {
      switch (result.kind) {
        case 'call_chain': {
          const { nodes, edges, call_chain_meta } = result.data;
          graph.setCallChainFromResponse(nodes, edges, call_chain_meta);
          break;
        }
        case 'text': {
          graph.setTextResult(result.tool, result.data.result);
          break;
        }
        case 'rag_answer': {
          setAiAnswer(result.data.answer);
          break;
        }
      }
    },
    [graph],
  );

  // ---- Generate outline shortcut ─────────────────────────────────
  const handleGenerateOutline = useCallback(async () => {
    setIsGenerating(true);
    try {
      const result = await api.ragQuery(
        'Generate a concise architectural outline of this codebase. List the main modules, their responsibilities, and key entry points.',
        repoPath || undefined,
      );
      setAiAnswer(result.answer);
      setDocContent(result.answer);
    } catch (err) {
      console.error('Outline error:', err);
    } finally {
      setIsGenerating(false);
    }
  }, [repoPath]);

  const showDocument = panelMode === 'document' || panelMode === 'both';
  const showCanvas = panelMode === 'canvas' || panelMode === 'both';
  const hasContent = graph.nodes.length > 0 || (graph.canvasMode === 'text_result' && graph.textResult);

  return (
    <div className="flex h-full flex-col bg-[#0b0e14]">
      {/* ── Top bar ────────────────────────────────────────────── */}
      <div className="flex h-11 items-center justify-between border-b border-gray-800 px-4">
        {/* Left: back + file name */}
        <div className="flex items-center gap-3">
          <button
            onClick={onBack}
            className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-gray-400 transition-colors hover:bg-gray-800 hover:text-gray-200"
          >
            <ChevronLeft size={14} />
            Back
          </button>
          <div className="flex items-center gap-2">
            <div className="flex h-5 w-5 items-center justify-center rounded bg-gradient-to-br from-red-500 to-blue-500">
              <span className="text-[9px] font-bold text-white">V</span>
            </div>
            <span className="text-sm font-medium text-gray-200">
              {repoName || 'Untitled File'}
            </span>
            <span className="text-gray-600">···</span>
          </div>
        </div>

        {/* Center: Document | Both | Canvas tabs */}
        <div className="flex items-center rounded-lg border border-gray-700/50 bg-gray-900/60">
          {(['document', 'both', 'canvas'] as const).map((mode) => (
            <button
              key={mode}
              onClick={() => setPanelMode(mode)}
              className={`px-4 py-1.5 text-xs font-medium transition-colors ${
                panelMode === mode
                  ? 'bg-gray-700/60 text-gray-100'
                  : 'text-gray-500 hover:text-gray-300'
              } ${mode === 'document' ? 'rounded-l-md' : ''} ${mode === 'canvas' ? 'rounded-r-md' : ''}`}
            >
              {mode.charAt(0).toUpperCase() + mode.slice(1)}
            </button>
          ))}
        </div>

        {/* Right: actions */}
        <div className="flex items-center gap-2">
          <kbd className="rounded border border-gray-700 bg-gray-800 px-2 py-0.5 text-[10px] text-gray-500">
            Ctrl K
          </kbd>
        </div>
      </div>

      {/* ── Main content area ──────────────────────────────────── */}
      <div ref={containerRef} className="flex flex-1 overflow-hidden">
        {/* Document panel */}
        {showDocument && (
          <div
            className="hide-scrollbar overflow-auto border-r border-gray-800"
            style={{ width: showCanvas ? `${splitPercent}%` : '100%' }}
          >
            <DocumentPanel
              title={repoName}
              content={docContent}
              onContentChange={setDocContent}
              aiAnswer={aiAnswer}
            />

            {/* Generate Outline button (shown when no AI answer yet) */}
            {!aiAnswer && !isGenerating && (
              <div className="flex justify-center pb-8">
                <button
                  onClick={handleGenerateOutline}
                  className="flex items-center gap-2 rounded-lg border border-gray-700/50 bg-gray-900/60 px-4 py-2 text-sm text-gray-300 transition-all hover:border-gray-600 hover:bg-gray-800/70 hover:text-gray-100"
                >
                  <Sparkles size={14} className="text-purple-400" />
                  Generate Outline
                  <kbd className="ml-2 rounded border border-gray-700 bg-gray-800 px-1.5 py-0.5 text-[9px] text-gray-500">
                    Ctrl⇧ J
                  </kbd>
                </button>
              </div>
            )}
            {isGenerating && (
              <div className="flex justify-center pb-8">
                <div className="flex items-center gap-2 text-sm text-purple-400">
                  <Loader2 size={14} className="animate-spin" />
                  Generating…
                </div>
              </div>
            )}
          </div>
        )}

        {/* Resizable splitter handle */}
        {showDocument && showCanvas && (
          <div
            onMouseDown={handleSplitterMouseDown}
            className="group relative z-10 flex w-1.5 shrink-0 cursor-col-resize items-center justify-center hover:bg-blue-500/10 active:bg-blue-500/20 transition-colors"
          >
            <div className="h-8 w-0.5 rounded-full bg-gray-700 transition-colors group-hover:bg-blue-400/60 group-active:bg-blue-400" />
          </div>
        )}

        {/* Canvas panel */}
        {showCanvas && (
          <div
            className="relative"
            style={{ width: showDocument ? `${100 - splitPercent}%` : '100%' }}
          >
            {/* Query bar toggle */}
            {showQueryBar && (
              <div className="absolute inset-x-0 top-0 z-20">
                <QueryBar onResult={handleQueryResult} repoPath={repoPath} />
              </div>
            )}

            {/* Canvas content */}
            <div className="h-full w-full">
              {hasContent ? (
                /* Show graph or text result */
                graph.canvasMode === 'text_result' && graph.textResult ? (
                  <div className="flex h-full flex-col bg-gray-950">
                    <div className="flex items-center justify-between border-b border-gray-700 px-4 py-2">
                      <span className="text-sm font-semibold text-gray-300">
                        {(graph.textTool ?? '').replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())} Result
                      </span>
                      <button
                        onClick={graph.clearAll}
                        className="text-gray-500 hover:text-gray-300 transition-colors"
                      >
                        ✕
                      </button>
                    </div>
                    <pre className="flex-1 overflow-auto whitespace-pre-wrap p-4 font-mono text-xs text-gray-300 leading-relaxed">
                      {graph.textResult}
                    </pre>
                  </div>
                ) : (
                  <ReactFlowProvider>
                    <GraphCanvas graph={graph} sse={sse} />
                  </ReactFlowProvider>
                )
              ) : (
                /* Empty canvas: show AI Diagram card */
                <div className="flex h-full w-full items-center justify-center bg-gray-950 px-12 pr-14">
                  <AIDiagramCard
                    onGenerate={handleAIDiagramGenerate}
                    isGenerating={isGenerating}
                  />
                </div>
              )}
            </div>

            {/* Drawing overlay — active when a shape tool is selected */}
            <DrawingOverlay
              activeTool={canvasTool}
              onToolReset={() => setCanvasTool('pointer')}
            />

            {/* Canvas toolbar */}
            <CanvasToolbar
              activeTool={canvasTool}
              onToolChange={setCanvasTool}
              onAdd={() => setShowQueryBar((p) => !p)}
              onAI={() => {
                graph.clearAll();
                setAiAnswer(null);
              }}
            />

            {/* Bottom bar: size + Create Figure */}
            <div className="absolute inset-x-0 bottom-0 z-20 flex items-center justify-between border-t border-gray-800 bg-gray-900/80 px-4 py-1.5 backdrop-blur">
              <span className="text-[11px] text-gray-500">
                {graph.nodes.length > 0
                  ? `${graph.nodes.length} nodes · ${graph.edges.length} edges`
                  : 'Empty canvas'}
              </span>
              <div className="flex items-center gap-2">
                <span className="rounded border border-gray-700/50 px-2 py-0.5 text-[11px] text-gray-500">
                  Small ▾
                </span>
                <button className="flex items-center gap-1.5 rounded-lg border border-gray-700/50 bg-gray-800 px-3 py-1 text-xs text-gray-300 transition-colors hover:bg-gray-700">
                  <svg width="14" height="14" viewBox="0 0 16 16" className="text-gray-400">
                    <rect x="2" y="2" width="12" height="12" rx="2" fill="none" stroke="currentColor" strokeWidth="1.2" />
                    <line x1="8" y1="5" x2="8" y2="11" stroke="currentColor" strokeWidth="1.2" />
                    <line x1="5" y1="8" x2="11" y2="8" stroke="currentColor" strokeWidth="1.2" />
                  </svg>
                  Create Figure
                  <kbd className="rounded border border-gray-700 bg-gray-800/80 px-1 text-[9px] text-gray-600">⇧ F</kbd>
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// Parse context-tree text output into architecture diagram nodes
// ---------------------------------------------------------------------------

import type { FlatNode, FlatEdge, CallChainMeta } from '../../types/graph';

interface ArchResult {
  nodes: FlatNode[];
  edges: FlatEdge[];
  meta: CallChainMeta;
}

function parseTreeToArchitectureNodes(treeText: string): ArchResult {
  const lines = treeText.split('\n').filter((l) => l.trim().length > 0);
  const nodes: FlatNode[] = [];
  const edges: FlatEdge[] = [];
  const depths: Record<string, number> = {};
  const annotations: Record<string, string[]> = {};
  const nodeRoles: Record<string, string> = {};

  // Parse indented tree structure into flat nodes
  // Format typically: "  src/api/handlers.go" or "    func handleCallChain()"
  let rootId = '';
  const stack: { id: string; indent: number }[] = [];

  for (const line of lines) {
    const trimmed = line.replace(/^[│├└─\s]+/, '');
    if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('---')) continue;

    const indent = line.length - line.trimStart().length;

    // Determine type
    let type: 'file' | 'function' | 'package' = 'file';
    let name = trimmed;

    if (/\.(ts|js|go|py|rs|java|tsx|jsx|css|html|md)$/.test(trimmed)) {
      type = 'file';
    } else if (/^(func|function|def|class|interface|type|struct|export)\s/.test(trimmed)) {
      type = 'function';
      name = trimmed.replace(/^(func|function|def|class|interface|type|struct|export)\s+/, '').split('(')[0].trim();
    } else if (trimmed.endsWith('/') || /^[a-z]/.test(trimmed) && !trimmed.includes('.')) {
      type = 'package';
    }

    const id = `arch:${type}:${name}:${nodes.length}`;

    if (nodes.length === 0) rootId = id;

    nodes.push({
      id,
      name,
      file: type === 'file' ? trimmed : '',
      line: 0,
      end_line: 0,
      signature: '',
      is_external: false,
    });

    // Determine depth for tree layout
    const depth = Math.floor(indent / 2);
    depths[id] = depth;
    annotations[id] = [type];
    nodeRoles[id] = depth === 0 ? 'target' : 'callee';

    // Connect to parent based on indentation
    while (stack.length > 0 && stack[stack.length - 1].indent >= indent) {
      stack.pop();
    }
    if (stack.length > 0) {
      const parentId = stack[stack.length - 1].id;
      edges.push({
        id: `${parentId}->${id}`,
        source: parentId,
        target: id,
      });
    }
    stack.push({ id, indent });

    // Limit to avoid overwhelming canvas
    if (nodes.length >= 50) break;
  }

  // Compute stats
  let maxDepth = 0;
  for (const d of Object.values(depths)) {
    if (d > maxDepth) maxDepth = d;
  }

  return {
    nodes,
    edges,
    meta: {
      root_id: rootId,
      depths,
      annotations,
      node_roles: nodeRoles,
      stats: {
        total_hops: maxDepth,
        leaf_count: nodes.filter((n) => !edges.some((e) => e.source === n.id)).length,
        external_count: 0,
        max_depth: maxDepth,
      },
    },
  };
}

export default memo(EraserEditor);
