package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Import represents a parsed import statement.
type Import struct {
	Module string   `json:"module"`
	Kind   string   `json:"kind"`
	Names  []string `json:"names,omitempty"`
	Line   int      `json:"line"`
}

// ImportResult holds imports extracted from a single file.
type ImportResult struct {
	File    string   `json:"file"`
	Imports []Import `json:"imports"`
	Error   string   `json:"error,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// ── Python ──────────────────────────────────────────────────────────────────

var pyImportRe = regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_][\w.]*(?:\s*,\s*[A-Za-z_][\w.]*)*)`)
var pyFromImportRe = regexp.MustCompile(`(?m)^\s*from\s+(\.{0,3}[A-Za-z_]?[\w.]*)\s+import\s+(.+)`)

func extractImportsPython(content string) []Import {
	var imports []Import
	type key struct {
		module string
		line   int
	}
	seen := make(map[key]bool)

	for _, m := range pyImportRe.FindAllStringSubmatchIndex(content, -1) {
		start := m[0]
		line := strings.Count(content[:start], "\n") + 1
		prefix := strings.TrimRight(content[:start], " \t")
		if strings.HasSuffix(prefix, "from") {
			continue
		}
		moduleStr := content[m[2]:m[3]]
		for _, mod := range strings.Split(moduleStr, ",") {
			mod = strings.TrimSpace(mod)
			if idx := strings.Index(mod, " as "); idx >= 0 {
				mod = strings.TrimSpace(mod[:idx])
			}
			k := key{mod, line}
			if mod != "" && !seen[k] {
				seen[k] = true
				imports = append(imports, Import{Module: mod, Kind: "import", Line: line})
			}
		}
	}

	for _, m := range pyFromImportRe.FindAllStringSubmatchIndex(content, -1) {
		start := m[0]
		line := strings.Count(content[:start], "\n") + 1
		module := strings.TrimSpace(content[m[2]:m[3]])
		namesStr := strings.TrimSpace(content[m[4]:m[5]])
		namesStr = strings.TrimRight(namesStr, "\\")

		if strings.Contains(namesStr, "(") && !strings.Contains(namesStr, ")") {
			restStart := m[1]
			parenEnd := strings.Index(content[restStart:], ")")
			if parenEnd >= 0 {
				namesStr = namesStr + content[restStart:restStart+parenEnd]
			}
		}
		namesStr = strings.Trim(namesStr, "() \t")

		var names []string
		for _, n := range strings.Split(namesStr, ",") {
			n = strings.TrimSpace(n)
			if idx := strings.Index(n, " as "); idx >= 0 {
				n = strings.TrimSpace(n[:idx])
			}
			if n != "" {
				names = append(names, n)
			}
		}

		k := key{module, line}
		if module != "" && !seen[k] {
			seen[k] = true
			imports = append(imports, Import{Module: module, Kind: "from", Names: names, Line: line})
		}
	}

	return imports
}

// ── TypeScript / JavaScript / Svelte ────────────────────────────────────────

var tsImportRe = regexp.MustCompile(`(?m)(?:^|\n)\s*import\s+(?:(?:type\s+)?(?:\{[^}]*\}|[A-Za-z_$][A-Za-z0-9_$]*|\*)(?:\s*,\s*(?:\{[^}]*\}|\*))?(?:\s+from\s+)?)?['"]([^'"]+)['"]`)
var tsRequireRe = regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
var tsDynamicImportRe = regexp.MustCompile(`import\s*\(\s*['"]([^'"]+)['"]\s*\)`)
var tsExportFromRe = regexp.MustCompile(`(?m)(?:^|\n)\s*export\s+(?:type\s+)?(?:\{[^}]*\}|\*(?:\s+as\s+\w+)?)\s+from\s+['"]([^'"]+)['"]`)

