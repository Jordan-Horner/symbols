package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ── Skip directories ────────────────────────────────────────────────────────

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true,
	"dist": true, "build": true, ".venv": true, "venv": true,
	".tox": true, ".mypy_cache": true, ".next": true, ".nuxt": true,
	"storybook-static": true, "coverage": true, ".turbo": true,
	".parcel-cache": true, "target": true, "vendor": true,
	"Pods": true, ".gradle": true, "bin": true, "obj": true,
}

// ── File collection ─────────────────────────────────────────────────────────

func collectFiles(paths []string, recursive bool) []string {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s not found\n", p)
			continue
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(p))
			if sourceExts[ext] {
				abs, err := filepath.Abs(p)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: %s: %v\n", p, err)
					continue
				}
				files = append(files, abs)
			}
			continue
		}
		if recursive {
			filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
					return nil
				}
				if info.IsDir() {
					name := info.Name()
					if skipDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
						return filepath.SkipDir
					}
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if sourceExts[ext] {
					abs, err := filepath.Abs(path)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
						return nil
					}
					files = append(files, abs)
				}
				return nil
			})
		} else {
			entries, err := os.ReadDir(p)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if sourceExts[ext] {
					abs, err := filepath.Abs(filepath.Join(p, e.Name()))
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: %s: %v\n", e.Name(), err)
						continue
					}
					files = append(files, abs)
				}
			}
		}
	}
	sort.Strings(files)
	return files
}

func findProjectRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	current := abs
	for {
		for _, marker := range []string{".git", "package.json", "pyproject.toml"} {
			if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return abs
}

func printJSON(v interface{}, pretty bool) {
	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	enc.Encode(v)
}

// ── Subcommands ─────────────────────────────────────────────────────────────

// parseFlags separates flags from positional args since Go's flag package
// stops at the first non-flag argument. This allows `syms file.py --json`.
func parseFlags(args []string, fs *flag.FlagSet) []string {
	// Build set of flags that take values
	valuedFlags := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		if f.DefValue != "false" && f.DefValue != "true" {
			valuedFlags[f.Name] = true
		}
	})

	var positional []string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			// If this flag takes a value and next arg exists, consume it
			name := strings.TrimLeft(args[i], "-")
			if eqIdx := strings.Index(name, "="); eqIdx >= 0 {
				// --root=value form, already included
			} else if valuedFlags[name] && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	fs.Parse(flagArgs)
	return positional
}

type stringListFlag []string

func (s *stringListFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringListFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "syms list - Extract top-level symbols from files")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: syms list [-r] [--json] [--pretty] [--ranges] [--count] [--filter KIND[,KIND...]] <paths...>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	recursive := fs.Bool("r", false, "Recursive directory scan")
	jsonOut := fs.Bool("json", false, "JSON output")
	pretty := fs.Bool("pretty", false, "Pretty-print JSON output (with --json)")
	ranges := fs.Bool("ranges", false, "Include symbol range metadata (start/end line/column)")
	count := fs.Bool("count", false, "Print symbol count instead of symbols")
	var filters stringListFlag
	fs.Var(&filters, "filter", "Filter symbols by kind (repeatable or comma-separated)")
	paths := parseFlags(args, fs)
	if len(paths) == 0 {
		fs.Usage()
		os.Exit(1)
	}

	files := collectFiles(paths, *recursive)
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "No supported files found.")
		os.Exit(1)
	}

	results := filterSymbolResultsByKind(ExtractSymbolsParallelWithOptions(files, ExtractionOptions{IncludeRanges: *ranges}), []string(filters))
	if *count {
		if *jsonOut {
			type CountResult struct {
				File  string `json:"file"`
				Count int    `json:"count"`
			}
			var counts []CountResult
			for _, r := range results {
				counts = append(counts, CountResult{File: r.File, Count: len(r.Symbols)})
			}
			printJSON(counts, *pretty)
		} else {
			for _, r := range results {
				fmt.Printf("%s: %d symbols\n", r.File, len(r.Symbols))
			}
		}
	} else {
		if *jsonOut {
			printJSON(results, *pretty)
		} else {
			for _, r := range results {
				fmt.Println(FormatSymbolResult(r))
			}
		}
	}
}

