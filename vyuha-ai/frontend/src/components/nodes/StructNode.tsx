import { memo, type FC } from 'react';
import { Handle, Position } from 'reactflow';
import type { GraphNode } from '../../types/graph';

interface StructNodeProps {
  data: GraphNode;
}

const StructNode: FC<StructNodeProps> = ({ data }) => {
  const meta = data.metadata ?? {};
  const fields = meta.fields ?? [];
  const embeddedTypes = meta.embedded_types ?? [];
  const implementsIfaces = meta.implements_interfaces ?? [];
  const statusDot =
    data.runtime_status === 'error'    ? 'bg-red-400' :
    data.runtime_status === 'degraded' ? 'bg-yellow-400' :
    data.runtime_status === 'healthy'  ? 'bg-green-400' :
                                         'bg-gray-500';

  return (
    <div className="min-w-[200px] rounded-lg border border-amber-500/60 bg-amber-950/80 px-3 py-2.5 shadow-lg">
      <Handle type="target" position={Position.Top} className="!bg-amber-400" />

      {/* Header */}
      <div className="flex items-center gap-2">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-amber-800/80 text-sm">
          🧱
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${statusDot}`} />
            <span className="text-[10px] font-semibold uppercase tracking-wider text-amber-400">
              STRUCT
            </span>
            {data.is_exported && (
              <span className="rounded bg-amber-700/50 px-1 py-0.5 text-[9px] font-medium text-amber-200">
                pub
              </span>
            )}
          </div>
          <div className="truncate text-sm font-semibold text-gray-100" title={data.name}>
            {data.name}
          </div>
        </div>
        {/* Field count badge */}
        {fields.length > 0 && (
          <div className="flex h-5 min-w-[20px] items-center justify-center rounded-full bg-amber-700/60 px-1.5 text-[10px] font-bold text-amber-200">
            {fields.length}
          </div>
        )}
      </div>

      {/* Fields list */}
      {fields.length > 0 && (
        <div className="mt-2 border-t border-amber-800/50 pt-1.5">
          <div className="text-[9px] font-medium uppercase tracking-wider text-amber-500">
            Fields
          </div>
          <div className="mt-0.5 space-y-0.5">
            {fields.slice(0, 5).map((f: { name: string; type: string }, i: number) => (
              <div key={i} className="flex items-center justify-between gap-2 text-[10px]">
                <span className="truncate font-medium text-amber-200/90">{f.name}</span>
                <span className="shrink-0 truncate text-amber-400/60" title={f.type}>
                  {f.type.length > 20 ? f.type.slice(0, 18) + '…' : f.type}
                </span>
              </div>
            ))}
            {fields.length > 5 && (
              <div className="text-[10px] text-amber-400/50">
                +{fields.length - 5} more fields
              </div>
            )}
          </div>
        </div>
      )}

      {/* Embedded types */}
      {embeddedTypes.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {embeddedTypes.map((t: string, i: number) => (
            <span
              key={i}
              className="rounded bg-amber-800/40 px-1.5 py-0.5 text-[9px] text-amber-300/70"
            >
              ⊂ {t}
            </span>
          ))}
        </div>
      )}

      {/* Implements */}
      {implementsIfaces.length > 0 && (
        <div className="mt-1.5 border-t border-amber-800/50 pt-1">
          <div className="text-[9px] font-medium uppercase tracking-wider text-amber-500">
            Implements
          </div>
          <div className="mt-0.5 flex flex-wrap gap-1">
            {implementsIfaces.map((iface: string, i: number) => (
              <span
                key={i}
                className="rounded bg-cyan-900/40 px-1.5 py-0.5 text-[9px] text-cyan-300/80"
              >
                ◆ {iface.split('/').pop()?.split(':').pop() ?? iface}
              </span>
            ))}
          </div>
        </div>
      )}

      <Handle type="source" position={Position.Bottom} className="!bg-amber-400" />
    </div>
  );
};

export default memo(StructNode);
