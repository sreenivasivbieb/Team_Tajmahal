// ---------------------------------------------------------------------------
// hooks/useNodeErrorAnimation.ts — Listen for vyuha:node_error events and
// return a CSS animation class for the given node, auto-cleared after 2s.
// ---------------------------------------------------------------------------

import { useEffect, useRef, useState } from 'react';

/**
 * Returns `'animate-error-pulse'`, `'animate-degraded-pulse'`, or `''`.
 * The class is applied for 2 seconds after the matching custom event fires.
 */
export function useNodeErrorAnimation(nodeId: string): string {
  const [animClass, setAnimClass] = useState('');
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    function handleNodeError(e: Event) {
      const detail = (e as CustomEvent<{ nodeId: string; status: string }>).detail;
      if (detail.nodeId !== nodeId) return;

      // Clear any existing timer so a rapid re-fire restarts the animation
      if (timerRef.current) clearTimeout(timerRef.current);

      const cls =
        detail.status === 'error'
          ? 'animate-error-pulse'
          : detail.status === 'degraded'
            ? 'animate-degraded-pulse'
            : '';

      setAnimClass(cls);

      // Remove class after 2 seconds
      timerRef.current = setTimeout(() => {
        setAnimClass('');
        timerRef.current = null;
      }, 2000);
    }

    window.addEventListener('vyuha:node_error', handleNodeError);
    return () => {
      window.removeEventListener('vyuha:node_error', handleNodeError);
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [nodeId]);

  return animClass;
}
