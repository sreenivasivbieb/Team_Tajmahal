import { memo, useState, type FC } from 'react';

export interface ScanDiff {
  nodesAdded: number;
  nodesRemoved: number;
  nodesModified: number;
  edgesAdded: number;
  edgesRemoved: number;
  totalNodes: number;
  totalEdges: number;
}

interface ScanDiffBannerProps {
  diff: ScanDiff;
  onDismiss: () => void;
}

const ScanDiffBanner: FC<ScanDiffBannerProps> = ({ diff, onDismiss }) => {
  const [expanded, setExpanded] = useState(false);

  const hasChanges =
    diff.nodesAdded > 0 ||
    diff.nodesRemoved > 0 ||
    diff.nodesModified > 0 ||
    diff.edgesAdded > 0 ||
    diff.edgesRemoved > 0;

  if (!hasChanges) {
    // No changes — show a brief "no changes" message
    return (
      <div className="flex items-center gap-3 bg-gray-800/90 px-4 py-2 text-sm text-gray-300">
        <span className="text-green-400">✓</span>
        <span>Rescan complete — no changes detected</span>
        <span className="text-[10px] text-gray-500">
          ({diff.totalNodes} nodes, {diff.totalEdges} edges)
        </span>
        <button
          onClick={onDismiss}
          className="ml-auto shrink-0 text-gray-500 transition-colors hover:text-gray-300"
          aria-label="Dismiss"
        >
          ✕
        </button>
      </div>
    );
  }

  return (
    <div className="bg-blue-900/80 px-4 py-2 text-sm text-blue-100">
      <div className="flex items-center gap-3">
        <span className="text-blue-400">↻</span>
        <span className="font-medium">Rescan complete</span>

        {/* Summary badges */}
        <div className="flex items-center gap-2">
          {diff.nodesAdded > 0 && (
            <span className="rounded bg-green-800/60 px-1.5 py-0.5 text-[11px] text-green-300">
              +{diff.nodesAdded} nodes
            </span>
          )}
          {diff.nodesRemoved > 0 && (
            <span className="rounded bg-red-800/60 px-1.5 py-0.5 text-[11px] text-red-300">
              −{diff.nodesRemoved} nodes
            </span>
          )}
          {diff.nodesModified > 0 && (
            <span className="rounded bg-yellow-800/60 px-1.5 py-0.5 text-[11px] text-yellow-300">
              ~{diff.nodesModified} modified
            </span>
          )}
          {diff.edgesAdded > 0 && (
            <span className="rounded bg-green-800/40 px-1.5 py-0.5 text-[11px] text-green-300/70">
              +{diff.edgesAdded} edges
            </span>
          )}
          {diff.edgesRemoved > 0 && (
            <span className="rounded bg-red-800/40 px-1.5 py-0.5 text-[11px] text-red-300/70">
              −{diff.edgesRemoved} edges
            </span>
          )}
        </div>

        <span className="text-[10px] text-blue-300/50">
          Total: {diff.totalNodes} nodes, {diff.totalEdges} edges
        </span>

        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={() => setExpanded(!expanded)}
            className="text-[11px] text-blue-300 transition-colors hover:text-blue-100"
          >
            {expanded ? 'Less' : 'Details'}
          </button>
          <button
            onClick={onDismiss}
            className="shrink-0 text-blue-400 transition-colors hover:text-blue-200"
            aria-label="Dismiss"
          >
            ✕
          </button>
        </div>
      </div>

      {expanded && (
        <div className="mt-2 grid grid-cols-2 gap-x-8 gap-y-1 border-t border-blue-800/50 pt-2 text-[11px] text-blue-200/70">
          <div>Nodes added: <span className="text-green-400">{diff.nodesAdded}</span></div>
          <div>Edges added: <span className="text-green-400">{diff.edgesAdded}</span></div>
          <div>Nodes removed: <span className="text-red-400">{diff.nodesRemoved}</span></div>
          <div>Edges removed: <span className="text-red-400">{diff.edgesRemoved}</span></div>
          <div>Nodes modified: <span className="text-yellow-400">{diff.nodesModified}</span></div>
          <div></div>
        </div>
      )}
    </div>
  );
};

export default memo(ScanDiffBanner);
