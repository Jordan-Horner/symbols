package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// tsPathAlias maps a prefix like "@/" to a list of absolute directory paths.
type tsPathAlias struct {
	prefix string   // e.g. "@/" (the part before the wildcard)
	dirs   []string // absolute directories to search
}

// ProjectIndex indexes all source files in a project for import resolution.
type ProjectIndex struct {
	Root          string
	GoModule      string
	tsAliases     []tsPathAlias       // tsconfig.json path aliases
	byModule      map[string]string   // "engine.routing" → abs path
	bySlash       map[string]string   // "engine/routing" → abs path
	byStem        map[string][]string // "routing" → [abs paths]
	byRelPath     map[string]string   // "engine/routing.py" → abs path
	allFiles      map[string]bool
}

var sourceExts = map[string]bool{
	".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".mjs": true, ".cjs": true, ".svelte": true, ".go": true,
	".java": true, ".kt": true, ".kts": true, ".rs": true,
	".cs": true, ".php": true, ".c": true, ".h": true,
	".cpp": true, ".cc": true, ".cxx": true, ".hpp": true,
	".rb": true, ".swift": true, ".scala": true, ".sh": true, ".bash": true,
}

// NewProjectIndex builds an index from a list of absolute file paths.
func NewProjectIndex(root string, files []string) *ProjectIndex {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	idx := &ProjectIndex{
		Root:      absRoot,
		byModule:  make(map[string]string),
		bySlash:   make(map[string]string),
		byStem:    make(map[string][]string),
		byRelPath: make(map[string]string),
		allFiles:  make(map[string]bool),
	}

	// Read go.mod for Go module name
	goModPath := filepath.Join(absRoot, "go.mod")
	if data, err := os.ReadFile(goModPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "module ") {
				idx.GoModule = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}

	// Read tsconfig.json / jsconfig.json for path aliases
	idx.tsAliases = loadTSPathAliases(absRoot)

	for _, f := range files {
		absF, err := filepath.Abs(f)
		if err != nil {
			continue
		}
		idx.allFiles[absF] = true

		rel, err := filepath.Rel(absRoot, absF)
		if err != nil {
			continue
		}
		// Normalize to forward slashes for cross-platform consistency
		rel = filepath.ToSlash(rel)
		idx.byRelPath[rel] = absF

		// Split into directory parts + filename
		parts := strings.Split(rel, "/")
		if len(parts) == 0 {
			continue
		}

		// Find and strip file extension
		filename := parts[len(parts)-1]
		ext := ""
		stem := filename
		for e := range sourceExts {
			if strings.HasSuffix(filename, e) {
				ext = e
				stem = filename[:len(filename)-len(e)]
				break
			}
		}

		partsNoExt := make([]string, len(parts))
		copy(partsNoExt, parts)
		partsNoExt[len(partsNoExt)-1] = stem

		// Dot-separated module key
		moduleKey := strings.Join(partsNoExt, ".")
		idx.byModule[moduleKey] = absF

		// Slash-separated path key
		slashKey := strings.Join(partsNoExt, "/")
		idx.bySlash[slashKey] = absF

		// Python __init__ / TS index shortcuts
		if stem == "__init__" && len(partsNoExt) > 1 {
			pkgKey := strings.Join(partsNoExt[:len(partsNoExt)-1], ".")
			idx.byModule[pkgKey] = absF
		}
		if stem == "index" && len(partsNoExt) > 1 {
			dirDot := strings.Join(partsNoExt[:len(partsNoExt)-1], ".")
			dirSlash := strings.Join(partsNoExt[:len(partsNoExt)-1], "/")
			idx.byModule[dirDot] = absF
			idx.bySlash[dirSlash] = absF
		}

		// Rust mod.rs
		if stem == "mod" && ext == ".rs" && len(partsNoExt) > 1 {
			pkgKey := strings.Join(partsNoExt[:len(partsNoExt)-1], ".")
			slashPkg := strings.Join(partsNoExt[:len(partsNoExt)-1], "/")
			idx.byModule[pkgKey] = absF
			idx.bySlash[slashPkg] = absF
		}

		// Bare stem for fuzzy fallback
		idx.byStem[stem] = append(idx.byStem[stem], absF)
	}

	return idx
}

// ── tsconfig.json path aliases ──────────────────────────────────────────────

// loadTSPathAliases reads tsconfig.json (or jsconfig.json) and extracts
// compilerOptions.paths entries. Supports "extends" for inherited configs.
func loadTSPathAliases(root string) []tsPathAlias {
	for _, name := range []string{"tsconfig.json", "jsconfig.json"} {
		aliases := parseTSConfig(root, filepath.Join(root, name))
		if len(aliases) > 0 {
			return aliases
		}
	}
	return nil
}