func cmdImports(args []string) {
	fs := flag.NewFlagSet("imports", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "syms imports - Show imports for files")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: syms imports [-r] [--json] [--pretty] <paths...>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	recursive := fs.Bool("r", false, "Recursive directory scan")
	jsonOut := fs.Bool("json", false, "JSON output")
	pretty := fs.Bool("pretty", false, "Pretty-print JSON output (with --json)")
	paths := parseFlags(args, fs)
	if len(paths) == 0 {
		fs.Usage()
		os.Exit(1)
	}

	files := collectFiles(paths, *recursive)
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "No supported files found.")
		os.Exit(1)
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

	if *jsonOut {
		printJSON(results, *pretty)
	} else {
		for _, r := range results {
			fmt.Println(FormatImportsText(r))
		}
	}
}

func buildGraphForFile(target string, root string) (*DepGraph, string) {
	if root == "" {
		root = findProjectRoot(target)
	}
	files := collectFiles([]string{root}, true)
	return BuildGraph(root, files), root
}

func cmdDeps(args []string) {
	fs := flag.NewFlagSet("deps", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "syms deps - Show what files a given file depends on")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: syms deps [-t] [--root DIR] [--json] [--pretty] <file>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	transitive := fs.Bool("t", false, "Include transitive deps")
	root := fs.String("root", "", "Project root (auto-detected if omitted)")
	jsonOut := fs.Bool("json", false, "JSON output")
	pretty := fs.Bool("pretty", false, "Pretty-print JSON output (with --json)")
	positional := parseFlags(args, fs)

	if len(positional) == 0 {
		fs.Usage()
		os.Exit(1)
	}
	file := positional[0]

	graph, graphRoot := buildGraphForFile(file, *root)
	deps := graph.Deps(file, *transitive)

	if *jsonOut {
		relDeps := make([]string, len(deps))
		for i, d := range deps {
			relDeps[i] = graph.rel(d)
		}
		printJSON(map[string]interface{}{"file": file, "deps": relDeps}, *pretty)
	} else {
		label := "depends on"
		if *transitive {
			label = "depends on (transitive)"
		}
		fmt.Println(FormatDepsText(file, deps, graphRoot, label))
	}
}

func cmdDependents(args []string) {
	fs := flag.NewFlagSet("dependents", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "syms dependents - What depends on this file?")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: syms dependents [-t] [--root DIR] [--json] [--pretty] <file>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	transitive := fs.Bool("t", false, "Include transitive dependents")
	root := fs.String("root", "", "Project root (auto-detected if omitted)")
	jsonOut := fs.Bool("json", false, "JSON output")
	pretty := fs.Bool("pretty", false, "Pretty-print JSON output (with --json)")
	positional := parseFlags(args, fs)

	if len(positional) == 0 {
		fs.Usage()
		os.Exit(1)
	}
	file := positional[0]

	graph, graphRoot := buildGraphForFile(file, *root)
	deps := graph.Dependents(file, *transitive)

	if *jsonOut {
		relDeps := make([]string, len(deps))
		for i, d := range deps {
			relDeps[i] = graph.rel(d)
		}
		printJSON(map[string]interface{}{"file": file, "dependents": relDeps}, *pretty)
	} else {
		label := "depended on by"
		if *transitive {
			label = "depended on by (transitive)"
		}
		fmt.Println(FormatDepsText(file, deps, graphRoot, label))
	}
}

func cmdImpact(args []string) {
	fs := flag.NewFlagSet("impact", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "syms impact - Impact analysis for a file")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: syms impact [--root DIR] [--json] [--pretty] <file>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	root := fs.String("root", "", "Project root (auto-detected if omitted)")
	jsonOut := fs.Bool("json", false, "JSON output")
	pretty := fs.Bool("pretty", false, "Pretty-print JSON output (with --json)")
	positional := parseFlags(args, fs)

	if len(positional) == 0 {
		fs.Usage()
		os.Exit(1)
	}
	file := positional[0]

	graph, _ := buildGraphForFile(file, *root)
	result := graph.Impact(file)

	if *jsonOut {
		printJSON(result, *pretty)
	} else {
		fmt.Println(FormatImpactText(result))
	}
}

