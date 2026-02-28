// ---------------------------------------------------------------------------
// panels/LogIngestion.tsx — Modal for testing runtime log ingestion
// ---------------------------------------------------------------------------

import { useCallback, useState, type FC, type FormEvent } from 'react';
import { api } from '../../api/client';
import type { LogEvent } from '../../types/graph';

interface LogIngestionProps {
  onClose: () => void;
}

type Mode = 'single' | 'batch';

const LogIngestion: FC<LogIngestionProps> = ({ onClose }) => {
  const [mode, setMode] = useState<Mode>('single');

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60">
      <div className="w-[560px] max-h-[90vh] overflow-y-auto rounded-lg border border-gray-700 bg-gray-900 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-gray-700 px-5 py-3">
          <h2 className="text-sm font-semibold text-gray-100">
            Log Ingestion
          </h2>
          <button
            onClick={onClose}
            className="rounded p-1 text-gray-400 hover:bg-gray-800 hover:text-gray-200"
          >
            ✕
          </button>
        </div>

        {/* Mode tabs */}
        <div className="flex border-b border-gray-800 px-5">
          <Tab
            label="Single Event"
            active={mode === 'single'}
            onClick={() => setMode('single')}
          />
          <Tab
            label="Batch (JSON)"
            active={mode === 'batch'}
            onClick={() => setMode('batch')}
          />
        </div>

        {/* Body */}
        <div className="px-5 py-4">
          {mode === 'single' ? <SingleForm /> : <BatchForm />}
        </div>
      </div>
    </div>
  );
};

export default LogIngestion;

// ---------------------------------------------------------------------------
// Single event form
// ---------------------------------------------------------------------------

function SingleForm() {
  const [nodeId, setNodeId] = useState('');
  const [eventType, setEventType] = useState('http_request');
  const [status, setStatus] = useState('success');
  const [errorMessage, setErrorMessage] = useState('');
  const [errorCode, setErrorCode] = useState('');
  const [latencyMs, setLatencyMs] = useState('');
  const [traceId, setTraceId] = useState('');
  const [spanId, setSpanId] = useState('');
  const [result, setResult] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (!nodeId.trim()) return;

      const event: LogEvent = {
        node_id: nodeId.trim(),
        event_type: eventType,
        status,
        error_message: errorMessage || undefined,
        error_code: errorCode || undefined,
        latency_ms: latencyMs ? Number(latencyMs) : undefined,
        trace_id: traceId || undefined,
        span_id: spanId || undefined,
      };

      setSubmitting(true);
      setResult(null);
      try {
        const res = await api.ingestLog(event);
        setResult(
          `✓ Node ${res.node_id} updated → status: ${res.runtime_status}`,
        );
      } catch (err) {
        setResult(`✗ ${err instanceof Error ? err.message : String(err)}`);
      } finally {
        setSubmitting(false);
      }
    },
    [nodeId, eventType, status, errorMessage, errorCode, latencyMs, traceId, spanId],
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <Field label="Node ID *">
        <input
          value={nodeId}
          onChange={(e) => setNodeId(e.target.value)}
          placeholder="function:my-service/pkg.MyFunc"
          className="input"
          required
        />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Event Type">
          <select
            value={eventType}
            onChange={(e) => setEventType(e.target.value)}
            className="input"
          >
            <option value="http_request">http_request</option>
            <option value="grpc_call">grpc_call</option>
            <option value="db_query">db_query</option>
            <option value="queue_publish">queue_publish</option>
            <option value="queue_consume">queue_consume</option>
            <option value="custom">custom</option>
          </select>
        </Field>
        <Field label="Status">
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="input"
          >
            <option value="success">success</option>
            <option value="error">error</option>
            <option value="timeout">timeout</option>
          </select>
        </Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Error Message">
          <input
            value={errorMessage}
            onChange={(e) => setErrorMessage(e.target.value)}
            className="input"
            placeholder="connection refused"
          />
        </Field>
        <Field label="Error Code">
          <input
            value={errorCode}
            onChange={(e) => setErrorCode(e.target.value)}
            className="input"
            placeholder="ECONNREFUSED"
          />
        </Field>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <Field label="Latency (ms)">
          <input
            type="number"
            value={latencyMs}
            onChange={(e) => setLatencyMs(e.target.value)}
            className="input"
            placeholder="42"
          />
        </Field>
        <Field label="Trace ID">
          <input
            value={traceId}
            onChange={(e) => setTraceId(e.target.value)}
            className="input"
          />
        </Field>
        <Field label="Span ID">
          <input
            value={spanId}
            onChange={(e) => setSpanId(e.target.value)}
            className="input"
          />
        </Field>
      </div>

      <button
        type="submit"
        disabled={submitting || !nodeId.trim()}
        className="w-full rounded-md bg-blue-600 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-40"
      >
        {submitting ? 'Submitting…' : 'Ingest Event'}
      </button>

      {result && (
        <div
          className={`rounded px-3 py-2 text-xs ${
            result.startsWith('✓')
              ? 'bg-green-900/40 text-green-300'
              : 'bg-red-900/40 text-red-300'
          }`}
        >
          {result}
        </div>
      )}
    </form>
  );
}

// ---------------------------------------------------------------------------
// Batch form
// ---------------------------------------------------------------------------

function BatchForm() {
  const [json, setJson] = useState('');
  const [result, setResult] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      setResult(null);

      let events: LogEvent[];
      try {
        events = JSON.parse(json);
        if (!Array.isArray(events)) throw new Error('Expected a JSON array');
      } catch (err) {
        setResult(`✗ Parse error: ${err instanceof Error ? err.message : String(err)}`);
        return;
      }

      setSubmitting(true);
      try {
        const res = await api.ingestLogs(events);
        setResult(
          `✓ Accepted: ${res.accepted}, Rejected: ${res.rejected}` +
            (res.errors?.length ? `\n  Errors: ${res.errors.join('; ')}` : ''),
        );
      } catch (err) {
        setResult(`✗ ${err instanceof Error ? err.message : String(err)}`);
      } finally {
        setSubmitting(false);
      }
    },
    [json],
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <Field label="JSON Array of events">
        <textarea
          value={json}
          onChange={(e) => setJson(e.target.value)}
          rows={10}
          className="input font-mono text-[11px]"
          placeholder={`[\n  {\n    "node_id": "function:...",\n    "event_type": "http_request",\n    "status": "error",\n    "error_message": "timeout"\n  }\n]`}
        />
      </Field>

      <button
        type="submit"
        disabled={submitting || !json.trim()}
        className="w-full rounded-md bg-blue-600 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-40"
      >
        {submitting ? 'Submitting…' : 'Ingest Batch'}
      </button>

      {result && (
        <div
          className={`whitespace-pre-wrap rounded px-3 py-2 text-xs ${
            result.startsWith('✓')
              ? 'bg-green-900/40 text-green-300'
              : 'bg-red-900/40 text-red-300'
          }`}
        >
          {result}
        </div>
      )}
    </form>
  );
}

// ---------------------------------------------------------------------------
// Shared components
// ---------------------------------------------------------------------------

function Tab({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`border-b-2 px-4 py-2 text-xs font-medium transition-colors ${
        active
          ? 'border-blue-500 text-blue-400'
          : 'border-transparent text-gray-500 hover:text-gray-300'
      }`}
    >
      {label}
    </button>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-0.5 block text-[11px] text-gray-500">{label}</span>
      {children}
    </label>
  );
}
