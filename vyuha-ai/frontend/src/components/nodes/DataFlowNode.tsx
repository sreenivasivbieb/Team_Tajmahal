// ---------------------------------------------------------------------------
// nodes/DataFlowNode.tsx — Custom React Flow node for data flow records
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import { Handle, Position, type NodeProps } from 'reactflow';
import type { GraphNode } from '../../types/graph';
import { useNodeErrorAnimation } from '../../hooks/useNodeErrorAnimation';

const kindColor: Record<string, string> = {
  input: 'border-purple-400 text-purple-300',
  fetched: 'border-blue-400 text-blue-300',
  computed: 'border-cyan-400 text-cyan-300',
  constructed: 'border-amber-400 text-amber-300',
  output: 'border-green-400 text-green-300',
  published: 'border-orange-400 text-orange-300',
};

const DataFlowNode: FC<NodeProps<GraphNode>> = ({ data, id }) => {
  const m = data.metadata;
  const kind = m?.kind ?? 'unknown';
  const isAggregate = m?.is_aggregate ?? false;
  const fanIn = m?.fan_in ?? 0;
  const colors =
    kindColor[kind] ??
    'border-gray-400 text-gray-300';
  const aggregateBorder = isAggregate ? 'border-amber-600' : '';
  const animClass = useNodeErrorAnimation(id);

  return (
    <div
      className={`rounded border bg-gray-900 px-2.5 py-1.5 shadow ${aggregateBorder || colors.split(' ')[0]} ${animClass}`}
      style={{ width: 160, minHeight: 50 }}
    >
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />

      <div className="flex items-center gap-1.5">
        <span
          className={`rounded px-1 py-0.5 text-[9px] font-bold ${colors}`}
        >
          {kind}
        </span>
        <span className="truncate text-xs text-gray-200">
          {m?.type_name ?? data.name}
        </span>
      </div>

      {isAggregate && fanIn > 1 && (
        <div className="mt-0.5 text-[10px] text-amber-400">
          fan_in: {fanIn}
        </div>
      )}

      <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
    </div>
  );
};

export default memo(DataFlowNode);
