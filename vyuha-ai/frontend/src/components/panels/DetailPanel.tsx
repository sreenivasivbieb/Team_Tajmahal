// ---------------------------------------------------------------------------
// panels/DetailPanel.tsx — Slide-in detail panel for selected nodes
// SHADCN: replaced Section/Badge/ScrollArea/Separator/Button with shadcn/ui
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useRef, useState, type FC } from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism';
import type { GraphNode, NodeDetail, RuntimeEvent } from '../../types/graph';
import { api } from '../../api/client';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '../ui/collapsible';
import { Badge } from '../ui/badge';
import { Button } from '../ui/button';
import { Separator } from '../ui/separator';
import { ScrollArea } from '../ui/scroll-area';

interface DetailPanelProps {
  node: GraphNode;
  onClose: () => void;
  onNavigate: (nodeId: string) => void;
  onNodeHighlight?: (nodeId: string) => void;
  onNodeSelect?: (node: GraphNode) => void;                                       // INTERCONNECTIONS
  onFocusNode?: (nodeId: string) => void;                                         // INTERCONNECTIONS
}

// ---------------------------------------------------------------------------  // INTERCONNECTIONS
// Connection grouping for structured Interconnections view                     // INTERCONNECTIONS
// ---------------------------------------------------------------------------  // INTERCONNECTIONS

interface ConnectionGroup {                                                       // INTERCONNECTIONS
  label:    string                                                                // INTERCONNECTIONS
  nodes:    GraphNode[]                                                           // INTERCONNECTIONS
  edgeType: string                                                                // INTERCONNECTIONS
  color:    string                                                                // INTERCONNECTIONS
  icon:     string                                                                // INTERCONNECTIONS
}                                                                                 // INTERCONNECTIONS

const buildConnectionGroups = (                                                   // INTERCONNECTIONS
  node: GraphNode,                                                                // INTERCONNECTIONS
  nodeDetail: NodeDetail | null                                                   // INTERCONNECTIONS
): ConnectionGroup[] => {                                                         // INTERCONNECTIONS
  if (!nodeDetail) return []                                                      // INTERCONNECTIONS

  return [                                                                        // INTERCONNECTIONS
    {                                                                             // INTERCONNECTIONS
      label:    'Called By',                                                      // INTERCONNECTIONS
      nodes:    nodeDetail.callers ?? [],                                         // INTERCONNECTIONS
      edgeType: 'calls',                                                          // INTERCONNECTIONS
      color:    'text-blue-400',                                                  // INTERCONNECTIONS
      icon:     '\u2190 ',                                                        // INTERCONNECTIONS
    },                                                                            // INTERCONNECTIONS
    {                                                                             // INTERCONNECTIONS
      label:    'Calls',                                                          // INTERCONNECTIONS
      nodes:    nodeDetail.callees ?? [],                                         // INTERCONNECTIONS
      edgeType: 'calls',                                                          // INTERCONNECTIONS
      color:    'text-blue-400',                                                  // INTERCONNECTIONS
      icon:     '\u2192 ',                                                        // INTERCONNECTIONS
    },                                                                            // INTERCONNECTIONS
    {                                                                             // INTERCONNECTIONS
      label:    'Cloud Dependencies',                                             // INTERCONNECTIONS
      nodes:    ((nodeDetail as unknown as Record<string, unknown>).cloud_deps as GraphNode[] | undefined) ?? [], // INTERCONNECTIONS
      edgeType: 'depends_on',                                                     // INTERCONNECTIONS
      color:    'text-yellow-400',                                                // INTERCONNECTIONS
      icon:     '\u2601 ',                                                        // INTERCONNECTIONS
    },                                                                            // INTERCONNECTIONS
    {                                                                             // INTERCONNECTIONS
      label:    'Implements',                                                     // INTERCONNECTIONS
      nodes:    ((nodeDetail as unknown as Record<string, unknown>).implements as GraphNode[] | undefined) ?? [], // INTERCONNECTIONS
      edgeType: 'implements',                                                     // INTERCONNECTIONS
      color:    'text-purple-400',                                                // INTERCONNECTIONS
      icon:     '\u2283 ',                                                        // INTERCONNECTIONS
    },                                                                            // INTERCONNECTIONS
    {                                                                             // INTERCONNECTIONS
      label:    'Produces To',                                                    // INTERCONNECTIONS
      nodes:    ((nodeDetail as unknown as Record<string, unknown>).produces_to as GraphNode[] | undefined) ?? [], // INTERCONNECTIONS
      edgeType: 'produces_to',                                                    // INTERCONNECTIONS
      color:    'text-green-400',                                                 // INTERCONNECTIONS
      icon:     '\u2192 queue ',                                                  // INTERCONNECTIONS
    },                                                                            // INTERCONNECTIONS
  ].filter(g => g.nodes.length > 0)                                               // INTERCONNECTIONS
}                                                                                 // INTERCONNECTIONS

