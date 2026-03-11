package main

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestGraph(t *testing.T) (*DepGraph, string) {
	t.Helper()
	dir := t.TempDir()
	g := NewDepGraph(dir)
	return g, dir
}

func TestDepGraphAddEdge(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")

	g.addEdge(a, b)

	if !g.Edges[a][b] {
		t.Error("expected edge a -> b")
	}
	if !g.Reverse[b][a] {
		t.Error("expected reverse edge b -> a")
	}
}

func TestDepGraphDeps(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")
	c := filepath.Join(dir, "c.py")

	g.addEdge(a, b)
	g.addEdge(b, c)

	// Direct deps
	direct := g.Deps(a, false)
	if len(direct) != 1 || direct[0] != b {
		t.Errorf("direct deps = %v, want [%s]", direct, b)
	}

	// Transitive deps
	trans := g.Deps(a, true)
	if len(trans) != 2 {
		t.Errorf("transitive deps = %v, want 2 deps", trans)
	}
}

func TestDepGraphDependents(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")
	c := filepath.Join(dir, "c.py")

	g.addEdge(a, b)
	g.addEdge(b, c)

	// Direct dependents of c
	direct := g.Dependents(c, false)
	if len(direct) != 1 || direct[0] != b {
		t.Errorf("direct dependents = %v, want [%s]", direct, b)
	}

	// Transitive dependents of c
	trans := g.Dependents(c, true)
	if len(trans) != 2 {
		t.Errorf("transitive dependents = %v, want 2", trans)
	}
}

func TestDepGraphNoDeps(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")

	deps := g.Deps(a, false)
	if deps != nil {
		t.Errorf("expected nil deps, got %v", deps)
	}
}

func TestDepGraphCircular(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")

	g.addEdge(a, b)
	g.addEdge(b, a)

	// Should not infinite loop
	trans := g.Deps(a, true)
	if len(trans) != 1 {
		t.Errorf("expected 1 transitive dep, got %d: %v", len(trans), trans)
	}
}

func TestDepGraphImpact(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")
	c := filepath.Join(dir, "c.py")

	g.addEdge(b, a) // b depends on a
	g.addEdge(c, b) // c depends on b

	result := g.Impact(a)
	if result.DirectDependents != 1 {
		t.Errorf("DirectDependents = %d, want 1", result.DirectDependents)
	}
	if result.TransitiveDependents != 2 {
		t.Errorf("TransitiveDependents = %d, want 2", result.TransitiveDependents)
	}
}

func TestDepGraphSummary(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")
	c := filepath.Join(dir, "c.py")

	g.addEdge(a, b)
	g.addEdge(a, c)
	g.addEdge(b, c)
	g.addUnresolved(a, "missing_module")

	s := g.Summary()
	if s.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", s.TotalFiles)
	}
	if s.TotalEdges != 3 {
		t.Errorf("TotalEdges = %d, want 3", s.TotalEdges)
	}
	if s.UnresolvedImports != 1 {
		t.Errorf("UnresolvedImports = %d, want 1", s.UnresolvedImports)
	}
}

func TestDepGraphSummaryCircular(t *testing.T) {
	g, dir := newTestGraph(t)
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")

	g.addEdge(a, b)
	g.addEdge(b, a)

	s := g.Summary()
	if len(s.CircularPairs) != 1 {
		t.Errorf("CircularPairs = %d, want 1", len(s.CircularPairs))
	}
}

func TestDepGraphRel(t *testing.T) {
	g, dir := newTestGraph(t)
	p := filepath.Join(dir, "src", "main.py")
	rel := g.rel(p)
	if rel != "src/main.py" {
		t.Errorf("rel = %q, want %q", rel, "src/main.py")
	}
}

func TestBuildGraphIntegration(t *testing.T) {
	dir := t.TempDir()

	// Create a small Python project
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("from lib import helper\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "lib"), 0755)
	os.WriteFile(filepath.Join(dir, "lib", "__init__.py"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "lib", "helper.py"), []byte("def help(): pass\n"), 0644)

	files := []string{
		filepath.Join(dir, "main.py"),
		filepath.Join(dir, "lib", "__init__.py"),
		filepath.Join(dir, "lib", "helper.py"),
	}

	graph := BuildGraph(dir, files)
	if graph == nil {
		t.Fatal("BuildGraph returned nil")
	}
	if len(graph.Edges) == 0 && len(graph.Unresolved) == 0 {
		t.Error("expected at least some edges or unresolved imports")
	}
}

func TestFormatDepsText(t *testing.T) {
	dir := t.TempDir()
	out := FormatDepsText("test.py", nil, dir, "depends on")
	if out == "" {
		t.Error("expected non-empty output for no deps")
	}

	out = FormatDepsText("test.py", []string{filepath.Join(dir, "other.py")}, dir, "depends on")
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatImpactText(t *testing.T) {
	r := ImpactResult{
		File:                 "a.py",
		DirectDependents:     1,
		TransitiveDependents: 2,
		Direct:               []string{"b.py"},
		Indirect:             []string{"c.py"},
	}
	out := FormatImpactText(r)
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatSummaryText(t *testing.T) {
	s := GraphSummary{
		TotalFiles:        10,
		TotalEdges:        20,
		UnresolvedImports: 3,
		HotSpots:          []FileCount{{File: "a.py", Count: 5}},
		HeaviestImporters: []FileCount{{File: "b.py", Count: 8}},
		CircularPairs:     [][2]string{{"a.py", "b.py"}},
	}
	out := FormatSummaryText(s)
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]bool{"c": true, "a": true, "b": true}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("sortedKeys = %v, want [a b c]", keys)
	}
}

func TestSortedKeysNil(t *testing.T) {
	keys := sortedKeys(nil)
	if keys != nil {
		t.Errorf("expected nil, got %v", keys)
	}
}
