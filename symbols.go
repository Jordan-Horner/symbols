package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Symbol represents a top-level declaration in a source file.
type Symbol struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Line int    `json:"line"`
}

// SymbolResult holds extraction results for a single file.
type SymbolResult struct {
	File    string   `json:"file"`
	Lines   int      `json:"lines,omitempty"`
	Symbols []Symbol `json:"symbols"`
	Error   string   `json:"error,omitempty"`
}

// Python: extract via regex on def/class lines
var pyDefRe = regexp.MustCompile(`^(async\s+)?def\s+(\w+)\s*\(`)
var pyClassRe = regexp.MustCompile(`^class\s+(\w+)`)
var pyAssignRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\s*(?::\s*[A-Za-z][\w\[\], .]*\s*)?=\s*[^=]`)

func extractSymbolsPython(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		// Only top-level (no leading whitespace)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			continue
		}
		if m := pyDefRe.FindStringSubmatch(trimmed); m != nil {
			kind := "def"
			if m[1] != "" {
				kind = "async def"
			}
			symbols = append(symbols, Symbol{Name: m[2], Kind: kind, Line: lineno})
		} else if m := pyClassRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: lineno})
		} else if m := pyAssignRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			kind := "variable"
			if name == strings.ToUpper(name) && len(name) > 1 {
				kind = "constant"
			}
			symbols = append(symbols, Symbol{Name: name, Kind: kind, Line: lineno})
		}
	}
	return symbols
}

// TS/JS/Svelte: regex on export/function/class/interface/type/enum
var tsFuncClassRe = regexp.MustCompile(`^(?:export\s+(?:default\s+)?)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?(function\*?|class|interface|type|enum)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
var tsVarExportRe = regexp.MustCompile(`^export\s+(?:declare\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

func extractSymbolsTS(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if m := tsFuncClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: m[1], Line: lineno})
		} else if m := tsVarExportRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "export const", Line: lineno})
		}
	}
	return symbols
}

// Go: regex for func/type declarations
var goFuncRe = regexp.MustCompile(`^func\s+(?:\(\s*\w+\s+\*?\w+\s*\)\s+)?(\w+)\s*\(`)
var goTypeRe = regexp.MustCompile(`^type\s+(\w+)\s+`)

func extractSymbolsGo(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if m := goFuncRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "function", Line: lineno})
		} else if m := goTypeRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "type", Line: lineno})
		}
	}
	return symbols
}

// Java/Kotlin: regex for class/interface/enum/fun declarations
var javaClassRe = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+)?(?:abstract\s+|final\s+|static\s+)*(class|interface|enum)\s+(\w+)`)
var javaMethodRe = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+)?(?:abstract\s+|static\s+|final\s+|synchronized\s+)*(?:[\w<>\[\],\s]+)\s+(\w+)\s*\(`)

func extractSymbolsJava(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if m := javaClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: m[1], Line: lineno})
		}
	}
	return symbols
}

// Rust: fn/struct/enum/trait/impl
var rustFnRe = regexp.MustCompile(`^(?:pub\s+)?(?:async\s+)?fn\s+(\w+)`)
var rustTypeRe = regexp.MustCompile(`^(?:pub\s+)?(struct|enum|trait)\s+(\w+)`)
var rustImplRe = regexp.MustCompile(`^impl(?:<[^>]*>)?\s+(\w+)`)

func extractSymbolsRust(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if m := rustFnRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "function", Line: lineno})
		} else if m := rustTypeRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: m[1], Line: lineno})
		} else if m := rustImplRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "impl", Line: lineno})
		}
	}
	return symbols
}

// C#: class/interface/struct/enum
var csharpClassRe = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|internal\s+)?(?:abstract\s+|static\s+|sealed\s+|partial\s+)*(class|interface|struct|enum|record)\s+(\w+)`)

func extractSymbolsCSharp(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if m := csharpClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: m[1], Line: lineno})
		}
	}
	return symbols
}

