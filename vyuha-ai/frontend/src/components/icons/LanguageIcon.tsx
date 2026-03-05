// ---------------------------------------------------------------------------
// icons/LanguageIcon.tsx — Language badge for eraser.io-style nodes
// ---------------------------------------------------------------------------

import { memo, type FC } from 'react';
import { Package } from 'lucide-react';

// ---- Language definitions --------------------------------------------------

interface LanguageDef {
  abbrev: string;
  color: string;
  textColor: string;
}

const DEFAULT_LANG: LanguageDef = { abbrev: '?', color: '#6B7280', textColor: '#ffffff' };

const LANGUAGES: Record<string, LanguageDef> = {
  // TypeScript / JavaScript
  ts:     { abbrev: 'TS',  color: '#3178C6', textColor: '#fff' },
  tsx:    { abbrev: 'TX',  color: '#3178C6', textColor: '#fff' },
  mts:    { abbrev: 'TS',  color: '#3178C6', textColor: '#fff' },
  cts:    { abbrev: 'TS',  color: '#3178C6', textColor: '#fff' },
  js:     { abbrev: 'JS',  color: '#F7DF1E', textColor: '#000' },
  jsx:    { abbrev: 'JX',  color: '#F7DF1E', textColor: '#000' },
  mjs:    { abbrev: 'JS',  color: '#F7DF1E', textColor: '#000' },
  cjs:    { abbrev: 'JS',  color: '#F7DF1E', textColor: '#000' },

  // Go
  go:     { abbrev: 'Go',  color: '#00ADD8', textColor: '#fff' },

  // Python
  py:     { abbrev: 'Py',  color: '#3776AB', textColor: '#fff' },
  pyw:    { abbrev: 'Py',  color: '#3776AB', textColor: '#fff' },
  pyi:    { abbrev: 'Py',  color: '#3776AB', textColor: '#fff' },

  // Rust
  rs:     { abbrev: 'Rs',  color: '#DEA584', textColor: '#1a1a1a' },

  // Java / JVM
  java:   { abbrev: 'Jv',  color: '#ED8B00', textColor: '#fff' },
  kt:     { abbrev: 'Kt',  color: '#7F52FF', textColor: '#fff' },
  kts:    { abbrev: 'Kt',  color: '#7F52FF', textColor: '#fff' },
  scala:  { abbrev: 'Sc',  color: '#DC322F', textColor: '#fff' },
  groovy: { abbrev: 'Gr',  color: '#4298B8', textColor: '#fff' },
  clj:    { abbrev: 'Cj',  color: '#5881D8', textColor: '#fff' },

  // C family
  c:      { abbrev: 'C',   color: '#A8B9CC', textColor: '#1a1a1a' },
  h:      { abbrev: 'H',   color: '#A8B9CC', textColor: '#1a1a1a' },
  cpp:    { abbrev: 'C+',  color: '#00599C', textColor: '#fff' },
  cc:     { abbrev: 'C+',  color: '#00599C', textColor: '#fff' },
  cxx:    { abbrev: 'C+',  color: '#00599C', textColor: '#fff' },
  hpp:    { abbrev: 'H+',  color: '#00599C', textColor: '#fff' },
  hh:     { abbrev: 'H+',  color: '#00599C', textColor: '#fff' },
  cs:     { abbrev: 'C#',  color: '#239120', textColor: '#fff' },
  m:      { abbrev: 'OC',  color: '#438EFF', textColor: '#fff' },  // Objective-C

  // Web
  html:   { abbrev: '<>',  color: '#E34F26', textColor: '#fff' },
  htm:    { abbrev: '<>',  color: '#E34F26', textColor: '#fff' },
  css:    { abbrev: 'Cs',  color: '#1572B6', textColor: '#fff' },
  scss:   { abbrev: 'Sc',  color: '#CC6699', textColor: '#fff' },
  sass:   { abbrev: 'Sa',  color: '#CC6699', textColor: '#fff' },
  less:   { abbrev: 'Le',  color: '#1D365D', textColor: '#fff' },
  vue:    { abbrev: 'Vu',  color: '#4FC08D', textColor: '#fff' },
  svelte: { abbrev: 'Sv',  color: '#FF3E00', textColor: '#fff' },
  astro:  { abbrev: 'As',  color: '#FF5D01', textColor: '#fff' },

  // Scripting
  rb:     { abbrev: 'Rb',  color: '#CC342D', textColor: '#fff' },
  php:    { abbrev: 'Ph',  color: '#777BB4', textColor: '#fff' },
  lua:    { abbrev: 'Lu',  color: '#000080', textColor: '#fff' },
  pl:     { abbrev: 'Pl',  color: '#39457E', textColor: '#fff' },
  r:      { abbrev: 'R',   color: '#276DC3', textColor: '#fff' },

  // Apple / Mobile
  swift:  { abbrev: 'Sw',  color: '#F05138', textColor: '#fff' },
  dart:   { abbrev: 'Dt',  color: '#0175C2', textColor: '#fff' },

  // Functional
  hs:     { abbrev: 'Hs',  color: '#5e5086', textColor: '#fff' },
  ex:     { abbrev: 'Ex',  color: '#6E4A7E', textColor: '#fff' },
  exs:    { abbrev: 'Ex',  color: '#6E4A7E', textColor: '#fff' },
  erl:    { abbrev: 'Er',  color: '#A90533', textColor: '#fff' },
  ml:     { abbrev: 'ML',  color: '#EC6813', textColor: '#fff' },
  fs:     { abbrev: 'F#',  color: '#378BBA', textColor: '#fff' },
  fsx:    { abbrev: 'F#',  color: '#378BBA', textColor: '#fff' },

  // Systems
  zig:    { abbrev: 'Zi',  color: '#F7A41D', textColor: '#1a1a1a' },
  nim:    { abbrev: 'Ni',  color: '#FFE953', textColor: '#1a1a1a' },
  v:      { abbrev: 'V',   color: '#5D87BF', textColor: '#fff' },

  // Data / Config
  json:   { abbrev: '{}',  color: '#292929', textColor: '#fff' },
  jsonc:  { abbrev: '{}',  color: '#292929', textColor: '#fff' },
  yaml:   { abbrev: 'Ym',  color: '#CB171E', textColor: '#fff' },
  yml:    { abbrev: 'Ym',  color: '#CB171E', textColor: '#fff' },
  toml:   { abbrev: 'Tm',  color: '#9C4121', textColor: '#fff' },
  xml:    { abbrev: 'Xl',  color: '#0060AC', textColor: '#fff' },
  graphql:{ abbrev: 'Gq',  color: '#E535AB', textColor: '#fff' },
  gql:    { abbrev: 'Gq',  color: '#E535AB', textColor: '#fff' },
  proto:  { abbrev: 'Pb',  color: '#4285F4', textColor: '#fff' },

  // Shell
  sh:     { abbrev: '$_',  color: '#4EAA25', textColor: '#fff' },
  bash:   { abbrev: '$_',  color: '#4EAA25', textColor: '#fff' },
  zsh:    { abbrev: '$_',  color: '#4EAA25', textColor: '#fff' },
  fish:   { abbrev: '$_',  color: '#4EAA25', textColor: '#fff' },
  ps1:    { abbrev: 'PS',  color: '#012456', textColor: '#fff' },
  bat:    { abbrev: 'Bt',  color: '#C1F12E', textColor: '#1a1a1a' },
  cmd:    { abbrev: 'Cd',  color: '#C1F12E', textColor: '#1a1a1a' },

  // SQL
  sql:    { abbrev: 'Sq',  color: '#e38c00', textColor: '#fff' },

  // Docs
  md:     { abbrev: 'Md',  color: '#083FA1', textColor: '#fff' },
  mdx:    { abbrev: 'Mx',  color: '#083FA1', textColor: '#fff' },
  txt:    { abbrev: 'Tx',  color: '#6B7280', textColor: '#fff' },

  // Docker / Infra
  dockerfile: { abbrev: 'Dk', color: '#2496ED', textColor: '#fff' },
  tf:     { abbrev: 'Tf',  color: '#7B42BC', textColor: '#fff' },
  hcl:    { abbrev: 'Hc',  color: '#7B42BC', textColor: '#fff' },

  // WASM
  wasm:   { abbrev: 'Wa',  color: '#654FF0', textColor: '#fff' },
  wat:    { abbrev: 'Wa',  color: '#654FF0', textColor: '#fff' },
};

