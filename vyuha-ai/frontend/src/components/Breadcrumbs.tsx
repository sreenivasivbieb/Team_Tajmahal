import { memo, useCallback, useEffect, useState, type FC } from 'react';
import { api } from '../api/client';
import type { GraphNode } from '../types/graph';

interface BreadcrumbsProps {
  /** The currently selected or focused node. Null means no breadcrumb shown. */
  currentNode: GraphNode | null;
  /** Called when user clicks a breadcrumb segment to navigate to that node. */
  onNavigate: (nodeId: string) => void;
}

interface Crumb {
  id: string;
  name: string;
  type: string;
}

const TYPE_ICONS: Record<string, string> = {
  repository: '🏠',
  service: '⚙️',
  package: '📦',
  file: '📄',
  function: 'ƒ',
  struct: '🧱',
  interface: '🔌',
  cloud_service: '☁️',
  data_flow: '🔀',
  runtime_instance: '⚡',
};

const Breadcrumbs: FC<BreadcrumbsProps> = ({ currentNode, onNavigate }) => {
  const [crumbs, setCrumbs] = useState<Crumb[]>([]);

  // Build breadcrumb chain by walking parent_id up the hierarchy
  useEffect(() => {
    if (!currentNode) {
      setCrumbs([]);
      return;
    }

    let cancelled = false;

    async function buildChain(node: GraphNode) {
      const chain: Crumb[] = [];
      let current: GraphNode | null = node;

      // Walk up the parent chain, max 10 levels to prevent infinite loops
      let depth = 0;
      while (current && depth < 10) {
        chain.unshift({
          id: current.id,
          name: extractShortName(current),
          type: current.type,
        });

        if (!current.parent_id || current.parent_id === '' || current.parent_id === current.id) {
          break;
        }

        try {
          const parent = await api.getNode(current.parent_id);
          if (cancelled) return;
          current = parent.node as unknown as GraphNode;
        } catch {
          // Parent not found — stop walking
          break;
        }
        depth++;
      }

      if (!cancelled) {
        setCrumbs(chain);
      }
    }

    buildChain(currentNode);
    return () => {
      cancelled = true;
    };
  }, [currentNode?.id, currentNode?.parent_id]);

  if (crumbs.length === 0) return null;

  return (
    <div className="flex items-center gap-0.5 overflow-x-auto px-3 py-1.5 text-xs scrollbar-none">
      {crumbs.map((crumb, i) => {
        const isLast = i === crumbs.length - 1;
        const icon = TYPE_ICONS[crumb.type] ?? '●';

        return (
          <span key={crumb.id} className="flex shrink-0 items-center gap-0.5">
            {i > 0 && (
              <span className="mx-1 text-gray-600 select-none">/</span>
            )}
            {isLast ? (
              <span className="flex items-center gap-1 rounded bg-gray-800 px-1.5 py-0.5 font-medium text-gray-200">
                <span className="text-[10px]">{icon}</span>
                <span>{crumb.name}</span>
              </span>
            ) : (
              <button
                onClick={() => onNavigate(crumb.id)}
                className="flex items-center gap-1 rounded px-1.5 py-0.5 text-gray-400 transition-colors hover:bg-gray-800 hover:text-gray-200"
                title={`Navigate to ${crumb.name} (${crumb.type})`}
              >
                <span className="text-[10px]">{icon}</span>
                <span>{crumb.name}</span>
              </button>
            )}
          </span>
        );
      })}
    </div>
  );
};

/** Extract a short display name from a node */
function extractShortName(node: GraphNode): string {
  if (node.name) {
    // For functions like "Server.handleScan", keep as-is
    // For packages like "github.com/vyuha/vyuha-ai/internal/api", take last segment
    if (node.type === 'package' || node.type === 'service' || node.type === 'repository') {
      const parts = node.name.split('/');
      return parts[parts.length - 1] || node.name;
    }
    return node.name;
  }
  // Fallback: extract from ID
  const parts = node.id.split(/[/:]/);
  return parts[parts.length - 1] || node.id;
}

export default memo(Breadcrumbs);
