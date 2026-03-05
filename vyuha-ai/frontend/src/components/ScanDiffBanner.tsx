// ---------------------------------------------------------------------------
// components/ScanDiffBanner.tsx — Shows a brief diff summary after a rescan
// ---------------------------------------------------------------------------

import type { FC } from 'react';

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
  const parts: string[] = [];
  if (diff.nodesAdded > 0) parts.push(`+${diff.nodesAdded} nodes`);
  if (diff.nodesRemoved > 0) parts.push(`-${diff.nodesRemoved} nodes`);
  if (diff.nodesModified > 0) parts.push(`~${diff.nodesModified} modified`);
  if (diff.edgesAdded > 0) parts.push(`+${diff.edgesAdded} edges`);
  if (diff.edgesRemoved > 0) parts.push(`-${diff.edgesRemoved} edges`);

  if (parts.length === 0) return null;

  return (
    <div className="flex items-center gap-3 rounded-lg border border-green-700 bg-green-900/40 px-4 py-2 text-xs text-green-300">
      <span className="font-semibold">Scan Complete</span>
      <span>{parts.join(' · ')}</span>
      <span className="text-green-500/60">
        ({diff.totalNodes} nodes, {diff.totalEdges} edges)
      </span>
      <button
        onClick={onDismiss}
        className="ml-auto rounded px-1.5 py-0.5 text-green-400 hover:bg-green-800"
      >
        ✕
      </button>
    </div>
  );
};

export default ScanDiffBanner;
