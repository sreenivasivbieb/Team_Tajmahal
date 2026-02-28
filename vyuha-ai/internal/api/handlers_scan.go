package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vyuha/vyuha-ai/internal/graph"
	golang "github.com/vyuha/vyuha-ai/internal/parser/golang"
)

// ---------------------------------------------------------------------------
// Scan job tracking
// ---------------------------------------------------------------------------

// scanJobStatus represents the current state of a background scan.
type scanJobStatus struct {
	mu             sync.RWMutex
	JobID          string    `json:"job_id"`
	Status         string    `json:"status"` // "scanning"|"completed"|"failed"
	FilesProcessed int       `json:"files_processed"`
	TotalFiles     int       `json:"total_files"`
	CurrentFile    string    `json:"current_file,omitempty"`
	NodeCount      int       `json:"node_count,omitempty"`
	EdgeCount      int       `json:"edge_count,omitempty"`
	DurationMs     int64     `json:"duration_ms,omitempty"`
	Errors         []string  `json:"errors,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
}

// scanJobSnapshot is a mutex-free copy of scanJobStatus for serialisation.
type scanJobSnapshot struct {
	JobID          string    `json:"job_id"`
	Status         string    `json:"status"`
	FilesProcessed int       `json:"files_processed"`
	TotalFiles     int       `json:"total_files"`
	CurrentFile    string    `json:"current_file,omitempty"`
	NodeCount      int       `json:"node_count,omitempty"`
	EdgeCount      int       `json:"edge_count,omitempty"`
	DurationMs     int64     `json:"duration_ms,omitempty"`
	Errors         []string  `json:"errors,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
}

func (j *scanJobStatus) snapshot() scanJobSnapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return scanJobSnapshot{
		JobID:          j.JobID,
		Status:         j.Status,
		FilesProcessed: j.FilesProcessed,
		TotalFiles:     j.TotalFiles,
		CurrentFile:    j.CurrentFile,
		NodeCount:      j.NodeCount,
		EdgeCount:      j.EdgeCount,
		DurationMs:     j.DurationMs,
		Errors:         j.Errors,
		StartedAt:      j.StartedAt,
		CompletedAt:    j.CompletedAt,
	}
}

// registerScanJob stores a scan job in the server's scan-job map.
func (s *Server) registerScanJob(job *scanJobStatus) {
	s.scanJobs.Store(job.JobID, job)
}

// getScanJob retrieves a scan job by ID from the server's scan-job map.
func (s *Server) getScanJob(jobID string) (*scanJobStatus, bool) {
	v, ok := s.scanJobs.Load(jobID)
	if !ok {
		return nil, false
	}
	return v.(*scanJobStatus), true
}

// ---------------------------------------------------------------------------
// POST /api/scan
// ---------------------------------------------------------------------------

// scanRequest is the JSON body for POST /api/scan.
type scanRequest struct {
	RootPath string `json:"root_path"`
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body: "+err.Error())
		return
	}

	if req.RootPath == "" {
		writeError(w, http.StatusBadRequest, "MISSING_ROOT_PATH", "root_path is required")
		return
	}

	// Validate root_path exists and contains go.mod.
	absRoot, err := filepath.Abs(req.RootPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", "cannot resolve path: "+err.Error())
		return
	}
	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		writeError(w, http.StatusBadRequest, "PATH_NOT_FOUND", "root_path does not exist")
		return
	}
	goModPath := filepath.Join(absRoot, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		writeError(w, http.StatusBadRequest, "NO_GO_MOD",
			"root_path does not contain a go.mod file")
		return
	}

	// Create the scan job.
	jobID := uuid.New().String()
	job := &scanJobStatus{
		JobID:     jobID,
		Status:    "scanning",
		StartedAt: time.Now().UTC(),
	}
	s.registerScanJob(job)

	// Launch scan in background goroutine.
	// Use WithoutCancel so the scan outlives the HTTP request.
	scanCtx := context.WithoutCancel(r.Context())
	go s.runScan(scanCtx, absRoot, job)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"data": map[string]interface{}{
			"status": "scanning",
			"job_id": jobID,
		},
	})
}

