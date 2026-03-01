// ---------------------------------------------------------------------------
// panels/LogIngestion.tsx — Modal for testing runtime log ingestion
// SHADCN: replaced modal/tabs/input/button/label with Dialog, Tabs, Input, Button, Label, Select
// ---------------------------------------------------------------------------

import { useCallback, useState, type FC, type FormEvent } from 'react';
import { api } from '../../api/client';
import type { LogEvent } from '../../types/graph';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'; // SHADCN: replaced modal
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';             // SHADCN: replaced tabs
import { Input } from '@/components/ui/input';                                                // SHADCN: replaced <input>
import { Button } from '@/components/ui/button';                                              // SHADCN: replaced <button>
import { Label } from '@/components/ui/label';                                                // SHADCN: replaced <label>
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'; // SHADCN: replaced <select>

interface LogIngestionProps {
  onClose: () => void;
}

const LogIngestion: FC<LogIngestionProps> = ({ onClose }) => {
  return (
    // SHADCN: replaced custom modal overlay with <Dialog>
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-w-[560px] max-h-[90vh] overflow-y-auto bg-gray-900 border-gray-700">
        <DialogHeader>
          <DialogTitle className="text-sm font-semibold text-gray-100">
            Log Ingestion
          </DialogTitle>
        </DialogHeader>

        {/* SHADCN: replaced custom Tab component with <Tabs> */}
        <Tabs defaultValue="single">
          <TabsList className="bg-gray-800">
            <TabsTrigger value="single">Single Event</TabsTrigger>
            <TabsTrigger value="batch">Batch (JSON)</TabsTrigger>
          </TabsList>

          <TabsContent value="single" className="mt-4">
            <SingleForm />
          </TabsContent>
          <TabsContent value="batch" className="mt-4">
            <BatchForm />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
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
      {/* SHADCN: replaced Field+input with Label+Input */}
      <div className="space-y-1">
        <Label className="text-[11px] text-gray-500">Node ID *</Label>
        <Input
          value={nodeId}
          onChange={(e) => setNodeId(e.target.value)}
          placeholder="function:my-service/pkg.MyFunc"
          className="bg-gray-800 border-gray-700 text-gray-100 placeholder:text-gray-500"
          required
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        {/* SHADCN: replaced <select> with <Select> */}
        <div className="space-y-1">
          <Label className="text-[11px] text-gray-500">Event Type</Label>
          <Select value={eventType} onValueChange={setEventType}>
            <SelectTrigger className="bg-gray-800 border-gray-700 text-gray-100">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="bg-gray-800 border-gray-700">
              <SelectItem value="http_request">http_request</SelectItem>
              <SelectItem value="grpc_call">grpc_call</SelectItem>
              <SelectItem value="db_query">db_query</SelectItem>
              <SelectItem value="queue_publish">queue_publish</SelectItem>
              <SelectItem value="queue_consume">queue_consume</SelectItem>
              <SelectItem value="custom">custom</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label className="text-[11px] text-gray-500">Status</Label>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="bg-gray-800 border-gray-700 text-gray-100">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="bg-gray-800 border-gray-700">
              <SelectItem value="success">success</SelectItem>
              <SelectItem value="error">error</SelectItem>
              <SelectItem value="timeout">timeout</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1">
          <Label className="text-[11px] text-gray-500">Error Message</Label>
          <Input
            value={errorMessage}
            onChange={(e) => setErrorMessage(e.target.value)}
            className="bg-gray-800 border-gray-700 text-gray-100 placeholder:text-gray-500"
            placeholder="connection refused"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-[11px] text-gray-500">Error Code</Label>
          <Input
            value={errorCode}
            onChange={(e) => setErrorCode(e.target.value)}
            className="bg-gray-800 border-gray-700 text-gray-100 placeholder:text-gray-500"
            placeholder="ECONNREFUSED"
          />
        </div>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <div className="space-y-1">
          <Label className="text-[11px] text-gray-500">Latency (ms)</Label>
          <Input
            type="number"
            value={latencyMs}
            onChange={(e) => setLatencyMs(e.target.value)}
            className="bg-gray-800 border-gray-700 text-gray-100 placeholder:text-gray-500"
            placeholder="42"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-[11px] text-gray-500">Trace ID</Label>
          <Input
            value={traceId}
            onChange={(e) => setTraceId(e.target.value)}
            className="bg-gray-800 border-gray-700 text-gray-100 placeholder:text-gray-500"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-[11px] text-gray-500">Span ID</Label>
          <Input
            value={spanId}
            onChange={(e) => setSpanId(e.target.value)}
            className="bg-gray-800 border-gray-700 text-gray-100 placeholder:text-gray-500"
          />
        </div>
      </div>

      {/* SHADCN: replaced <button> with <Button> */}
      <Button
        type="submit"
        disabled={submitting || !nodeId.trim()}
        className="w-full"
      >
        {submitting ? 'Submitting…' : 'Ingest Event'}
      </Button>

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
      <div className="space-y-1">
        <Label className="text-[11px] text-gray-500">JSON Array of events</Label>
        <textarea
          value={json}
          onChange={(e) => setJson(e.target.value)}
          rows={10}
          className="w-full rounded-md border border-gray-700 bg-gray-800 px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500 outline-none focus:border-blue-500 font-mono text-[11px]"
          placeholder={`[\n  {\n    "node_id": "function:...",\n    "event_type": "http_request",\n    "status": "error",\n    "error_message": "timeout"\n  }\n]`}
        />
      </div>

      {/* SHADCN: replaced <button> with <Button> */}
      <Button
        type="submit"
        disabled={submitting || !json.trim()}
        className="w-full"
      >
        {submitting ? 'Submitting…' : 'Ingest Batch'}
      </Button>

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
