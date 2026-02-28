// ---------------------------------------------------------------------------
// ScanSetup.tsx — Welcome modal asking for the repo folder path to scan
// ---------------------------------------------------------------------------

import { useState, type FC } from 'react';

interface ScanSetupProps {
  onScan: (rootPath: string) => void;
  isScanning: boolean;
  error: string | null;
}

const ScanSetup: FC<ScanSetupProps> = ({ onScan, isScanning, error }) => {
  const [path, setPath] = useState('');

  const handleSubmit = () => {
    const trimmed = path.trim();
    if (!trimmed) return;
    onScan(trimmed);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-xl border border-gray-700 bg-gray-900 shadow-2xl">
        {/* Header */}
        <div className="border-b border-gray-700 px-6 py-5">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-600/20 text-indigo-400">
              <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
              </svg>
            </div>
            <div>
              <h2 className="text-lg font-semibold text-gray-100">Welcome to VYUHA AI</h2>
              <p className="text-sm text-gray-400">Enter the path to your Go project to get started</p>
            </div>
          </div>
        </div>

        {/* Body */}
        <div className="px-6 py-5">
          <label className="mb-2 block text-sm font-medium text-gray-300">
            Project Root Path
          </label>
          <input
            type="text"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSubmit()}
            placeholder="C:\path\to\your\go-project"
            className="w-full rounded-lg border border-gray-600 bg-gray-800 px-4 py-2.5 text-sm text-gray-100 placeholder-gray-500 outline-none transition-colors focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500"
            autoFocus
            disabled={isScanning}
          />
          <p className="mt-2 text-xs text-gray-500">
            The folder must contain a <code className="rounded bg-gray-800 px-1 py-0.5 text-indigo-400">go.mod</code> file.
            VYUHA will parse all Go source files and build a dependency graph.
          </p>

          {error && (
            <div className="mt-3 rounded-lg border border-red-800 bg-red-900/30 px-3 py-2 text-sm text-red-300">
              {error}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 border-t border-gray-700 px-6 py-4">
          <button
            onClick={handleSubmit}
            disabled={!path.trim() || isScanning}
            className="flex items-center gap-2 rounded-lg bg-indigo-600 px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isScanning ? (
              <>
                <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Scanning…
              </>
            ) : (
              <>
                <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
                Scan Project
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};

export default ScanSetup;
