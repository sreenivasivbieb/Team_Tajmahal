// ---------------------------------------------------------------------------
// App.tsx — Eraser.io-style two-panel layout
//   Left sidebar : repos + nav
//   Main area    : dashboard (diagrams list) OR diagram editor (QueryBar + canvas)
// ---------------------------------------------------------------------------

import { useCallback, useState, type FC } from 'react';
import { TooltipProvider } from '@/components/ui/tooltip';

import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import AddRepoDialog, { type AddRepoResult } from './components/AddRepoDialog';
import EraserEditor from './components/eraser/EraserEditor';
import StatusBar from './components/StatusBar';

import { useWorkspace } from './hooks/useWorkspace';
import { useGraph } from './hooks/useGraph';
import { useSSE } from './hooks/useSSE';
import { api } from './api/client';

// ---------------------------------------------------------------------------
// View mode
// ---------------------------------------------------------------------------

type ViewMode = 'dashboard' | 'editor';

// ---------------------------------------------------------------------------
// Root App — combines sidebar + main panel
// ---------------------------------------------------------------------------

const App: FC = () => {
  const workspace = useWorkspace();
  const graph = useGraph();
  const sse = useSSE();

  const [view, setView] = useState<ViewMode>('dashboard');
  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [isScanning, setIsScanning] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);
  const [scanSuccess, setScanSuccess] = useState<string | null>(null);

  // Auto-dismiss success notification after 5 seconds
  // (using a ref so the effect doesn't need scanSuccess in deps)
  const successTimerRef = useState<ReturnType<typeof setTimeout> | null>(null);
  if (scanSuccess && !successTimerRef[0]) {
    successTimerRef[0] = setTimeout(() => {
      setScanSuccess(null);
      successTimerRef[0] = null;
    }, 5000);
  }
  if (!scanSuccess && successTimerRef[0]) {
    clearTimeout(successTimerRef[0]);
    successTimerRef[0] = null;
  }

  // ---- Add repo handler (GitHub clone or local path) -----------------------
  const handleAddRepo = useCallback(
    async (result: AddRepoResult) => {
      setIsScanning(true);
      setScanError(null);
      try {
        if (result.source === 'github') {
          // Clone from GitHub → backend handles git clone + scan
          const data = await api.cloneRepo(result.path);
          const entry = workspace.addRepo(data.name || result.name, data.local_path);
          workspace.markRepoReady(entry.id, data.symbol_count);
          setScanSuccess(`Repository "${data.name || result.name}" cloned & scanned — ${data.symbol_count ?? 0} symbols indexed`);
        } else {
          // Local path → scan only
          const entry = workspace.addRepo(result.name, result.path);
          const data = await api.scanRepo(result.path);
          workspace.markRepoReady(entry.id, data.symbol_count);
          setScanSuccess(`Repository "${result.name}" scanned — ${data.symbol_count ?? 0} symbols indexed`);
        }
        // Auto-navigate to Eraser-style editor after successful clone+scan
        graph.clearAll();
        setView('editor');
        setIsScanning(false);
        setAddRepoOpen(false);
      } catch (err) {
        console.error('Add repo error:', err);
        const msg = err instanceof Error ? err.message : String(err);
        setScanError(msg || 'An unknown error occurred. Is the Vyuha server running?');
        setIsScanning(false);
        // Do NOT close the dialog — keep it open so user sees the error
      }
    },
    [workspace, graph],
  );

  // ---- Open diagram handler (go to editor view) --------------------------
  const handleOpenDiagram = useCallback(
    (diagramId: string) => {
      workspace.setActiveDiagram(diagramId);
      setView('editor');
    },
    [workspace],
  );

  // ---- New blank diagram → editor view -----------------------------------
  const handleNewBlank = useCallback(() => {
    graph.clearAll();
    setView('editor');
  }, [graph]);

  // ---- AI diagram → editor with pre-selected AI tool --------------------
  const handleAIDiagram = useCallback(() => {
    graph.clearAll();
    setView('editor');
  }, [graph]);

  // ---- Save diagram from editor ----------------------------------------
  const handleSaveDiagram = useCallback(
    (tool: string, query: string) => {
      if (!workspace.activeRepo) return;
      workspace.saveDiagram(workspace.activeRepo.id, query || 'Untitled Diagram', tool, query);
    },
    [workspace],
  );

  // ---- Back to dashboard when clicking a repo in sidebar ----------------
  const handleSelectRepo = useCallback(
    (id: string) => {
      workspace.setActiveRepo(id);
      setView('dashboard');
    },
    [workspace],
  );

  return (
    <TooltipProvider delayDuration={200}>
        <div className="flex h-screen w-screen bg-[#0b0e14] text-gray-100">
          {/* ── Sidebar ────────────────────────────────────── */}
          <Sidebar
            repos={workspace.repos}
            activeRepoId={workspace.activeRepo?.id ?? null}
            onSelectRepo={handleSelectRepo}
            onRemoveRepo={workspace.removeRepo}
            onAddRepo={() => setAddRepoOpen(true)}
          />

          {/* ── Main area ──────────────────────────────────── */}
          <div className="flex flex-1 flex-col overflow-hidden">
            {view === 'dashboard' ? (
              <>
                <Dashboard
                  activeRepo={workspace.activeRepo}
                  diagrams={workspace.diagrams}
                  onNewBlankDiagram={handleNewBlank}
                  onAIDiagram={handleAIDiagram}
                  onOpenDiagram={handleOpenDiagram}
                  onRemoveDiagram={workspace.removeDiagram}
                />
                <StatusBar
                  nodeCount={graph.nodes.length}
                  edgeCount={graph.edges.length}
                  isConnected={sse.isConnected}
                  toolProgress={sse.toolProgress}
                />
              </>
            ) : (
              <EraserEditor
                repoName={workspace.activeRepo?.name ?? 'Untitled'}
                repoPath={workspace.activeRepo?.path ?? ''}
                graph={graph}
                sse={sse}
                onBack={() => setView('dashboard')}
              />
            )}
          </div>

          {/* ── Success notification ──────────────────────── */}
          {scanSuccess && (
            <div className="fixed bottom-4 right-4 z-[100] flex items-center gap-3 rounded-lg border border-emerald-700 bg-emerald-900/90 px-4 py-3 text-sm text-emerald-200 shadow-2xl backdrop-blur animate-in slide-in-from-bottom-4">
              <span className="text-emerald-400">✓</span>
              <span>{scanSuccess}</span>
              <button
                onClick={() => setScanSuccess(null)}
                className="ml-2 rounded px-1.5 py-0.5 text-emerald-400 hover:bg-emerald-800"
              >
                ✕
              </button>
            </div>
          )}

          {/* ── Add repo dialog ────────────────────────────── */}
          <AddRepoDialog
            open={addRepoOpen}
            onClose={() => { setAddRepoOpen(false); setScanError(null); }}
            onSubmit={handleAddRepo}
            isScanning={isScanning}
            error={scanError}
            onClearError={() => setScanError(null)}
          />
        </div>
    </TooltipProvider>
  );
};

export default App;
