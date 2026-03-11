package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// SearchResult holds a matched symbol with its file context.
type SearchResult struct {
	File   string `json:"file"`
	Symbol Symbol `json:"symbol"`
}

// SearchSymbols searches for symbols matching a query across all files.
// Matches are ranked: exact > prefix > contains (case-insensitive).
func SearchSymbols(root string, files []string, query string) []SearchResult {
	results := ExtractSymbolsParallel(files)
	queryLower := strings.ToLower(query)

	var exact, prefix, contains []SearchResult

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	for _, r := range results {
		if r.Error != "" {
			continue
		}
		relFile := r.File
		if rel, err := filepath.Rel(absRoot, r.File); err == nil {
			relFile = filepath.ToSlash(rel)
		}

		for _, sym := range r.Symbols {
			// Strip params from name for matching (e.g. "foo(bar, baz)" → "foo")
			name := sym.Name
			if idx := strings.Index(name, "("); idx >= 0 {
				name = name[:idx]
			}
			nameLower := strings.ToLower(name)

			sr := SearchResult{File: relFile, Symbol: sym}

			if nameLower == queryLower {
				exact = append(exact, sr)
			} else if strings.HasPrefix(nameLower, queryLower) {
				prefix = append(prefix, sr)
			} else if strings.Contains(nameLower, queryLower) {
				contains = append(contains, sr)
			}
		}
	}

	// Sort each group by file path for stable output
	sortResults := func(s []SearchResult) {
		sort.Slice(s, func(i, j int) bool {
			if s[i].File != s[j].File {
				return s[i].File < s[j].File
			}
			return s[i].Symbol.Line < s[j].Symbol.Line
		})
	}
	sortResults(exact)
	sortResults(prefix)
	sortResults(contains)

	all := make([]SearchResult, 0, len(exact)+len(prefix)+len(contains))
	all = append(all, exact...)
	all = append(all, prefix...)
	all = append(all, contains...)
	return all
}

// FormatSearchText formats search results as human-readable text.
func FormatSearchText(results []SearchResult, query string) string {
	if len(results) == 0 {
		return fmt.Sprintf("No symbols matching %q found.\n", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d symbols matching %q:\n\n", len(results), query)
	for _, r := range results {
		fmt.Fprintf(&b, "  %s %s  %s:%d\n", r.Symbol.Kind, r.Symbol.Name, r.File, r.Symbol.Line)
	}
	return b.String()
}
