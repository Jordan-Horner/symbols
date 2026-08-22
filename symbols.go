package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// Symbol represents a top-level declaration in a source file.
type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Line      int    `json:"line"`
	Exported  *bool  `json:"exported,omitempty"`
	StartLine *int   `json:"start_line,omitempty"`
	EndLine   *int   `json:"end_line,omitempty"`
	StartCol  *int   `json:"start_col,omitempty"`
	EndCol    *int   `json:"end_col,omitempty"`
}

// SymbolResult holds extraction results for a single file.
type SymbolResult struct {
	File    string   `json:"file"`
	Lines   int      `json:"lines,omitempty"`
	Symbols []Symbol `json:"symbols"`
	Error   string   `json:"error,omitempty"`
}

type ExtractionOptions struct {
	IncludeRanges bool
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

// C/C++: fallback for when tree-sitter is unavailable.
// Limited to common primitives for return types to avoid false positives;
// tree-sitter handles richer C/C++ extraction correctly.
var cFuncRe = regexp.MustCompile(`^(?:(?:static|extern|inline)\s+)*` +
	`(?:(?:const|volatile|unsigned|signed|long|short)\s+)*` +
	`(?:void|int|char|long|short|float|double|bool|size_t)\s*\*?\s*` +
	`(\w+)\s*\(`)
var cTypeRe = regexp.MustCompile(`^(?:typedef\s+)?(?:(struct|enum|union))\s+(\w+)`)
var cppClassRe = regexp.MustCompile(`^(?:template\s*<[^>]*>\s*)?class\s+(\w+)`)

func extractSymbolsC(content string) []Symbol {
	var symbols []Symbol
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if m := cTypeRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[2], Kind: m[1], Line: lineno})
		} else if m := cppClassRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "class", Line: lineno})
		} else if m := cFuncRe.FindStringSubmatch(trimmed); m != nil {
			symbols = append(symbols, Symbol{Name: m[1], Kind: "function", Line: lineno})
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
	return ExtractSymbolsWithOptions(filePath, content, ExtractionOptions{})
}

func ExtractSymbolsWithOptions(filePath, content string, opts ExtractionOptions) SymbolResult {
	ext := strings.ToLower(filepath.Ext(filePath))
	lines := strings.Count(content, "\n") + 1

	// Try tree-sitter first for richer extraction
	if HasTreeSitter(ext) {
		symbols := ExtractSymbolsTreeSitterWithOptions(ext, []byte(content), opts)
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
		symbols = extractSymbolsC(content)
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
	return ReadAndExtractSymbolsWithOptions(filePath, ExtractionOptions{})
}

func ReadAndExtractSymbolsWithOptions(filePath string, opts ExtractionOptions) SymbolResult {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SymbolResult{File: filePath, Error: err.Error(), Symbols: []Symbol{}}
	}
	return ExtractSymbolsWithOptions(filePath, string(data), opts)
}

// ReadAndExtractWithExports reads a file, extracts symbols, and marks exports.
func ReadAndExtractWithExports(filePath string, opts ExtractionOptions) SymbolResult {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SymbolResult{File: filePath, Error: err.Error(), Symbols: []Symbol{}}
	}
	content := string(data)
	result := ExtractSymbolsWithOptions(filePath, content, opts)
	MarkExports(&result, content)
	return result
}

// ExtractSymbolsParallelWithExports processes files concurrently and marks export visibility.
func ExtractSymbolsParallelWithExports(files []string, opts ExtractionOptions) []SymbolResult {
	results := make([]SymbolResult, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)

	for i, f := range files {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			results[idx] = ReadAndExtractWithExports(path, opts)
			<-sem
		}(i, f)
	}
	wg.Wait()
	return results
}

// ExtractSymbolsParallel processes multiple files concurrently.
func ExtractSymbolsParallel(files []string) []SymbolResult {
	return ExtractSymbolsParallelWithOptions(files, ExtractionOptions{})
}

// ExtractSymbolsParallelWithOptions processes multiple files concurrently with extraction options.
func ExtractSymbolsParallelWithOptions(files []string, opts ExtractionOptions) []SymbolResult {
	results := make([]SymbolResult, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32) // limit concurrency

	for i, f := range files {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			results[idx] = ReadAndExtractSymbolsWithOptions(path, opts)
			<-sem
		}(i, f)
	}
	wg.Wait()
	return results
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

