package query

import (
	"fmt"
	"math"
	"sort"

	"github.com/vyuha/vyuha-ai/internal/graph"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ScoredNode pairs a graph node with a relevance score.
type ScoredNode struct {
	Node    *graph.Node `json:"node"`
	Score   float64     `json:"score"`
	Reasons []string    `json:"reasons"` // why this node scored high
}

// ---------------------------------------------------------------------------
// NodeScorer
// ---------------------------------------------------------------------------

// NodeScorer computes relevance scores for graph nodes according to the
// current query type.
type NodeScorer struct {
	index *graph.GraphIndex
}

// NewNodeScorer creates a ready-to-use scorer.
func NewNodeScorer(index *graph.GraphIndex) *NodeScorer {
	return &NodeScorer{index: index}
}

// ---------------------------------------------------------------------------
// Score – single node
// ---------------------------------------------------------------------------

// Score computes a relevance score for a single node in the context of the
// given query type.
func (s *NodeScorer) Score(node *graph.Node, queryType SubgraphQueryType) ScoredNode {
	switch queryType {
	case QueryServiceOverview:
		return s.scoreServiceOverview(node)
	case QueryCallChain:
		return s.scoreCallChain(node)
	case QueryFailurePath:
		return s.scoreFailurePath(node)
	case QueryDataLineage:
		return s.scoreDataLineage(node)
	case QueryDependencyImpact:
		return s.scoreDependencyImpact(node)
	default:
		return ScoredNode{Node: node, Score: 0.5, Reasons: []string{"default"}}
	}
}

// ---------------------------------------------------------------------------
// ScoreAll – batch score + sort descending
// ---------------------------------------------------------------------------

// ScoreAll scores every node and returns them sorted by score descending.
func (s *NodeScorer) ScoreAll(nodes []*graph.Node, queryType SubgraphQueryType) []ScoredNode {
	scored := make([]ScoredNode, 0, len(nodes))
	for _, n := range nodes {
		scored = append(scored, s.Score(n, queryType))
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored
}

// ---------------------------------------------------------------------------
// Per-query-type scoring
// ---------------------------------------------------------------------------

func (s *NodeScorer) scoreServiceOverview(node *graph.Node) ScoredNode {
	sn := ScoredNode{Node: node}

	fanIn := s.index.ComputeFanIn(node.ID)
	fanOut := s.index.ComputeFanOut(node.ID)

	// fan_in_score = min(fanIn/10, 1.0) * 0.30
	fanInScore := math.Min(float64(fanIn)/10.0, 1.0) * 0.30
	if fanInScore > 0 {
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("fan_in=%d (%.2f)", fanIn, fanInScore))
	}

	// fan_out_score = min(fanOut/10, 1.0) * 0.15
	fanOutScore := math.Min(float64(fanOut)/10.0, 1.0) * 0.15
	if fanOutScore > 0 {
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("fan_out=%d (%.2f)", fanOut, fanOutScore))
	}

	// error_score
	var errorScore float64
	if node.ErrorCount > 0 {
		errorScore = 0.25
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("errors=%d (0.25)", node.ErrorCount))
	}

	// cloud_score
	var cloudScore float64
	if node.IsCloud() {
		cloudScore = 0.15
		sn.Reasons = append(sn.Reasons, "cloud_service (0.15)")
	}

	// entry_score
	var entryScore float64
	if s.isEntryPoint(node) {
		entryScore = 0.15
		sn.Reasons = append(sn.Reasons, "entry_point (0.15)")
	}

	sn.Score = fanInScore + fanOutScore + errorScore + cloudScore + entryScore
	return sn
}