// runScan executes the full parse→detect→extract→save pipeline.
func (s *Server) runScan(ctx context.Context, rootPath string, job *scanJobStatus) {
	start := time.Now()

	defer func() {
		if rv := recover(); rv != nil {
			job.mu.Lock()
			job.Status = "failed"
			job.Errors = append(job.Errors, fmt.Sprintf("panic: %v", rv))
			job.CompletedAt = time.Now().UTC()
			job.DurationMs = time.Since(start).Milliseconds()
			job.mu.Unlock()
			log.Printf("scan: panic during scan job %s: %v", job.JobID, rv)
		}
	}()

	// ---- 1. Load existing checksums from storage --------------------
	checksums := s.loadExistingChecksums(ctx)

	// ---- 2. Create GoParser -----------------------------------------
	parser, err := golang.New(rootPath)
	if err != nil {
		s.failJob(job, start, "failed to create parser: "+err.Error())
		return
	}

	// ---- 3. Parse with progress callback ----------------------------
	progress := func(processed, total int, file string) {
		job.mu.Lock()
		job.FilesProcessed = processed
		job.TotalFiles = total
		job.CurrentFile = file
		job.mu.Unlock()

		s.sse.Broadcast(SSEEvent{
			Event: "scan_progress",
			Data: map[string]interface{}{
				"job_id":          job.JobID,
				"files_processed": processed,
				"total":           total,
				"current_file":    file,
			},
		})
	}

	result, err := parser.Parse(ctx, checksums, progress)
	if err != nil {
		s.failJob(job, start, "parse failed: "+err.Error())
		return
	}

	// ---- 4. Run cloud detector --------------------------------------
	cloudDetector := golang.NewCloudDetector(parser.ModulePath(), parser.Fset())
	cloudDetector.Detect(ctx, result, result.ASTFiles)

	// ---- 5. Run data flow extractor ---------------------------------
	dfExtractor := golang.NewDataFlowExtractor(parser.ModulePath(), parser.Fset())
	dfResults, err := dfExtractor.ExtractAll(ctx, result, result.ASTFiles)
	if err != nil {
		log.Printf("scan: data flow extraction warning: %v", err)
		// Non-fatal — continue with what we have.
	}

	// ---- 6. Save all nodes to storage -------------------------------
	if err := s.store.SaveNodes(ctx, result.Nodes); err != nil {
		s.failJob(job, start, "failed to save nodes: "+err.Error())
		return
	}

	// ---- 7. Save all edges to storage -------------------------------
	if err := s.store.SaveEdges(ctx, result.Edges); err != nil {
		s.failJob(job, start, "failed to save edges: "+err.Error())
		return
	}

	// ---- 8. Save data flow records ----------------------------------
	for _, fdf := range dfResults {
		for _, rec := range fdf.DataFlowNodes {
			if err := s.store.SaveDataFlow(ctx, rec); err != nil {
				log.Printf("scan: failed to save data flow record: %v", err)
			}
		}
		// Save data-flow edges.
		if len(fdf.Edges) > 0 {
			if err := s.store.SaveEdges(ctx, fdf.Edges); err != nil {
				log.Printf("scan: failed to save data flow edges: %v", err)
			}
		}
	}

	// ---- 9. Reload graph index from storage -------------------------
	if err := s.index.LoadFromStorage(ctx, s.store); err != nil {
		log.Printf("scan: warning: failed to reload graph index: %v", err)
	}

	// ---- 10. Finalise job -------------------------------------------
	dur := time.Since(start)
	nodeCount := len(result.Nodes)
	edgeCount := len(result.Edges)

	var errStrs []string
	for _, pe := range result.ParseErrors {
		errStrs = append(errStrs, fmt.Sprintf("%s:%d: %s", pe.FilePath, pe.Line, pe.Message))
	}

	job.mu.Lock()
	job.Status = "completed"
	job.NodeCount = nodeCount
	job.EdgeCount = edgeCount
	job.DurationMs = dur.Milliseconds()
	job.Errors = errStrs
	job.CompletedAt = time.Now().UTC()
	job.TotalFiles = result.FileCount
	job.FilesProcessed = result.FileCount
	job.mu.Unlock()

	// ---- 11. Emit SSE completion event ------------------------------
	s.sse.Broadcast(SSEEvent{
		Event: "scan_complete",
		Data: map[string]interface{}{
			"job_id":       job.JobID,
			"nodes":        nodeCount,
			"edges":        edgeCount,
			"duration_ms":  dur.Milliseconds(),
			"files_parsed": result.FileCount,
		},
	})

	log.Printf("scan: completed job %s — %d nodes, %d edges, %d files in %s",
		job.JobID, nodeCount, edgeCount, result.FileCount, dur.Round(time.Millisecond))

	// Prune old completed/failed scan jobs to prevent memory leaks.
	s.pruneOldScanJobs(10)
}

