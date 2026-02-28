package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/runtime"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// ---------------------------------------------------------------------------
// POST /api/ingest/log  — single event ingestion
// ---------------------------------------------------------------------------

// logEventRequest matches the incoming JSON for a single runtime event.
type logEventRequest struct {
	Service      string `json:"service"`
	File         string `json:"file"`
	Function     string `json:"function"`
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id"`
	Status       string `json:"status"`
	EventType    string `json:"event_type"`
	Error        string `json:"error"`
	ErrorCode    string `json:"error_code"`
	LatencyMs    int    `json:"latency_ms"`
	Timestamp    string `json:"timestamp"` // RFC 3339
}

func (s *Server) handleIngestLog(w http.ResponseWriter, r *http.Request) {
	var req logEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON",
			"invalid request body: "+err.Error())
		return
	}

	if req.Service == "" || req.Function == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELDS",
			"service and function are required")
		return
	}

	ctx := r.Context()
	nodeID, err := s.processEvent(ctx, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INGEST_ERROR",
			"failed to process event: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"node_id": nodeID,
			"status":  "accepted",
		},
	})
}

// ---------------------------------------------------------------------------
// POST /api/ingest/logs  — batch event ingestion
// ---------------------------------------------------------------------------

type batchLogRequest struct {
	Events []logEventRequest `json:"events"`
}

func (s *Server) handleIngestLogs(w http.ResponseWriter, r *http.Request) {
	// Limit batch request body to 10 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var req batchLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON",
			"invalid request body: "+err.Error())
		return
	}

	const maxBatchSize = 1000
	if len(req.Events) > maxBatchSize {
		http.Error(w,
			fmt.Sprintf(`{"error":"batch too large: max %d events"}`, maxBatchSize),
			http.StatusRequestEntityTooLarge,
		)
		return
	}

	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "EMPTY_BATCH",
			"events array must not be empty")
		return
	}

	ctx := r.Context()
	var accepted, failed int
	var errors []string

	for i, ev := range req.Events {
		if _, err := s.processEvent(ctx, ev); err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("event[%d]: %s", i, err.Error()))
		} else {
			accepted++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"accepted": accepted,
			"failed":   failed,
			"errors":   errors,
		},
	})
}

// ---------------------------------------------------------------------------
// processEvent — core logic shared by single and batch ingestion
// ---------------------------------------------------------------------------

func (s *Server) processEvent(ctx context.Context, req logEventRequest) (string, error) {
	// ---- 1. Parse timestamp ------------------------------------------
	ts := time.Now().UTC()
	if req.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			ts = parsed
		}
	}

	// ---- 2. Resolve node_id from (service + file + function) ---------
	nodeID := s.resolveNodeID(ctx, req)

	// ---- 3. Create RuntimeEvent and save to storage ------------------
	event := &storage.RuntimeEvent{
		ID:           uuid.New().String(),
		NodeID:       nodeID,
		ServiceID:    "service:" + req.Service,
		FunctionID:   nodeID,
		TraceID:      req.TraceID,
		SpanID:       req.SpanID,
		ParentSpanID: req.ParentSpanID,
		EventType:    req.EventType,
		Status:       req.Status,
		ErrorMessage: req.Error,
		ErrorCode:    req.ErrorCode,
		LatencyMs:    req.LatencyMs,
		Timestamp:    ts,
	}

	if err := s.store.SaveRuntimeEvent(ctx, event); err != nil {
		return "", fmt.Errorf("save runtime event: %w", err)
	}

	// ---- 4. Recompute node status -----------------------------------
	node, nodeFound := s.index.GetNode(nodeID)
	if !nodeFound {
		return nodeID, nil
	}

	newStatus, newErrorCount := computeNodeStatus(req.Status, node.ErrorCount)

	if err := s.store.UpdateNodeStatus(ctx, nodeID, newStatus, newErrorCount); err != nil {
		slog.Error("failed to update node status", "node_id", nodeID, "error", err)
	}
	s.index.UpdateNodeStatus(nodeID, newStatus)

	// Update in-memory node error count.
	node.ErrorCount = newErrorCount
	node.RuntimeStatus = newStatus

	// ---- 5. Error propagation (1 hop) --------------------------------
	var propagated []string
	if req.Status == "error" {
		propagated = s.propagateError(ctx, nodeID)
	}

	// ---- 6. Emit SSE event ------------------------------------------
	s.sse.Broadcast(SSEEvent{
		Event: "node_status_update",
		Data: map[string]interface{}{
			"node_id":       nodeID,
			"status":        newStatus,
			"error_count":   newErrorCount,
			"propagated_to": propagated,
		},
	})

	return nodeID, nil
}