func parseTSConfig(root, configPath string) []tsPathAlias {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var config struct {
		Extends         string `json:"extends"`
		CompilerOptions struct {
			BaseURL string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}

	// Handle "extends" — merge parent config first
	var aliases []tsPathAlias
	if config.Extends != "" {
		parentPath := config.Extends
		if !filepath.IsAbs(parentPath) {
			parentPath = filepath.Join(filepath.Dir(configPath), parentPath)
		}
		// Add .json if missing
		if filepath.Ext(parentPath) == "" {
			parentPath += ".json"
		}
		aliases = parseTSConfig(root, parentPath)
	}

	if len(config.CompilerOptions.Paths) == 0 {
		return aliases
	}

	// Resolve baseUrl relative to the config file's directory
	baseDir := filepath.Dir(configPath)
	if config.CompilerOptions.BaseURL != "" {
		baseDir = filepath.Join(filepath.Dir(configPath), config.CompilerOptions.BaseURL)
	}

	for pattern, targets := range config.CompilerOptions.Paths {
		// Pattern is like "@/*" or "@components/*" or "utils"
		prefix := strings.TrimSuffix(pattern, "*")

		var dirs []string
		for _, target := range targets {
			// Target is like "./src/*" or "src/components/*"
			dir := strings.TrimSuffix(target, "*")
			dir = strings.TrimSuffix(dir, "/")
			absDir := filepath.Join(baseDir, dir)
			absDir, err := filepath.Abs(absDir)
			if err != nil {
				continue
			}
			dirs = append(dirs, absDir)
		}
		if len(dirs) > 0 {
			aliases = append(aliases, tsPathAlias{prefix: prefix, dirs: dirs})
		}
	}

	return aliases
}

// resolveTSAlias tries to resolve a specifier using tsconfig path aliases.
func (idx *ProjectIndex) resolveTSAlias(specifier string) string {
	for _, alias := range idx.tsAliases {
		if !strings.HasPrefix(specifier, alias.prefix) {
			continue
		}
		remainder := specifier[len(alias.prefix):]
		for _, dir := range alias.dirs {
			target := filepath.Join(dir, remainder)

			// Try exact match
			if idx.allFiles[target] {
				return target
			}
			// Try with extensions
			for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".svelte"} {
				if idx.allFiles[target+ext] {
					return target + ext
				}
			}
			// Try index files
			for _, idx2 := range []string{"/index.ts", "/index.tsx", "/index.js"} {
				if idx.allFiles[target+idx2] {
					return target + idx2
				}
			}
		}
	}
	return ""
}

// ── Python resolver ─────────────────────────────────────────────────────────

func (idx *ProjectIndex) ResolvePython(module, fromFile string) string {
	if strings.HasPrefix(module, ".") {
		dots := 0
		for _, c := range module {
			if c == '.' {
				dots++
			} else {
				break
			}
		}
		remainder := module[dots:]
		fromAbs, err := filepath.Abs(fromFile)
		if err != nil {
			return ""
		}
		base := filepath.Dir(fromAbs)
		for i := 0; i < dots-1; i++ {
			base = filepath.Dir(base)
		}
		if remainder != "" {
			relPath := strings.ReplaceAll(remainder, ".", "/")
			candidate := filepath.Join(base, relPath, "__init__.py")
			if idx.allFiles[candidate] {
				return candidate
			}
			candidate = filepath.Join(base, relPath+".py")
			if idx.allFiles[candidate] {
				return candidate
			}
		}
		return ""
	}

	if p, ok := idx.byModule[module]; ok {
		return p
	}
	parts := strings.Split(module, ".")
	for i := len(parts) - 1; i > 0; i-- {
		prefix := strings.Join(parts[:i], ".")
		if p, ok := idx.byModule[prefix]; ok {
			return p
		}
	}
	return ""
}

// ── TypeScript / JavaScript resolver ────────────────────────────────────────

