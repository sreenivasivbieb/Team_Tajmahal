package query

import (
	"testing"

	"github.com/vyuha/vyuha-ai/internal/graph"
)

func TestClassifyQuestion(t *testing.T) {
	// Create a minimal QueryLayer — classifyQuestion only uses string matching,
	// so nil dependencies are safe.
	idx := graph.NewGraphIndex()
	ql := &QueryLayer{
		index: idx,
	}

	tests := []struct {
		question string
		wantMode QueryMode
	}{
		{"show me the graph stats", ModeDirectGraph},
		{"what is the architecture overview", ModeSubgraph},
		{"show the call chain from HandleScan", ModeSubgraph},
		{"why is HandleScan failing", ModeSubgraph},
		{"explain how the parser works", ModeAgent},
		{"what does NewServer do", ModeAgent},
		{"how many functions are there", ModeDirectGraph},
		{"show dependencies of the api package", ModeAgent},
		{"what is the blast radius if storage fails", ModeSubgraph},
		{"trace the data flow from ingest to storage", ModeSubgraph},
	}

	for _, tt := range tests {
		t.Run(tt.question, func(t *testing.T) {
			decision := ql.classifyQuestion(tt.question)
			if decision.Mode != tt.wantMode {
				t.Errorf("classifyQuestion(%q) = mode %q, want %q", tt.question, decision.Mode, tt.wantMode)
			}
		})
	}
}