// resolveNodeID tries to find an existing function node matching the event.
// If not found, it creates a ghost node.
func (s *Server) resolveNodeID(ctx context.Context, req logEventRequest) string {
	funcName := req.Function
	if funcName == "" {
		return ""
	}

	// Attempt 1: exact FQN match — O(1).
	if req.Service != "" {
		if id := s.index.GetFuncByFQN(req.Service, funcName); id != "" {
			return id
		}
	}

	// Attempt 2: name-based match — O(k) where k = nodes with same name.
	candidates := s.index.GetFuncByName(funcName)

	if len(candidates) == 1 {
		return candidates[0] // unambiguous
	}

	if len(candidates) > 1 {
		// Disambiguate: prefer candidate whose file path contains the service name.
		if req.Service != "" {
			svcLower := strings.ToLower(req.Service)
			for _, id := range candidates {
				if node, ok := s.index.GetNode(id); ok {
					if strings.Contains(strings.ToLower(node.FilePath), svcLower) {
						return id
					}
				}
			}
		}
		// No file-path match; return first candidate.
		return candidates[0]
	}

	// Not found — create a ghost node with deterministic ID.
	ghostID := fmt.Sprintf("ghost:%s:%s:%s",
		sanitizeGhostID(req.Service),
		sanitizeGhostID(filepath.Base(req.File)),
		sanitizeGhostID(req.Function))

	if _, ok := s.index.GetNode(ghostID); ok {
		return ghostID // already exists from a prior event
	}

	ghostNode := graph.NewNode(ghostID, graph.NodeTypeFunction, funcName)
	ghostNode.FilePath = req.File
	ghostNode.RuntimeStatus = "ghost"
	ghostNode.Language = "go"
	ghostNode.Metadata.Receiver = ""
	ghostNode.Metadata.SourceSnippet = "runtime_only"
	ghostNode.Metadata.DocComment = fmt.Sprintf(
		"Ghost node: seen in runtime logs but not found "+
			"in static analysis for service=%s file=%s",
		req.Service, req.File)

	// --- Attach ghost node to a parent in the hierarchy ---

	// Step 1: Find matching service by name.
	services := s.index.GetByType(graph.NodeTypeService)
	var parentFound bool
	serviceLower := strings.ToLower(req.Service)
	for _, svc := range services {
		if strings.Contains(strings.ToLower(svc.Name), serviceLower) {
			ghostNode.ParentID = svc.ID
			ghostNode.Depth = svc.Depth + 1
			ghostNode.Metadata.IsMainPackage = false
			parentFound = true
			break
		}
	}

	// Step 2: If no service match, try matching service by file path.
	if !parentFound && req.File != "" {
		fileLower := strings.ToLower(req.File)
		for _, svc := range services {
			if strings.Contains(fileLower, strings.ToLower(svc.Name)) {
				ghostNode.ParentID = svc.ID
				ghostNode.Depth = svc.Depth + 1
				ghostNode.Metadata.IsMainPackage = false
				parentFound = true
				break
			}
		}
	}

	// Step 3: Fall back to repository root node.
	if !parentFound {
		repoNodes := s.index.GetByType(graph.NodeTypeRepository)
		if len(repoNodes) > 0 {
			ghostNode.ParentID = repoNodes[0].ID
			ghostNode.Depth = 2 // repo(0) → service(1) → func(2)
		}
	}

	// Save in storage and index.
	if err := s.store.SaveNode(ctx, ghostNode); err != nil {
		slog.Error("failed to save ghost node", "error", err, "ghost_id", ghostID)
	}
	s.index.AddNode(ghostNode)

	slog.Warn("ghost node created",
		"ghost_id", ghostID,
		"parent_id", ghostNode.ParentID,
		"service", req.Service,
		"file", req.File,
		"function", req.Function,
	)
	return ghostID
}

