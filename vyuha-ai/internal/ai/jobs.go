package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Broadcaster interface — avoids circular import with api package
// ---------------------------------------------------------------------------

// Broadcaster is a minimal interface for pushing events to connected clients.
// The api.SSEBroadcaster satisfies this interface.
type Broadcaster interface {
	Broadcast(event BroadcastEvent)
}

// BroadcastEvent mirrors api.SSEEvent without importing the api package.
type BroadcastEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// ---------------------------------------------------------------------------
// Job types
// ---------------------------------------------------------------------------

// JobKind identifies the type of AI work to perform.
type JobKind string

const (
	JobExplainFunction  JobKind = "explain_function"
	JobWhyFailing       JobKind = "why_failing"
	JobServiceOverview  JobKind = "service_overview"
	JobDataLineage      JobKind = "data_lineage"
	JobEmbedNode        JobKind = "embed_node"
	JobEmbedAll         JobKind = "embed_all"
	JobSimilaritySearch JobKind = "similarity_search"
	JobFreeformQuery    JobKind = "freeform_query"
)

// JobStatus tracks the lifecycle of an AI job.
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusRunning    JobStatus = "running"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

// AIJob represents a queued or completed AI task.
type AIJob struct {
	ID        string          `json:"id"`
	Kind      JobKind         `json:"kind"`
	Status    JobStatus       `json:"status"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	StartedAt *time.Time      `json:"started_at,omitempty"`
	DoneAt    *time.Time      `json:"done_at,omitempty"`
}

// ---------------------------------------------------------------------------
// JobQueue
// ---------------------------------------------------------------------------

const defaultQueueSize = 256

// JobQueue manages asynchronous AI jobs with a background worker pool.
type JobQueue struct {
	mu   sync.RWMutex
	jobs map[string]*AIJob

	queue       chan string
	provider    Provider
	embedSvc    *EmbeddingService
	broadcaster Broadcaster

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	closeOnce sync.Once
}

// NewJobQueue creates a job queue with the given number of workers.
// Pass nil for broadcaster if SSE notifications are not needed.
func NewJobQueue(
	provider Provider,
	embedSvc *EmbeddingService,
	broadcaster Broadcaster,
	workers int,
) *JobQueue {
	if workers <= 0 {
		workers = 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	q := &JobQueue{
		jobs:        make(map[string]*AIJob),
		queue:       make(chan string, defaultQueueSize),
		provider:    provider,
		embedSvc:    embedSvc,
		broadcaster: broadcaster,
		ctx:         ctx,
		cancel:      cancel,
	}

	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}

	// Background eviction of completed/failed jobs older than 1 hour.
	q.wg.Add(1)
	go q.evictExpiredJobs()

	slog.Info("ai job queue started", "workers", workers)
	return q
}

// Enqueue creates a new job and puts it on the processing queue.
// It returns the job ID immediately.
func (q *JobQueue) Enqueue(kind JobKind, params json.RawMessage) (string, error) {
	job := &AIJob{
		ID:        uuid.New().String(),
		Kind:      kind,
		Status:    JobStatusPending,
		Params:    params,
		CreatedAt: time.Now().UTC(),
	}

	q.mu.Lock()
	q.jobs[job.ID] = job
	q.mu.Unlock()

	select {
	case q.queue <- job.ID:
		slog.Debug("ai job enqueued", "job_id", job.ID, "kind", string(kind))
	default:
		q.mu.Lock()
		job.Status = JobStatusFailed
		job.Error = "queue full"
		now := time.Now().UTC()
		job.DoneAt = &now
		q.mu.Unlock()
		return job.ID, fmt.Errorf("ai/jobs: queue full")
	}

	return job.ID, nil
}

// GetJob returns the current state of a job.
func (q *JobQueue) GetJob(id string) (*AIJob, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	j, ok := q.jobs[id]
	if !ok {
		return nil, false
	}
	// Return a copy.
	cp := *j
	return &cp, true
}

// ListJobs returns all jobs, ordered by creation time descending.
func (q *JobQueue) ListJobs(limit int) []*AIJob {
	q.mu.RLock()
	defer q.mu.RUnlock()

	all := make([]*AIJob, 0, len(q.jobs))
	for _, j := range q.jobs {
		cp := *j
		all = append(all, &cp)
	}

	// Sort descending by CreatedAt — O(n log n).
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// Close signals workers to stop and waits for them to finish.
// Safe to call multiple times.
func (q *JobQueue) Close() {
	q.closeOnce.Do(func() {
		q.cancel()
		close(q.queue)
		q.wg.Wait()
		slog.Info("ai job queue shut down")
	})
}

// evictExpiredJobs removes completed/failed jobs older than 1 hour
// every 15 minutes. Runs as a background goroutine until ctx is cancelled.
func (q *JobQueue) evictExpiredJobs() {
	defer q.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().UTC().Add(-1 * time.Hour)
			var evicted int

			q.mu.Lock()
			for id, job := range q.jobs {
				if (job.Status == JobStatusCompleted || job.Status == JobStatusFailed) &&
					job.DoneAt != nil && job.DoneAt.Before(cutoff) {
					delete(q.jobs, id)
					evicted++
				}
			}
			q.mu.Unlock()

			if evicted > 0 {
				slog.Debug("job eviction",
					"evicted", evicted,
					"remaining", q.jobCount(),
				)
			}
		}
	}
}

// jobCount returns the number of jobs in the map (lock-safe).
func (q *JobQueue) jobCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.jobs)
}

// ---------------------------------------------------------------------------
// Worker loop
// ---------------------------------------------------------------------------

func (q *JobQueue) worker(id int) {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case jobID, ok := <-q.queue:
			if !ok {
				return
			}
			q.processJob(jobID, id)
		}
	}
}

func (q *JobQueue) processJob(jobID string, workerID int) {
	q.mu.Lock()
	job, ok := q.jobs[jobID]
	if !ok {
		q.mu.Unlock()
		return
	}
	job.Status = JobStatusRunning
	now := time.Now().UTC()
	job.StartedAt = &now
	q.mu.Unlock()

	slog.Debug("ai job processing", "worker", workerID, "job_id", jobID, "kind", string(job.Kind))
	q.broadcast("ai:job_started", map[string]interface{}{
		"job_id": jobID,
		"kind":   job.Kind,
	})

	result, err := q.executeJob(q.ctx, job)

	q.mu.Lock()
	doneAt := time.Now().UTC()
	job.DoneAt = &doneAt
	if err != nil {
		job.Status = JobStatusFailed
		job.Error = err.Error()
		slog.Error("ai job failed", "worker", workerID, "job_id", jobID, "error", err)
	} else {
		job.Status = JobStatusCompleted
		job.Result = result
		slog.Info("ai job complete", "worker", workerID, "job_id", jobID, "kind", string(job.Kind))
	}
	q.mu.Unlock()

	q.broadcast("ai:job_completed", map[string]interface{}{
		"job_id": jobID,
		"kind":   job.Kind,
		"status": job.Status,
		"error":  job.Error,
	})
}

// ---------------------------------------------------------------------------
// Job execution dispatch
// ---------------------------------------------------------------------------

func (q *JobQueue) executeJob(ctx context.Context, job *AIJob) (json.RawMessage, error) {
	switch job.Kind {
	case JobExplainFunction, JobWhyFailing, JobServiceOverview, JobDataLineage, JobFreeformQuery:
		return q.executeGenerateJob(ctx, job)
	case JobEmbedNode:
		return q.executeEmbedNodeJob(ctx, job)
	case JobEmbedAll:
		return q.executeEmbedAllJob(ctx, job)
	case JobSimilaritySearch:
		return q.executeSimilaritySearchJob(ctx, job)
	default:
		return nil, fmt.Errorf("unknown job kind %q", job.Kind)
	}
}

// ---------------------------------------------------------------------------
// Generate jobs (LLM text generation)
// ---------------------------------------------------------------------------

type generateJobParams struct {
	Messages []Message       `json:"messages"`
	Options  GenerateOptions `json:"options,omitempty"`
}

type generateJobResult struct {
	Response string `json:"response"`
}

func (q *JobQueue) executeGenerateJob(ctx context.Context, job *AIJob) (json.RawMessage, error) {
	var params generateJobParams
	if err := json.Unmarshal(job.Params, &params); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}

	if len(params.Messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	opts := params.Options
	if opts.MaxTokens == 0 {
		opts = DefaultGenerateOptions()
	}

	msg, err := q.provider.Generate(ctx, params.Messages, opts)
	if err != nil {
		return nil, err
	}

	return json.Marshal(generateJobResult{Response: msg.Content})
}

// ---------------------------------------------------------------------------
// Embed node job
// ---------------------------------------------------------------------------

type embedNodeParams struct {
	NodeID string `json:"node_id"`
	Force  bool   `json:"force,omitempty"`
}

type embedNodeResult struct {
	EmbeddingID string `json:"embedding_id"`
	Dimensions  int    `json:"dimensions"`
}

func (q *JobQueue) executeEmbedNodeJob(ctx context.Context, job *AIJob) (json.RawMessage, error) {
	if q.embedSvc == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}

	var params embedNodeParams
	if err := json.Unmarshal(job.Params, &params); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}

	emb, err := q.embedSvc.EmbedNode(ctx, params.NodeID, params.Force)
	if err != nil {
		return nil, err
	}

	return json.Marshal(embedNodeResult{
		EmbeddingID: emb.ID,
		Dimensions:  emb.Dimensions,
	})
}

// ---------------------------------------------------------------------------
// Embed-all job
// ---------------------------------------------------------------------------

type embedAllParams struct {
	Force bool `json:"force,omitempty"`
}

type embedAllResult struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
}

func (q *JobQueue) executeEmbedAllJob(ctx context.Context, job *AIJob) (json.RawMessage, error) {
	if q.embedSvc == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}

	var params embedAllParams
	if err := json.Unmarshal(job.Params, &params); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}

	var lastProg EmbedProgress
	err := q.embedSvc.EmbedAll(ctx, params.Force, func(p EmbedProgress) {
		lastProg = p
		// Broadcast periodic progress.
		q.broadcast("ai:embed_progress", map[string]interface{}{
			"job_id":    job.ID,
			"total":     p.Total,
			"completed": p.Completed,
			"skipped":   p.Skipped,
			"errors":    p.Errors,
		})
	})
	if err != nil {
		return nil, err
	}

	return json.Marshal(embedAllResult{
		Total:     lastProg.Total,
		Completed: lastProg.Completed,
		Skipped:   lastProg.Skipped,
		Errors:    lastProg.Errors,
	})
}

// ---------------------------------------------------------------------------
// Similarity search job
// ---------------------------------------------------------------------------

type similaritySearchParams struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

func (q *JobQueue) executeSimilaritySearchJob(ctx context.Context, job *AIJob) (json.RawMessage, error) {
	if q.embedSvc == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}

	var params similaritySearchParams
	if err := json.Unmarshal(job.Params, &params); err != nil {
		return nil, fmt.Errorf("unmarshal params: %w", err)
	}

	results, err := q.embedSvc.SimilaritySearch(ctx, params.Query, params.TopK)
	if err != nil {
		return nil, err
	}

	return json.Marshal(results)
}

// ---------------------------------------------------------------------------
// Broadcast helper
// ---------------------------------------------------------------------------

func (q *JobQueue) broadcast(event string, data interface{}) {
	if q.broadcaster == nil {
		return
	}
	q.broadcaster.Broadcast(BroadcastEvent{
		Event: event,
		Data:  data,
	})
}
