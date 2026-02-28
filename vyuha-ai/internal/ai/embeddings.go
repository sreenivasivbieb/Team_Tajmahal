package ai

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// ---------------------------------------------------------------------------
// EmbeddingService
// ---------------------------------------------------------------------------

const embeddingWorkers = 5

// EmbeddingService generates, stores and searches vector embeddings for
// graph nodes. It delegates actual embedding generation to a Provider.
type EmbeddingService struct {
	provider Provider
	store    *storage.Storage
	index    *graph.GraphIndex

	// cache holds all embeddings in memory for similarity search.
	mu    sync.RWMutex
	cache []*storage.Embedding

	// expectedDimensions is learned from the first embedding and used to
	// validate that all subsequent embeddings use the same model.
	expectedDimensions int
}

// NewEmbeddingService creates an EmbeddingService and loads existing
// embeddings from storage into its in-memory cache.
func NewEmbeddingService(
	ctx context.Context,
	provider Provider,
	store *storage.Storage,
	index *graph.GraphIndex,
) (*EmbeddingService, error) {
	svc := &EmbeddingService{
		provider: provider,
		store:    store,
		index:    index,
	}

	// Pre-load cached embeddings.
	all, err := store.GetAllEmbeddings(ctx)
	if err != nil {
		return nil, fmt.Errorf("ai/embeddings: preload: %w", err)
	}
	svc.cache = all
	slog.Info("embeddings loaded", "count", len(all))

	return svc, nil
}

// ---------------------------------------------------------------------------
// EmbedNode — embed a single node
// ---------------------------------------------------------------------------

// EmbedNode generates an embedding for the given node and persists it.
// If the node already has an up-to-date embedding, it is skipped (unless force=true).
func (s *EmbeddingService) EmbedNode(ctx context.Context, nodeID string, force bool) (*storage.Embedding, error) {
	node, ok := s.index.GetNode(nodeID)
	if !ok || node == nil {
		return nil, fmt.Errorf("ai/embeddings: node %q not found", nodeID)
	}

	// Skip if already embedded and not forced.
	if !force && node.Metadata.AIEmbeddingID != "" {
		existing, err := s.store.GetEmbedding(ctx, nodeID)
		if err == nil && existing != nil {
			return existing, nil
		}
	}

	content := buildNodeContent(node)
	if content == "" {
		return nil, fmt.Errorf("ai/embeddings: empty content for node %q", nodeID)
	}

	vec, err := s.provider.Embed(ctx, content, "")
	if err != nil {
		return nil, fmt.Errorf("ai/embeddings: embed node %q: %w", nodeID, err)
	}

	// Learn dimensions from the first embedding.
	if s.expectedDimensions == 0 && len(vec) > 0 {
		s.expectedDimensions = len(vec)
		slog.Info("embedding dimensions set",
			"dimensions", s.expectedDimensions,
			"model", s.provider.Name(),
		)
	}

	// Validate subsequent embeddings have matching dimensions.
	if s.expectedDimensions > 0 && len(vec) != s.expectedDimensions {
		return nil, fmt.Errorf(
			"embedding dimension mismatch: expected %d got %d. "+
				"Inconsistent model usage detected",
			s.expectedDimensions, len(vec),
		)
	}

	emb := &storage.Embedding{
		ID:         uuid.New().String(),
		NodeID:     nodeID,
		Content:    content,
		Vector:     vec,
		Model:      s.provider.Name(),
		Dimensions: len(vec),
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.store.SaveEmbedding(ctx, emb); err != nil {
		return nil, fmt.Errorf("ai/embeddings: save: %w", err)
	}

	// Update the in-memory cache.
	s.mu.Lock()
	s.cache = append(s.cache, emb)
	s.mu.Unlock()

	slog.Debug("node embedded", "node_id", nodeID, "dimensions", len(vec))
	return emb, nil
}

// ---------------------------------------------------------------------------
// EmbedAll — embed every eligible node using a worker pool
// ---------------------------------------------------------------------------

// EmbedProgress reports batch embedding progress.
type EmbedProgress struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
}

