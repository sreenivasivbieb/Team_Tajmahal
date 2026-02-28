// ---------------------------------------------------------------------------
// nodes/CloudNode.tsx — Custom React Flow node for cloud services
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import { Handle, Position, type NodeProps } from 'reactflow';
import type { GraphNode } from '../../types/graph';
import { useNodeErrorAnimation } from '../../hooks/useNodeErrorAnimation';

const providerLabel: Record<string, string> = {
  aws: 'AWS',
  gcp: 'GCP',
  azure: 'Azure',
};

const CloudNode: FC<NodeProps<GraphNode>> = ({ data, id }) => {
  const m = data.metadata;
  const provider = m?.cloud_provider ?? '';
  const badge = providerLabel[provider.toLowerCase()] ?? provider.toUpperCase();
  const opsCount = m?.operations?.length ?? 0;
  const animClass = useNodeErrorAnimation(id);

  return (
    <div
      className={`rounded-md border border-blue-500 bg-gray-900 px-3 py-2 shadow ${animClass}`}
      style={{ width: 160, minHeight: 60 }}
    >
      <Handle type="target" position={Position.Top} className="!bg-blue-400" />

      <div className="flex items-center gap-1.5">
        {/* Provider badge */}
        <span className="rounded bg-blue-600 px-1 py-0.5 text-[9px] font-bold text-white">
          {badge}
        </span>
        <span className="truncate text-xs font-medium text-blue-200">
          {data.name}
        </span>
      </div>

      {opsCount > 0 && (
        <div className="mt-1 text-[10px] text-gray-400">
          {opsCount} operation{opsCount > 1 ? 's' : ''} detected
        </div>
      )}

      <Handle type="source" position={Position.Bottom} className="!bg-blue-400" />
    </div>
  );
};

export default memo(CloudNode);
