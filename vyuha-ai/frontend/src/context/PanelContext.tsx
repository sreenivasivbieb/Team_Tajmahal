import {
  createContext,
  useCallback,
  useContext,
  useState,
  type FC,
  type ReactNode,
} from 'react';
import type { UseSSEReturn } from '../hooks/useSSE';

interface PanelContextValue {
  showAgent: boolean;
  showIngestion: boolean;
  openAgentPanel: () => void;
  closeAgentPanel: () => void;
  openIngestionPanel: () => void;
  closeIngestionPanel: () => void;
  closeTopPanel: () => void;
}

const PanelContext = createContext<PanelContextValue | null>(null);

export function usePanels(): PanelContextValue {
  const ctx = useContext(PanelContext);
  if (!ctx) throw new Error('usePanels must be used within PanelProvider');
  return ctx;
}

interface PanelProviderProps {
  sse: UseSSEReturn;
  children: ReactNode;
}

export const PanelProvider: FC<PanelProviderProps> = ({ sse, children }) => {
  const [showAgent, setShowAgent] = useState(false);
  const [showIngestion, setShowIngestion] = useState(false);

  const openAgentPanel = useCallback(() => setShowAgent(true), []);
  const closeAgentPanel = useCallback(() => {
    setShowAgent(false);
    sse.clearAgentSteps();
  }, [sse]);
  const openIngestionPanel = useCallback(() => setShowIngestion(true), []);
  const closeIngestionPanel = useCallback(() => setShowIngestion(false), []);

  const closeTopPanel = useCallback(() => {
    if (showAgent) {
      setShowAgent(false);
      sse.clearAgentSteps();
    } else if (showIngestion) {
      setShowIngestion(false);
    }
  }, [showAgent, showIngestion, sse]);

  const value: PanelContextValue = {
    showAgent,
    showIngestion,
    openAgentPanel,
    closeAgentPanel,
    openIngestionPanel,
    closeIngestionPanel,
    closeTopPanel,
  };

  return <PanelContext.Provider value={value}>{children}</PanelContext.Provider>;
};
