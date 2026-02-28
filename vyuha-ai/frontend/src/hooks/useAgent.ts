// ---------------------------------------------------------------------------
// hooks/useAgent.ts — Manages agent interaction state
// ---------------------------------------------------------------------------

import { useCallback, useState } from 'react';
import { api } from '../api/client';
import type { AgentRun, QueryDecision } from '../types/graph';

export interface UseAgentReturn {
  isRunning: boolean;
  decision: QueryDecision | null;
  agentRun: AgentRun | null;
  error: string | null;
  ask: (question: string) => Promise<QueryDecision | null>;
  reset: () => void;
}

export function useAgent(): UseAgentReturn {
  const [isRunning, setIsRunning] = useState(false);
  const [decision, setDecision] = useState<QueryDecision | null>(null);
  const [agentRun, setAgentRun] = useState<AgentRun | null>(null);
  const [error, setError] = useState<string | null>(null);

  const ask = useCallback(async (question: string) => {
    setIsRunning(true);
    setError(null);
    setDecision(null);
    setAgentRun(null);
    try {
      const result = await api.askQuestion(question);
      setDecision(result);

      if (result.agent_run) {
        setAgentRun(result.agent_run);
      }

      return result;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      setError(msg);
      return null;
    } finally {
      setIsRunning(false);
    }
  }, []);

  const reset = useCallback(() => {
    setDecision(null);
    setAgentRun(null);
    setError(null);
    setIsRunning(false);
  }, []);

  return { isRunning, decision, agentRun, error, ask, reset };
}
