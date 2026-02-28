// ---------------------------------------------------------------------------
// components/StatusBar.tsx — Fixed bottom bar with graph stats & SSE status
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useRef, useState, type FC } from 'react';
import type { Node as RFNode } from 'reactflow';
import type { GraphNode, GraphStats, WatchStatus } from '../types/graph';
import { api } from '../api/client';

interface StatusBarProps {
  nodeCount: number;
  edgeCount: number;
  isConnected: boolean;
  onRescan: () => void;
  onOpenIngestion: () => void;
  /** React Flow nodes — used by quick-inject buttons */
  rfNodes: RFNode[];
}

// ---------------------------------------------------------------------------
// Toast notification system
// ---------------------------------------------------------------------------

interface Toast {
  id: number;
  message: string;
  variant: 'success' | 'error';
}

let _toastId = 0;

const ToastContainer: FC<{
  toasts: Toast[];
  onDismiss: (id: number) => void;
}> = ({ toasts, onDismiss }) => {
  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-10 right-4 z-[60] flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-xs shadow-lg backdrop-blur animate-fade-in ${
            t.variant === 'success'
              ? 'border-green-700 bg-green-900/90 text-green-200'
              : 'border-red-700 bg-red-900/90 text-red-200'
          }`}
        >
          <span>{t.variant === 'success' ? '✓' : '✗'}</span>
          <span className="flex-1">{t.message}</span>
          <button
            onClick={() => onDismiss(t.id)}
            className="ml-2 text-gray-500 hover:text-gray-300"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  );
};

// ---------------------------------------------------------------------------
// Watch Modal (file path input + start/stop + status)
// ---------------------------------------------------------------------------

const WatchModal: FC<{ onClose: () => void }> = ({ onClose }) => {
  const [filePath, setFilePath] = useState('');
  const [status, setStatus] = useState<WatchStatus | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Poll watch status every 2s while modal is open
  const fetchStatus = useCallback(async () => {
    try {
      const s = await api.getWatchStatus();
      setStatus(s);
      if (s.file_path) setFilePath(s.file_path);
    } catch {
      // ignore poll failures
    }
  }, []);

  useEffect(() => {
    fetchStatus();
    pollRef.current = setInterval(fetchStatus, 2000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [fetchStatus]);

  const handleStart = async () => {
    if (!filePath.trim()) return;
    setLoading(true);
    setError(null);
    try {
      await api.startWatch(filePath.trim());
      await fetchStatus();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleStop = async () => {
    setLoading(true);
    setError(null);
    try {
      await api.stopWatch();
      setStatus({ active: false });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const isWatching = status?.active ?? false;

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center pb-12">
      {/* Backdrop */}
      <div className="absolute inset-0" onClick={onClose} />

      {/* Modal */}
      <div className="relative w-96 rounded-lg border border-gray-700 bg-gray-900 p-4 shadow-2xl">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-gray-200">Watch File</h3>
          <button
            onClick={onClose}
            className="text-gray-500 transition-colors hover:text-gray-300"
          >
            ✕
          </button>
        </div>

        {/* File path input */}
        <div className="mb-3">
          <input
            type="text"
            value={filePath}
            onChange={(e) => setFilePath(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && !isWatching && handleStart()}
            placeholder="/var/log/payments.log"
            disabled={isWatching}
            className="w-full rounded border border-gray-700 bg-gray-800 px-3 py-1.5 text-xs text-gray-100 placeholder-gray-500 outline-none focus:border-blue-500 disabled:opacity-50"
          />
        </div>

        {/* Start / Stop button */}
        <div className="mb-3">
          {isWatching ? (
            <button
              onClick={handleStop}
              disabled={loading}
              className="w-full rounded bg-red-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-red-500 disabled:opacity-50"
            >
              {loading ? 'Stopping…' : 'Stop Watching'}
            </button>
          ) : (
            <button
              onClick={handleStart}
              disabled={loading || !filePath.trim()}
              className="w-full rounded bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-500 disabled:opacity-50"
            >
              {loading ? 'Starting…' : 'Start Watching'}
            </button>
          )}
        </div>

        {/* Error */}
        {error && (
          <div className="mb-3 rounded bg-red-900/60 px-3 py-1.5 text-xs text-red-300">
            {error}
          </div>
        )}

        {/* Status indicator */}
        {isWatching && status && (
          <div className="space-y-1.5 border-t border-gray-800 pt-3">
            <div className="flex items-center gap-2">
              <span className="relative flex h-2 w-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
                <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
              </span>
              <span className="truncate text-xs text-green-300">
                Watching: {status.file_path}
              </span>
            </div>
            <div className="flex items-center gap-4 text-xs text-gray-400">
              <span>
                <span className="font-medium text-gray-300">{status.event_count ?? 0}</span> events
              </span>
              <span>
                <span className="font-medium text-gray-300">{status.lines_read ?? 0}</span> lines
              </span>
              {(status.parse_errors ?? 0) > 0 && (
                <span className="text-yellow-400">
                  <span className="font-medium">{status.parse_errors}</span> parse errs
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// StatusBar
// ---------------------------------------------------------------------------

const StatusBar: FC<StatusBarProps> = ({
  nodeCount,
  edgeCount,
  isConnected,
  onRescan,
  onOpenIngestion,
  rfNodes,
}) => {
  const [stats, setStats] = useState<GraphStats | null>(null);
  const [lastScan, setLastScan] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [showWatch, setShowWatch] = useState(false);
  const [watchActive, setWatchActive] = useState(false);
  const [watchEventCount, setWatchEventCount] = useState(0);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [injecting, setInjecting] = useState(false);
  const [recovering, setRecovering] = useState(false);

  const addToast = useCallback((message: string, variant: 'success' | 'error') => {
    const id = ++_toastId;
    setToasts((prev) => [...prev, { id, message, variant }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 3000);
  }, []);

  const dismissToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  // Helper: extract service name from a node's parent_id or best guess
  const getServiceName = useCallback(
    (node: GraphNode): string => {
      // Walk up: look for a parent with type === 'service'
      if (node.parent_id) {
        const parent = rfNodes.find((n) => n.id === node.parent_id);
        if (parent) {
          const pd = parent.data as GraphNode;
          if (pd.type === 'service') return pd.name;
          // one more hop
          if (pd.parent_id) {
            const gp = rfNodes.find((n) => n.id === pd.parent_id);
            if (gp && (gp.data as GraphNode).type === 'service') {
              return (gp.data as GraphNode).name;
            }
          }
        }
      }
      // fallback: use the node's name as service
      return node.name;
    },
    [rfNodes],
  );

  // 💥 Inject Error
  const handleInjectError = useCallback(async () => {
    setInjecting(true);
    try {
      // Find healthy function nodes
      const healthy = rfNodes
        .map((n) => n.data as GraphNode)
        .filter(
          (d) =>
            d.type === 'function' &&
            (d.runtime_status === 'healthy' ||
              d.runtime_status === '' ||
              !d.runtime_status),
        );
      if (healthy.length === 0) {
        addToast('No healthy function nodes to inject into', 'error');
        return;
      }
      const target = healthy[Math.floor(Math.random() * healthy.length)];
      const svc = getServiceName(target);
      await api.injectError({
        service: svc,
        name: target.name,
        file_path: target.file_path ?? '',
      });
      addToast(`Error injected into ${target.name}`, 'success');
    } catch (e) {
      addToast(
        `Inject failed: ${e instanceof Error ? e.message : String(e)}`,
        'error',
      );
    } finally {
      setInjecting(false);
    }
  }, [rfNodes, addToast, getServiceName]);

  // ✅ Recover All
  const handleRecoverAll = useCallback(async () => {
    setRecovering(true);
    try {
      const failing = rfNodes
        .map((n) => n.data as GraphNode)
        .filter((d) => d.runtime_status === 'error');
      if (failing.length === 0) {
        addToast('No failing nodes to recover', 'error');
        return;
      }
      const toRecover = failing.map((d) => ({
        service: getServiceName(d),
        name: d.name,
        file_path: d.file_path ?? '',
      }));
      const count = await api.recoverAll(toRecover);
      addToast(`Recovery sent to ${count} nodes`, 'success');
    } catch (e) {
      addToast(
        `Recovery failed: ${e instanceof Error ? e.message : String(e)}`,
        'error',
      );
    } finally {
      setRecovering(false);
    }
  }, [rfNodes, addToast, getServiceName]);

  // Fetch stats on mount and when node/edge count changes
  useEffect(() => {
    api.getStats().then(setStats).catch(() => {});
  }, [nodeCount, edgeCount]);

  // Poll watch status for the status bar indicator (every 5s)
  useEffect(() => {
    const poll = () => {
      api
        .getWatchStatus()
        .then((s) => {
          setWatchActive(s.active);
          setWatchEventCount(s.event_count ?? 0);
        })
        .catch(() => {});
    };
    poll();
    const id = setInterval(poll, 5000);
    return () => clearInterval(id);
  }, []);

  const handleRescan = async () => {
    setScanning(true);
    try {
      await onRescan();
      setLastScan(new Date().toLocaleTimeString());
    } finally {
      setScanning(false);
    }
  };

  return (
    <>
      <div className="flex h-8 items-center justify-between border-t border-gray-800 bg-gray-900 px-4 text-xs text-gray-400">
        {/* Left — counts */}
        <div className="flex items-center gap-4">
          <span>
            <span className="font-medium text-gray-300">{nodeCount}</span> nodes
          </span>
          <span>
            <span className="font-medium text-gray-300">{edgeCount}</span> edges
          </span>
          {stats && (
            <span>
              <span className="font-medium text-gray-300">{stats.nodes_by_type?.['service'] ?? 0}</span> services
            </span>
          )}
          {stats && (stats.status_counts?.['error'] ?? 0) > 0 && (
            <span className="text-red-400">
              <span className="font-medium">{stats.status_counts?.['error']}</span> errors
            </span>
          )}
        </div>

        {/* Center — SSE connection + watch indicator */}
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                isConnected ? 'bg-green-500' : 'bg-red-500'
              }`}
            />
            <span>{isConnected ? 'Connected' : 'Disconnected'}</span>
          </div>
          {watchActive && (
            <div className="flex items-center gap-1.5 text-green-400">
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green-500" />
              </span>
              <span>{watchEventCount} events</span>
            </div>
          )}
        </div>

        {/* Right — actions & last scan */}
        <div className="flex items-center gap-3">
          {lastScan && <span>Last scan: {lastScan}</span>}

          {/* Quick-inject buttons */}
          <button
            onClick={handleInjectError}
            disabled={injecting}
            className="rounded bg-red-900/60 px-2 py-0.5 text-red-300 transition-colors hover:bg-red-800/60 disabled:opacity-50"
            title="Inject a synthetic error into a random healthy function"
          >
            {injecting ? '…' : '💥'} Inject Error
          </button>
          <button
            onClick={handleRecoverAll}
            disabled={recovering}
            className="rounded bg-green-900/60 px-2 py-0.5 text-green-300 transition-colors hover:bg-green-800/60 disabled:opacity-50"
            title="Send recovery events for all failing nodes"
          >
            {recovering ? '…' : '✅'} Recover All
          </button>

          <span className="mx-0.5 h-3 border-l border-gray-700" />

          <button
            onClick={() => setShowWatch(true)}
            className={`rounded px-2 py-0.5 transition-colors ${
              watchActive
                ? 'bg-green-900/60 text-green-300 hover:bg-green-800/60'
                : 'bg-gray-800 text-gray-300 hover:bg-gray-700'
            }`}
          >
            {watchActive ? '● Watching' : 'Watch File'}
          </button>
          <button
            onClick={onOpenIngestion}
            className="rounded bg-gray-800 px-2 py-0.5 text-gray-300 transition-colors hover:bg-gray-700"
          >
            Ingest Logs
          </button>
          <button
            onClick={handleRescan}
            disabled={scanning}
            className="rounded bg-blue-600 px-2 py-0.5 text-white transition-colors hover:bg-blue-500 disabled:opacity-50"
          >
            {scanning ? 'Scanning…' : 'Rescan'}
          </button>
        </div>
      </div>

      {/* Toast notifications */}
      <ToastContainer toasts={toasts} onDismiss={dismissToast} />

      {/* Watch file modal */}
      {showWatch && <WatchModal onClose={() => setShowWatch(false)} />}
    </>
  );
};

export default StatusBar;
