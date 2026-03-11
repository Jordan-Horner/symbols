package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ── JSON-RPC types ──────────────────────────────────────────────────────────

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcTransportMode string

const (
	transportLine   rpcTransportMode = "line"
	transportHeader rpcTransportMode = "header"
)

// ── MCP types ───────────────────────────────────────────────────────────────

type mcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpInitResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    mcpCapabilities `json:"capabilities"`
	ServerInfo      mcpServerInfo   `json:"serverInfo"`
}

type mcpCapabilities struct {
	Tools *mcpToolsCap `json:"tools,omitempty"`
}

type mcpToolsCap struct{}

type mcpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// ── Tool definitions ────────────────────────────────────────────────────────

func mcpTools() []mcpTool {
	pathProp := map[string]interface{}{
		"type":        "string",
		"description": "Absolute file path",
	}
	pathsProp := map[string]interface{}{
		"type":        "array",
		"items":       map[string]interface{}{"type": "string"},
		"description": "File or directory paths",
	}
	rootProp := map[string]interface{}{
		"type":        "string",
		"description": "Project root (auto-detected if omitted)",
	}

	return []mcpTool{
		{
			Name:        "syms_list",
			Description: "Extract top-level symbols (functions, classes, types, constants) from source files",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"paths":     pathsProp,
					"recursive": map[string]interface{}{"type": "boolean", "description": "Scan directories recursively"},
				},
				"required": []string{"paths"},
			},
		},
		{
			Name:        "syms_imports",
			Description: "Parse import statements from source files",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"paths":     pathsProp,
					"recursive": map[string]interface{}{"type": "boolean", "description": "Scan directories recursively"},
				},
				"required": []string{"paths"},
			},
		},
		{
			Name:        "syms_deps",
			Description: "List files that a given file depends on (imports from)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file":       pathProp,
					"transitive": map[string]interface{}{"type": "boolean", "description": "Include transitive dependencies"},
					"root":       rootProp,
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "syms_dependents",
			Description: "List files that depend on (import from) a given file",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file":       pathProp,
					"transitive": map[string]interface{}{"type": "boolean", "description": "Include transitive dependents"},
					"root":       rootProp,
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "syms_impact",
			Description: "Impact analysis: direct and transitive dependents of a file",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file": pathProp,
					"root": rootProp,
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "syms_search",
			Description: "Search for symbols by name across a project. Matches are ranked: exact > prefix > contains (case-insensitive).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Symbol name to search for",
					},
					"root": rootProp,
				},
				"required": []string{"query", "root"},
			},
		},
		{
			Name:        "syms_graph",
			Description: "Project-wide dependency graph summary with hot spots, heaviest importers, and circular dependencies",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"root": map[string]interface{}{
						"type":        "string",
						"description": "Project root directory",
					},
				},
				"required": []string{"root"},
			},
		},
	}
}

// ── Tool handlers ───────────────────────────────────────────────────────────

func handleToolCall(name string, args json.RawMessage) mcpToolResult {
	switch name {
	case "syms_list":
		return handleList(args)
	case "syms_imports":
		return handleImports(args)
	case "syms_deps":
		return handleDeps(args)
	case "syms_dependents":
		return handleDependents(args)
	case "syms_impact":
		return handleImpact(args)
	case "syms_search":
		return handleSearch(args)
	case "syms_graph":
		return handleGraph(args)
	default:
		return mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "unknown tool: " + name}},
			IsError: true,
		}
	}
}

func handleList(args json.RawMessage) mcpToolResult {
	var p struct {
		Paths     []string `json:"paths"`
		Recursive bool     `json:"recursive"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	files := collectFiles(p.Paths, p.Recursive)
	if len(files) == 0 {
		return textResult("No supported files found.")
	}
	results := ExtractSymbolsParallel(files)
	return jsonResult(results)
}

func handleImports(args json.RawMessage) mcpToolResult {
	var p struct {
		Paths     []string `json:"paths"`
		Recursive bool     `json:"recursive"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	files := collectFiles(p.Paths, p.Recursive)
	if len(files) == 0 {
		return textResult("No supported files found.")
	}
	var results []ImportResult
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			results = append(results, ImportResult{File: f, Error: err.Error()})
			continue
		}
		results = append(results, ExtractImports(f, string(data)))
	}
	return jsonResult(results)
}

