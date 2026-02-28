// ---------------------------------------------------------------------------
// panels/AgentPanel.tsx — Real-time agent reasoning panel
// ---------------------------------------------------------------------------

import { useEffect, useRef, type FC } from 'react';
import type { AgentRun, AgentStep } from '../../types/graph';

interface AgentPanelProps {
  /** Live steps from SSE */
  steps: AgentStep[];
  /** Final run result (when complete) */
  agentRun: AgentRun | null;
  isRunning: boolean;
  onClose: () => void;
  /** Called when the user clicks a node name in the agent narrative */
  onNodeClick: (nodeId: string) => void;
}

const AgentPanel: FC<AgentPanelProps> = ({
  steps,
  agentRun,
  isRunning,
  onClose,
  onNodeClick,
}) => {
  const scrollRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to bottom as new steps appear
  useEffect(() => {
    scrollRef.current?.scrollTo({
      top: scrollRef.current.scrollHeight,
      behavior: 'smooth',
    });
  }, [steps.length, agentRun]);

  return (
    <div className="animate-slide-in-left absolute left-0 top-0 z-50 flex h-full w-[380px] flex-col border-r border-gray-700 bg-gray-900 shadow-2xl">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-gray-700 px-4 py-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold text-gray-100">Agent</span>
          {isRunning && (
            <span className="h-2 w-2 animate-pulse rounded-full bg-blue-400" />
          )}
        </div>
        <button
          onClick={onClose}
          className="rounded p-1 text-gray-400 hover:bg-gray-800 hover:text-gray-200"
        >
          ✕
        </button>
      </div>

      {/* Steps list */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto px-4 py-3 text-sm">
        {steps.map((step, i) => (
          <StepRow key={i} step={step} />
        ))}

        {isRunning && steps.length > 0 && (
          <div className="mt-3 flex items-center gap-2 text-xs text-gray-500">
            <span className="h-3 w-3 animate-spin rounded-full border-2 border-gray-600 border-t-blue-400" />
            Reasoning…
          </div>
        )}

        {/* Final answer */}
        {agentRun && (
          <div className="mt-4 border-t border-gray-700 pt-3">
            <div className="text-xs font-semibold text-gray-400">Answer</div>
            <div className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-gray-200">
              <ClickableNodeText
                text={agentRun.answer}
                onClick={onNodeClick}
              />
            </div>
            <div className="mt-2 text-[10px] text-gray-600">
              {agentRun.steps.length} steps · {agentRun.duration_ms}ms
            </div>
          </div>
        )}

        {!isRunning && steps.length === 0 && !agentRun && (
          <div className="py-8 text-center text-xs text-gray-600">
            Ask a question to start the agent.
          </div>
        )}
      </div>
    </div>
  );
};

export default AgentPanel;

// ---------------------------------------------------------------------------
// StepRow
// ---------------------------------------------------------------------------

function StepRow({ step }: { step: AgentStep }) {
  return (
    <div className="animate-fade-in mb-3">
      <div className="flex items-center gap-2">
        <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-gray-800 text-[10px] font-bold text-gray-400">
          {step.step}
        </span>
        {step.reasoning && (
          <span className="text-xs text-gray-300">{step.reasoning}</span>
        )}
      </div>

      {/* Tool calls */}
      {step.tool_calls?.map((tc, j) => (
        <div key={j} className="ml-7 mt-1 text-[11px]">
          <span className="text-gray-500">→ </span>
          <span className="font-mono text-blue-400">{tc.name}</span>
          {tc.arguments && (
            <span className="text-gray-500">({truncate(tc.arguments, 60)})</span>
          )}
        </div>
      ))}

      {/* Results */}
      {step.results?.map((r, j) => (
        <div key={j} className="ml-7 mt-0.5 text-[11px]">
          <span className="text-green-500">✓ </span>
          <span className="text-gray-400">{truncate(r, 80)}</span>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// ClickableNodeText — makes node:xxx references clickable
// ---------------------------------------------------------------------------

function ClickableNodeText({
  text,
  onClick,
}: {
  text: string;
  onClick: (nodeId: string) => void;
}) {
  // Match patterns like "node_id:xxx" or "id=xxx" or just quoted identifiers
  const parts = text.split(/((?:id=|node:)[a-zA-Z0-9_:.\-/]+)/g);

  return (
    <>
      {parts.map((part, i) => {
        const match = part.match(/(?:id=|node:)([a-zA-Z0-9_:.\-/]+)/);
        if (match) {
          const nodeId = match[1];
          return (
            <button
              key={i}
              onClick={() => onClick(nodeId)}
              className="font-mono text-blue-400 hover:underline"
            >
              {nodeId}
            </button>
          );
        }
        return <span key={i}>{part}</span>;
      })}
    </>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + '…' : s;
}