func (idx *ProjectIndex) ResolveTS(specifier, fromFile string) string {
	if !strings.HasPrefix(specifier, ".") && !strings.HasPrefix(specifier, "/") {
		// Try tsconfig path aliases first
		if resolved := idx.resolveTSAlias(specifier); resolved != "" {
			return resolved
		}

		// Fallback: naive alias like $lib/foo or @app/foo
		cleaned := specifier
		if len(specifier) > 0 && (specifier[0] == '$' || specifier[0] == '@') {
			if idx2 := strings.Index(specifier, "/"); idx2 >= 0 {
				cleaned = specifier[idx2+1:]
			} else {
				return ""
			}
		}
		if cleaned != specifier {
			moduleKey := strings.ReplaceAll(cleaned, "/", ".")
			if p, ok := idx.byModule[moduleKey]; ok {
				return p
			}
			if p, ok := idx.bySlash[cleaned]; ok {
				return p
			}
		}
		return ""
	}

	fromAbs, err := filepath.Abs(fromFile)
	if err != nil {
		return ""
	}
	base := filepath.Dir(fromAbs)
	if strings.HasPrefix(specifier, "/") {
		base = idx.Root
		specifier = specifier[1:]
	}

	target := filepath.Join(base, specifier)
	target, err2 := filepath.Abs(target)
	if err2 != nil {
		return ""
	}

	if idx.allFiles[target] {
		return target
	}
	tsExts := []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".svelte",
		"/index.ts", "/index.js", "/index.tsx"}
	for _, ext := range tsExts {
		candidate := target + ext
		if idx.allFiles[candidate] {
			return candidate
		}
	}
	return ""
}

// ── Go resolver ─────────────────────────────────────────────────────────────

func (idx *ProjectIndex) ResolveGo(pkg, fromFile string) string {
	relPath := ""
	if idx.GoModule != "" && strings.HasPrefix(pkg, idx.GoModule) {
		relPath = strings.TrimPrefix(pkg[len(idx.GoModule):], "/")
	} else if !strings.HasPrefix(pkg, ".") && !strings.Contains(pkg, "/") {
		// Standard library
		return ""
	} else {
		relPath = pkg
	}

	if relPath == "" {
		relPath = "."
	}

	targetDir := filepath.Join(idx.Root, relPath)
	if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
		for f := range idx.allFiles {
			if filepath.Ext(f) == ".go" && filepath.Dir(f) == targetDir {
				return f
			}
		}
	}
	if p, ok := idx.bySlash[relPath]; ok {
		return p
	}
	return ""
}

// ── Java resolver ───────────────────────────────────────────────────────────

