// ---------------------------------------------------------------------------
// components/Sidebar.tsx — Left panel: scanned repos, nav items, + button
// Matches the eraser.io dashboard sidebar aesthetic.
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import {
  FolderGit2,
  Plus,
  Trash2,
  Bot,
  Sparkles,
  LayoutTemplate,
  Archive,
  Loader2,
} from 'lucide-react';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { RepoEntry } from '../types/workspace';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface SidebarProps {
  repos: RepoEntry[];
  activeRepoId: string | null;
  onSelectRepo: (id: string) => void;
  onRemoveRepo: (id: string) => void;
  onAddRepo: () => void;
}

// ---------------------------------------------------------------------------
// Navigation items (static, below the repo list)
// ---------------------------------------------------------------------------

const NAV_ITEMS = [
  { icon: Bot,            label: 'Vyuha Bot',        shortcut: 'B' },
  { icon: Sparkles,       label: 'AI Presets',        shortcut: 'C' },
  { icon: LayoutTemplate, label: 'Team Templates',    shortcut: 'T' },
  { icon: Archive,        label: 'Archive',           shortcut: 'E' },
] as const;

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const Sidebar: FC<SidebarProps> = ({
  repos,
  activeRepoId,
  onSelectRepo,
  onRemoveRepo,
  onAddRepo,
}) => {
  return (
    <aside className="flex h-full w-[240px] flex-col border-r border-gray-800 bg-[#0d1117]">
      {/* ── Header ─────────────────────────────────────────────── */}
      <div className="flex items-center gap-2 px-4 py-4">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-gradient-to-br from-blue-600 to-violet-600">
          <span className="text-xs font-bold text-white">V</span>
        </div>
        <span className="text-sm font-semibold text-gray-100 tracking-tight">
          Vyuha AI
        </span>
      </div>

      {/* ── All Repos label ────────────────────────────────────── */}
      <div className="flex items-center justify-between px-4 pb-1 pt-2">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-gray-500">
          Repositories
        </span>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={onAddRepo}
              className="flex h-5 w-5 items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-800 hover:text-gray-300"
            >
              <Plus size={14} />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right" className="text-xs">
            Scan new repository
          </TooltipContent>
        </Tooltip>
      </div>

      {/* ── Repo list ──────────────────────────────────────────── */}
      <ScrollArea className="flex-1 px-2">
        <div className="flex flex-col gap-0.5 py-1">
          {repos.length === 0 && (
            <p className="px-2 py-4 text-center text-[11px] text-gray-600">
              No repositories scanned yet.
              <br />
              Click <span className="text-blue-400">+</span> to add one.
            </p>
          )}

          {repos.map((repo) => {
            const isActive = repo.id === activeRepoId;
            return (
              <div
                key={repo.id}
                onClick={() => onSelectRepo(repo.id)}
                className={`group flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-2 text-[13px] transition-colors ${
                  isActive
                    ? 'bg-gray-800/80 text-gray-100'
                    : 'text-gray-400 hover:bg-gray-800/50 hover:text-gray-200'
                }`}
              >
                <FolderGit2
                  size={15}
                  className={isActive ? 'text-blue-400' : 'text-gray-500'}
                />
                <span className="flex-1 truncate font-medium">{repo.name}</span>

                {/* Status indicator */}
                {!repo.ready ? (
                  <Loader2 size={12} className="animate-spin text-amber-400" />
                ) : (
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 opacity-0 group-hover:opacity-100 transition-opacity" />
                )}

                {/* Delete button (shown on hover) */}
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onRemoveRepo(repo.id);
                  }}
                  className="ml-auto hidden h-5 w-5 items-center justify-center rounded text-gray-600 transition-colors hover:bg-red-500/20 hover:text-red-400 group-hover:flex"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            );
          })}
        </div>
      </ScrollArea>

      {/* ── Static nav items ───────────────────────────────────── */}
      <div className="border-t border-gray-800 px-2 py-2">
        {NAV_ITEMS.map((item) => (
          <div
            key={item.label}
            className="flex cursor-default items-center gap-2.5 rounded-md px-2.5 py-1.5 text-[13px] text-gray-500 transition-colors hover:bg-gray-800/50 hover:text-gray-300"
          >
            <item.icon size={15} />
            <span className="flex-1">{item.label}</span>
            <kbd className="text-[10px] text-gray-600">{item.shortcut}</kbd>
          </div>
        ))}
      </div>

      {/* ── New Repo button (bottom) ───────────────────────────── */}
      <div className="px-3 pb-3 pt-1">
        <button
          onClick={onAddRepo}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-3 py-2 text-[13px] font-medium text-white transition-colors hover:bg-blue-500"
        >
          <Plus size={14} />
          New Repo
        </button>
      </div>
    </aside>
  );
};

export default memo(Sidebar);
