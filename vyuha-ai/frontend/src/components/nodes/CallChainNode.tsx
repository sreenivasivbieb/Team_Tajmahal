// ---------------------------------------------------------------------------
// nodes/CallChainNode.tsx — Eraser.io-style node for call chain visualisation
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import { Handle, Position, type NodeProps } from 'reactflow';
import LanguageIcon, { getLanguageFromPath } from '../icons/LanguageIcon';

// ---- Annotation pill palette -----------------------------------------------

const ANNOTATION_STYLES: Record<string, { bg: string; text: string; label: string }> = {
  leaf:     { bg: 'bg-emerald-500/15', text: 'text-emerald-400', label: 'leaf' },
  db:       { bg: 'bg-blue-500/15',    text: 'text-blue-400',    label: 'store' },
  cycle:    { bg: 'bg-amber-500/15',   text: 'text-amber-400',   label: 'cycle' },
  error:    { bg: 'bg-red-500/15',     text: 'text-red-400',     label: 'error' },
  external: { bg: 'bg-purple-500/15',  text: 'text-purple-400',  label: 'ext' },
};

// ---- Role → subtle top-border accent colour --------------------------------

const ROLE_TOP_ACCENT: Record<string, string> = {
  target:     '#3B82F6', // blue-500
  caller:     '#9CA3AF', // gray-400
  callee:     '#64748B', // slate-500
  dependency: '#8B5CF6', // violet-500
  failing:    '#EF4444', // red-500
};

// ---- Data shape expected by the node ---------------------------------------

export interface CallChainNodeData {
  name: string;
  file_path?: string;
  file?: string;
  line_start?: number;
  line?: number;
  end_line?: number;
  signature?: string;
  is_external?: boolean;
  annotations?: string[];
  chainDepth?: number;
  role?: string;
}

// ---- Node width (keep in sync with layout constants) -----------------------

const NODE_WIDTH = 220;

// ---- Component -------------------------------------------------------------

const CallChainNode: FC<NodeProps<CallChainNodeData>> = ({ data }) => {
  const annotations = data.annotations ?? [];
  const role = data.role ?? 'callee';

  // File & language
  const filePath = data.file_path ?? data.file ?? '';
  const lang = getLanguageFromPath(filePath);
  const lineNum = data.line_start ?? data.line ?? 0;

  // Short filename (last segment only)
  const fileName = filePath
    ? (filePath.split('/').pop()?.split('\\').pop() ?? filePath)
    : null;
  const location = fileName
    ? `${fileName}${lineNum ? ':' + lineNum : ''}`
    : null;

  // Accent derived from role (subtle top border highlight)
  const accent = ROLE_TOP_ACCENT[role] ?? '#475569';

  return (
    <div
      className="group relative flex flex-col items-center rounded-xl transition-all duration-200"
      style={{
        width: NODE_WIDTH,
        minHeight: 96,
        padding: '14px 12px 10px',
        background: `linear-gradient(180deg, ${lang.color}0A 0%, #12162200 50%), #121622`,
        border: `1px solid ${lang.color}20`,
        borderTop: `2px solid ${accent}90`,
        boxShadow: [
          `0 4px 24px rgba(0,0,0,0.35)`,
          `0 0 0 0.5px ${lang.color}15`,
          `inset 0 1px 0 ${lang.color}08`,
        ].join(', '),
      }}
      title={data.signature ?? data.name}
    >
      {/* ── Handles ───────────────────────────────────────────── */}
      <Handle
        type="target"
        position={Position.Top}
        className="!w-1.5 !h-1.5 !border-0 !bg-gray-500/60"
      />

      {/* ── Language icon badge (centred top) ────────────────── */}
      <LanguageIcon filePath={filePath} isExternal={data.is_external} size={34} />

      {/* ── Function / symbol name ───────────────────────────── */}
      <p
        className="mt-2 w-full truncate text-center text-[13px] font-semibold leading-tight text-gray-100"
        title={data.name}
      >
        {data.name}
      </p>

      {/* ── File location ────────────────────────────────────── */}
      {location && (
        <p className="mt-0.5 w-full truncate text-center font-mono text-[10px] text-gray-500">
          {location}
        </p>
      )}

      {/* ── Annotation pills ─────────────────────────────────── */}
      {annotations.length > 0 && (
        <div className="mt-2 flex flex-wrap justify-center gap-1">
          {annotations.map((a) => {
            const s = ANNOTATION_STYLES[a] ?? {
              bg: 'bg-gray-500/15',
              text: 'text-gray-400',
              label: a,
            };
            return (
              <span
                key={a}
                className={`rounded-full px-2 py-px text-[9px] font-medium ${s.bg} ${s.text}`}
              >
                {s.label}
              </span>
            );
          })}
        </div>
      )}

      <Handle
        type="source"
        position={Position.Bottom}
        className="!w-1.5 !h-1.5 !border-0 !bg-gray-500/60"
      />
    </div>
  );
};

export default memo(CallChainNode);
