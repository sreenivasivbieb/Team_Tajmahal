import {
  createContext,
  useCallback,
  useContext,
  useState,
  type FC,
  type ReactNode,
} from 'react';
import type { GraphNode } from '../types/graph';

interface NavigationContextValue {
  /** Node ID that the canvas should navigate/zoom to. Null means no pending navigation. */
  navigateToNodeId: string | null;
  /** The currently selected node (drives the breadcrumb trail). */
  breadcrumbNode: GraphNode | null;
  /** Request the canvas to navigate to a specific node. */
  navigateTo: (nodeId: string) => void;
  /** Called by the canvas after it has completed the navigation animation. */
  clearNavigation: () => void;
  /** Called by the canvas or detail panel when a node is selected/deselected. */
  selectNode: (node: GraphNode | null) => void;
}

const NavigationContext = createContext<NavigationContextValue | null>(null);

export function useNavigation(): NavigationContextValue {
  const ctx = useContext(NavigationContext);
  if (!ctx) throw new Error('useNavigation must be used within NavigationProvider');
  return ctx;
}

interface NavigationProviderProps {
  children: ReactNode;
}

export const NavigationProvider: FC<NavigationProviderProps> = ({ children }) => {
  const [navigateToNodeId, setNavigateToNodeId] = useState<string | null>(null);
  const [breadcrumbNode, setBreadcrumbNode] = useState<GraphNode | null>(null);

  const navigateTo = useCallback((nodeId: string) => {
    setNavigateToNodeId(nodeId);
  }, []);

  const clearNavigation = useCallback(() => {
    setNavigateToNodeId(null);
  }, []);

  const selectNode = useCallback((node: GraphNode | null) => {
    setBreadcrumbNode(node);
  }, []);

  const value: NavigationContextValue = {
    navigateToNodeId,
    breadcrumbNode,
    navigateTo,
    clearNavigation,
    selectNode,
  };

  return (
    <NavigationContext.Provider value={value}>{children}</NavigationContext.Provider>
  );
};