func extractImportsTS(content string) []Import {
	var imports []Import
	seen := make(map[string]bool)

	type patternKind struct {
		re   *regexp.Regexp
		kind string
	}
	patterns := []patternKind{
		{tsImportRe, "import"},
		{tsRequireRe, "require"},
		{tsDynamicImportRe, "dynamic"},
		{tsExportFromRe, "re-export"},
	}

	for _, pk := range patterns {
		for _, m := range pk.re.FindAllStringSubmatchIndex(content, -1) {
			module := content[m[2]:m[3]]
			if !seen[module] {
				seen[module] = true
				line := strings.Count(content[:m[0]], "\n") + 1
				imports = append(imports, Import{Module: module, Kind: pk.kind, Line: line})
			}
		}
	}
	return imports
}

// ── Go ──────────────────────────────────────────────────────────────────────

var goImportSingleRe = regexp.MustCompile("(?m)^\\s*import\\s+(?:\\w+\\s+)?[\"` ]([^\"` ]+)[\"` ]")
var goImportBlockRe = regexp.MustCompile(`(?s)import\s*\((.*?)\)`)
var goImportLineRe = regexp.MustCompile("[\"` ]([^\"` ]+)[\"` ]")

func extractImportsGo(content string) []Import {
	var imports []Import
	seen := make(map[string]bool)

	for _, m := range goImportSingleRe.FindAllStringSubmatchIndex(content, -1) {
		pkg := content[m[2]:m[3]]
		if !seen[pkg] {
			seen[pkg] = true
			line := strings.Count(content[:m[0]], "\n") + 1
			imports = append(imports, Import{Module: pkg, Kind: "import", Line: line})
		}
	}

	for _, blockM := range goImportBlockRe.FindAllStringSubmatchIndex(content, -1) {
		blockStart := strings.Count(content[:blockM[0]], "\n") + 1
		blockContent := content[blockM[2]:blockM[3]]
		for i, blockLine := range strings.Split(blockContent, "\n") {
			lineM := goImportLineRe.FindStringSubmatch(blockLine)
			if lineM != nil {
				pkg := lineM[1]
				if !seen[pkg] {
					seen[pkg] = true
					imports = append(imports, Import{Module: pkg, Kind: "import", Line: blockStart + i})
				}
			}
		}
	}

	return imports
}

// ── Java ────────────────────────────────────────────────────────────────────

var javaImportStatementRe = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][\w.]*(?:\.\*)?)\s*;`)

func extractImportsJava(content string) []Import {
	var imports []Import
	for _, m := range javaImportStatementRe.FindAllStringSubmatchIndex(content, -1) {
		line := strings.Count(content[:m[0]], "\n") + 1
		imports = append(imports, Import{Module: content[m[2]:m[3]], Kind: "import", Line: line})
	}
	return imports
}

// ── Kotlin ──────────────────────────────────────────────────────────────────

var kotlinImportRe = regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_][\w.]*(?:\.\*)?)\s*$`)

func extractImportsKotlin(content string) []Import {
	var imports []Import
	for _, m := range kotlinImportRe.FindAllStringSubmatchIndex(content, -1) {
		line := strings.Count(content[:m[0]], "\n") + 1
		imports = append(imports, Import{Module: content[m[2]:m[3]], Kind: "import", Line: line})
	}
	return imports
}

// ── Rust ────────────────────────────────────────────────────────────────────

var rustUseRe = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?use\s+((?:crate|self|super|[a-z_]\w*)(?:::\w+)*(?:::\{[^}]+\}|::\*)?)\s*;`)
var rustModRe = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?mod\s+(\w+)\s*;`)

func extractImportsRust(content string) []Import {
	var imports []Import
	for _, m := range rustUseRe.FindAllStringSubmatchIndex(content, -1) {
		line := strings.Count(content[:m[0]], "\n") + 1
		imports = append(imports, Import{Module: content[m[2]:m[3]], Kind: "use", Line: line})
	}
	for _, m := range rustModRe.FindAllStringSubmatchIndex(content, -1) {
		line := strings.Count(content[:m[0]], "\n") + 1
		imports = append(imports, Import{Module: content[m[2]:m[3]], Kind: "mod", Line: line})
	}
	return imports
}

