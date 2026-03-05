// ---------------------------------------------------------------------------
// eraser/AIDiagramCard.tsx — The "AI Diagram" card shown on empty canvas
// Matches eraser.io's AI Diagram prompt card with sparkle icons.
// ---------------------------------------------------------------------------

import { memo, useCallback, useState, type FC, type FormEvent } from 'react';
import { Sparkles, Loader2, ArrowRight } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';

interface AIDiagramCardProps {
  /** Called when user submits a prompt for AI diagram generation */
  onGenerate: (prompt: string) => void;
  /** Whether the AI is currently generating */
  isGenerating: boolean;
}

const AIDiagramCard: FC<AIDiagramCardProps> = ({ onGenerate, isGenerating }) => {
  const [prompt, setPrompt] = useState('');
  const [expanded, setExpanded] = useState(false);

  const handleSubmit = useCallback(
    (e: FormEvent) => {
      e.preventDefault();
      const q = prompt.trim();
      if (!q || isGenerating) return;
      onGenerate(q);
    },
    [prompt, isGenerating, onGenerate],
  );

  if (!expanded) {
    return (
      <button
        onClick={() => setExpanded(true)}
        className="group flex h-[480px] w-full max-w-[600px] flex-col items-center justify-center rounded-2xl border-2 border-dashed border-blue-500/30 bg-gray-900/60 px-8 py-10 transition-all hover:border-blue-400/50 hover:bg-gray-900/80 hover:shadow-xl hover:shadow-blue-500/5"
      >
        {/* Header */}
        <div className="mb-2 flex items-center gap-2">
          <div className="flex items-center gap-1 rounded-full bg-gray-800 px-3 py-1 text-xs text-gray-300">
            <svg width="14" height="14" viewBox="0 0 16 16" className="text-blue-400">
              <path d="M3 1h10l3 3v8l-3 3H3l-3-3V4z" fill="none" stroke="currentColor" strokeWidth="1.2" />
            </svg>
            Diagram
          </div>
          <div className="flex items-center gap-1 rounded-full bg-blue-600/20 px-3 py-1 text-xs font-medium text-blue-400">
            Open Editor
            <ArrowRight size={12} />
          </div>
        </div>

        {/* Title */}
        <h3 className="mt-4 text-xl font-semibold text-gray-100">AI Diagram</h3>
        <p className="mt-1 text-sm text-gray-500">Generate diagram with natural language</p>

        {/* Sparkle icons */}
        <div className="relative mt-8 flex items-center justify-center">
          <Sparkles
            size={56}
            className="text-gray-500/60 transition-colors group-hover:text-blue-400/80"
            strokeWidth={1}
          />
          <Sparkles
            size={32}
            className="absolute -left-4 -top-2 text-gray-500/40 transition-colors group-hover:text-purple-400/60"
            strokeWidth={1.2}
          />
          <Sparkles
            size={24}
            className="absolute -bottom-2 -right-6 text-gray-500/30 transition-colors group-hover:text-indigo-400/50"
            strokeWidth={1.2}
          />
        </div>
      </button>
    );
  }

  // ── Expanded state: prompt input ─────────────────────────────────
  return (
    <div className="flex h-[480px] w-full max-w-[600px] flex-col rounded-2xl border border-gray-700/50 bg-gray-900/90 p-6 shadow-2xl shadow-black/40 backdrop-blur">
      {/* Header */}
      <div className="mb-4 flex items-center gap-2">
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-purple-600/20">
          <Sparkles size={16} className="text-purple-400" />
        </div>
        <div>
          <h3 className="text-sm font-semibold text-gray-100">AI Diagram Generator</h3>
          <p className="text-[11px] text-gray-500">Describe the architecture or flow you want to visualize</p>
        </div>
      </div>

      {/* Prompt input */}
      <form onSubmit={handleSubmit} className="flex flex-1 flex-col gap-4">
        <Input
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="e.g. Show me the architecture of this codebase"
          className="bg-gray-800 border-gray-700 text-gray-100 placeholder:text-gray-500 focus-visible:ring-purple-500"
          disabled={isGenerating}
          autoFocus
        />

        {/* Quick prompts */}
        <div className="flex flex-wrap gap-1.5">
          {QUICK_PROMPTS.map((qp) => (
            <button
              key={qp}
              type="button"
              onClick={() => setPrompt(qp)}
              className="rounded-full border border-gray-700/50 bg-gray-800/50 px-3 py-1 text-[11px] text-gray-400 transition-colors hover:border-purple-500/30 hover:text-purple-300"
            >
              {qp}
            </button>
          ))}
        </div>

        <div className="mt-auto flex items-center justify-between pt-4">
          <button
            type="button"
            onClick={() => setExpanded(false)}
            className="text-xs text-gray-500 hover:text-gray-300"
          >
            Cancel
          </button>
          <Button
            type="submit"
            disabled={!prompt.trim() || isGenerating}
            size="sm"
            className="bg-purple-600 hover:bg-purple-500"
          >
            {isGenerating ? (
              <span className="flex items-center gap-1.5">
                <Loader2 size={14} className="animate-spin" />
                Generating…
              </span>
            ) : (
              <span className="flex items-center gap-1.5">
                <Sparkles size={14} />
                Generate Diagram
              </span>
            )}
          </Button>
        </div>
      </form>
    </div>
  );
};

const QUICK_PROMPTS = [
  'Architecture overview',
  'API request flow',
  'Database schema relationships',
  'Module dependency graph',
  'Call chain for main entry point',
];

export default memo(AIDiagramCard);
