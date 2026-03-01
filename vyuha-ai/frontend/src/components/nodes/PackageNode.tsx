import { memo, type FC } from 'react';
import { Handle, Position } from 'reactflow';
import type { GraphNode } from '../../types/graph';

interface PackageNodeProps {
  data: GraphNode;
}

const PackageNode: FC<PackageNodeProps> = ({ data }) => {
  const meta = data.metadata ?? {};
  const funcCount = meta.function_count ?? 0;
  const lineCount = meta.total_lines ?? meta.line_count ?? 0;
  const statusDot =
    data.runtime_status === 'error'    ? 'bg-red-400' :
    data.runtime_status === 'degraded' ? 'bg-yellow-400' :
    data.runtime_status === 'healthy'  ? 'bg-green-400' :
                                         'bg-gray-500';

  // Extract short package name from full ID like "pkg:github.com/vyuha/vyuha-ai/internal/api"
  const shortName = data.name || data.id.split('/').pop() || 'pkg';

  return (
    <div className="min-w-[180px] rounded-lg border border-indigo-500/60 bg-indigo-950/80 px-3 py-2.5 shadow-lg">
      <Handle type="target" position={Position.Top} className="!bg-indigo-400" />

      {/* Header */}
      <div className="flex items-center gap-2">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-indigo-800/80 text-sm">
          📦
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${statusDot}`} />
            <span className="text-[10px] font-semibold uppercase tracking-wider text-indigo-400">
              PACKAGE
            </span>
          </div>
          <div className="truncate text-sm font-semibold text-gray-100" title={data.name}>
            {shortName}
          </div>
        </div>
      </div>

      {/* Stats row */}
      <div className="mt-2 flex items-center gap-3 text-[10px] text-indigo-300/80">
        {funcCount > 0 && (
          <span className="flex items-center gap-1">
            <span className="text-indigo-400">ƒ</span> {funcCount} funcs
          </span>
        )}
        {lineCount > 0 && (
          <span className="flex items-center gap-1">
            <span className="text-indigo-400">#</span> {lineCount} lines
          </span>
        )}
        {data.is_exported && (
          <span className="rounded bg-indigo-700/50 px-1 py-0.5 text-[9px] font-medium text-indigo-200">
            exported
          </span>
        )}
      </div>

      {/* Import paths preview */}
      {meta.import_paths && meta.import_paths.length > 0 && (
        <div className="mt-1.5 border-t border-indigo-800/50 pt-1.5">
          <div className="text-[9px] font-medium uppercase tracking-wider text-indigo-500">
            Imports ({meta.import_paths.length})
          </div>
          <div className="mt-0.5 space-y-0.5">
            {meta.import_paths.slice(0, 3).map((imp: string, i: number) => (
              <div key={i} className="truncate text-[10px] text-indigo-300/60" title={imp}>
                {imp.split('/').pop()}
              </div>
            ))}
            {meta.import_paths.length > 3 && (
              <div className="text-[10px] text-indigo-400/50">
                +{meta.import_paths.length - 3} more
              </div>
            )}
          </div>
        </div>
      )}

      {/* Cloud dependencies badge */}
      {meta.cloud_dependencies && meta.cloud_dependencies.length > 0 && (
        <div className="mt-1.5 flex items-center gap-1 text-[10px] text-orange-400/80">
          <span>☁</span>
          <span>{meta.cloud_dependencies.length} cloud deps</span>
        </div>
      )}

      <Handle type="source" position={Position.Bottom} className="!bg-indigo-400" />
    </div>
  );
};

export default memo(PackageNode);
