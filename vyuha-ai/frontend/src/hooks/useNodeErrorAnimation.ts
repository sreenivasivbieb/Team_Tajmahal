// ---------------------------------------------------------------------------
// hooks/useNodeErrorAnimation.ts — Returns a CSS class that pulses when a
// node transitions into an error state. Uses a simple time-window check.
// ---------------------------------------------------------------------------

import { useEffect, useRef, useState } from 'react';

/**
 * Returns a class name string that applies a brief "shake" / "pulse-red"
 * animation when the node is first detected in an error state.
 * After ~2 seconds the animation class is removed so the node stays still.
 */
export function useNodeErrorAnimation(nodeId: string): string {
  const [animating, setAnimating] = useState(false);
  const prevId = useRef(nodeId);

  useEffect(() => {
    if (nodeId !== prevId.current) {
      prevId.current = nodeId;
      setAnimating(true);
      const timer = setTimeout(() => setAnimating(false), 2000);
      return () => clearTimeout(timer);
    }
  }, [nodeId]);

  return animating ? 'animate-pulse' : '';
}
