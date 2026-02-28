// ---------------------------------------------------------------------------
// nodes/ServiceNode.tsx — Custom React Flow node for services
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import { Handle, Position, type NodeProps } from 'reactflow';
import type { GraphNode } from '../../types/graph';
import { useNodeErrorAnimation } from '../../hooks/useNodeErrorAnimation';

const borderColor: Record<string, string> = {
  healthy: 'border-green-500',
  degraded: 'border-yellow-500',
  error: 'border-red-500',
};

const ServiceNode: FC<NodeProps<GraphNode>> = ({ data, id }) => {
  const status = data.runtime_status || 'unknown';
  const border = borderColor[status] ?? 'border-gray-400';
  const dataAny = data as unknown as Record<string, unknown>;
  const justChanged = dataAny._statusChanged
    && Date.now() - (dataAny._statusChanged as number) < 3000;
  const animClass = useNodeErrorAnimation(id);

  return (
    <div
      className={`relative rounded-lg border-2 bg-gray-900 px-4 py-3 shadow-lg
        ${border} ${justChanged ? 'animate-pulse-fast' : ''} ${animClass}`}
      style={{ width: 200, minHeight: 80 }}
    >
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />

      {/* Error badge */}
      {data.error_count > 0 && (
        <span className="absolute -right-2 -top-2 flex h-5 min-w-[20px] items-center justify-center rounded-full bg-red-600 px-1 text-[10px] font-bold text-white">
          {data.error_count}
        </span>
      )}

      {/* Name */}
      <div className="truncate text-sm font-semibold text-gray-100">
        {data.name}
      </div>

      {/* Stats row */}
      <div className="mt-1 flex items-center gap-3 text-[11px] text-gray-400">
        {data.metadata?.package_count != null && (
          <span>{data.metadata.package_count} pkgs</span>
        )}
        {data.metadata?.function_count != null && (
          <span>{data.metadata.function_count} fns</span>
        )}
      </div>

      {/* Status dot */}
      <div className="mt-1.5 flex items-center gap-1.5 text-[10px]">
        <span
          className={`inline-block h-2 w-2 rounded-full ${
            status === 'healthy'
              ? 'bg-green-400'
              : status === 'degraded'
                ? 'bg-yellow-400'
                : status === 'error'
                  ? 'bg-red-400'
                  : 'bg-gray-500'
          }`}
        />
        <span className="text-gray-500">{status}</span>
      </div>

      <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
    </div>
  );
};

export default memo(ServiceNode);
