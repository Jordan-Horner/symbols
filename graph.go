package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// DepGraph is a directed dependency graph with query methods.
type DepGraph struct {
	Root       string
	Edges      map[string]map[string]bool // from → set of to
	Reverse    map[string]map[string]bool // to → set of from
	Unresolved map[string][]string        // from → list of unresolved specifiers
}

// NewDepGraph creates an empty dependency graph rooted at the given path.
func NewDepGraph(root string) *DepGraph {
	absRoot, _ := filepath.Abs(root)
	return &DepGraph{
		Root:       absRoot,
		Edges:      make(map[string]map[string]bool),
		Reverse:    make(map[string]map[string]bool),
		Unresolved: make(map[string][]string),
	}
}

func (g *DepGraph) addEdge(from, to string) {
	if g.Edges[from] == nil {
		g.Edges[from] = make(map[string]bool)
	}
	g.Edges[from][to] = true
	if g.Reverse[to] == nil {
		g.Reverse[to] = make(map[string]bool)
	}
	g.Reverse[to][from] = true
}

func (g *DepGraph) addUnresolved(from, specifier string) {
	g.Unresolved[from] = append(g.Unresolved[from], specifier)
}

func (g *DepGraph) rel(path string) string {
	r, err := filepath.Rel(g.Root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}

// Deps returns the direct (or transitive) dependencies of a file.
func (g *DepGraph) Deps(filePath string, transitive bool) []string {
	target, _ := filepath.Abs(filePath)
	if !transitive {
		return sortedKeys(g.Edges[target])
	}
	visited := make(map[string]bool)
	queue := []string{target}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for dep := range g.Edges[current] {
			if !visited[dep] && dep != target {
				visited[dep] = true
				queue = append(queue, dep)
			}
		}
	}
	return sortedKeysFromMap(visited)
}

// Dependents returns the direct (or transitive) dependents of a file.
func (g *DepGraph) Dependents(filePath string, transitive bool) []string {
	target, _ := filepath.Abs(filePath)
	if !transitive {
		return sortedKeys(g.Reverse[target])
	}
	visited := make(map[string]bool)
	queue := []string{target}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for dep := range g.Reverse[current] {
			if !visited[dep] && dep != target {
				visited[dep] = true
				queue = append(queue, dep)
			}
		}
	}
	return sortedKeysFromMap(visited)
}

// ImpactResult holds impact analysis data for a single file.
type ImpactResult struct {
	File                 string   `json:"file"`
	DirectDependents     int      `json:"direct_dependents"`
	TransitiveDependents int      `json:"transitive_dependents"`
	Direct               []string `json:"direct"`
	Indirect             []string `json:"indirect"`
}

// Impact computes impact analysis for a file.
func (g *DepGraph) Impact(filePath string) ImpactResult {
	target, _ := filepath.Abs(filePath)
	directSet := g.Reverse[target]
	transitiveList := g.Dependents(filePath, true)
	transitiveSet := make(map[string]bool)
	for _, f := range transitiveList {
		transitiveSet[f] = true
	}

	var direct, indirect []string
	for f := range directSet {
		direct = append(direct, g.rel(f))
	}
	sort.Strings(direct)
	for _, f := range transitiveList {
		if !directSet[f] {
			indirect = append(indirect, g.rel(f))
		}
	}
	sort.Strings(indirect)

	return ImpactResult{
		File:                 g.rel(target),
		DirectDependents:     len(directSet),
		TransitiveDependents: len(transitiveSet),
		Direct:               direct,
		Indirect:             indirect,
	}
}

// GraphSummary holds project-wide graph statistics.
type GraphSummary struct {
	TotalFiles        int                `json:"total_files"`
	TotalEdges        int                `json:"total_edges"`
	UnresolvedImports int                `json:"unresolved_imports"`
	HotSpots          []FileCount        `json:"hot_spots"`
	HeaviestImporters []FileCount        `json:"heaviest_importers"`
	CircularPairs     [][2]string        `json:"circular_pairs"`
}

// FileCount pairs a file path with a count.
type FileCount struct {
	File  string `json:"file"`
	Count int    `json:"count"`
}

// Summary computes project-wide dependency statistics.
func (g *DepGraph) Summary() GraphSummary {
	allFiles := make(map[string]bool)
	for f := range g.Edges {
		allFiles[f] = true
	}
	for f := range g.Reverse {
		allFiles[f] = true
	}

	totalEdges := 0
	for _, deps := range g.Edges {
		totalEdges += len(deps)
	}

	totalUnresolved := 0
	for _, specs := range g.Unresolved {
		totalUnresolved += len(specs)
	}

	// Hot spots: most depended-on files
	type fc struct {
		file  string
		count int
	}
	var hotSpots []fc
	for f, deps := range g.Reverse {
		hotSpots = append(hotSpots, fc{f, len(deps)})
	}
	sort.Slice(hotSpots, func(i, j int) bool { return hotSpots[i].count > hotSpots[j].count })
	if len(hotSpots) > 15 {
		hotSpots = hotSpots[:15]
	}

	var heaviest []fc
	for f, deps := range g.Edges {
		heaviest = append(heaviest, fc{f, len(deps)})
	}
	sort.Slice(heaviest, func(i, j int) bool { return heaviest[i].count > heaviest[j].count })
	if len(heaviest) > 10 {
		heaviest = heaviest[:10]
	}

	// Circular dependencies
	circularSet := make(map[[2]string]bool)
	var circular [][2]string
	for a, aDeps := range g.Edges {
		for b := range aDeps {
			if g.Edges[b] != nil && g.Edges[b][a] {
				pair := [2]string{g.rel(a), g.rel(b)}
				if pair[0] > pair[1] {
					pair[0], pair[1] = pair[1], pair[0]
				}
				if !circularSet[pair] {
					circularSet[pair] = true
					circular = append(circular, pair)
				}
			}
		}
	}
	if len(circular) > 20 {
		circular = circular[:20]
	}

	result := GraphSummary{
		TotalFiles:        len(allFiles),
		TotalEdges:        totalEdges,
		UnresolvedImports: totalUnresolved,
		CircularPairs:     circular,
	}
	for _, hs := range hotSpots {
		result.HotSpots = append(result.HotSpots, FileCount{g.rel(hs.file), hs.count})
	}
	for _, h := range heaviest {
		result.HeaviestImporters = append(result.HeaviestImporters, FileCount{g.rel(h.file), h.count})
	}

	return result
}

