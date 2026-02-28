// ---------------------------------------------------------------------------
// hooks/useSSE.ts — Server-Sent Events with reconnection
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useRef, useState } from 'react';
import type { AgentStep, ScanProgress, SSEEvent } from '../types/graph';

const SSE_URL = '/api/events';
const MAX_BACKOFF_MS = 30_000;

export interface UseSSEReturn {
  isConnected: boolean;
  lastEvent: SSEEvent | null;
  nodeStatusUpdates: Map<string, string>;
  agentSteps: AgentStep[];
  scanProgress: ScanProgress | null;
  clearAgentSteps: () => void;
}

export function useSSE(): UseSSEReturn {
  const [isConnected, setIsConnected] = useState(false);
  const [lastEvent, setLastEvent] = useState<SSEEvent | null>(null);
  const [nodeStatusUpdates, setNodeStatusUpdates] = useState<Map<string, string>>(
    () => new Map(),
  );
  const [agentSteps, setAgentSteps] = useState<AgentStep[]>([]);
  const [scanProgress, setScanProgress] = useState<ScanProgress | null>(null);

  const retryCount = useRef(0);
  const esRef = useRef<EventSource | null>(null);

  const clearAgentSteps = useCallback(() => setAgentSteps([]), []);

  // ------ SSE connection ---------------------------------------------------
  useEffect(() => {
    let mounted = true;
    let timer: ReturnType<typeof setTimeout> | null = null;

    function connect() {
      if (!mounted) return;

      const es = new EventSource(SSE_URL);
      esRef.current = es;

      es.onopen = () => {
        if (!mounted) return;
        setIsConnected(true);
        retryCount.current = 0;
      };

      es.onmessage = (ev: MessageEvent) => {
        if (!mounted) return;
        try {
          const parsed: SSEEvent = {
            event: ev.type === 'message' ? 'message' : ev.type,
            data: JSON.parse(ev.data),
          };
          setLastEvent(parsed);
        } catch {
          /* ignore malformed */
        }
      };

      // Named event handlers
      const handleEvent = (ev: MessageEvent) => {
        if (!mounted) return;
        try {
          const data = JSON.parse(ev.data);
          const sseEvent: SSEEvent = { event: ev.type, data };
          setLastEvent(sseEvent);
          dispatch(ev.type, data);
        } catch {
          /* ignore */
        }
      };

      const eventTypes = [
        'node_status_update',
        'scan_progress',
        'scan_complete',
        'agent_start',
        'agent_step',
        'agent_tool_call',
        'agent_tool_result',
        'agent_done',
        'agent_error',
        'job_update',
      ];

      for (const t of eventTypes) {
        es.addEventListener(t, handleEvent as EventListener);
      }

      es.onerror = () => {
        if (!mounted) return;
        setIsConnected(false);
        es.close();
        esRef.current = null;

        // Exponential backoff
        const delay = Math.min(
          1000 * Math.pow(2, retryCount.current),
          MAX_BACKOFF_MS,
        );
        retryCount.current += 1;
        timer = setTimeout(connect, delay);
      };
    }

    function dispatch(type: string, data: unknown) {
      const d = data as Record<string, unknown>;

      switch (type) {
        case 'node_status_update': {
          const nodeId = d.node_id as string;
          const status = d.status as string;
          if (nodeId && status) {
            setNodeStatusUpdates((prev) => {
              const next = new Map(prev);
              next.set(nodeId, status);
              return next;
            });

            // Dispatch custom event so node components can animate
            if (status === 'error' || status === 'degraded') {
              window.dispatchEvent(
                new CustomEvent('vyuha:node_error', {
                  detail: { nodeId, status },
                }),
              );
            }
          }
          break;
        }

        case 'scan_progress': {
          setScanProgress(d as unknown as ScanProgress);
          break;
        }

        case 'scan_complete': {
          setScanProgress(null);
          break;
        }

        case 'agent_step':
        case 'agent_tool_call':
        case 'agent_tool_result': {
          setAgentSteps((prev) => [
            ...prev,
            {
              step: (d.step as number) ?? prev.length + 1,
              tool_calls: d.tool ? [{ id: '', name: d.tool as string, arguments: d.args as string ?? '' }] : undefined,
              results: d.length ? [`(${d.length} chars)`] : undefined,
              reasoning: d.reasoning as string | undefined,
              timestamp: new Date().toISOString(),
            },
          ]);
          break;
        }

        case 'agent_start': {
          setAgentSteps([]);
          break;
        }

        default:
          break;
      }
    }

    connect();

    return () => {
      mounted = false;
      if (timer) clearTimeout(timer);
      esRef.current?.close();
    };
  }, []);

  return {
    isConnected,
    lastEvent,
    nodeStatusUpdates,
    agentSteps,
    scanProgress,
    clearAgentSteps,
  };
}
