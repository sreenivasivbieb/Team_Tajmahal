import { memo, type FC } from 'react';
import { Handle, Position } from 'reactflow';
import type { GraphNode } from '../../types/graph';

interface InterfaceNodeProps {
  data: GraphNode;
}

const InterfaceNode: FC<InterfaceNodeProps> = ({ data }) => {
  const meta = data.metadata ?? {};
  const methods = meta.methods ?? [];
  const implementedBy = meta.implemented_by ?? [];
  const statusDot =
    data.runtime_status === 'error'    ? 'bg-red-400' :
    data.runtime_status === 'degraded' ? 'bg-yellow-400' :
    data.runtime_status === 'healthy'  ? 'bg-green-400' :
                                         'bg-gray-500';

  return (
    <div className="min-w-[200px] rounded-lg border border-cyan-500/60 bg-cyan-950/80 px-3 py-2.5 shadow-lg">
      <Handle type="target" position={Position.Top} className="!bg-cyan-400" />

      {/* Header */}
      <div className="flex items-center gap-2">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-cyan-800/80 text-sm">
          🔌
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${statusDot}`} />
            <span className="text-[10px] font-semibold uppercase tracking-wider text-cyan-400">
              INTERFACE
            </span>
            {data.is_exported && (
              <span className="rounded bg-cyan-700/50 px-1 py-0.5 text-[9px] font-medium text-cyan-200">
                pub
              </span>
            )}
          </div>
          <div className="truncate text-sm font-semibold text-gray-100" title={data.name}>
            {data.name}
          </div>
        </div>
        {/* Method count badge */}
        {methods.length > 0 && (
          <div className="flex h-5 min-w-[20px] items-center justify-center rounded-full bg-cyan-700/60 px-1.5 text-[10px] font-bold text-cyan-200">
            {methods.length}
          </div>
        )}
      </div>

      {/* Method signatures */}
      {methods.length > 0 && (
        <div className="mt-2 border-t border-cyan-800/50 pt-1.5">
          <div className="text-[9px] font-medium uppercase tracking-wider text-cyan-500">
            Methods
          </div>
          <div className="mt-0.5 space-y-0.5">
            {methods.slice(0, 5).map((m: string, i: number) => (
              <div key={i} className="truncate text-[10px] font-mono text-cyan-200/80" title={m}>
                {m}
              </div>
            ))}
            {methods.length > 5 && (
              <div className="text-[10px] text-cyan-400/50">
                +{methods.length - 5} more methods
              </div>
            )}
          </div>
        </div>
      )}

      {/* Implemented by */}
      {implementedBy.length > 0 && (
        <div className="mt-1.5 border-t border-cyan-800/50 pt-1">
          <div className="text-[9px] font-medium uppercase tracking-wider text-cyan-500">
            Implemented by
          </div>
          <div className="mt-0.5 flex flex-wrap gap-1">
            {implementedBy.slice(0, 4).map((s: string, i: number) => (
              <span
                key={i}
                className="rounded bg-amber-900/40 px-1.5 py-0.5 text-[9px] text-amber-300/80"
              >
                🧱 {s.split('/').pop()?.split(':').pop() ?? s}
              </span>
            ))}
            {implementedBy.length > 4 && (
              <span className="text-[10px] text-cyan-400/50">
                +{implementedBy.length - 4} more
              </span>
            )}
          </div>
        </div>
      )}

      <Handle type="source" position={Position.Bottom} className="!bg-cyan-400" />
    </div>
  );
};

export default memo(InterfaceNode);