// ── C# ──────────────────────────────────────────────────────────────────────

var csharpUsingRe = regexp.MustCompile(`(?m)^\s*using\s+(?:static\s+)?([A-Za-z_][\w.]*)\s*;`)

func extractImportsCSharp(content string) []Import {
	var imports []Import
	for _, m := range csharpUsingRe.FindAllStringSubmatchIndex(content, -1) {
		line := strings.Count(content[:m[0]], "\n") + 1
		imports = append(imports, Import{Module: content[m[2]:m[3]], Kind: "using", Line: line})
	}
	return imports
}

// ── PHP ─────────────────────────────────────────────────────────────────────

var phpUseRe = regexp.MustCompile(`(?m)^\s*use\s+([A-Za-z_\\][\w\\]*(?:\s+as\s+\w+)?)\s*;`)
var phpRequireRe = regexp.MustCompile(`(?:require|include)(?:_once)?\s*[( ]?\s*['"]([^'"]+)['"]\s*[) ]?\s*;`)

func extractImportsPHP(content string) []Import {
	var imports []Import
	seen := make(map[string]bool)
	for _, m := range phpUseRe.FindAllStringSubmatchIndex(content, -1) {
		module := content[m[2]:m[3]]
		if idx := strings.Index(module, " as "); idx >= 0 {
			module = strings.TrimSpace(module[:idx])
		}
		if !seen[module] {
			seen[module] = true
			line := strings.Count(content[:m[0]], "\n") + 1
			imports = append(imports, Import{Module: module, Kind: "use", Line: line})
		}
	}
	for _, m := range phpRequireRe.FindAllStringSubmatchIndex(content, -1) {
		path := content[m[2]:m[3]]
		if !seen[path] {
			seen[path] = true
			line := strings.Count(content[:m[0]], "\n") + 1
			imports = append(imports, Import{Module: path, Kind: "require", Line: line})
		}
	}
	return imports
}

// ── Dispatch ────────────────────────────────────────────────────────────────

// ExtractImports extracts import statements from file content based on extension.
func ExtractImports(filePath, content string) ImportResult {
	ext := strings.ToLower(filepath.Ext(filePath))

	var imports []Import
	switch ext {
	case ".py":
		imports = extractImportsPython(content)
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte":
		imports = extractImportsTS(content)
	case ".go":
		imports = extractImportsGo(content)
	case ".java":
		imports = extractImportsJava(content)
	case ".kt", ".kts":
		imports = extractImportsKotlin(content)
	case ".rs":
		imports = extractImportsRust(content)
	case ".cs":
		imports = extractImportsCSharp(content)
	case ".php":
		imports = extractImportsPHP(content)
	default:
		return ImportResult{File: filePath, Note: "import extraction not supported"}
	}

	if imports == nil {
		imports = []Import{}
	}
	return ImportResult{File: filePath, Imports: imports}
}

// FormatImportsText formats an ImportResult as human-readable text.
func FormatImportsText(r ImportResult) string {
	var b strings.Builder
	b.WriteString("### `" + r.File + "`\n")
	if r.Error != "" {
		b.WriteString("  error: " + r.Error + "\n")
		return b.String()
	}
	if len(r.Imports) == 0 {
		b.WriteString("  (no imports)\n")
		return b.String()
	}
	for _, imp := range r.Imports {
		if len(imp.Names) > 0 && imp.Kind == "from" {
			names := imp.Names
			suffix := ""
			if len(names) > 5 {
				names = names[:5]
				suffix = ", ..."
			}
			b.WriteString("  from " + imp.Module + " import " + strings.Join(names, ", ") + suffix)
		} else {
			b.WriteString("  " + imp.Kind + " " + imp.Module)
		}
		b.WriteString("  # line " + itoa(imp.Line) + "\n")
	}
	return b.String()
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