func (idx *ProjectIndex) ResolveJava(module, fromFile string) string {
	moduleClean := strings.TrimRight(module, ".*")

	if p, ok := idx.byModule[moduleClean]; ok {
		return p
	}

	slashPath := strings.ReplaceAll(moduleClean, ".", "/")
	if p, ok := idx.bySlash[slashPath]; ok {
		return p
	}

	for _, prefix := range []string{"src/main/java/", "src/", "java/", "app/src/main/java/"} {
		if p, ok := idx.bySlash[prefix+slashPath]; ok {
			return p
		}
	}

	className := moduleClean
	if idx2 := strings.LastIndex(moduleClean, "."); idx2 >= 0 {
		className = moduleClean[idx2+1:]
	}
	if candidates, ok := idx.byStem[className]; ok && len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

// ── Kotlin resolver ─────────────────────────────────────────────────────────

func (idx *ProjectIndex) ResolveKotlin(module, fromFile string) string {
	moduleClean := strings.TrimRight(module, ".*")

	if p, ok := idx.byModule[moduleClean]; ok {
		return p
	}

	slashPath := strings.ReplaceAll(moduleClean, ".", "/")
	if p, ok := idx.bySlash[slashPath]; ok {
		return p
	}

	for _, prefix := range []string{"src/main/kotlin/", "src/main/java/", "src/", "app/src/main/kotlin/"} {
		if p, ok := idx.bySlash[prefix+slashPath]; ok {
			return p
		}
	}

	className := moduleClean
	if idx2 := strings.LastIndex(moduleClean, "."); idx2 >= 0 {
		className = moduleClean[idx2+1:]
	}
	if candidates, ok := idx.byStem[className]; ok && len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

// ── Rust resolver ───────────────────────────────────────────────────────────

func (idx *ProjectIndex) ResolveRust(module, kind, fromFile string) string {
	fromAbs, err := filepath.Abs(fromFile)
	if err != nil {
		return ""
	}

	if kind == "mod" {
		parent := filepath.Dir(fromAbs)
		candidate := filepath.Join(parent, module+".rs")
		if idx.allFiles[candidate] {
			return candidate
		}
		candidate = filepath.Join(parent, module, "mod.rs")
		if idx.allFiles[candidate] {
			return candidate
		}
		return ""
	}

	// use statement: strip tree part
	path := module
	if braceIdx := strings.Index(path, "::{"); braceIdx >= 0 {
		path = path[:braceIdx]
	}
	path = strings.TrimRight(path, "::*")
	segments := strings.Split(path, "::")

	if len(segments) == 0 {
		return ""
	}

	var base string
	switch segments[0] {
	case "crate":
		segments = segments[1:]
		base = filepath.Join(idx.Root, "src")
		if info, err := os.Stat(base); err != nil || !info.IsDir() {
			base = idx.Root
		}
	case "super":
		base = filepath.Dir(filepath.Dir(fromAbs))
		segments = segments[1:]
	case "self":
		base = filepath.Dir(fromAbs)
		segments = segments[1:]
	default:
		return ""
	}

	if len(segments) == 0 {
		return ""
	}

	rel := strings.Join(segments, "/")

	candidate := filepath.Join(base, rel+".rs")
	if idx.allFiles[candidate] {
		return candidate
	}
	candidate = filepath.Join(base, rel, "mod.rs")
	if idx.allFiles[candidate] {
		return candidate
	}

	for i := len(segments) - 1; i > 0; i-- {
		rel = strings.Join(segments[:i], "/")
		candidate = filepath.Join(base, rel+".rs")
		if idx.allFiles[candidate] {
			return candidate
		}
		candidate = filepath.Join(base, rel, "mod.rs")
		if idx.allFiles[candidate] {
			return candidate
		}
	}
	return ""
}

// ── C# resolver ─────────────────────────────────────────────────────────────

func (idx *ProjectIndex) ResolveCSharp(namespace, fromFile string) string {
	if p, ok := idx.byModule[namespace]; ok {
		return p
	}

	slashPath := strings.ReplaceAll(namespace, ".", "/")
	if p, ok := idx.bySlash[slashPath]; ok {
		return p
	}

	parts := strings.Split(namespace, ".")
	for start := 1; start < len(parts); start++ {
		sub := strings.Join(parts[start:], ".")
		if p, ok := idx.byModule[sub]; ok {
			return p
		}
		subSlash := strings.Join(parts[start:], "/")
		if p, ok := idx.bySlash[subSlash]; ok {
			return p
		}
	}

	className := parts[len(parts)-1]
	if candidates, ok := idx.byStem[className]; ok && len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

// ── PHP resolver ────────────────────────────────────────────────────────────

func (idx *ProjectIndex) ResolvePHP(module, kind, fromFile string) string {
	if kind == "require" {
		fromAbs, err := filepath.Abs(fromFile)
		if err != nil {
			return ""
		}
		candidate, err := filepath.Abs(filepath.Join(filepath.Dir(fromAbs), module))
		if err == nil && idx.allFiles[candidate] {
			return candidate
		}
		candidate, err = filepath.Abs(filepath.Join(idx.Root, module))
		if err != nil {
			return ""
		}
		if idx.allFiles[candidate] {
			return candidate
		}
		return ""
	}

	slashPath := strings.ReplaceAll(module, "\\", "/")
	if p, ok := idx.bySlash[slashPath]; ok {
		return p
	}

	parts := strings.Split(slashPath, "/")
	if len(parts) > 1 {
		remainder := strings.Join(parts[1:], "/")
		for _, prefix := range []string{"src/", "app/", "lib/", ""} {
			if p, ok := idx.bySlash[prefix+remainder]; ok {
				return p
			}
		}
	}

	moduleDot := strings.ReplaceAll(module, "\\", ".")
	if p, ok := idx.byModule[moduleDot]; ok {
		return p
	}

	className := parts[len(parts)-1]
	if candidates, ok := idx.byStem[className]; ok {
		var phpOnly []string
		for _, c := range candidates {
			if strings.HasSuffix(c, ".php") {
				phpOnly = append(phpOnly, c)
			}
		}
		if len(phpOnly) == 1 {
			return phpOnly[0]
		}
	}
	return ""
}

// ── Resolver dispatch ───────────────────────────────────────────────────────

var resolverLangs = map[string]string{
	".py": "python", ".ts": "ts", ".tsx": "ts", ".js": "ts", ".jsx": "ts",
	".mjs": "ts", ".cjs": "ts", ".svelte": "ts", ".go": "go",
	".java": "java", ".kt": "kotlin", ".kts": "kotlin", ".rs": "rust",
	".cs": "csharp", ".php": "php",
}

// ResolveImport dispatches to the right language resolver.
func (idx *ProjectIndex) ResolveImport(ext, module, kind, fromFile string) string {
	lang := resolverLangs[ext]
	switch lang {
	case "python":
		return idx.ResolvePython(module, fromFile)
	case "ts":
		return idx.ResolveTS(module, fromFile)
	case "go":
		return idx.ResolveGo(module, fromFile)
	case "java":
		return idx.ResolveJava(module, fromFile)
	case "kotlin":
		return idx.ResolveKotlin(module, fromFile)
	case "rust":
		return idx.ResolveRust(module, kind, fromFile)
	case "csharp":
		return idx.ResolveCSharp(module, fromFile)
	case "php":
		return idx.ResolvePHP(module, kind, fromFile)
	}
	return ""
}
