package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func decodeStructured(t *testing.T, result mcpToolResult, out interface{}) {
	t.Helper()
	if result.StructuredContent == nil {
		t.Fatal("expected structuredContent")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("failed to marshal structuredContent: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("failed to decode structuredContent: %v", err)
	}
}

func TestMCPTools(t *testing.T) {
	tools := mcpTools()
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %s has nil input schema", tool.Name)
		}
	}

	for _, want := range []string{"syms_list", "syms_imports", "syms_deps", "syms_dependents", "syms_impact", "syms_search", "syms_graph"} {
		if !names[want] {
			t.Errorf("missing tool %s", want)
		}
	}
}

func TestHandleList(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.py")
	os.WriteFile(f, []byte("def foo(): pass\nclass Bar: pass\n"), 0644)

	args, _ := json.Marshal(map[string]interface{}{"paths": []string{f}})
	result := handleToolCall("syms_list", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if result.StructuredContent == nil {
		t.Fatal("missing structuredContent")
	}

	var results []SymbolResult
	decodeStructured(t, result, &results)
	if len(results) != 1 {
		t.Fatalf("expected 1 file result, got %d", len(results))
	}
	if len(results[0].Symbols) == 0 {
		t.Error("expected symbols, got none")
	}
}

func TestHandleImports(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.py")
	os.WriteFile(f, []byte("import os\nfrom sys import argv\n"), 0644)

	args, _ := json.Marshal(map[string]interface{}{"paths": []string{f}})
	result := handleToolCall("syms_imports", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var results []ImportResult
	decodeStructured(t, result, &results)
	if len(results[0].Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(results[0].Imports))
	}
}

func TestHandleDeps(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("from lib import utils\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "lib"), 0755)
	os.WriteFile(filepath.Join(dir, "lib", "__init__.py"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "lib", "utils.py"), []byte(""), 0644)

	args, _ := json.Marshal(map[string]interface{}{
		"file": filepath.Join(dir, "main.py"),
		"root": dir,
	})
	result := handleToolCall("syms_deps", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var parsed map[string]interface{}
	decodeStructured(t, result, &parsed)
	if parsed["file"] == nil {
		t.Error("result missing 'file' field")
	}
}

func TestHandleDependents(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("from lib import utils\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "lib"), 0755)
	os.WriteFile(filepath.Join(dir, "lib", "__init__.py"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "lib", "utils.py"), []byte(""), 0644)

	args, _ := json.Marshal(map[string]interface{}{
		"file": filepath.Join(dir, "lib", "utils.py"),
		"root": dir,
	})
	result := handleToolCall("syms_dependents", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
}

func TestHandleImpact(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.py"), []byte("import b\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte(""), 0644)

	args, _ := json.Marshal(map[string]interface{}{
		"file": filepath.Join(dir, "b.py"),
		"root": dir,
	})
	result := handleToolCall("syms_impact", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var parsed ImpactResult
	decodeStructured(t, result, &parsed)
}

func TestHandleGraph(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.py"), []byte("import b\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte(""), 0644)

	args, _ := json.Marshal(map[string]interface{}{"root": dir})
	result := handleToolCall("syms_graph", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var parsed GraphSummary
	decodeStructured(t, result, &parsed)
	if parsed.TotalFiles == 0 {
		t.Error("expected files in graph")
	}
}

func TestHandleUnknownTool(t *testing.T) {
	result := handleToolCall("nonexistent", nil)
	if !result.IsError {
		t.Error("expected error for unknown tool")
	}
}

func TestHandleListNoFiles(t *testing.T) {
	args, _ := json.Marshal(map[string]interface{}{"paths": []string{"/nonexistent"}})
	result := handleToolCall("syms_list", args)
	if result.IsError {
		t.Error("should not be an error, just empty")
	}
}

func TestHandleListRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "mod.py"), []byte("def f(): pass\n"), 0644)

	args, _ := json.Marshal(map[string]interface{}{
		"paths":     []string{dir},
		"recursive": true,
	})
	result := handleToolCall("syms_list", args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var results []SymbolResult
	decodeStructured(t, result, &results)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestHandleInvalidArgs(t *testing.T) {
	result := handleToolCall("syms_list", []byte(`{invalid json`))
	if !result.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestTextResult(t *testing.T) {
	r := textResult("hello")
	if r.IsError {
		t.Error("unexpected error")
	}
	if r.Content[0].Text != "hello" {
		t.Errorf("got %q, want 'hello'", r.Content[0].Text)
	}
	if r.StructuredContent != nil {
		t.Error("expected no structuredContent for textResult")
	}
}

func TestErrResult(t *testing.T) {
	r := errResult("bad")
	if !r.IsError {
		t.Error("expected isError=true")
	}
	if r.Content[0].Text != "bad" {
		t.Errorf("got %q, want 'bad'", r.Content[0].Text)
	}
	if r.StructuredContent != nil {
		t.Error("expected no structuredContent for errResult")
	}
}

func TestReadRPCMessageLineMode(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	msg, mode, err := readRPCMessage(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("readRPCMessage error: %v", err)
	}
	if mode != transportLine {
		t.Fatalf("expected line mode, got %q", mode)
	}
	var req jsonRPCRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		t.Fatalf("invalid json payload: %v", err)
	}
	if req.Method != "ping" {
		t.Fatalf("expected ping method, got %q", req.Method)
	}
}

func TestReadRPCMessageHeaderMode(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	input := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	msg, mode, err := readRPCMessage(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("readRPCMessage error: %v", err)
	}
	if mode != transportHeader {
		t.Fatalf("expected header mode, got %q", mode)
	}
	if string(msg) != body {
		t.Fatalf("unexpected body: %q", string(msg))
	}
}

func TestWriteRPCResponseHeaderMode(t *testing.T) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Result:  map[string]string{"ok": "yes"},
	}
	var out bytes.Buffer
	if err := writeRPCResponse(&out, transportHeader, resp); err != nil {
		t.Fatalf("writeRPCResponse error: %v", err)
	}
	s := out.String()
	if !strings.HasPrefix(s, "Content-Length: ") {
		t.Fatalf("missing content-length header: %q", s)
	}
	if !strings.Contains(s, "\r\n\r\n") {
		t.Fatalf("missing header/body separator: %q", s)
	}
}

func TestWriteRPCResponseLineMode(t *testing.T) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Result:  map[string]string{"ok": "yes"},
	}
	var out bytes.Buffer
	if err := writeRPCResponse(&out, transportLine, resp); err != nil {
		t.Fatalf("writeRPCResponse error: %v", err)
	}
	s := out.String()
	if !strings.HasSuffix(s, "\n") {
		t.Fatalf("expected newline-delimited output, got %q", s)
	}
	var parsed jsonRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &parsed); err != nil {
		t.Fatalf("line response is not valid json: %v", err)
	}
}