func (s *NodeScorer) scoreCallChain(node *graph.Node) ScoredNode {
	sn := ScoredNode{Node: node}

	// depth_score = (maxDepth - nodeDepth) / maxDepth * 0.40
	// Use a reasonable max depth of 10 for normalisation.
	const maxDepth = 10.0
	depthScore := math.Max(0, (maxDepth-float64(node.Depth))/maxDepth) * 0.40
	if depthScore > 0 {
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("depth=%d (%.2f)", node.Depth, depthScore))
	}

	// error_score
	var errorScore float64
	if node.ErrorCount > 0 {
		errorScore = 0.30
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("errors=%d (0.30)", node.ErrorCount))
	}

	// cloud_score
	var cloudScore float64
	if node.IsCloud() {
		cloudScore = 0.20
		sn.Reasons = append(sn.Reasons, "cloud_service (0.20)")
	}

	sn.Score = depthScore + errorScore + cloudScore
	return sn
}

func (s *NodeScorer) scoreFailurePath(node *graph.Node) ScoredNode {
	sn := ScoredNode{Node: node}

	// error_score
	var errorScore float64
	if node.ErrorCount > 0 || node.RuntimeStatus == "error" {
		errorScore = 0.50
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("failing (errors=%d, status=%s) (0.50)", node.ErrorCount, node.RuntimeStatus))
	}

	// caller_score: if this node has out-CALLS edges → might be a caller
	var callerScore float64
	callees := s.index.GetCallees(node.ID)
	for _, callee := range callees {
		if callee.IsFailing() {
			callerScore = 0.30
			sn.Reasons = append(sn.Reasons, "caller_of_failing (0.30)")
			break
		}
	}

	// dependency_score: if this node is called by failing nodes
	var depScore float64
	callers := s.index.GetCallers(node.ID)
	for _, caller := range callers {
		if caller.IsFailing() {
			depScore = 0.20
			sn.Reasons = append(sn.Reasons, "dependency_of_failing (0.20)")
			break
		}
	}

	sn.Score = errorScore + callerScore + depScore
	return sn
}

func (s *NodeScorer) scoreDataLineage(node *graph.Node) ScoredNode {
	sn := ScoredNode{Node: node}

	// Data flow nodes get a base boost.
	if node.Type == graph.NodeTypeDataFlow {
		sn.Score += 0.40
		sn.Reasons = append(sn.Reasons, "data_flow_node (0.40)")
	}

	// Nodes that transform data are important.
	fanIn := s.index.ComputeFanIn(node.ID)
	fanOut := s.index.ComputeFanOut(node.ID)
	if fanIn > 0 && fanOut > 0 {
		sn.Score += 0.25
		sn.Reasons = append(sn.Reasons, "data_transformer (0.25)")
	}

	// Nodes with errors are risky in data pipelines.
	if node.ErrorCount > 0 {
		sn.Score += 0.20
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("errors=%d (0.20)", node.ErrorCount))
	}

	// Cloud services handling data.
	if node.IsCloud() {
		sn.Score += 0.15
		sn.Reasons = append(sn.Reasons, "cloud_data_service (0.15)")
	}

	return sn
}

func (s *NodeScorer) scoreDependencyImpact(node *graph.Node) ScoredNode {
	sn := ScoredNode{Node: node}

	// Nodes with high fan-in are impactful if they fail.
	fanIn := s.index.ComputeFanIn(node.ID)
	fanInScore := math.Min(float64(fanIn)/10.0, 1.0) * 0.40
	if fanInScore > 0 {
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("blast_fan_in=%d (%.2f)", fanIn, fanInScore))
	}

	// Services have bigger blast radius.
	var serviceScore float64
	if node.Type == graph.NodeTypeService {
		serviceScore = 0.30
		sn.Reasons = append(sn.Reasons, "service_level (0.30)")
	}

	// Already failing compounds the impact.
	var errorScore float64
	if node.ErrorCount > 0 {
		errorScore = 0.30
		sn.Reasons = append(sn.Reasons, fmt.Sprintf("already_failing=%d (0.30)", node.ErrorCount))
	}

	sn.Score = fanInScore + serviceScore + errorScore
	return sn
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isEntryPoint checks if the node is an entry point by testing whether it
// has zero in-CALLS edges (i.e. nobody inside its service calls it).
func (s *NodeScorer) isEntryPoint(node *graph.Node) bool {
	if node.Type != graph.NodeTypeFunction {
		return false
	}
	callers := s.index.GetCallers(node.ID)
	return len(callers) == 0
}