// ---- Public helpers --------------------------------------------------------

/**
 * Resolve language definition from a file path or filename.
 */
export function getLanguageFromPath(filePath: string): LanguageDef {
  if (!filePath) return DEFAULT_LANG;

  // Handle names like "Dockerfile" (no extension)
  const base = filePath.split('/').pop()?.split('\\').pop() ?? '';
  if (base.toLowerCase() === 'dockerfile') return LANGUAGES['dockerfile'];
  if (base.toLowerCase() === 'makefile') return { abbrev: 'Mk', color: '#6B4C3B', textColor: '#fff' };

  const ext = base.split('.').pop()?.toLowerCase() ?? '';
  return LANGUAGES[ext] ?? DEFAULT_LANG;
}

// ---- Component -------------------------------------------------------------

interface LanguageIconProps {
  filePath?: string;
  isExternal?: boolean;
  /** Badge dimension in px (default 32) */
  size?: number;
}

const LanguageIcon: FC<LanguageIconProps> = ({ filePath, isExternal, size = 32 }) => {
  // External node with no file — show package icon
  if (isExternal && !filePath) {
    return (
      <div
        className="flex items-center justify-center rounded-lg"
        style={{
          width: size,
          height: size,
          backgroundColor: '#4B5563',
          boxShadow: '0 2px 8px rgba(75, 85, 99, 0.4)',
        }}
      >
        <Package size={size * 0.55} color="#e5e7eb" strokeWidth={2} />
      </div>
    );
  }

  const lang = getLanguageFromPath(filePath ?? '');

  return (
    <div
      className="flex items-center justify-center rounded-lg select-none"
      style={{
        width: size,
        height: size,
        backgroundColor: lang.color,
        color: lang.textColor,
        fontSize: size * 0.38,
        fontWeight: 700,
        letterSpacing: '-0.02em',
        lineHeight: 1,
        boxShadow: `0 2px 10px ${lang.color}50`,
      }}
    >
      {lang.abbrev}
    </div>
  );
};

export default memo(LanguageIcon);
