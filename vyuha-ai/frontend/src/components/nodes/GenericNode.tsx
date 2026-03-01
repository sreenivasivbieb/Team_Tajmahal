import { memo, type FC } from 'react';
import { Handle, Position } from 'reactflow';
import type { GraphNode } from '../../types/graph';

interface GenericNodeProps {
  data: GraphNode;
}

const TYPE_CONFIG: Record<string, { icon: string; color: string; label: string }> = {
  package:          { icon: '📦', color: 'border-indigo-500/60 bg-indigo-950/80', label: 'PKG' },
  file:             { icon: '📄', color: 'border-slate-500/60 bg-slate-950/80',   label: 'FILE' },
  struct:           { icon: '🧱', color: 'border-amber-500/60 bg-amber-950/80',   label: 'STRUCT' },
  interface:        { icon: '🔌', color: 'border-cyan-500/60 bg-cyan-950/80',     label: 'IFACE' },
  repository:       { icon: '🏠', color: 'border-purple-500/60 bg-purple-950/80', label: 'REPO' },
  runtime_instance: { icon: '⚡', color: 'border-green-500/60 bg-green-950/80',   label: 'RUNTIME' },
};

const GenericNode: FC<GenericNodeProps> = ({ data }) => {
  const cfg = TYPE_CONFIG[data.type] ?? {
    icon: '●',
    color: 'border-gray-500/60 bg-gray-950/80',
    label: '?',
  };
  const statusDot =
    data.runtime_status === 'error'    ? 'bg-red-400' :
    data.runtime_status === 'degraded' ? 'bg-yellow-400' :
    data.runtime_status === 'healthy'  ? 'bg-green-400' :
                                         'bg-gray-500';

  return (
    <div className={`rounded-lg border px-3 py-2 shadow-md ${cfg.color} min-w-[140px]`}>
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />
      <div className="flex items-center gap-2">
        <span className="text-base">{cfg.icon}</span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${statusDot}`} />
            <span className="text-[10px] font-semibold uppercase tracking-wider text-gray-400">
              {cfg.label}
            </span>
          </div>
          <div className="truncate text-sm font-medium text-gray-100" title={data.name}>
            {data.name}
          </div>
        </div>
      </div>
      {data.type === 'struct' && data.metadata?.fields && (
        <div className="mt-1 text-[10px] text-gray-400">
          {data.metadata.fields.length} fields
        </div>
      )}
      {data.type === 'interface' && data.metadata?.methods && (
        <div className="mt-1 text-[10px] text-gray-400">
          {data.metadata.methods.length} methods
        </div>
      )}
      {data.type === 'file' && data.metadata?.line_count != null && (
        <div className="mt-1 text-[10px] text-gray-400">
          {data.metadata.line_count} lines
        </div>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
    </div>
  );
};

export default memo(GenericNode);
