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

function complexityColor(c: number | undefined): string {
  if (!c || c <= 5) return 'bg-green-400';
  if (c <= 10) return 'bg-yellow-400';
  return 'bg-red-400';
}

const FunctionNode: FC<NodeProps<GraphNode>> = ({ data, id }) => {
  const status = data.runtime_status || 'unknown';
  const border = statusBorder[status] ?? 'border-gray-500';
  const m = data.metadata;
  const hasCloud =
    m?.cloud_dependencies && m.cloud_dependencies.length > 0;
  const animClass = useNodeErrorAnimation(id);

  return (
    <div
      className={`rounded-md border bg-gray-900 px-3 py-2 shadow ${border} ${animClass}`}
      style={{ width: 180, minHeight: 60 }}
    >
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />

      {/* Function name */}
      <div className="flex items-center gap-1.5">
        {/* Complexity dot */}
        <span
          className={`inline-block h-2 w-2 shrink-0 rounded-full ${complexityColor(m?.cyclomatic_complexity)}`}
          title={`complexity: ${m?.cyclomatic_complexity ?? '?'}`}
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

      <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
    </div>
  );
};

export default memo(FunctionNode);