const DetailPanel: FC<DetailPanelProps> = ({ node, onClose, onNavigate, onNodeHighlight, onNodeSelect, onFocusNode }) => {
  const [detail, setDetail] = useState<NodeDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    () => new Set(['metadata', 'connections']),
  );
  const [aiResult, setAiResult] = useState<string | null>(null);
  const [aiLoading, setAiLoading] = useState(false);
  const [aiError, setAiError] = useState(false);
  const [aiStreaming, setAiStreaming] = useState(false);
  const [lastAiAction, setLastAiAction] = useState<'explain' | 'why-failing' | null>(null);
  const [copied, setCopied] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setAiResult(null);
    setAiError(false);
    setAiStreaming(false);
    api
      .getNode(node.id)
      .then((d) => {
        if (!cancelled) setDetail(d);
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
      // Abort any in-flight streaming request when node changes
      abortRef.current?.abort();
    };
  }, [node.id]);

  const toggle = (section: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      next.has(section) ? next.delete(section) : next.add(section);
      return next;
    });
  };

  const isOpen = (section: string) => expandedSections.has(section);

  // ---------- Streaming AI fetch -------------------------------------------
  const streamAiQuery = useCallback(
    async (question: string) => {
      // Abort previous stream if still running
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;

      setAiLoading(true);
      setAiStreaming(true);
      setAiError(false);
      setAiResult('');

      try {
        const res = await fetch('/api/ai/query', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Accept: 'text/event-stream',
          },
          body: JSON.stringify({ question }),
          signal: controller.signal,
        });

        if (!res.ok) {
          throw new Error(`API ${res.status}`);
        }

        const reader = res.body?.getReader();
        if (!reader) throw new Error('No readable stream');

        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          // Keep the last partial line in buffer
          buffer = lines.pop() ?? '';

          for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed || !trimmed.startsWith('data:')) continue;
            const jsonStr = trimmed.slice(5).trim();
            if (!jsonStr) continue;

            try {
              const event = JSON.parse(jsonStr) as {
                type: string;
                content?: string;
                node_id?: string;
              };

              switch (event.type) {
                case 'token':
                  if (event.content) {
                    setAiResult((prev) => (prev ?? '') + event.content);
                  }
                  break;
                case 'node_ref':
                  if (event.node_id && onNodeHighlight) {
                    onNodeHighlight(event.node_id);
                  }
                  break;
                case 'done':
                  // Stream complete
                  break;
              }
            } catch {
              // Ignore malformed JSON lines
            }
          }
        }
      } catch (e) {
        if ((e as Error).name !== 'AbortError') {
          setAiError(true);
          setAiResult(null);
        }
      } finally {
        setAiLoading(false);
        setAiStreaming(false);
        abortRef.current = null;
      }
    },
    [onNodeHighlight],
  );

  const handleExplain = useCallback(() => {
    setLastAiAction('explain');
    streamAiQuery(`Explain the function ${node.name}`);
  }, [node.name, streamAiQuery]);

  const handleWhyFailing = useCallback(() => {
    setLastAiAction('why-failing');
    streamAiQuery(`Why is ${node.name} failing?`);
  }, [node.name, streamAiQuery]);

  const handleRetryAi = useCallback(() => {
    if (lastAiAction === 'explain') handleExplain();
    else if (lastAiAction === 'why-failing') handleWhyFailing();
  }, [lastAiAction, handleExplain, handleWhyFailing]);

  // ---------- Copy source snippet ------------------------------------------
  const handleCopySnippet = useCallback(() => {
    const snippet = node.metadata?.source_snippet;
    if (!snippet) return;
    navigator.clipboard.writeText(snippet).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [node.metadata?.source_snippet]);

  const m = node.metadata;

  return (
    <div className="animate-slide-in-right absolute right-0 top-0 z-50 flex h-full w-[400px] flex-col border-l border-white/[0.08] bg-gray-950/80 backdrop-blur-2xl shadow-2xl">
      {/* Header */}
      <div className="flex items-start justify-between border-b border-border px-4 py-3">
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-base font-semibold text-gray-100">
            {node.name}
          </h2>
          <div className="mt-1 flex items-center gap-2">
            {/* SHADCN: replaced TypeBadge with Badge variant="secondary" */}
            <Badge variant="secondary" className="text-[10px]">{node.type}</Badge>
            {/* SHADCN: replaced StatusBadge with Badge */}
            <Badge
              variant={node.runtime_status === 'error' ? 'destructive' : 'outline'}
              className={`text-[10px] ${
                node.runtime_status === 'healthy'
                  ? 'border-green-700 bg-green-900 text-green-300'
                  : node.runtime_status === 'degraded'
                    ? 'border-yellow-700 bg-yellow-900 text-yellow-300'
                    : ''
              }`}
            >
              {node.runtime_status || 'unknown'}
            </Badge>
          </div>
          {node.file_path && (
            <div className="mt-1 truncate text-[11px] text-gray-500">
              {node.file_path}
              {node.line_start > 0 && `:${node.line_start}–${node.line_end}`}
            </div>
          )}          {/* INTERCONNECTIONS — Focus on graph button */}
          {/* SHADCN: replaced <button> with <Button> */}
          <Button                                                                 // INTERCONNECTIONS
            variant="ghost"                                                       // SHADCN: ghost variant
            size="sm"                                                             // SHADCN: small size
            onClick={() => onFocusNode?.(node.id)}                                // INTERCONNECTIONS
            className="mt-1.5 h-auto px-0 text-xs text-blue-400                   
                       hover:text-blue-300 flex items-center gap-1"              // INTERCONNECTIONS
          >                                                                       {/* INTERCONNECTIONS */}
            <span>{'\u2299'}</span>                                               {/* INTERCONNECTIONS */}
            <span>Focus on graph</span>                                           {/* INTERCONNECTIONS */}
          </Button>                                                               {/* INTERCONNECTIONS */}        </div>
        {/* SHADCN: replaced close button with <Button> */}
        <Button
          variant="ghost"
          size="sm"
          onClick={onClose}
          className="ml-2 shrink-0 h-7 w-7 p-0 text-gray-400 hover:bg-gray-800 hover:text-gray-200"
        >
          ✕
        </Button>
      </div>

      {/* SHADCN: replaced overflow-y-auto with ScrollArea */}
      <ScrollArea className="flex-1 px-4 py-3 text-sm">
        {loading && <Spinner />}

        {/* 1. Metadata */}
        <Section title="Metadata" open={isOpen('metadata')} onToggle={() => toggle('metadata')}>
          {node.type === 'function' && (
            <>
              {m?.signature && <KV label="Signature" value={m.signature} />}
              {m?.parameters && m.parameters.length > 0 && (
                <KV
                  label="Params"
                  value={m.parameters.map((p: { name: string; type: string }) => `${p.name} ${p.type}`).join(', ')}
                />
              )}
              {m?.return_types && m.return_types.length > 0 && (
                <KV label="Returns" value={m.return_types.join(', ')} />
              )}
              {m?.cyclomatic_complexity != null && (
                <KV label="Complexity" value={String(m.cyclomatic_complexity)} />
              )}
              {m?.doc_comment && (
                <div className="mt-1 rounded bg-gray-800 px-2 py-1 text-[11px] text-gray-400">
                  {m.doc_comment}
                </div>
              )}
            </>
          )}
          {node.type === 'struct' && m?.fields && m.fields.length > 0 && (
            <table className="mt-1 w-full text-[11px]">
              <thead>
                <tr className="text-gray-500">
                  <th className="text-left">Field</th>
                  <th className="text-left">Type</th>
                </tr>
              </thead>
              <tbody>
                {m.fields.map((f: { name: string; type: string }, i: number) => (
                  <tr key={i} className="text-gray-300">
                    <td>{f.name}</td>
                    <td className="text-gray-400">{f.type}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {node.type === 'interface' && m?.methods && m.methods.length > 0 && (
            <ul className="mt-1 list-inside list-disc text-[11px] text-gray-300">
              {m.methods.map((method: string, i: number) => (
                <li key={i}>{method}</li>
              ))}
            </ul>
          )}
          {node.type === 'cloud_service' && (
            <>
              {m?.cloud_provider && <KV label="Provider" value={m.cloud_provider} />}
              {m?.operations && m.operations.length > 0 && (
                <KV label="Operations" value={m.operations.join(', ')} />
              )}
              {m?.detected_in_files && m.detected_in_files.length > 0 && (
                <KV label="Found in" value={m.detected_in_files.join(', ')} />
              )}
            </>
          )}
        </Section>

        {/* 2. Source Code (functions with source_snippet) */}
        {node.type === 'function' && m?.source_snippet && (
          <Section title="Source" open={isOpen('source')} onToggle={() => toggle('source')}>
            <div className="relative">
              {/* SHADCN: replaced copy button with <Button> */}
              <Button
                variant="secondary"
                size="sm"
                onClick={handleCopySnippet}
                className="absolute right-2 top-2 z-10 h-5 px-2 text-[10px]"
              >
                {copied ? '✓ Copied' : 'Copy'}
              </Button>
              <div className="max-h-64 overflow-auto rounded bg-black/40">
                <SyntaxHighlighter
                  language="go"
                  style={vscDarkPlus}
                  customStyle={{
                    margin: 0,
                    padding: '0.75rem',
                    fontSize: '11px',
                    background: 'transparent',
                  }}
                  showLineNumbers
                  startingLineNumber={node.line_start > 0 ? node.line_start : 1}
                >
                  {m.source_snippet}
                </SyntaxHighlighter>
              </div>
            </div>
          </Section>
        )}

        {/* 3. Interconnections */}                                               {/* INTERCONNECTIONS */}
        <Section title="Interconnections" open={isOpen('connections')} onToggle={() => toggle('connections')}>
          {detail ? (                                                             /* INTERCONNECTIONS */
            (() => {                                                              /* INTERCONNECTIONS */
              const groups = buildConnectionGroups(node, detail)                   // INTERCONNECTIONS
              if (groups.length === 0) return (                                    /* INTERCONNECTIONS */
                <span className="text-[11px] text-gray-500">No connections</span> /* INTERCONNECTIONS */
              )                                                                   /* INTERCONNECTIONS */
              return groups.map(group => (                                         /* INTERCONNECTIONS */
                <div key={group.label} className="mt-2">                          {/* INTERCONNECTIONS */}
                  <div className={`text-xs uppercase tracking-wider               
                                   mb-1 ${group.color}`}>                        {/* INTERCONNECTIONS */}
                    {group.icon}{group.label} ({group.nodes.length})               {/* INTERCONNECTIONS */}
                  </div>                                                          {/* INTERCONNECTIONS */}
                  <div className="space-y-0.5">                                   {/* INTERCONNECTIONS */}
                    {group.nodes.slice(0, 8).map(n => (                            /* INTERCONNECTIONS */
                      <button                                                     // INTERCONNECTIONS
                        key={n.id}                                                // INTERCONNECTIONS
                        onClick={() => onNodeSelect?.(n)}                         // INTERCONNECTIONS
                        className="w-full text-left flex items-center             
                                   gap-2 px-2 py-1 rounded text-xs              
                                   hover:bg-gray-800 group"                      // INTERCONNECTIONS
                      >                                                           {/* INTERCONNECTIONS */}
                        <StatusDot status={n.runtime_status ?? 'unknown'} />                   {/* INTERCONNECTIONS */}
                        <span className="text-gray-300 truncate                   
                                         group-hover:text-white">                {/* INTERCONNECTIONS */}
                          {n.name}                                                {/* INTERCONNECTIONS */}
                        </span>                                                   {/* INTERCONNECTIONS */}
                        {n.error_count > 0 && (                                   /* INTERCONNECTIONS */
                          <span className="ml-auto text-red-400                   
                                           shrink-0">                            {/* INTERCONNECTIONS */}
                            {n.error_count} err                                   {/* INTERCONNECTIONS */}
                          </span>                                                 /* INTERCONNECTIONS */
                        )}                                                        {/* INTERCONNECTIONS */}
                      </button>                                                   /* INTERCONNECTIONS */
                    ))}                                                           {/* INTERCONNECTIONS */}
                    {group.nodes.length > 8 && (                                  /* INTERCONNECTIONS */
                      <div className="text-xs text-gray-600 px-2">               {/* INTERCONNECTIONS */}
                        +{group.nodes.length - 8} more                            {/* INTERCONNECTIONS */}
                      </div>                                                      /* INTERCONNECTIONS */
                    )}                                                            {/* INTERCONNECTIONS */}
                  </div>                                                          {/* INTERCONNECTIONS */}
                </div>                                                            /* INTERCONNECTIONS */
              ))                                                                  /* INTERCONNECTIONS */
            })()                                                                  /* INTERCONNECTIONS */
          ) : (                                                                   /* INTERCONNECTIONS */
            !loading && <span className="text-gray-500">\u2013</span>             /* INTERCONNECTIONS */
          )}                                                                      {/* INTERCONNECTIONS */}
        </Section>

        {/* 4. Data Flow (for functions) */}
        {node.type === 'function' && detail?.data_flow && detail.data_flow.length > 0 && (
          <Section title="Data Flow" open={isOpen('dataflow')} onToggle={() => toggle('dataflow')}>
            <div className="space-y-1 text-[11px]">
              {detail.data_flow.map((df, i) => (
                <div key={i} className="flex items-center gap-1 text-gray-300">
                  <KindDot kind={df.kind} />
                  <span className="font-mono">{df.type_name}</span>
                  {df.source && <span className="text-gray-500">← {df.source}</span>}
                  {df.sink && <span className="text-gray-500">→ {df.sink}</span>}
                </div>
              ))}
            </div>
          </Section>
        )}

        {/* 5. Failure History */}
        {node.error_count > 0 && detail?.recent_events && (
          <Section title="Failure History" open={isOpen('failures')} onToggle={() => toggle('failures')}>
            <div className="space-y-1">
              {detail.recent_events.slice(0, 10).map((ev, i) => (
                <EventRow key={i} event={ev} />
              ))}
            </div>
          </Section>
        )}

        {/* 6. AI Panel */}
        <Section title="AI" open={isOpen('ai')} onToggle={() => toggle('ai')}>
          <div className="flex gap-2">
            {/* SHADCN: replaced <button> with <Button> */}
            <Button
              onClick={handleExplain}
              disabled={aiLoading}
              size="sm"
              className="text-xs"
            >
              Explain this
            </Button>
            {node.error_count > 0 && (
              <Button
                onClick={handleWhyFailing}
                disabled={aiLoading}
                variant="destructive"
                size="sm"
                className="text-xs"
              >
                Why failing?
              </Button>
            )}
          </div>

          {/* AI loading state — animated dots */}
          {aiLoading && !aiResult && (
            <div className="mt-3 flex items-center gap-2 text-[11px] text-gray-400">
              <span className="flex gap-0.5">
                <span className="inline-block h-1.5 w-1.5 animate-bounce rounded-full bg-blue-400" style={{ animationDelay: '0ms' }} />
                <span className="inline-block h-1.5 w-1.5 animate-bounce rounded-full bg-blue-400" style={{ animationDelay: '150ms' }} />
                <span className="inline-block h-1.5 w-1.5 animate-bounce rounded-full bg-blue-400" style={{ animationDelay: '300ms' }} />
              </span>
              Codrix is analyzing…
            </div>
          )}

          {/* AI error state — inline red text + retry */}
          {aiError && !aiLoading && (
            <div className="mt-2 flex items-center gap-2 rounded bg-red-900/40 px-2.5 py-1.5 text-[11px]">
              <span className="text-red-400">AI explanation failed</span>
              {/* SHADCN: replaced retry button with <Button> */}
              <Button
                onClick={handleRetryAi}
                variant="destructive"
                size="sm"
                className="ml-auto h-5 px-2 text-[10px]"
              >
                Retry
              </Button>
            </div>
          )}

          {/* AI result (with streaming typewriter cursor) */}
          {aiResult && (
            <div
              className={`mt-2 whitespace-pre-wrap rounded bg-gray-800 px-3 py-2 text-[11px] text-gray-300 ${
                aiStreaming ? "after:content-['▋'] after:animate-pulse after:text-blue-400" : ''
              }`}
            >
              {aiResult}
            </div>
          )}
        </Section>
      </ScrollArea>
    </div>
  );
};

export default DetailPanel;

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

// SHADCN: replaced hand-written Section with Collapsible
function Section({
  title,
  open,
  onToggle,
  children,
}: {
  title: string;
  open: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}) {
  return (
    <Collapsible open={open} onOpenChange={onToggle}>
      <Separator className="my-0" />
      <CollapsibleTrigger asChild>
        <button className="flex w-full items-center justify-between py-2 text-xs font-semibold text-gray-300">
          {title}
          <span className="text-gray-500">{open ? '▾' : '▸'}</span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mb-2">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function KV({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2 py-0.5 text-[11px]">
      <span className="shrink-0 text-gray-500">{label}:</span>
      <span className="break-all text-gray-300">{value}</span>
    </div>
  );
}

function ConnList({
  label,
  nodes,
  onNav,
}: {
  label: string;
  nodes: GraphNode[];
  onNav: (id: string) => void;
}) {
  if (!nodes || nodes.length === 0) {
    return (
      <div className="py-0.5 text-[11px] text-gray-500">
        {label}: none
      </div>
    );
  }
  return (
    <div className="py-0.5">
      <div className="text-[11px] text-gray-500">{label}:</div>
      <ul className="ml-2 mt-0.5 space-y-0.5">
        {nodes.map((n) => (
          <li key={n.id} className="flex items-center gap-1.5 text-[11px]">
            <span
              className={`inline-block h-1.5 w-1.5 rounded-full ${
                n.runtime_status === 'healthy'
                  ? 'bg-green-400'
                  : n.runtime_status === 'error'
                    ? 'bg-red-400'
                    : 'bg-gray-500'
              }`}
            />
            <button
              onClick={() => onNav(n.id)}
              className="truncate text-blue-400 hover:underline"
            >
              {n.name}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

function KindDot({ kind }: { kind: string }) {
  const c: Record<string, string> = {
    input: 'bg-purple-400',
    fetched: 'bg-blue-400',
    computed: 'bg-cyan-400',
    constructed: 'bg-amber-400',
    output: 'bg-green-400',
    published: 'bg-orange-400',
  };
  return (
    <span className={`inline-block h-2 w-2 rounded-full ${c[kind] ?? 'bg-gray-400'}`} />
  );
}

function EventRow({ event }: { event: RuntimeEvent }) {
  const ts = new Date(event.timestamp).toLocaleTimeString();
  return (
    <div className="flex items-start gap-2 text-[11px]">
      <span className="shrink-0 text-gray-500">{ts}</span>
      <span
        className={`mt-0.5 inline-block h-2 w-2 shrink-0 rounded-full ${
          event.status === 'error' ? 'bg-red-400' : 'bg-green-400'
        }`}
      />
      <div className="min-w-0">
        <span className="text-gray-400">{event.event_type}</span>
        {event.latency_ms > 0 && (
          <span className="ml-1 text-gray-500">{event.latency_ms}ms</span>
        )}
        {event.error_message && (
          <div className="truncate text-red-400">{event.error_message}</div>
        )}
      </div>
    </div>
  );
}

function StatusDot({ status }: { status: string }) {                              // INTERCONNECTIONS
  const c: Record<string, string> = {                                             // INTERCONNECTIONS
    healthy: 'bg-green-400',                                                      // INTERCONNECTIONS
    error: 'bg-red-400',                                                          // INTERCONNECTIONS
    degraded: 'bg-yellow-400',                                                    // INTERCONNECTIONS
  };                                                                              // INTERCONNECTIONS
  return (                                                                        // INTERCONNECTIONS
    <span                                                                         // INTERCONNECTIONS
      className={`inline-block h-1.5 w-1.5 shrink-0 rounded-full ${             
        c[status] ?? 'bg-gray-500'                                                // INTERCONNECTIONS
      }`}                                                                         // INTERCONNECTIONS
    />                                                                            // INTERCONNECTIONS
  );                                                                              // INTERCONNECTIONS
}                                                                                 // INTERCONNECTIONS

function Spinner() {
  return (
    <div className="flex items-center gap-2 py-2 text-[11px] text-gray-500">
      <div className="h-3 w-3 animate-spin rounded-full border-2 border-gray-600 border-t-blue-400" />
      Loading…
    </div>
  );
}
