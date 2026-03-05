// ---------------------------------------------------------------------------
// eraser/CanvasToolbar.tsx — Vertical toolbar on the canvas (eraser.io style)
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import {
  Plus,
  Sparkles,
  MousePointer2,
  Square,
  Circle,
  ArrowUpRight,
  Minus,
  Link2,
  Type,
  Search,
  Frame,
  MessageSquare,
} from 'lucide-react';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';

export type CanvasTool =
  | 'pointer'
  | 'rectangle'
  | 'circle'
  | 'arrow'
  | 'line'
  | 'connector'
  | 'text'
  | 'search'
  | 'frame'
  | 'comment';

interface CanvasToolbarProps {
  activeTool: CanvasTool;
  onToolChange: (tool: CanvasTool) => void;
  onAdd: () => void;
  onAI: () => void;
}

const MAIN_TOOLS: { tool: CanvasTool; icon: typeof Square; label: string; shortcut: string }[] = [
  { tool: 'pointer',   icon: MousePointer2, label: 'Select',    shortcut: 'V' },
  { tool: 'rectangle', icon: Square,         label: 'Rectangle', shortcut: 'R' },
  { tool: 'circle',    icon: Circle,         label: 'Ellipse',   shortcut: 'O' },
  { tool: 'arrow',     icon: ArrowUpRight,   label: 'Arrow',     shortcut: 'A' },
  { tool: 'line',      icon: Minus,          label: 'Line',      shortcut: 'L' },
  { tool: 'connector', icon: Link2,          label: 'Connector', shortcut: 'D' },
  { tool: 'text',      icon: Type,           label: 'Text',      shortcut: 'T' },
  { tool: 'search',    icon: Search,         label: 'Find',      shortcut: 'I' },
];

const BOTTOM_TOOLS: { tool: CanvasTool; icon: typeof Square; label: string; shortcut: string }[] = [
  { tool: 'frame',   icon: Frame,          label: 'Frame',   shortcut: 'F' },
  { tool: 'comment', icon: MessageSquare,  label: 'Comment', shortcut: 'C' },
];

const CanvasToolbar: FC<CanvasToolbarProps> = ({ activeTool, onToolChange, onAdd, onAI }) => (
  <div className="absolute right-4 top-1/2 z-30 flex -translate-y-1/2 flex-col items-center gap-1">
    {/* Add button */}
    <ToolButton icon={Plus} label="Add" shortcut="/" onClick={onAdd} />

    {/* AI button */}
    <ToolButton icon={Sparkles} label="AI" shortcut="Ctrl J" onClick={onAI} className="text-purple-400" />

    <div className="my-1 h-px w-6 bg-gray-700" />

    {/* Main tools */}
    {MAIN_TOOLS.map((t) => (
      <ToolButton
        key={t.tool}
        icon={t.icon}
        label={t.label}
        shortcut={t.shortcut}
        isActive={activeTool === t.tool}
        onClick={() => onToolChange(t.tool)}
      />
    ))}

    <div className="my-1 h-px w-6 bg-gray-700" />

    {/* Bottom tools */}
    {BOTTOM_TOOLS.map((t) => (
      <ToolButton
        key={t.tool}
        icon={t.icon}
        label={t.label}
        shortcut={t.shortcut}
        isActive={activeTool === t.tool}
        onClick={() => onToolChange(t.tool)}
      />
    ))}
  </div>
);

// ---------------------------------------------------------------------------
// Individual toolbar button
// ---------------------------------------------------------------------------

interface ToolButtonProps {
  icon: typeof Square;
  label: string;
  shortcut: string;
  isActive?: boolean;
  onClick: () => void;
  className?: string;
}

const ToolButton: FC<ToolButtonProps> = ({
  icon: Icon,
  label,
  shortcut,
  isActive,
  onClick,
  className,
}) => (
  <Tooltip>
    <TooltipTrigger asChild>
      <button
        onClick={onClick}
        className={`flex h-8 w-8 items-center justify-center rounded-lg transition-colors ${
          isActive
            ? 'bg-blue-600/20 text-blue-400'
            : `text-gray-400 hover:bg-gray-800 hover:text-gray-200 ${className ?? ''}`
        }`}
      >
        <Icon size={16} />
      </button>
    </TooltipTrigger>
    <TooltipContent side="left" className="text-xs">
      {label} <kbd className="ml-1 rounded border border-gray-700 bg-gray-800 px-1 text-[9px] text-gray-500">{shortcut}</kbd>
    </TooltipContent>
  </Tooltip>
);

export default memo(CanvasToolbar);