func handleDeps(args json.RawMessage) mcpToolResult {
	var p struct {
		File       string `json:"file"`
		Transitive bool   `json:"transitive"`
		Root       string `json:"root"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	graph, _ := buildGraphForFile(p.File, p.Root)
	deps := graph.Deps(p.File, p.Transitive)

	relDeps := make([]string, len(deps))
	for i, d := range deps {
		relDeps[i] = graph.rel(d)
	}
	return jsonResult(map[string]interface{}{
		"file": graph.rel(absPath(p.File)),
		"deps": relDeps,
	})
}

func handleDependents(args json.RawMessage) mcpToolResult {
	var p struct {
		File       string `json:"file"`
		Transitive bool   `json:"transitive"`
		Root       string `json:"root"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	graph, _ := buildGraphForFile(p.File, p.Root)
	deps := graph.Dependents(p.File, p.Transitive)

	relDeps := make([]string, len(deps))
	for i, d := range deps {
		relDeps[i] = graph.rel(d)
	}
	return jsonResult(map[string]interface{}{
		"file":       graph.rel(absPath(p.File)),
		"dependents": relDeps,
	})
}

func handleImpact(args json.RawMessage) mcpToolResult {
	var p struct {
		File string `json:"file"`
		Root string `json:"root"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	graph, _ := buildGraphForFile(p.File, p.Root)
	result := graph.Impact(p.File)
	return jsonResult(result)
}

func handleSearch(args json.RawMessage) mcpToolResult {
	var p struct {
		Query string `json:"query"`
		Root  string `json:"root"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}
	root := p.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return errResult("invalid root: " + err.Error())
	}
	files := collectFiles([]string{absRoot}, true)
	results := SearchSymbols(absRoot, files, p.Query)
	return jsonResult(results)
}

func handleGraph(args json.RawMessage) mcpToolResult {
	var p struct {
		Root string `json:"root"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	root := p.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return errResult("invalid root: " + err.Error())
	}
	files := collectFiles([]string{absRoot}, true)
	graph := BuildGraph(absRoot, files)
	summary := graph.Summary()
	return jsonResult(summary)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func absPath(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

func textResult(text string) mcpToolResult {
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: text}}}
}

func errResult(text string) mcpToolResult {
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: text}}, IsError: true}
}

func jsonResult(v interface{}) mcpToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("json marshal error: " + err.Error())
	}
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(data)}}}
}

// ── Server loop ─────────────────────────────────────────────────────────────

func readRPCMessage(reader *bufio.Reader) ([]byte, rpcTransportMode, error) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF && len(line) == 0 {
				return nil, transportHeader, io.EOF
			}
			if err != io.EOF {
				return nil, transportHeader, err
			}
		}

		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			if err == io.EOF {
				return nil, transportHeader, io.EOF
			}
			continue
		}

		lower := strings.ToLower(string(trimmed))
		if strings.HasPrefix(lower, "content-length:") {
			val := strings.TrimSpace(string(trimmed[len("content-length:"):]))
			contentLength, parseErr := strconv.Atoi(val)
			if parseErr != nil || contentLength < 0 {
				return nil, transportHeader, fmt.Errorf("invalid Content-Length: %q", val)
			}

			for {
				hdrLine, hdrErr := reader.ReadBytes('\n')
				if hdrErr != nil {
					return nil, transportHeader, hdrErr
				}
				if len(bytes.TrimSpace(hdrLine)) == 0 {
					break
				}
			}

			body := make([]byte, contentLength)
			if _, readErr := io.ReadFull(reader, body); readErr != nil {
				return nil, transportHeader, readErr
			}
			return body, transportHeader, nil
		}

		return trimmed, transportLine, nil
	}
}

func writeRPCResponse(w io.Writer, mode rpcTransportMode, resp jsonRPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if mode == transportLine {
		_, err = w.Write(append(data, '\n'))
		return err
	}

	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func runMCP() {
	reader := bufio.NewReader(os.Stdin)

	for {
		msg, mode, err := readRPCMessage(reader)
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintf(os.Stderr, "mcp: read error: %v\n", err)
			return
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &rpcError{Code: -32700, Message: "parse error"},
			}
			if writeErr := writeRPCResponse(os.Stdout, mode, resp); writeErr != nil {
				fmt.Fprintf(os.Stderr, "mcp: write error: %v\n", writeErr)
				return
			}
			continue
		}

		// Notifications (no ID) — no response needed
		if req.ID == nil {
			continue
		}

		var result interface{}
		var rpcErr *rpcError

		switch req.Method {
		case "initialize":
			result = mcpInitResult{
				ProtocolVersion: "2025-11-25",
				Capabilities:    mcpCapabilities{Tools: &mcpToolsCap{}},
				ServerInfo:      mcpServerInfo{Name: "syms", Version: "1.0.0"},
			}

		case "tools/list":
			result = mcpToolsListResult{Tools: mcpTools()}

		case "tools/call":
			var params mcpToolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				rpcErr = &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
			} else {
				result = handleToolCall(params.Name, params.Arguments)
			}

		case "ping":
			result = map[string]interface{}{}

		default:
			rpcErr = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
			Error:   rpcErr,
		}
		if writeErr := writeRPCResponse(os.Stdout, mode, resp); writeErr != nil {
			fmt.Fprintf(os.Stderr, "mcp: write error: %v\n", writeErr)
			return
		}
	}
}
