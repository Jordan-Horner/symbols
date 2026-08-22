package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type SymbolDefinition struct {
	File   string `json:"file"`
	Symbol Symbol `json:"symbol"`
}

type SymbolReference struct {
	File         string `json:"file"`
	Line         int    `json:"line"`
	Text         string `json:"text,omitempty"`
	IsDefinition bool   `json:"is_definition,omitempty"`
}

type SymbolEdgesResult struct {
	Query       string             `json:"query"`
	Definitions []SymbolDefinition `json:"definitions"`
	References  []SymbolReference  `json:"references"`
}

func FindSymbolEdges(root string, files []string, query string, opts ExtractionOptions, includeDefinitions bool) SymbolEdgesResult {
	allMatches := SearchSymbolsWithKindsAndOptions(root, files, query, nil, opts)

	var defs []SymbolDefinition
	for _, m := range allMatches {
		if symbolBaseName(m.Symbol.Name) == query {
			defs = append(defs, SymbolDefinition{File: m.File, Symbol: m.Symbol})
		}
	}
	if len(defs) == 0 {
		for _, m := range allMatches {
			if strings.EqualFold(symbolBaseName(m.Symbol.Name), query) {
				defs = append(defs, SymbolDefinition{File: m.File, Symbol: m.Symbol})
			}
		}
	}

	defSet := make(map[string]bool)
	for _, d := range defs {
		defSet[fmt.Sprintf("%s:%d", d.File, d.Symbol.Line)] = true
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	pat := regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(query) + `([^A-Za-z0-9_]|$)`)
	var refs []SymbolReference
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		rel := f
		if r, err := filepath.Rel(absRoot, f); err == nil {
			rel = filepath.ToSlash(r)
		}

		sc := bufio.NewScanner(strings.NewReader(string(data)))
		sc.Buffer(make([]byte, 1024*1024), 1024*1024) // handle lines up to 1MB (minified JS)
		line := 0
		for sc.Scan() {
			line++
			txt := sc.Text()
			if !pat.MatchString(txt) {
				continue
			}
			isDef := defSet[fmt.Sprintf("%s:%d", rel, line)]
			if isDef && !includeDefinitions {
				continue
			}
			snippet := strings.TrimSpace(txt)
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			refs = append(refs, SymbolReference{
				File:         rel,
				Line:         line,
				Text:         snippet,
				IsDefinition: isDef,
			})
		}
		_ = sc.Err() // scanner errors (e.g. line > max buffer) silently skip the file
	}

	sort.Slice(defs, func(i, j int) bool {
		if defs[i].File != defs[j].File {
			return defs[i].File < defs[j].File
		}
		return defs[i].Symbol.Line < defs[j].Symbol.Line
	})
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].File != refs[j].File {
			return refs[i].File < refs[j].File
		}
		return refs[i].Line < refs[j].Line
	})

	return SymbolEdgesResult{
		Query:       query,
		Definitions: defs,
		References:  refs,
	}
}

func FormatSymbolEdgesText(r SymbolEdgesResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Symbol edges for %q\n\n", r.Query)
	fmt.Fprintf(&b, "Definitions: %d\n", len(r.Definitions))
	for _, d := range r.Definitions {
		fmt.Fprintf(&b, "  %s %s  %s:%d\n", d.Symbol.Kind, d.Symbol.Name, d.File, d.Symbol.Line)
	}
	fmt.Fprintf(&b, "\nReferences: %d\n", len(r.References))
	for _, ref := range r.References {
		tag := ""
		if ref.IsDefinition {
			tag = " [def]"
		}
		fmt.Fprintf(&b, "  %s:%d%s\n", ref.File, ref.Line, tag)
	}
	return b.String()
}