// loadExistingChecksums extracts SHA-256 checksums from file-type nodes.
func (s *Server) loadExistingChecksums(ctx context.Context) map[string]string {
	checksums := make(map[string]string)
	fileNodes := s.index.GetByType(graph.NodeTypeFile)
	for _, n := range fileNodes {
		if n.FilePath != "" && n.Metadata.Checksum != "" {
			checksums[n.FilePath] = n.Metadata.Checksum
		}
	}
	return checksums
}

// failJob marks a scan job as failed and emits an SSE event.
func (s *Server) failJob(job *scanJobStatus, start time.Time, msg string) {
	job.mu.Lock()
	job.Status = "failed"
	job.Errors = append(job.Errors, msg)
	job.DurationMs = time.Since(start).Milliseconds()
	job.CompletedAt = time.Now().UTC()
	job.mu.Unlock()

	s.sse.Broadcast(SSEEvent{
		Event: "scan_failed",
		Data: map[string]interface{}{
			"job_id": job.JobID,
			"error":  msg,
		},
	})

	log.Printf("scan: job %s failed: %s", job.JobID, msg)

	// Prune old completed/failed scan jobs to prevent memory leaks.
	s.pruneOldScanJobs(10)
}

// pruneOldScanJobs removes completed/failed scan jobs from the map,
// keeping only the most recent keepLast entries.  Running jobs are never
// pruned.
func (s *Server) pruneOldScanJobs(keepLast int) {
	type entry struct {
		id        string
		startedAt time.Time
		status    string
	}

	var entries []entry
	s.scanJobs.Range(func(k, v interface{}) bool {
		job := v.(*scanJobStatus)
		snap := job.snapshot()
		entries = append(entries, entry{
			id:        k.(string),
			startedAt: snap.StartedAt,
			status:    snap.Status,
		})
		return true
	})

	// Only prune completed/failed jobs, keep all running.
	var prunable []entry
	for _, e := range entries {
		if e.status == "completed" || e.status == "failed" {
			prunable = append(prunable, e)
		}
	}

	if len(prunable) <= keepLast {
		return
	}

	// Sort by startedAt descending (newest first).
	sort.Slice(prunable, func(i, j int) bool {
		return prunable[i].startedAt.After(prunable[j].startedAt)
	})

	// Delete everything beyond keepLast.
	evicted := 0
	for _, e := range prunable[keepLast:] {
		s.scanJobs.Delete(e.id)
		evicted++
	}
	if evicted > 0 {
		log.Printf("scan: pruned %d old scan job(s)", evicted)
	}
}

// ---------------------------------------------------------------------------
// GET /api/scan/status
// ---------------------------------------------------------------------------

func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_JOB_ID", "job_id query parameter is required")
		return
	}

	job, ok := s.getScanJob(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "no scan job with that ID")
		return
	}

	snap := job.snapshot()
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": snap})
}
