import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type FC,
  type ReactNode,
} from 'react';
import { api } from '../api/client';
import type { UseGraphReturn } from '../hooks/useGraph';
import type { ScanDiff } from '../components/ScanDiffBanner';

interface ScanContextValue {
  isScanning: boolean;
  scanError: string | null;
  needsSetup: boolean;
  scanDiff: ScanDiff | null;
  doScan: (rootPath: string) => Promise<void>;
  handleRescan: () => void;
  handleLoadDemo: () => Promise<void>;
  dismissScanError: () => void;
  dismissScanDiff: () => void;
  setNeedsSetup: (v: boolean) => void;
}

const ScanContext = createContext<ScanContextValue | null>(null);

export function useScan(): ScanContextValue {
  const ctx = useContext(ScanContext);
  if (!ctx) throw new Error('useScan must be used within ScanProvider');
  return ctx;
}

interface ScanProviderProps {
  graph: UseGraphReturn;
  children: ReactNode;
}

export const ScanProvider: FC<ScanProviderProps> = ({ graph, children }) => {
  const [isScanning, setIsScanning] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [scanDiff, setScanDiff] = useState<ScanDiff | null>(null);
  const rootPathRef = useRef<string>(localStorage.getItem('vyuha_root_path') || '');
  const preRescanCounts = useRef<{ nodes: number; edges: number } | null>(null);

  // Show setup if graph has 0 nodes and we haven't scanned yet
  useEffect(() => {
    if (!isScanning && graph.nodes.length === 0 && !rootPathRef.current) {
      setNeedsSetup(true);
    }
  }, [graph.nodes.length, isScanning]);

  // Auto-dismiss scan diff after 15 seconds
  useEffect(() => {
    if (!scanDiff) return;
    const timer = setTimeout(() => setScanDiff(null), 15000);
    return () => clearTimeout(timer);
  }, [scanDiff]);

  const computeScanDiff = useCallback(() => {
    const pre = preRescanCounts.current;
    if (!pre) return;

    const currentNodes = graph.nodes.length;
    const currentEdges = graph.edges.length;

    setScanDiff({
      nodesAdded: Math.max(0, currentNodes - pre.nodes),
      nodesRemoved: Math.max(0, pre.nodes - currentNodes),
      nodesModified: 0,
      edgesAdded: Math.max(0, currentEdges - pre.edges),
      edgesRemoved: Math.max(0, pre.edges - currentEdges),
      totalNodes: currentNodes,
      totalEdges: currentEdges,
    });

    preRescanCounts.current = null;
  }, [graph.nodes.length, graph.edges.length]);

  const pollScanJob = useCallback(
    async (jobId: string) => {
      let attempts = 0;
      let networkFailures = 0;

      const poll = async () => {
        attempts++;
        try {
          const status = await api.getScanStatus(jobId);
          networkFailures = 0;

          if (!status) {
            setIsScanning(false);
            setNeedsSetup(false);
            graph.loadServices();
            return;
          }
          if (status.status === 'complete' || status.status === 'completed') {
            setIsScanning(false);
            setNeedsSetup(false);
            graph.loadServices();
            setTimeout(() => computeScanDiff(), 1500);
          } else if (status.status === 'failed' || status.status === 'error') {
            setIsScanning(false);
            setScanError(status.error || 'Scan failed');
          } else {
            if (attempts < 120) {
              setTimeout(poll, 1000);
            } else {
              setIsScanning(false);
              setScanError(
                'Scan timed out after 2 minutes. The scan may still be running — try refreshing.',
              );
            }
          }
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);

          if (msg.includes('404') || msg.includes('not found')) {
            setIsScanning(false);
            setNeedsSetup(false);
            graph.loadServices();
            return;
          }

          networkFailures++;
          if (networkFailures < 3 && attempts < 120) {
            setTimeout(poll, 2000 * networkFailures);
          } else {
            setIsScanning(false);
            setScanError(`Lost connection while scanning: ${msg}. Try rescanning.`);
          }
        }
      };

      setTimeout(poll, 1000);
    },
    [graph, computeScanDiff],
  );

  const doScan = useCallback(
    async (rootPath: string) => {
      preRescanCounts.current = {
        nodes: graph.nodes.length,
        edges: graph.edges.length,
      };
      setScanDiff(null);
      setScanError(null);
      setIsScanning(true);
      try {
        const res = await api.scan(rootPath);
        rootPathRef.current = rootPath;
        localStorage.setItem('vyuha_root_path', rootPath);
        if (res.job_id) {
          pollScanJob(res.job_id);
        } else {
          setTimeout(() => {
            setIsScanning(false);
            setNeedsSetup(false);
            graph.loadServices();
            setTimeout(() => computeScanDiff(), 1500);
          }, 1500);
        }
      } catch (e) {
        setIsScanning(false);
        setScanError(e instanceof Error ? e.message : String(e));
      }
    },
    [graph, pollScanJob, computeScanDiff],
  );

  const handleRescan = useCallback(() => {
    const saved = rootPathRef.current;
    if (saved && saved !== '__demo__') {
      doScan(saved);
    } else {
      setNeedsSetup(true);
    }
  }, [doScan]);

  const handleLoadDemo = useCallback(async () => {
    setScanError(null);
    setIsScanning(true);
    try {
      await api.loadDemo();
      setIsScanning(false);
      setNeedsSetup(false);
      rootPathRef.current = '__demo__';
      localStorage.setItem('vyuha_root_path', '__demo__');
      graph.loadServices();
    } catch (e) {
      setIsScanning(false);
      setScanError(e instanceof Error ? e.message : String(e));
    }
  }, [graph]);

  const value: ScanContextValue = {
    isScanning,
    scanError,
    needsSetup,
    scanDiff,
    doScan,
    handleRescan,
    handleLoadDemo,
    dismissScanError: () => setScanError(null),
    dismissScanDiff: () => setScanDiff(null),
    setNeedsSetup,
  };

  return <ScanContext.Provider value={value}>{children}</ScanContext.Provider>;
};
