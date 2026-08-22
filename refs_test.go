package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSymbolEdges(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.py")
	b := filepath.Join(dir, "b.py")
	os.WriteFile(a, []byte("def foo(x):\n    return x\n"), 0644)
	os.WriteFile(b, []byte("from a import foo\n\nfoo(1)\n"), 0644)

	files := collectFiles([]string{dir}, true)
	result := FindSymbolEdges(dir, files, "foo", ExtractionOptions{}, false)

	if len(result.Definitions) == 0 {
		t.Fatal("expected at least one definition")
	}
	if len(result.References) == 0 {
		t.Fatal("expected references")
	}
	for _, ref := range result.References {
		if ref.IsDefinition {
			t.Fatalf("expected definitions excluded from references by default, got %+v", ref)
		}
	}
}

func TestFindSymbolEdgesIncludeDefinitions(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.py")
	os.WriteFile(a, []byte("def foo(x):\n    return x\nfoo(1)\n"), 0644)

	files := collectFiles([]string{dir}, true)
	result := FindSymbolEdges(dir, files, "foo", ExtractionOptions{}, true)

	hasDefRef := false
	for _, ref := range result.References {
		if ref.IsDefinition {
			hasDefRef = true
			break
		}
	}
	if !hasDefRef {
		t.Fatal("expected definition line included in references when includeDefinitions=true")
	}
}