// baseKind returns the terminal word of a compound kind so that filters like
// "function" match compound kinds like "async function", "export function", etc.
func baseKind(kind string) string {
	if i := strings.LastIndex(kind, " "); i >= 0 {
		return kind[i+1:]
	}
	return kind
}

func symbolBaseName(name string) string {
	if idx := strings.Index(name, "("); idx >= 0 {
		return name[:idx]
	}
	return name
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
		norm := normalizeKind(s.Kind)
		if kindSet[norm] || kindSet[baseKind(norm)] {
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

// ── Export detection ─────────────────────────────────────────────────────────

var pyAllRe = regexp.MustCompile(`(?m)^__all__\s*=\s*\[([^\]]*)\]`)
var cjsModuleObjectRe = regexp.MustCompile(`(?s)module\.exports\s*=\s*\{([^}]*)\}`)
var cjsNamedExportRe = regexp.MustCompile(`(?m)(?:module\.)?exports\.([A-Za-z_$][\w$]*)\s*=`)
var cjsModuleDefaultRe = regexp.MustCompile(`(?m)module\.exports\s*=\s*([A-Za-z_$][\w$]*)\s*;?`)
var cjsIdentifierRe = regexp.MustCompile(`[A-Za-z_$][\w$]*`)

func boolPtr(b bool) *bool { return &b }

// MarkExports annotates each symbol with Exported based on language visibility rules.
func MarkExports(result *SymbolResult, content string) {
	ext := strings.ToLower(filepath.Ext(result.File))

	switch ext {
	case ".py":
		markExportsPython(result, content)
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte":
		markExportsTS(result, content)
	case ".go":
		markExportsGo(result)
	case ".rs":
		markExportsRust(result, content)
	case ".java", ".kt", ".kts":
		markExportsJava(result, content)
	case ".cs":
		markExportsCSharp(result, content)
	case ".php":
		markExportsPHP(result, content)
	default:
		// No export rules for this language — mark all as exported
		for i := range result.Symbols {
			result.Symbols[i].Exported = boolPtr(true)
		}
	}
}

func markExportsPython(result *SymbolResult, content string) {
	// If __all__ is defined, only those names are exported
	allMatch := pyAllRe.FindStringSubmatch(content)
	if allMatch != nil {
		allNames := make(map[string]bool)
		for _, name := range strings.Split(allMatch[1], ",") {
			name = strings.TrimSpace(name)
			name = strings.Trim(name, `"'`)
			if name != "" {
				allNames[name] = true
			}
		}
		for i := range result.Symbols {
			baseName := symbolBaseName(result.Symbols[i].Name)
			result.Symbols[i].Exported = boolPtr(allNames[baseName])
		}
		return
	}
	// No __all__: public = not starting with underscore
	for i := range result.Symbols {
		baseName := symbolBaseName(result.Symbols[i].Name)
		result.Symbols[i].Exported = boolPtr(!strings.HasPrefix(baseName, "_"))
	}
}

func markExportsTS(result *SymbolResult, content string) {
	lines := strings.Split(content, "\n")
	commonJSExports := commonJSExportNames(content)
	for i := range result.Symbols {
		kind := result.Symbols[i].Kind
		if strings.HasPrefix(kind, "export") {
			result.Symbols[i].Exported = boolPtr(true)
			continue
		}
		baseName := symbolBaseName(result.Symbols[i].Name)
		if commonJSExports[baseName] {
			result.Symbols[i].Exported = boolPtr(true)
			continue
		}
		// Tree-sitter may store the inner kind (function, class, etc.) even when
		// wrapped in export_statement. Check the source line for "export ".
		line := result.Symbols[i].Line
		if line > 0 && line <= len(lines) {
			trimmed := strings.TrimSpace(lines[line-1])
			result.Symbols[i].Exported = boolPtr(strings.HasPrefix(trimmed, "export "))
		} else {
			result.Symbols[i].Exported = boolPtr(false)
		}
	}
}

func commonJSExportNames(content string) map[string]bool {
	names := make(map[string]bool)

	for _, match := range cjsModuleObjectRe.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		for _, entry := range strings.Split(match[1], ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			entry = strings.SplitN(entry, "//", 2)[0]
			entry = strings.SplitN(entry, "/*", 2)[0]
			entry = strings.TrimSpace(entry)
			if entry == "" || strings.HasPrefix(entry, "...") {
				continue
			}
			if colon := strings.Index(entry, ":"); colon >= 0 {
				entry = strings.TrimSpace(entry[colon+1:])
			}
			if match := cjsIdentifierRe.FindString(entry); match != "" {
				names[match] = true
			}
		}
	}

	for _, match := range cjsNamedExportRe.FindAllStringSubmatch(content, -1) {
		if len(match) >= 2 {
			names[match[1]] = true
		}
	}

	for _, match := range cjsModuleDefaultRe.FindAllStringSubmatch(content, -1) {
		if len(match) >= 2 && match[1] != "exports" {
			names[match[1]] = true
		}
	}

	return names
}

func markExportsGo(result *SymbolResult) {
	for i := range result.Symbols {
		name := symbolBaseName(result.Symbols[i].Name)
		if len(name) == 0 {
			result.Symbols[i].Exported = boolPtr(false)
			continue
		}
		// Go exports are identifiers starting with an uppercase letter.
		// Use []rune conversion so multi-byte Unicode identifiers (e.g. Ä) work.
		first := []rune(name)[0]
		result.Symbols[i].Exported = boolPtr(unicode.IsUpper(first))
	}
}

func markExportsRust(result *SymbolResult, content string) {
	// Check if the source line starts with "pub "
	lines := strings.Split(content, "\n")
	for i := range result.Symbols {
		line := result.Symbols[i].Line
		if line > 0 && line <= len(lines) {
			trimmed := strings.TrimSpace(lines[line-1])
			result.Symbols[i].Exported = boolPtr(strings.HasPrefix(trimmed, "pub ") || strings.HasPrefix(trimmed, "pub("))
		} else {
			result.Symbols[i].Exported = boolPtr(false)
		}
	}
}

func markExportsJava(result *SymbolResult, content string) {
	lines := strings.Split(content, "\n")
	for i := range result.Symbols {
		line := result.Symbols[i].Line
		if line > 0 && line <= len(lines) {
			trimmed := strings.TrimSpace(lines[line-1])
			// Java/Kotlin: public is exported; package-private (no modifier) is semi-exported
			// For API documentation purposes, treat non-private as exported
			result.Symbols[i].Exported = boolPtr(!strings.Contains(trimmed, "private "))
		} else {
			result.Symbols[i].Exported = boolPtr(true)
		}
	}
}

func markExportsCSharp(result *SymbolResult, content string) {
	lines := strings.Split(content, "\n")
	for i := range result.Symbols {
		line := result.Symbols[i].Line
		if line > 0 && line <= len(lines) {
			trimmed := strings.TrimSpace(lines[line-1])
			result.Symbols[i].Exported = boolPtr(strings.Contains(trimmed, "public ") || strings.Contains(trimmed, "internal "))
		} else {
			result.Symbols[i].Exported = boolPtr(false)
		}
	}
}

func markExportsPHP(result *SymbolResult, content string) {
	lines := strings.Split(content, "\n")
	for i := range result.Symbols {
		line := result.Symbols[i].Line
		if line > 0 && line <= len(lines) {
			trimmed := strings.TrimSpace(lines[line-1])
			result.Symbols[i].Exported = boolPtr(!strings.Contains(trimmed, "private "))
		} else {
			result.Symbols[i].Exported = boolPtr(true)
		}
	}
}

// FilterExported returns only symbols marked as exported.
func FilterExported(results []SymbolResult) []SymbolResult {
	filtered := make([]SymbolResult, len(results))
	for i, r := range results {
		filtered[i] = SymbolResult{File: r.File, Lines: r.Lines, Error: r.Error}
		for _, s := range r.Symbols {
			if s.Exported != nil && *s.Exported {
				filtered[i].Symbols = append(filtered[i].Symbols, s)
			}
		}
		if filtered[i].Symbols == nil {
			filtered[i].Symbols = []Symbol{}
		}
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