// PHP: function/class
var phpFuncRe = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+)?(?:static\s+)?function\s+(\w+)`)
var phpClassRe = regexp.MustCompile(`^\s*(?:abstract\s+|final\s+)?class\s+(\w+)`)

func extractSymbolsPHP(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if m := phpFuncRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "function", Line: lineno})
		} else if m := phpClassRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: lineno})
		}
	}
	return symbols
}

// Bash: function declarations
var bashFuncRe = regexp.MustCompile(`^(?:function\s+)?(\w+)\s*\(\s*\)`)

func extractSymbolsBash(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if m := bashFuncRe.FindStringSubmatch(line); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "function", Line: lineno})
		}
	}
	return symbols
}

// ExtractSymbols dispatches to the right extractor based on file extension.
// Prefers tree-sitter for richer output (function signatures with args),
// falls back to regex if tree-sitter is unavailable for a language.
func ExtractSymbols(filePath, content string) SymbolResult {
	ext := strings.ToLower(filepath.Ext(filePath))
	lines := strings.Count(content, "\n") + 1

	// Try tree-sitter first for richer extraction
	if HasTreeSitter(ext) {
		symbols := ExtractSymbolsTreeSitter(ext, []byte(content))
		if symbols != nil {
			return SymbolResult{File: filePath, Lines: lines, Symbols: symbols}
		}
	}

	// Regex fallback
	var symbols []Symbol
	switch ext {
	case ".py":
		symbols = extractSymbolsPython(content)
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte":
		symbols = extractSymbolsTS(content)
	case ".go":
		symbols = extractSymbolsGo(content)
	case ".java":
		symbols = extractSymbolsJava(content)
	case ".kt", ".kts":
		symbols = extractSymbolsJava(content)
	case ".rs":
		symbols = extractSymbolsRust(content)
	case ".cs":
		symbols = extractSymbolsCSharp(content)
	case ".php":
		symbols = extractSymbolsPHP(content)
	case ".sh", ".bash":
		symbols = extractSymbolsBash(content)
	case ".c", ".h", ".cpp", ".cc", ".cxx", ".hpp":
		symbols = extractSymbolsRust(content)
	case ".rb", ".scala", ".swift":
		// These only work with tree-sitter, no regex fallback
		return SymbolResult{File: filePath, Lines: lines, Symbols: []Symbol{}}
	default:
		return SymbolResult{File: filePath, Lines: lines, Error: "unsupported extension: " + ext}
	}

	if symbols == nil {
		symbols = []Symbol{}
	}
	return SymbolResult{File: filePath, Lines: lines, Symbols: symbols}
}

// ReadAndExtractSymbols reads a file and extracts symbols.
func ReadAndExtractSymbols(filePath string) SymbolResult {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SymbolResult{File: filePath, Error: err.Error(), Symbols: []Symbol{}}
	}
	return ExtractSymbols(filePath, string(data))
}

// ExtractSymbolsParallel processes multiple files concurrently.
func ExtractSymbolsParallel(files []string) []SymbolResult {
	results := make([]SymbolResult, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32) // limit concurrency

	for i, f := range files {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			results[idx] = ReadAndExtractSymbols(path)
			<-sem
		}(i, f)
	}
	wg.Wait()
	return results
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func buildKindSet(kinds []string) map[string]bool {
	set := make(map[string]bool)
	for _, raw := range kinds {
		for _, part := range strings.Split(raw, ",") {
			k := normalizeKind(part)
			if k != "" {
				set[k] = true
			}
		}
	}
	return set
}

func filterSymbolsByKind(symbols []Symbol, kindSet map[string]bool) []Symbol {
	if len(kindSet) == 0 {
		return symbols
	}
	filtered := make([]Symbol, 0, len(symbols))
	for _, s := range symbols {
		if kindSet[normalizeKind(s.Kind)] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func filterSymbolResultsByKind(results []SymbolResult, kinds []string) []SymbolResult {
	kindSet := buildKindSet(kinds)
	if len(kindSet) == 0 {
		return results
	}
	filtered := make([]SymbolResult, len(results))
	for i, r := range results {
		filtered[i] = r
		filtered[i].Symbols = filterSymbolsByKind(r.Symbols, kindSet)
	}
	return filtered
}

// FormatSymbolResult formats a single result as human-readable text.
func FormatSymbolResult(r SymbolResult) string {
	var b strings.Builder
	if r.Error != "" {
		fmt.Fprintf(&b, "### `%s` — error: %s\n", r.File, r.Error)
		return b.String()
	}
	if len(r.Symbols) == 0 {
		fmt.Fprintf(&b, "### `%s` — %d lines (no symbols found)\n", r.File, r.Lines)
		return b.String()
	}
	fmt.Fprintf(&b, "### `%s` — %d lines\n\n", r.File, r.Lines)
	for _, s := range r.Symbols {
		fmt.Fprintf(&b, "  %s %s  # line %d\n", s.Kind, s.Name, s.Line)
	}
	return b.String()
}