func cmdGraph(args []string) {
	fs := flag.NewFlagSet("graph", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "syms graph - Project-wide dependency graph summary")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: syms graph [--root DIR] [--json] [--pretty] [dir]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}
	root := fs.String("root", "", "Project root (auto-detected if omitted)")
	jsonOut := fs.Bool("json", false, "JSON output")
	pretty := fs.Bool("pretty", false, "Pretty-print JSON output (with --json)")
	positional := parseFlags(args, fs)

	dir := "."
	if len(positional) > 0 {
		dir = positional[0]
	}

	graphRoot := *root
	if graphRoot == "" {
		graphRoot = findProjectRoot(dir)
	}
	files := collectFiles([]string{graphRoot}, true)
	graph := BuildGraph(graphRoot, files)
	summary := graph.Summary()

	if *jsonOut {
		printJSON(summary, *pretty)
	} else {
		fmt.Println(FormatSummaryText(summary))
	}
}

func cmdSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	root := fs.String("root", "", "Project root (auto-detected if omitted)")
	jsonOut := fs.Bool("json", false, "JSON output")
	pretty := fs.Bool("pretty", false, "Pretty-print JSON output (with --json)")
	ranges := fs.Bool("ranges", false, "Include symbol range metadata (start/end line/column)")
	var filters stringListFlag
	fs.Var(&filters, "filter", "Filter symbols by kind (repeatable or comma-separated)")
	positional := parseFlags(args, fs)

	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: syms search [--root DIR] [--json] [--pretty] [--ranges] [--filter KIND[,KIND...]] <query>")
		os.Exit(1)
	}
	query := positional[0]

	searchRoot := *root
	if searchRoot == "" {
		searchRoot = findProjectRoot(".")
	}
	files := collectFiles([]string{searchRoot}, true)
	results := SearchSymbolsWithKindsAndOptions(searchRoot, files, query, []string(filters), ExtractionOptions{IncludeRanges: *ranges})

	if *jsonOut {
		printJSON(results, *pretty)
	} else {
		fmt.Print(FormatSearchText(results, query))
	}
}

// ── Main ────────────────────────────────────────────────────────────────────

var version = "1.0.3"

var subcommands = map[string]bool{
	"list": true, "imports": true, "deps": true,
	"dependents": true, "impact": true, "graph": true,
	"search": true, "mcp": true,
}

func main() {
	args := os.Args[1:]

	for _, arg := range args {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("syms version %s\n", version)
			os.Exit(0)
		}
	}

	if len(args) == 0 {
		fmt.Println("Usage: syms <command> [options] [args]")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -v, --version  Print version information and exit")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  list         Extract top-level symbols from files")
		fmt.Println("  imports      Show imports for files")
		fmt.Println("  deps         What does this file depend on?")
		fmt.Println("  dependents   What depends on this file?")
		fmt.Println("  impact       Impact analysis for a file")
		fmt.Println("  graph        Project-wide dependency graph summary")
		fmt.Println("  search       Search for symbols by name across a project")
		fmt.Println("  mcp          Run as MCP server (stdio)")

		os.Exit(1)
	}

	// Backward compat: if first non-flag arg isn't a subcommand, inject "list"
	firstPositional := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			firstPositional = a
			break
		}
	}
	if firstPositional != "" && !subcommands[firstPositional] {
		args = append([]string{"list"}, args...)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "list":
		cmdList(rest)
	case "imports":
		cmdImports(rest)
	case "deps":
		cmdDeps(rest)
	case "dependents":
		cmdDependents(rest)
	case "impact":
		cmdImpact(rest)
	case "graph":
		cmdGraph(rest)
	case "search":
		cmdSearch(rest)
	case "mcp":
		runMCP()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
