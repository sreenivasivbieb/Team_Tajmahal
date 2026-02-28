// ---------------------------------------------------------------------------
// panels/QueryBar.tsx — Top query bar for natural-language questions
// ---------------------------------------------------------------------------

import { useCallback, useEffect, useState, type FC, type FormEvent } from 'react';
import { api } from '../../api/client';
import type { GraphNode } from '../../types/graph';

interface QueryBarProps {
  onResult: (result: Awaited<ReturnType<typeof api.askQuestion>>) => void;
  isRunning: boolean;
}

const QueryBar: FC<QueryBarProps> = ({ onResult, isRunning }) => {
  const [question, setQuestion] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [suggestions, setSuggestions] = useState<string[]>([]);

  // Load dynamic suggestions from graph data
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const services = await api.getServices();
        const chips: string[] = [];
        for (const svc of services.slice(0, 3)) {
          chips.push(`How does ${svc.name} work?`);
          if (svc.error_count > 0) {
            chips.push(`Why is ${svc.name} failing?`);
          }
        }
        // Try to find a function for the "What calls X?" suggestion
        const fns = await api.searchNodes('%', 'function').catch(() => [] as GraphNode[]);
        if (fns.length > 0) {
          chips.push(`What calls ${fns[0].name}?`);
        }
        if (services.length > 0) {
          chips.push(`What breaks if ${services[0].name} goes down?`);
        }
        if (!cancelled) setSuggestions(chips.slice(0, 4));
      } catch {
        if (!cancelled)
          setSuggestions([
            'How does the main service work?',
            'What are the top failures?',
          ]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      const q = question.trim();
      if (!q || submitting) return;

      setSubmitting(true);
      try {
        const result = await api.askQuestion(q);
        onResult(result);
      } catch {
        /* error handled by caller */
      } finally {
        setSubmitting(false);
      }
    },
    [question, submitting, onResult],
  );

  const handleChip = (text: string) => {
    setQuestion(text);
  };

  const busy = submitting || isRunning;

  return (
    <div className="border-b border-gray-800 bg-gray-900/80 px-4 py-2 backdrop-blur">
      <form onSubmit={handleSubmit} className="flex items-center gap-2">
        <input
          type="text"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="Ask anything about your codebase…"
          className="flex-1 rounded-md border border-gray-700 bg-gray-800 px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500 outline-none focus:border-blue-500"
          disabled={busy}
        />
        <button
          type="submit"
          disabled={busy || !question.trim()}
          className="rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-40"
        >
          {busy ? (
            <span className="flex items-center gap-1">
              <span className="h-3 w-3 animate-spin rounded-full border-2 border-white/40 border-t-white" />
              Thinking…
            </span>
          ) : (
            'Ask'
          )}
        </button>
      </form>

      {/* Suggestion chips */}
      {suggestions.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {suggestions.map((s, i) => (
            <button
              key={i}
              onClick={() => handleChip(s)}
              className="rounded-full border border-gray-700 px-2.5 py-0.5 text-[11px] text-gray-400 hover:border-blue-500 hover:text-blue-300"
            >
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
};

export default QueryBar;