// sanitizeGhostID lowercases a string and replaces / and . with _
// to produce a stable, readable ghost node ID segment.
func sanitizeGhostID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// computeNodeStatus determines the new status and error count.
func computeNodeStatus(eventStatus string, currentErrorCount int) (string, int) {
	switch eventStatus {
	case "error":
		return "error", currentErrorCount + 1
	case "success":
		if currentErrorCount > 0 {
			return "degraded", currentErrorCount
		}
		return "healthy", 0
	default:
		return "healthy", currentErrorCount
	}
}

// propagateError sets callers of the failing node to "degraded" (1 hop).
func (s *Server) propagateError(ctx context.Context, nodeID string) []string {
	callers := s.index.GetCallers(nodeID)
	var propagated []string

	for _, caller := range callers {
		if caller.RuntimeStatus == "error" {
			continue // already in worse state
		}

		s.index.UpdateNodeStatus(caller.ID, "degraded")
		if err := s.store.UpdateNodeStatus(ctx, caller.ID, "degraded", caller.ErrorCount); err != nil {
			slog.Debug("failed to propagate status",
				"target_node", caller.ID,
				"caused_by", nodeID,
				"error", err,
			)
			continue
		}

		propagated = append(propagated, caller.ID)

		s.sse.Broadcast(SSEEvent{
			Event: "node_status_update",
			Data: map[string]interface{}{
				"node_id":      caller.ID,
				"status":       "degraded",
				"error_count":  caller.ErrorCount,
				"caused_by":    nodeID,
			},
		})
	}

	return propagated
}

// ---------------------------------------------------------------------------
// GET /api/runtime/failures?window=1h|24h|7d
// ---------------------------------------------------------------------------

