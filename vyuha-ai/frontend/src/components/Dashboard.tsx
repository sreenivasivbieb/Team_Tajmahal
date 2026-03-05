// ---------------------------------------------------------------------------
// components/Dashboard.tsx — Main panel: action cards + saved diagrams table
// Mirrors the eraser.io dashboard style.
// ---------------------------------------------------------------------------

import { memo, type ElementType, type FC } from 'react';
import {
  Plus,
  Sparkles,
  FileText,
  Trash2,
  MoreHorizontal,
  Search as SearchIcon,
} from 'lucide-react';
import { ScrollArea } from '@/components/ui/scroll-area';
import type { RepoEntry, SavedDiagram } from '../types/workspace';

// ---------------------------------------------------------------------------
// Action cards shown at the top
// ---------------------------------------------------------------------------

interface ActionCardProps {
  icon: ElementType;
  label: string;
  onClick: () => void;
}

const ActionCard: FC<ActionCardProps> = ({ icon: Icon, label, onClick }) => (
  <button
    onClick={onClick}
    className="group flex flex-1 flex-col items-center gap-3 rounded-xl border border-gray-700/50 bg-gray-900/60 px-6 py-6 text-sm text-gray-300 transition-all duration-200 ease-out hover:border-blue-500/40 hover:bg-gray-800/70 hover:text-gray-100 hover:shadow-xl hover:shadow-blue-500/5 hover:-translate-y-0.5 active:scale-[0.97] active:shadow-inner"
  >
    <Icon
      size={28}
      className="text-gray-500 transition-all duration-200 group-hover:text-blue-400 group-hover:scale-110"
    />
    <span className="font-medium">{label}</span>
  </button>
);

// ---------------------------------------------------------------------------
// Column header for the diagram table
// ---------------------------------------------------------------------------

const COLUMNS = [
  { key: 'name',      label: 'NAME',      flex: '2' },
  { key: 'tool',      label: 'TOOL',      flex: '1' },
  { key: 'created',   label: 'CREATED',   flex: '1' },
  { key: 'edited',    label: 'EDITED',    flex: '1' },
] as const;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  if (days < 7) return `${days}d ago`;
  const weeks = Math.floor(days / 7);
  return `${weeks}w ago`;
}

function toolLabel(tool: string): string {
  return tool
    .replace(/-/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface DashboardProps {
  activeRepo: RepoEntry | null;
  diagrams: SavedDiagram[];
  onNewBlankDiagram: () => void;
  onAIDiagram: () => void;
  onOpenDiagram: (id: string) => void;
  onRemoveDiagram: (id: string) => void;
}

const Dashboard: FC<DashboardProps> = ({
  activeRepo,
  diagrams,
  onNewBlankDiagram,
  onAIDiagram,
  onOpenDiagram,
  onRemoveDiagram,
}) => {
  // Only diagrams for the active repo
  const repoDiagrams = activeRepo
    ? diagrams.filter((d) => d.repoId === activeRepo.id)
    : [];

  return (
    <div className="flex h-full flex-col bg-[#0b0e14]">
      {/* ── Top bar (tabs / search) ──────────────────────────── */}
      <div className="flex items-center gap-6 border-b border-gray-800 px-6 py-3">
        <span className="text-sm font-semibold text-gray-200">All</span>
        <span className="text-sm text-gray-500 hover:text-gray-300 cursor-pointer">Recents</span>
        <span className="text-sm text-gray-500 hover:text-gray-300 cursor-pointer">Created by Me</span>
        <span className="text-sm text-gray-500 hover:text-gray-300 cursor-pointer">Unsorted</span>
        <div className="flex-1" />
        <div className="flex items-center gap-2 rounded-lg border border-gray-700/50 bg-gray-900 px-3 py-1.5 text-sm text-gray-500">
          <SearchIcon size={14} />
          <span>Search</span>
          <kbd className="ml-4 rounded border border-gray-700 bg-gray-800 px-1.5 py-0.5 text-[10px] text-gray-500">
            /
          </kbd>
        </div>
      </div>

      {/* ── Action cards ─────────────────────────────────────── */}
      <div className="flex gap-4 px-6 py-6 w-full">
        <ActionCard icon={Plus} label="Create a Blank File" onClick={onNewBlankDiagram} />
        <ActionCard icon={Sparkles} label="Generate an AI Diagram" onClick={onAIDiagram} />
        <ActionCard icon={FileText} label="Generate an AI outline" onClick={onNewBlankDiagram} />
      </div>

      {/* ── Active repo indicator ────────────────────────────── */}
      {activeRepo && (
        <div className="mx-6 mb-3 flex items-center gap-2 rounded-lg border border-gray-800 bg-gray-900/50 px-4 py-2">
          <span className={`h-2 w-2 rounded-full ${activeRepo.ready ? 'bg-emerald-500' : 'bg-amber-500 animate-pulse'}`} />
          <span className="text-[13px] font-medium text-gray-300">{activeRepo.name}</span>
          <span className="text-[11px] text-gray-600">{activeRepo.path}</span>
          {activeRepo.ready && activeRepo.symbolCount != null && (
            <span className="ml-auto text-[11px] text-gray-500">
              {activeRepo.symbolCount} symbols indexed
            </span>
          )}
          {!activeRepo.ready && (
            <span className="ml-auto text-[11px] text-amber-400 animate-pulse">
              Scanning…
            </span>
          )}
        </div>
      )}

      {/* ── Diagram table ────────────────────────────────────── */}
      <div className="flex items-center gap-4 px-6 py-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
        {COLUMNS.map((col) => (
          <span key={col.key} style={{ flex: col.flex }}>
            {col.label}
          </span>
        ))}
        <span className="w-8" /> {/* actions col */}
      </div>

      <ScrollArea className="flex-1 px-6">
        {repoDiagrams.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <p className="text-sm text-gray-500">
              {activeRepo
                ? 'No diagrams yet — create one above.'
                : 'Select a repository from the sidebar to get started.'}
            </p>
          </div>
        ) : (
          <div className="flex flex-col">
            {repoDiagrams.map((d) => (
              <div
                key={d.id}
                onClick={() => onOpenDiagram(d.id)}
                className="group flex cursor-pointer items-center gap-4 rounded-lg px-2 py-3 transition-colors hover:bg-gray-800/40"
              >
                {/* Name */}
                <span className="flex-[2] truncate text-[13px] font-medium text-gray-200">
                  {d.name}
                </span>
                {/* Tool */}
                <span className="flex-[1] truncate text-[12px] text-gray-500">
                  {toolLabel(d.tool)}
                </span>
                {/* Created */}
                <span className="flex-[1] text-[12px] text-gray-600">
                  {timeAgo(d.createdAt)}
                </span>
                {/* Edited */}
                <span className="flex-[1] text-[12px] text-gray-600">
                  {timeAgo(d.editedAt)}
                </span>
                {/* Actions */}
                <div className="flex w-8 items-center justify-end gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      onRemoveDiagram(d.id);
                    }}
                    className="rounded p-1 text-gray-600 hover:bg-red-500/20 hover:text-red-400"
                  >
                    <Trash2 size={13} />
                  </button>
                  <button className="rounded p-1 text-gray-600 hover:bg-gray-700 hover:text-gray-300">
                    <MoreHorizontal size={13} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </ScrollArea>
    </div>
  );
};

export default memo(Dashboard);
