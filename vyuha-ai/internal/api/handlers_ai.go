package api

import (
	"encoding/json"
	"net/http"

	"github.com/vyuha/vyuha-ai/internal/ai"
	"github.com/vyuha/vyuha-ai/internal/query"
)

// ---------------------------------------------------------------------------
// SSE → ai.Broadcaster adapter
// ---------------------------------------------------------------------------

// sseBroadcasterAdapter wraps an SSEBroadcaster so it satisfies the
// ai.Broadcaster interface used by the query and AI packages.
type sseBroadcasterAdapter struct {
	inner *SSEBroadcaster
}

func (a *sseBroadcasterAdapter) Broadcast(event ai.BroadcastEvent) {
	a.inner.Broadcast(SSEEvent{
		Event: event.Event,
		Data:  event.Data,
	})
}

// SSEBroadcasterAsAI returns an ai.Broadcaster backed by the server's SSE hub.
func (s *Server) SSEBroadcasterAsAI() ai.Broadcaster {
	return &sseBroadcasterAdapter{inner: s.sse}
}

// NewAIBroadcaster creates an ai.Broadcaster backed by the given SSE hub.
// Use this when you need a broadcaster before the Server is constructed.
func NewAIBroadcaster(sse *SSEBroadcaster) ai.Broadcaster {
	return &sseBroadcasterAdapter{inner: sse}
}

// ---------------------------------------------------------------------------
// SetQueryLayer attaches the query layer after construction.
// ---------------------------------------------------------------------------

func (s *Server) SetQueryLayer(ql *query.QueryLayer) {
	s.queryLayer = ql
}

// ---------------------------------------------------------------------------
// POST /api/ai/query — natural-language question
// ---------------------------------------------------------------------------

type aiQueryRequest struct {
	Question string `json:"question"`
}

func (s *Server) handleAIQuery(w http.ResponseWriter, r *http.Request) {
	if s.queryLayer == nil {
		writeError(w, http.StatusServiceUnavailable, "AI_NOT_CONFIGURED",
			"AI query layer is not configured (no AI provider set)")
		return
	}

	var req aiQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON",
			"invalid request body: "+err.Error())
		return
	}

	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "MISSING_QUESTION",
			"question field is required")
		return
	}

	ctx := r.Context()
	result, err := s.queryLayer.HandleQuestion(ctx, req.Question)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_ERROR",
			"query failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": result,
	})
}

// ---------------------------------------------------------------------------
// GET /api/ai/jobs/:job_id — AI job status
// ---------------------------------------------------------------------------

func (s *Server) handleAIJobStatus(w http.ResponseWriter, r *http.Request) {
	if s.jobQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "AI_NOT_CONFIGURED",
			"AI job queue is not configured (no AI provider set)")
		return
	}

	jobID := extractPathParam(r.URL.Path, "/api/ai/jobs/")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "MISSING_JOB_ID",
			"job_id is required in the URL path")
		return
	}

	job, ok := s.jobQueue.GetJob(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "JOB_NOT_FOUND",
			"no AI job with that ID")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": job,
	})
}