// ── Graph building ──────────────────────────────────────────────────────────

type fileImports struct {
	absPath string
	ext     string
	imports []Import
}

// BuildGraph reads files in parallel, extracts imports, resolves them, and builds the graph.
func BuildGraph(root string, files []string) *DepGraph {
	index := NewProjectIndex(root, files)
	graph := NewDepGraph(root)

	// Read + extract imports in parallel
	results := make([]fileImports, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)

	for i, f := range files {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			absF, _ := filepath.Abs(path)
			ext := strings.ToLower(filepath.Ext(path))
			data, err := os.ReadFile(path)
			if err != nil {
				results[idx] = fileImports{absPath: absF, ext: ext}
				return
			}
			content := string(data)
			result := ExtractImports(path, content)
			results[idx] = fileImports{absPath: absF, ext: ext, imports: result.Imports}
		}(i, f)
	}
	wg.Wait()

	// Resolve imports and build edges
	for _, fi := range results {
		for _, imp := range fi.imports {
			resolved := index.ResolveImport(fi.ext, imp.Module, imp.Kind, fi.absPath)
			if resolved != "" {
				graph.addEdge(fi.absPath, resolved)
			} else {
				graph.addUnresolved(fi.absPath, imp.Module)
			}
		}
	}

	return graph
}

// ── Formatting ──────────────────────────────────────────────────────────────

// FormatDepsText formats a list of dependencies as human-readable text.
func FormatDepsText(filePath string, deps []string, root, label string) string {
	rootAbs, _ := filepath.Abs(root)
	relFile := func(p string) string {
		r, err := filepath.Rel(rootAbs, p)
		if err != nil {
			return p
		}
		return filepath.ToSlash(r)
	}

	fileAbs, _ := filepath.Abs(filePath)
	rf := relFile(fileAbs)
	if len(deps) == 0 {
		return fmt.Sprintf("### `%s` — %s: (none)\n", rf, label)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### `%s` — %s: %d files\n\n", rf, label, len(deps))
	for _, d := range deps {
		fmt.Fprintf(&b, "  %s\n", relFile(d))
	}
	return b.String()
}

// FormatImpactText formats an ImpactResult as human-readable text.
func FormatImpactText(r ImpactResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### `%s` — impact analysis\n\n", r.File)
	fmt.Fprintf(&b, "  Direct dependents:     %d\n", r.DirectDependents)
	fmt.Fprintf(&b, "  Transitive dependents: %d\n", r.TransitiveDependents)
	if len(r.Direct) > 0 {
		b.WriteString("\n  Direct:\n")
		for _, f := range r.Direct {
			fmt.Fprintf(&b, "    %s\n", f)
		}
	}
	if len(r.Indirect) > 0 {
		b.WriteString("\n  Indirect (transitive):\n")
		for _, f := range r.Indirect {
			fmt.Fprintf(&b, "    %s\n", f)
		}
	}
	return b.String()
}

// FormatSummaryText formats a GraphSummary as human-readable text.
func FormatSummaryText(s GraphSummary) string {
	var b strings.Builder
	b.WriteString("Project dependency graph\n\n")
	fmt.Fprintf(&b, "  Files:              %d\n", s.TotalFiles)
	fmt.Fprintf(&b, "  Import edges:       %d\n", s.TotalEdges)
	fmt.Fprintf(&b, "  Unresolved imports: %d\n", s.UnresolvedImports)
	if len(s.HotSpots) > 0 {
		b.WriteString("\n  Most depended-on files:\n")
		for _, hs := range s.HotSpots {
			fmt.Fprintf(&b, "    %s  (%d dependents)\n", hs.File, hs.Count)
		}
	}
	if len(s.HeaviestImporters) > 0 {
		b.WriteString("\n  Heaviest importers:\n")
		for _, h := range s.HeaviestImporters {
			fmt.Fprintf(&b, "    %s  (%d imports)\n", h.File, h.Count)
		}
	}
	if len(s.CircularPairs) > 0 {
		fmt.Fprintf(&b, "\n  Circular dependencies (%d):\n", len(s.CircularPairs))
		for _, pair := range s.CircularPairs {
			fmt.Fprintf(&b, "    %s <-> %s\n", pair[0], pair[1])
		}
	}
	return b.String()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func sortedKeys(m map[string]bool) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysFromMap(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
