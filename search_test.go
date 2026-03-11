package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupSearchProject(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"app.py":      "def handle_request(): pass\nclass RequestHandler: pass\nAPI_URL = 'http://'\n",
		"utils.py":    "def format_date(): pass\ndef handle_error(): pass\n",
		"models.py":   "class User: pass\nclass UserProfile: pass\n",
		"src/index.ts": "export function handleClick() {}\nexport function handleSubmit() {}\n",
	}
	var paths []string
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(content), 0644)
		paths = append(paths, p)
	}
	return dir, paths
}

func TestSearchExactMatch(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "User")

	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// Exact match should come first
	if results[0].Symbol.Name != "User" {
		t.Errorf("first result = %q, want exact match 'User'", results[0].Symbol.Name)
	}
}

func TestSearchPrefixMatch(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "handle")

	if len(results) < 3 {
		t.Fatalf("expected at least 3 results for 'handle', got %d", len(results))
	}
}

func TestSearchContainsMatch(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "request")

	found := false
	for _, r := range results {
		if r.Symbol.Name == "RequestHandler" || r.Symbol.Name == "handle_request()" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find RequestHandler or handle_request, got: %+v", results)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "user")
	if len(results) == 0 {
		t.Fatal("expected case-insensitive match for 'user'")
	}
}

func TestSearchNoResults(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "zzzznonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchRanking(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "User")

	// Should have exact "User" before prefix "UserProfile"
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	exactIdx := -1
	prefixIdx := -1
	for i, r := range results {
		name := r.Symbol.Name
		if name == "User" {
			exactIdx = i
		}
		if name == "UserProfile" {
			prefixIdx = i
		}
	}
	if exactIdx < 0 || prefixIdx < 0 {
		t.Fatal("expected both User and UserProfile in results")
	}
	if exactIdx > prefixIdx {
		t.Error("exact match should rank before prefix match")
	}
}

func TestSearchConstants(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "API_URL")
	if len(results) == 0 {
		t.Fatal("expected to find API_URL constant")
	}
	if results[0].Symbol.Kind != "constant" {
		t.Errorf("expected kind 'constant', got %q", results[0].Symbol.Kind)
	}
}

func TestSearchRelativePaths(t *testing.T) {
	dir, files := setupSearchProject(t)
	results := SearchSymbols(dir, files, "handleClick")
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// Should use relative paths, not absolute
	for _, r := range results {
		if filepath.IsAbs(r.File) {
			t.Errorf("expected relative path, got %q", r.File)
		}
	}
}

func TestFormatSearchText(t *testing.T) {
	results := []SearchResult{
		{File: "app.py", Symbol: Symbol{Name: "foo", Kind: "def", Line: 1}},
	}
	out := FormatSearchText(results, "foo")
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatSearchTextEmpty(t *testing.T) {
	out := FormatSearchText(nil, "missing")
	if out == "" {
		t.Error("expected non-empty output for no results")
	}
}
