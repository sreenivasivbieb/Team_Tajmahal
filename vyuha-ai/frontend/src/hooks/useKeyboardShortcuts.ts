import { useEffect } from 'react';

export interface KeyboardShortcutHandlers {
  onFocusQueryBar: () => void;
  onClosePanel: () => void;
  onRescan: () => void;
  onFitView: () => void;
}

/**
 * Registers global keyboard shortcuts for the application.
 * - Ctrl/Cmd + K       → Focus query bar
 * - Escape             → Close open panels
 * - Ctrl/Cmd + Shift + R → Rescan repository
 * - F (when no input focused) → Fit graph to view
 */
export function useKeyboardShortcuts(handlers: KeyboardShortcutHandlers): void {
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey;
      const target = e.target as HTMLElement;
      const isInputFocused =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      // Ctrl/Cmd + K → Focus query bar
      if (mod && e.key === 'k') {
        e.preventDefault();
        handlers.onFocusQueryBar();
        return;
      }

      // Escape → Close panels
      if (e.key === 'Escape') {
        e.preventDefault();
        handlers.onClosePanel();
        return;
      }

      // Ctrl/Cmd + Shift + R → Rescan
      if (mod && e.shiftKey && (e.key === 'r' || e.key === 'R')) {
        e.preventDefault();
        handlers.onRescan();
        return;
      }

      // F (when not in an input) → Fit view
      if (!isInputFocused && (e.key === 'f' || e.key === 'F') && !mod && !e.shiftKey) {
        e.preventDefault();
        handlers.onFitView();
        return;
      }
    }

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handlers]);
}
