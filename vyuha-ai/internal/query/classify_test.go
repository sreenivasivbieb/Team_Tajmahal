package query

import (
	"strings"
	"testing"
)

func TestClassifyQuestionKeyword(t *testing.T) {
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
			decision := classifyQuestionKeyword(tt.question)
			if decision.Mode != tt.wantMode {
				t.Errorf("classifyQuestionKeyword(%q) = mode %q, want %q",
					tt.question, decision.Mode, tt.wantMode)
			}
		})
	}
}

func TestBuildClassificationPrompt(t *testing.T) {
	prompt := buildClassificationPrompt("what calls HandleScan")

	if !strings.Contains(prompt, "what calls HandleScan") {
		t.Error("prompt should contain the original question")
	}
	if !strings.Contains(prompt, "mode") {
		t.Error("prompt should mention mode field")
	}
	if !strings.Contains(prompt, "subgraph_type") {
		t.Error("prompt should mention subgraph_type field")
	}
	if !strings.Contains(prompt, "confidence") {
		t.Error("prompt should mention confidence field")
	}
}

func TestParseClassificationResponse(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		question     string
		wantMode     QueryMode
		wantSGType   SubgraphQueryType
		wantErr      bool
	}{
		{
			name:     "valid subgraph response",
			raw:      `{"mode":"subgraph","subgraph_type":"call_chain","confidence":0.9,"reasoning":"asks about callers"}`,
			question: "who calls HandleScan",
			wantMode: ModeSubgraph,
			wantSGType: QueryCallChain,
		},
		{
			name:     "valid agent response",
			raw:      `{"mode":"agent","subgraph_type":"","confidence":0.8,"reasoning":"complex question"}`,
			question: "explain how the parser works",
			wantMode: ModeAgent,
		},
		{
			name:     "valid direct_graph response",
			raw:      `{"mode":"direct_graph","subgraph_type":"","confidence":0.95,"reasoning":"simple count"}`,
			question: "how many services exist",
			wantMode: ModeDirectGraph,
		},
		{
			name:     "markdown-wrapped JSON",
			raw:      "```json\n{\"mode\":\"sql\",\"subgraph_type\":\"\",\"confidence\":0.7,\"reasoning\":\"runtime stats\"}\n```",
			question: "top failing nodes",
			wantMode: ModeSQL,
		},
		{
			name:    "invalid JSON",
			raw:     "I don't understand",
			wantErr: true,
		},
		{
			name:    "unknown mode",
			raw:     `{"mode":"unknown","confidence":0.5,"reasoning":"test"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := parseClassificationResponse(tt.raw, tt.question)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseClassificationResponse(%q) expected error, got nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseClassificationResponse(%q) unexpected error: %v", tt.raw, err)
			}
			if decision.Mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", decision.Mode, tt.wantMode)
			}
			if tt.wantSGType != "" && decision.SubgraphType != tt.wantSGType {
				t.Errorf("subgraph_type = %q, want %q", decision.SubgraphType, tt.wantSGType)
			}
			if decision.Question != tt.question {
				t.Errorf("question = %q, want %q", decision.Question, tt.question)
			}
		})
	}
}