func (s *Server) handleRuntimeFailures(w http.ResponseWriter, r *http.Request) {
	windowStr := r.URL.Query().Get("window")
	if windowStr == "" {
		windowStr = "24h"
	}

	window, err := parseWindow(windowStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WINDOW",
			"window must be a valid duration like 1h, 24h, 7d")
		return
	}

	ctx := r.Context()
	stats, err := s.store.GetTopFailingNodes(ctx, window, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_ERROR",
			"failed to query failures: "+err.Error())
		return
	}

	type failureView struct {
		NodeID       string `json:"node_id"`
		NodeName     string `json:"node_name"`
		FailureCount int    `json:"failure_count"`
		LastFailure  string `json:"last_failure"`
	}

	failures := make([]failureView, 0, len(stats))
	for _, st := range stats {
		failures = append(failures, failureView{
			NodeID:       st.NodeID,
			NodeName:     st.NodeName,
			FailureCount: st.FailureCount,
			LastFailure:  st.LastFailure.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"failures": failures,
			"window":   windowStr,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/runtime/trace/:trace_id
// ---------------------------------------------------------------------------

func (s *Server) handleRuntimeTrace(w http.ResponseWriter, r *http.Request) {
	traceID := extractPathParam(r.URL.Path, "/api/runtime/trace/")
	if traceID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_TRACE_ID",
			"trace_id is required in the URL path")
		return
	}

	ctx := r.Context()
	events, err := s.store.GetTraceEvents(ctx, traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_ERROR",
			"failed to query trace: "+err.Error())
		return
	}

	if len(events) == 0 {
		writeError(w, http.StatusNotFound, "TRACE_NOT_FOUND",
			"no events found for this trace")
		return
	}

	// Compute total duration (first event → last event).
	var durationMs int64
	traceStatus := "success"
	if len(events) >= 2 {
		first := events[0].Timestamp
		last := events[len(events)-1].Timestamp
		durationMs = last.Sub(first).Milliseconds()
	}

	for _, ev := range events {
		if ev.Status == "error" {
			traceStatus = "error"
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"trace_id":    traceID,
			"events":      events,
			"duration_ms": durationMs,
			"status":      traceStatus,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/runtime/node/:node_id/events?limit=N
// ---------------------------------------------------------------------------

func (s *Server) handleRuntimeNodeEvents(w http.ResponseWriter, r *http.Request) {
	// Path: /api/runtime/node/{node_id}/events
	path := r.URL.Path
	const prefix = "/api/runtime/node/"
	const suffix = "/events"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		writeError(w, http.StatusBadRequest, "INVALID_PATH",
			"expected /api/runtime/node/{node_id}/events")
		return
	}

	nodeID := path[len(prefix) : len(path)-len(suffix)]
	if nodeID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_NODE_ID",
			"node_id is required in the URL path")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v >= 1 {
			limit = v
		}
	}
	if limit > 100 {
		limit = 100
	}

	ctx := r.Context()
	events, err := s.store.GetRecentEvents(ctx, nodeID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_ERROR",
			"failed to query events: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"node_id": nodeID,
			"events":  events,
			"total":   len(events),
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseWindow converts shorthand duration strings (e.g. "1h", "24h", "7d")
// to time.Duration.
func parseWindow(s string) (time.Duration, error) {
	// Handle day suffix.
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(numStr)
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid day value: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	// Fall back to Go's time.ParseDuration.
	return time.ParseDuration(s)
}

// ---------------------------------------------------------------------------
// POST /api/ingest/watch — start tailing a log file
// ---------------------------------------------------------------------------

type watchRequest struct {
	FilePath string `json:"file_path"`
}

func (s *Server) handleWatchStart(w http.ResponseWriter, r *http.Request) {
	var req watchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON",
			"invalid request body: "+err.Error())
		return
	}

	if req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FILE_PATH",
			"file_path is required")
		return
	}

	// Validate file exists and is readable.
	info, err := os.Stat(req.FilePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "FILE_NOT_FOUND",
			"file not found or not accessible: "+err.Error())
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "NOT_A_FILE",
			"path is a directory, not a file")
		return
	}

	// Stop any existing tailer first.
	s.stopActiveTailer()

	// Create ingestor with a handler that routes through processEvent.
	ingestor := runtime.NewIngestor(func(ctx context.Context, event runtime.LogEvent) error {
		apiEvent := logEventRequest{
			Service:      event.Service,
			File:         event.File,
			Function:     event.Function,
			TraceID:      event.TraceID,
			SpanID:       event.SpanID,
			ParentSpanID: event.ParentSpanID,
			Status:       event.Status,
			EventType:    event.EventType,
			Error:        event.Error,
			ErrorCode:    event.ErrorCode,
			LatencyMs:    event.LatencyMs,
			Timestamp:    event.Timestamp,
		}
		_, procErr := s.processEvent(ctx, apiEvent)
		return procErr
	})

	// Create and start the tailer.
	tailer := runtime.NewLogTailer(req.FilePath, ingestor)
	if err := tailer.Start(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "TAILER_START_ERROR",
			"failed to start file tailer: "+err.Error())
		return
	}

	s.tailerMu.Lock()
	s.activeTailer = tailer
	s.tailerCancel = nil // context-based cancellation handled by Stop()
	s.tailerMu.Unlock()

	slog.Info("file watch started", "file", req.FilePath)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"status": "watching",
			"file":   req.FilePath,
		},
	})
}

// ---------------------------------------------------------------------------
// DELETE /api/ingest/watch — stop the active file tailer
// ---------------------------------------------------------------------------

func (s *Server) handleWatchStop(w http.ResponseWriter, r *http.Request) {
	s.stopActiveTailer()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"status": "stopped",
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/ingest/watch — get tailer status
// ---------------------------------------------------------------------------

func (s *Server) handleWatchStatus(w http.ResponseWriter, r *http.Request) {
	s.tailerMu.Lock()
	tailer := s.activeTailer
	s.tailerMu.Unlock()

	if tailer == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"active": false,
			},
		})
		return
	}

	status := tailer.Status()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": status,
	})
}

// stopActiveTailer stops and clears the current tailer, if any.
func (s *Server) stopActiveTailer() {
	s.tailerMu.Lock()
	tailer := s.activeTailer
	s.activeTailer = nil
	s.tailerMu.Unlock()

	if tailer != nil {
		tailer.Stop()
		slog.Info("file watch stopped", "file", tailer.FilePath())
	}
}
