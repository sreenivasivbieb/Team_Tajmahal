package graph

import (
	"testing"
)

func TestGraphIndex_AddAndGetNode(t *testing.T) {
	idx := NewGraphIndex()

	node := &Node{
		ID:       "func:test/pkg:MyFunc",
		Type:     NodeTypeFunction,
		Name:     "MyFunc",
		ParentID: "pkg:test/pkg",
		FilePath: "pkg/main.go",
		Depth:    3,
		Metadata: NodeMetadata{},
	}

	idx.AddNode(node)

	// Get by ID
	got, ok := idx.GetNode("func:test/pkg:MyFunc")
	if !ok || got == nil {
		t.Fatal("expected node, got nil")
	}
	if got.Name != "MyFunc" {
		t.Errorf("expected name MyFunc, got %s", got.Name)
	}
	if got.Type != NodeTypeFunction {
		t.Errorf("expected type function, got %s", string(got.Type))
	}

	// Get by type
	funcs := idx.GetByType(NodeTypeFunction)
	if len(funcs) != 1 {
		t.Fatalf("expected 1 function node, got %d", len(funcs))
	}

	// Get by file
	fileNodes := idx.GetByFile("pkg/main.go")
	if len(fileNodes) != 1 {
		t.Fatalf("expected 1 node in file, got %d", len(fileNodes))
	}

	// Children
	children := idx.GetChildren("pkg:test/pkg")
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}

	// Node count
	if idx.NodeCount() != 1 {
		t.Errorf("expected node count 1, got %d", idx.NodeCount())
	}

	// Non-existent node
	if _, found := idx.GetNode("does-not-exist"); found {
		t.Error("expected not-found for non-existent node")
	}
}

func TestGraphIndex_AddEdgeAndAdjacency(t *testing.T) {
	idx := NewGraphIndex()

	caller := &Node{ID: "func:a:Caller", Type: NodeTypeFunction, Name: "Caller", Metadata: NodeMetadata{}}
	callee := &Node{ID: "func:a:Callee", Type: NodeTypeFunction, Name: "Callee", Metadata: NodeMetadata{}}
	idx.AddNode(caller)
	idx.AddNode(callee)

	edge := &Edge{
		ID:       "edge-1",
		SourceID: "func:a:Caller",
		TargetID: "func:a:Callee",
		Type:     EdgeTypeCalls,
		Metadata: EdgeMetadata{},
	}
	idx.AddEdge(edge)

	// Out edges from caller
	out := idx.GetOutEdges("func:a:Caller")
	if len(out) != 1 {
		t.Fatalf("expected 1 out edge, got %d", len(out))
	}
	if out[0].TargetID != "func:a:Callee" {
		t.Errorf("expected target func:a:Callee, got %s", out[0].TargetID)
	}

	// In edges to callee
	in := idx.GetInEdges("func:a:Callee")
	if len(in) != 1 {
		t.Fatalf("expected 1 in edge, got %d", len(in))
	}
	if in[0].SourceID != "func:a:Caller" {
		t.Errorf("expected source func:a:Caller, got %s", in[0].SourceID)
	}

	// Edge count
	if idx.EdgeCount() != 1 {
		t.Errorf("expected edge count 1, got %d", idx.EdgeCount())
	}

	// No out edges from callee
	if len(idx.GetOutEdges("func:a:Callee")) != 0 {
		t.Error("expected 0 out edges from callee")
	}

	// No in edges to caller
	if len(idx.GetInEdges("func:a:Caller")) != 0 {
		t.Error("expected 0 in edges to caller")
	}
}

func TestGraphIndex_GetDescendants(t *testing.T) {
	idx := NewGraphIndex()

	// Build a 3-level hierarchy: repo -> pkg -> func
	repo := &Node{ID: "repo:test", Type: NodeTypeRepository, Name: "test", Metadata: NodeMetadata{}}
	pkg := &Node{ID: "pkg:test/a", Type: NodeTypePackage, Name: "a", ParentID: "repo:test", Metadata: NodeMetadata{}}
	fn := &Node{ID: "func:test/a:Do", Type: NodeTypeFunction, Name: "Do", ParentID: "pkg:test/a", Metadata: NodeMetadata{}}

	idx.AddNode(repo)
	idx.AddNode(pkg)
	idx.AddNode(fn)

	// Descendants of repo at depth 1 should include pkg
	desc1 := idx.GetDescendants("repo:test", 1)
	found := false
	for _, n := range desc1 {
		if n.ID == "pkg:test/a" {
			found = true
		}
	}
	if !found {
		t.Error("expected pkg:test/a in depth-1 descendants of repo:test")
	}

	// Descendants at depth 3 should include the function
	desc3 := idx.GetDescendants("repo:test", 3)
	found = false
	for _, n := range desc3 {
		if n.ID == "func:test/a:Do" {
			found = true
		}
	}
	if !found {
		t.Error("expected func:test/a:Do in depth-3 descendants of repo:test")
	}
}

func TestGraphIndex_Stats(t *testing.T) {
	idx := NewGraphIndex()

	idx.AddNode(&Node{ID: "func:a:HandleScan", Type: NodeTypeFunction, Name: "HandleScan", Metadata: NodeMetadata{}})
	idx.AddNode(&Node{ID: "func:a:HandleQuery", Type: NodeTypeFunction, Name: "HandleQuery", Metadata: NodeMetadata{}})
	idx.AddNode(&Node{ID: "pkg:a", Type: NodeTypePackage, Name: "a", Metadata: NodeMetadata{}})

	idx.AddEdge(&Edge{ID: "e1", SourceID: "func:a:HandleScan", TargetID: "func:a:HandleQuery", Type: EdgeTypeCalls, Metadata: EdgeMetadata{}})

	stats := idx.Stats()
	if stats.TotalNodes != 3 {
		t.Errorf("expected 3 total nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges != 1 {
		t.Errorf("expected 1 total edge, got %d", stats.TotalEdges)
	}
	if stats.NodesByType["function"] != 2 {
		t.Errorf("expected 2 function nodes, got %d", stats.NodesByType["function"])
	}
	if stats.NodesByType["package"] != 1 {
		t.Errorf("expected 1 package node, got %d", stats.NodesByType["package"])
	}
}