// EmbedAll embeds all functions, structs, interfaces and services in the index.
// progress receives periodic updates; it may be nil.
func (s *EmbeddingService) EmbedAll(ctx context.Context, force bool, progress func(EmbedProgress)) error {
	eligibleTypes := map[graph.NodeType]bool{
		graph.NodeTypeFunction:  true,
		graph.NodeTypeStruct:    true,
		graph.NodeTypeInterface: true,
		graph.NodeTypeService:   true,
	}

	var nodes []*graph.Node
	for _, nt := range []graph.NodeType{
		graph.NodeTypeFunction,
		graph.NodeTypeStruct,
		graph.NodeTypeInterface,
		graph.NodeTypeService,
	} {
		_ = eligibleTypes // suppress unused warning
		nodes = append(nodes, s.index.GetByType(nt)...)
	}

	prog := EmbedProgress{Total: len(nodes)}
	type work struct {
		node *graph.Node
	}
	ch := make(chan work, len(nodes))
	for _, n := range nodes {
		ch <- work{node: n}
	}
	close(ch)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < embeddingWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range ch {
				if ctx.Err() != nil {
					return
				}
				_, err := s.EmbedNode(ctx, w.node.ID, force)
				mu.Lock()
				if err != nil {
					prog.Errors++
					slog.Warn("embedding error", "node_id", w.node.ID, "error", err)
				} else if !force && w.node.Metadata.AIEmbeddingID != "" {
					prog.Skipped++
				} else {
					prog.Completed++
				}
				if progress != nil {
					progress(prog)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	slog.Info("embedding batch complete",
		"total", prog.Total,
		"completed", prog.Completed,
		"skipped", prog.Skipped,
		"errors", prog.Errors,
	)
	return nil
}

// ---------------------------------------------------------------------------
// SimilaritySearch — cosine similarity over cached embeddings
// ---------------------------------------------------------------------------

// SimilarityResult is a single search hit.
type SimilarityResult struct {
	NodeID     string  `json:"node_id"`
	NodeName   string  `json:"node_name"`
	NodeType   string  `json:"node_type"`
	Score      float64 `json:"score"`
	Content    string  `json:"content"`
}

// SimilaritySearch embeds the query text and returns the top-k most similar
// nodes from the in-memory cache.
func (s *EmbeddingService) SimilaritySearch(ctx context.Context, query string, topK int) ([]SimilarityResult, error) {
	if topK <= 0 {
		topK = 10
	}

	queryVec, err := s.provider.Embed(ctx, query, "")
	if err != nil {
		return nil, fmt.Errorf("ai/embeddings: embed query: %w", err)
	}

	s.mu.RLock()
	embeddings := make([]*storage.Embedding, len(s.cache))
	copy(embeddings, s.cache)
	s.mu.RUnlock()

	type scored struct {
		emb   *storage.Embedding
		score float64
	}
	results := make([]scored, 0, len(embeddings))
	for _, emb := range embeddings {
		sim, err := cosineSimilarity(queryVec, emb.Vector)
		if err != nil {
			slog.Debug("skipping embedding",
				"node_id", emb.NodeID,
				"error", err.Error(),
			)
			continue
		}
		results = append(results, scored{emb: emb, score: sim})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]SimilarityResult, 0, len(results))
	for _, r := range results {
		node, found := s.index.GetNode(r.emb.NodeID)
		name := r.emb.NodeID
		nodeType := ""
		if found && node != nil {
			name = node.Name
			nodeType = string(node.Type)
		}
		out = append(out, SimilarityResult{
			NodeID:   r.emb.NodeID,
			NodeName: name,
			NodeType: nodeType,
			Score:    r.score,
			Content:  r.emb.Content,
		})
	}

	return out, nil
}

// ReloadCache refreshes the in-memory embedding cache from storage.
func (s *EmbeddingService) ReloadCache(ctx context.Context) error {
	all, err := s.store.GetAllEmbeddings(ctx)
	if err != nil {
		return fmt.Errorf("ai/embeddings: reload: %w", err)
	}
	s.mu.Lock()
	s.cache = all
	s.mu.Unlock()

	// Check for dimension inconsistencies in loaded embeddings.
	var expectedDim int
	mismatchCount := 0
	for _, emb := range all {
		if expectedDim == 0 {
			expectedDim = len(emb.Vector)
			continue
		}
		if len(emb.Vector) != expectedDim {
			mismatchCount++
		}
	}
	if mismatchCount > 0 {
		slog.Warn("embedding dimension inconsistency in cache",
			"mismatched", mismatchCount,
			"total", len(all),
			"expected_dimensions", expectedDim,
		)
	}

	return nil
}

// CacheSize returns the number of embeddings in the in-memory cache.
func (s *EmbeddingService) CacheSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

// ---------------------------------------------------------------------------
// Content builder — turns a Node into text suitable for embedding
// ---------------------------------------------------------------------------

// buildNodeContent produces a textual representation of a node for embedding.
func buildNodeContent(n *graph.Node) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("[%s] %s\n", n.Type, n.Name))

	if n.FilePath != "" {
		b.WriteString(fmt.Sprintf("File: %s", n.FilePath))
		if n.LineStart > 0 {
			b.WriteString(fmt.Sprintf(" L%d", n.LineStart))
			if n.LineEnd > 0 {
				b.WriteString(fmt.Sprintf("-%d", n.LineEnd))
			}
		}
		b.WriteString("\n")
	}

	m := &n.Metadata

	if m.DocComment != "" {
		b.WriteString("Doc: " + m.DocComment + "\n")
	}
	if m.Signature != "" {
		b.WriteString("Sig: " + m.Signature + "\n")
	}
	if m.Receiver != "" {
		b.WriteString("Receiver: " + m.Receiver + "\n")
	}
	if len(m.Parameters) > 0 {
		params := make([]string, len(m.Parameters))
		for i, p := range m.Parameters {
			params[i] = p.Name + " " + p.Type
		}
		b.WriteString("Params: " + strings.Join(params, ", ") + "\n")
	}
	if len(m.ReturnTypes) > 0 {
		b.WriteString("Returns: " + strings.Join(m.ReturnTypes, ", ") + "\n")
	}
	if len(m.Fields) > 0 {
		fields := make([]string, 0, len(m.Fields))
		for _, f := range m.Fields {
			fields = append(fields, f.Name+" "+f.Type)
		}
		b.WriteString("Fields: " + strings.Join(fields, ", ") + "\n")
	}
	if len(m.InterfaceMethods) > 0 {
		names := make([]string, 0, len(m.InterfaceMethods))
		for _, im := range m.InterfaceMethods {
			names = append(names, im.Name)
		}
		b.WriteString("Methods: " + strings.Join(names, ", ") + "\n")
	}
	if len(m.Calls) > 0 {
		b.WriteString("Calls: " + strings.Join(m.Calls, ", ") + "\n")
	}
	if m.HasErrorReturn {
		b.WriteString("ErrorReturn: yes\n")
	}
	if m.HasContextParam {
		b.WriteString("ContextParam: yes\n")
	}
	if m.SourceSnippet != "" {
		b.WriteString("Source:\n" + m.SourceSnippet + "\n")
	}
	if len(m.CloudDependencies) > 0 {
		b.WriteString("CloudDeps: " + strings.Join(m.CloudDependencies, ", ") + "\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Cosine similarity
// ---------------------------------------------------------------------------

func cosineSimilarity(a, b []float32) (float64, error) {
	if len(a) == 0 || len(b) == 0 {
		return 0, fmt.Errorf("empty vector: len(a)=%d len(b)=%d", len(a), len(b))
	}
	if len(a) != len(b) {
		return 0, fmt.Errorf(
			"dimension mismatch: query has %d dims, stored has %d dims. "+
				"Check that the same embedding model is used for indexing and querying",
			len(a), len(b),
		)
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0, nil
	}
	return dot / denom, nil
}
