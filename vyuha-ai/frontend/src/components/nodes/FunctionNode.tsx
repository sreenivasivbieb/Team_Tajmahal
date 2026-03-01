// ---------------------------------------------------------------------------
// nodes/FunctionNode.tsx — Custom React Flow node for functions
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import { Handle, Position, type NodeProps } from 'reactflow';
import type { GraphNode } from '../../types/graph';
import { useNodeErrorAnimation } from '../../hooks/useNodeErrorAnimation';

const statusBorder: Record<string, string> = {
  healthy: 'border-green-500',
  degraded: 'border-yellow-500',
  error: 'border-red-500',
};

/**
 * Maps cyclomatic complexity to a color gradient.
 * 1-5:   green  (simple)
 * 6-10:  yellow (moderate)
 * 11-20: orange (complex)
 * 21+:   red    (very complex)
 */
function complexityColor(cc: number | undefined): {
  bg: string;
  text: string;
  border: string;
  label: string;
} {
  if (cc == null || cc <= 0) return { bg: 'bg-gray-700/40', text: 'text-gray-400', border: 'border-gray-600/40', label: '—' };
  if (cc <= 5)  return { bg: 'bg-green-900/40',  text: 'text-green-400',  border: 'border-green-500/40',  label: 'Simple' };
  if (cc <= 10) return { bg: 'bg-yellow-900/40', text: 'text-yellow-400', border: 'border-yellow-500/40', label: 'Moderate' };
  if (cc <= 20) return { bg: 'bg-orange-900/40', text: 'text-orange-400', border: 'border-orange-500/40', label: 'Complex' };
  return { bg: 'bg-red-900/40', text: 'text-red-400', border: 'border-red-500/40', label: 'Very Complex' };
}

const FunctionNode: FC<NodeProps<GraphNode>> = ({ data, id }) => {
  const status = data.runtime_status || 'unknown';
  const border = statusBorder[status] ?? 'border-gray-500';
  const m = data.metadata;
  const hasCloud =
    m?.cloud_dependencies && m.cloud_dependencies.length > 0;
  const animClass = useNodeErrorAnimation(id);

  const cc = m?.cyclomatic_complexity;
  const ccColor = complexityColor(cc);

  // Dynamic border based on complexity for at-a-glance heatmap
  const outerBorder =
    cc != null && cc > 20 ? 'border-red-500/60 bg-gray-900' :
    cc != null && cc > 10 ? 'border-orange-500/60 bg-gray-900' :
    `${border} bg-gray-900`;

  return (
    <div
      className={`rounded-md border px-3 py-2 shadow ${outerBorder} ${animClass}`}
      style={{ width: 180, minHeight: 60 }}
    >
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />

      {/* Function name */}
      <div className="flex items-center gap-1.5">
        {/* Complexity dot */}
        <span
          className="inline-block h-2 w-2 shrink-0 rounded-full"
          style={{
            backgroundColor:
              cc == null || cc <= 0 ? '#6b7280' :
              cc <= 5  ? '#22c55e' :
              cc <= 10 ? '#eab308' :
              cc <= 20 ? '#f97316' :
                         '#ef4444',
          }}
          title={`complexity: ${cc ?? '?'}`}
        />
        <span className="truncate text-xs font-medium text-gray-100">
          {m?.receiver ? `${m.receiver}.` : ''}
          {data.name}
        </span>
        {hasCloud && (
          <span className="ml-auto text-[10px] text-blue-400" title="Calls cloud service">
            ☁
          </span>
        )}
      </div>

      {/* Signature hint */}
      {m?.return_types && m.return_types.length > 0 && (
        <div className="mt-0.5 truncate text-[10px] text-gray-500">
          → {m.return_types.join(', ')}
        </div>
      )}

      {/* Complexity heatmap indicator */}
      {cc != null && cc > 0 && (
        <div className={`mt-1.5 flex items-center gap-1.5 rounded px-1.5 py-0.5 ${ccColor.bg}`}>
          <div
            className="h-2.5 w-2.5 rounded-sm"
            style={{
              backgroundColor:
                cc <= 5  ? '#22c55e' :
                cc <= 10 ? '#eab308' :
                cc <= 20 ? '#f97316' :
                           '#ef4444',
            }}
          />
          <span className={`text-[10px] font-medium ${ccColor.text}`}>
            CC: {cc}
          </span>
          <span className={`text-[9px] ${ccColor.text} opacity-70`}>
            ({ccColor.label})
          </span>
        </div>
      )}

      {/* Function traits */}
      <div className="mt-1 flex flex-wrap gap-1">
        {m?.has_error_return && (
          <span className="rounded bg-red-900/30 px-1 py-0.5 text-[9px] text-red-300/70">
            returns error
          </span>
        )}
        {m?.has_context_param && (
          <span className="rounded bg-violet-900/30 px-1 py-0.5 text-[9px] text-violet-300/70">
            ctx-aware
          </span>
        )}
        {m?.receiver && (
          <span className="rounded bg-blue-900/30 px-1 py-0.5 text-[9px] text-blue-300/70">
            method ({m.receiver})
          </span>
        )}
      </div>

      <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
    </div>
  );
};

export default memo(FunctionNode);
