package main

import (
	"strings"
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	tree_sitter_kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	tree_sitter_bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	tree_sitter_csharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_scala "github.com/tree-sitter/tree-sitter-scala/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// ── Language registry ───────────────────────────────────────────────────────

var (
	tsLanguages   map[string]*tree_sitter.Language
	tsLanguagesMu sync.Once
)

func getTSLanguages() map[string]*tree_sitter.Language {
	tsLanguagesMu.Do(func() {
		tsLanguages = map[string]*tree_sitter.Language{
			"python":     tree_sitter.NewLanguage(tree_sitter_python.Language()),
			"javascript": tree_sitter.NewLanguage(tree_sitter_javascript.Language()),
			"typescript": tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript()),
			"tsx":        tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTSX()),
			"go":         tree_sitter.NewLanguage(tree_sitter_go.Language()),
			"java":       tree_sitter.NewLanguage(tree_sitter_java.Language()),
			"kotlin":     tree_sitter.NewLanguage(tree_sitter_kotlin.Language()),
			"rust":       tree_sitter.NewLanguage(tree_sitter_rust.Language()),
			"c_sharp":    tree_sitter.NewLanguage(tree_sitter_csharp.Language()),
			"php":        tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP()),
			"c":          tree_sitter.NewLanguage(tree_sitter_c.Language()),
			"cpp":        tree_sitter.NewLanguage(tree_sitter_cpp.Language()),
			"ruby":       tree_sitter.NewLanguage(tree_sitter_ruby.Language()),
			"scala":      tree_sitter.NewLanguage(tree_sitter_scala.Language()),
			"bash":       tree_sitter.NewLanguage(tree_sitter_bash.Language()),
		}
	})
	return tsLanguages
}

// Extension → tree-sitter language name
var tsExtMap = map[string]string{
	".py":     "python",
	".js":     "javascript",
	".jsx":    "javascript",
	".mjs":    "javascript",
	".cjs":    "javascript",
	".ts":     "typescript",
	".tsx":    "tsx",
	".svelte": "typescript", // script blocks are TS/JS
	".go":     "go",
	".java":   "java",
	".kt":     "kotlin",
	".kts":    "kotlin",
	".rs":     "rust",
	".cs":     "c_sharp",
	".php":    "php",
	".c":      "c",
	".h":      "c",
	".cpp":    "cpp",
	".cc":     "cpp",
	".cxx":    "cpp",
	".hpp":    "cpp",
	".rb":     "ruby",
	".scala":  "scala",
	".sh":     "bash",
	".bash":   "bash",
}

// Node types we extract as symbols, per language
var tsSymbolTypes = map[string]map[string]bool{
	"python": {
		"function_definition":  true,
		"class_definition":     true,
		"expression_statement": true, // module-level assignments
	},
	"javascript": {
		"function_declaration":           true,
		"class_declaration":              true,
		"method_definition":              true,
		"generator_function_declaration": true,
	},
	"typescript": {
		"function_declaration":       true,
		"class_declaration":          true,
		"method_definition":          true,
		"interface_declaration":      true,
		"type_alias_declaration":     true,
		"enum_declaration":           true,
		"lexical_declaration":        true,
		"abstract_class_declaration": true,
	},
	"tsx": {
		"function_declaration":       true,
		"class_declaration":          true,
		"method_definition":          true,
		"interface_declaration":      true,
		"type_alias_declaration":     true,
		"enum_declaration":           true,
		"lexical_declaration":        true,
		"abstract_class_declaration": true,
	},
	"go": {
		"function_declaration": true,
		"method_declaration":   true,
		"type_spec":            true,
	},
	"java": {
		"class_declaration":     true,
		"interface_declaration": true,
		"enum_declaration":      true,
		"method_declaration":    true,
	},
	"kotlin": {
		"class_declaration":    true,
		"function_declaration": true,
		"object_declaration":   true,
	},
	"rust": {
		"function_item": true,
		"struct_item":   true,
		"impl_item":     true,
		"trait_item":    true,
		"enum_item":     true,
	},
	"c_sharp": {
		"class_declaration":     true,
		"interface_declaration": true,
		"method_declaration":    true,
		"struct_declaration":    true,
		"enum_declaration":      true,
	},
	"php": {
		"function_definition":   true,
		"class_declaration":     true,
		"interface_declaration": true,
		"method_declaration":    true,
	},
	"c": {
		"function_definition": true,
		"struct_specifier":    true,
	},
	"cpp": {
		"function_definition": true,
		"class_specifier":     true,
		"struct_specifier":    true,
	},
	"ruby": {
		"class":            true,
		"module":           true,
		"method":           true,
		"singleton_method": true,
	},
	"scala": {
		"function_definition": true,
		"class_definition":    true,
		"object_definition":   true,
		"trait_definition":    true,
	},
	"bash": {
		"function_definition": true,
	},
}

// Parent node types that qualify as "top-level"
var tsToplevelParents = map[string]bool{
	"source_file":                true,
	"program":                    true,
	"compilation_unit":           true,
	"class_body":                 true,
	"declaration_list":           true,
	"class_declaration":          true,
	"interface_body":             true,
	"type_declaration":           true,
	"module":                     true, // Python module-level
	"template":                   true, // PHP
	"export_statement":           true, // TS/JS: export function/class/interface
	"abstract_class_declaration": true,
}

// ── Extraction ──────────────────────────────────────────────────────────────

// extractTreeSitter parses source with tree-sitter and extracts symbols.
func extractTreeSitter(content []byte, langName string, includeRanges bool) []Symbol {
	languages := getTSLanguages()
	lang, ok := languages[langName]
	if !ok {
		return nil
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(lang)

	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	symbolTypes := tsSymbolTypes[langName]
	if symbolTypes == nil {
		return nil
	}

	var symbols []Symbol
	walkNode(tree.RootNode(), content, langName, symbolTypes, &symbols, includeRanges)
	return symbols
}

func walkNode(node *tree_sitter.Node, source []byte, langName string, symbolTypes map[string]bool, symbols *[]Symbol, includeRanges bool) {
	if node == nil {
		return
	}

	nodeType := node.Kind()

	if symbolTypes[nodeType] {
		// Check parent is top-level-ish
		parent := node.Parent()
		parentType := "source_file"
		if parent != nil {
			parentType = parent.Kind()
		}
		if tsToplevelParents[parentType] {
			sym := extractSymbolFromNode(node, source, langName, nodeType, includeRanges)
			if sym.Name != "" {
				*symbols = append(*symbols, sym)
			}
		}
	}

	// Recurse into children
	childCount := node.ChildCount()
	for i := uint(0); i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			walkNode(child, source, langName, symbolTypes, symbols, includeRanges)
		}
	}
}

func symbolWithRange(name, kind string, line int, node *tree_sitter.Node, includeRanges bool) Symbol {
	s := Symbol{Name: name, Kind: kind, Line: line}
	if includeRanges && node != nil {
		sl := int(node.StartPosition().Row) + 1
		el := int(node.EndPosition().Row) + 1
		sc := int(node.StartPosition().Column) + 1
		ec := int(node.EndPosition().Column) + 1
		s.StartLine = &sl
		s.EndLine = &el
		s.StartCol = &sc
		s.EndCol = &ec
	}
	return s
}

func extractSymbolFromNode(node *tree_sitter.Node, source []byte, langName, nodeType string, includeRanges bool) Symbol {
	line := int(node.StartPosition().Row) + 1

	// Special handling for lexical_declaration (export const/let/var)
	if nodeType == "lexical_declaration" {
		return extractLexicalDecl(node, source, line, includeRanges)
	}

	// Special handling for Python module-level assignments
	if nodeType == "expression_statement" {
		return extractPythonAssignment(node, source, line, includeRanges)
	}

	// Get the name
	nameNode := node.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = nameNode.Utf8Text(source)
	}

	// Determine kind
	kind := cleanKind(nodeType)

	// For Python, check async
	if langName == "python" && nodeType == "function_definition" {
		// Check if there's an "async" keyword before "def"
		text := node.Utf8Text(source)
		if strings.HasPrefix(strings.TrimSpace(text), "async") {
			kind = "async def"
		} else {
			kind = "def"
		}
	}

	// Extract parameters for function-like nodes
	if isFunctionLike(nodeType) {
		params := extractParams(node, source, langName, nodeType)
		name = name + "(" + params + ")"
	}

	return symbolWithRange(name, kind, line, node, includeRanges)
}

func extractPythonAssignment(node *tree_sitter.Node, source []byte, line int, includeRanges bool) Symbol {
	// Look for assignment child: expression_statement > assignment > identifier
	childCount := node.ChildCount()
	for i := uint(0); i < childCount; i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "assignment" {
			continue
		}
		// The left side should be a simple identifier (not attribute or subscript)
		left := child.ChildByFieldName("left")
		if left == nil || left.Kind() != "identifier" {
			return Symbol{}
		}
		name := left.Utf8Text(source)
		// Skip private/dunder names
		if strings.HasPrefix(name, "_") {
			return Symbol{}
		}
		kind := "variable"
		if name == strings.ToUpper(name) && len(name) > 1 {
			kind = "constant"
		}
		return symbolWithRange(name, kind, line, child, includeRanges)
	}
	return Symbol{}
}

func extractLexicalDecl(node *tree_sitter.Node, source []byte, line int, includeRanges bool) Symbol {
	// Get the declaration keyword (const/let/var)
	keyword := "const"
	firstChild := node.Child(0)
	if firstChild != nil {
		keyword = firstChild.Utf8Text(source)
	}

	// Find the variable_declarator child to get the name
	childCount := node.ChildCount()
	for i := uint(0); i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Kind() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(source)
				// Check if parent is export_statement
				parent := node.Parent()
				kind := keyword
				if parent != nil && parent.Kind() == "export_statement" {
					kind = "export " + keyword
				}
				return symbolWithRange(name, kind, line, child, includeRanges)
			}
		}
	}
	return Symbol{}
}

func isFunctionLike(nodeType string) bool {
	return strings.Contains(nodeType, "function") ||
		strings.Contains(nodeType, "method") ||
		nodeType == "function_item" // Rust
}

func cleanKind(nodeType string) string {
	kind := nodeType
	kind = strings.ReplaceAll(kind, "_declaration", "")
	kind = strings.ReplaceAll(kind, "_definition", "")
	kind = strings.ReplaceAll(kind, "_item", "")
	kind = strings.ReplaceAll(kind, "_specifier", "")
	kind = strings.ReplaceAll(kind, "_spec", "")
	kind = strings.ReplaceAll(kind, "_alias", "")
	return kind
}

func extractParams(node *tree_sitter.Node, source []byte, langName, nodeType string) string {
	// Find the parameters/formal_parameters node
	var paramsNode *tree_sitter.Node

	// Try common field names
	for _, fieldName := range []string{"parameters", "formal_parameters", "type_parameters"} {
		n := node.ChildByFieldName(fieldName)
		if n != nil {
			paramsNode = n
			break
		}
	}

	// Also search children for parameter-like nodes
	if paramsNode == nil {
		childCount := node.ChildCount()
		for i := uint(0); i < childCount; i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			kind := child.Kind()
			if kind == "parameters" || kind == "formal_parameters" ||
				kind == "parameter_list" || kind == "function_parameters" {
				paramsNode = child
				break
			}
		}
	}

	if paramsNode == nil {
		return ""
	}

	// Extract individual parameter names (not the full text with types)
	var params []string
	childCount := paramsNode.ChildCount()
	for i := uint(0); i < childCount; i++ {
		child := paramsNode.Child(i)
		if child == nil {
			continue
		}
		kind := child.Kind()

		// Skip delimiters
		if kind == "," || kind == "(" || kind == ")" || kind == "comment" {
			continue
		}

		paramName := extractParamName(child, source, langName)
		if paramName != "" {
			params = append(params, paramName)
		}
	}

	// Truncate long param lists
	if len(params) > 4 {
		return strings.Join(params[:4], ", ") + ", ..."
	}
	return strings.Join(params, ", ")
}

func extractParamName(node *tree_sitter.Node, source []byte, langName string) string {
	kind := node.Kind()

	// Try "name" field first
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return nameNode.Utf8Text(source)
	}

	// Language-specific fallbacks
	switch langName {
	case "python":
		// Python: identifier, default_parameter, typed_parameter, etc.
		if kind == "identifier" {
			return node.Utf8Text(source)
		}
		// For typed_parameter, default_parameter — get the first identifier child
		return firstIdentifier(node, source)

	case "go":
		// Go: parameter_declaration has name(s) + type
		// Get all identifiers before the type
		names := collectGoParamNames(node, source)
		if len(names) > 0 {
			return strings.Join(names, ", ")
		}

	case "javascript", "typescript", "tsx":
		if kind == "identifier" || kind == "shorthand_property_identifier_pattern" {
			return node.Utf8Text(source)
		}
		return firstIdentifier(node, source)

	case "java", "kotlin", "c_sharp":
		return firstIdentifier(node, source)

	case "rust":
		// Rust parameters: pattern + type
		patNode := node.ChildByFieldName("pattern")
		if patNode != nil {
			return patNode.Utf8Text(source)
		}
		return firstIdentifier(node, source)

	case "php":
		// PHP: $param
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return nameNode.Utf8Text(source)
		}
		return firstIdentifier(node, source)

	case "ruby":
		if kind == "identifier" || kind == "simple_symbol" {
			return node.Utf8Text(source)
		}
		return firstIdentifier(node, source)
	}

	// Generic fallback: return the full text if it's short enough
	text := node.Utf8Text(source)
	if len(text) < 30 && !strings.Contains(text, "\n") {
		return text
	}
	return ""
}

func firstIdentifier(node *tree_sitter.Node, source []byte) string {
	childCount := node.ChildCount()
	for i := uint(0); i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Kind() == "identifier" {
			return child.Utf8Text(source)
		}
	}
	return ""
}

func collectGoParamNames(node *tree_sitter.Node, source []byte) []string {
	var names []string
	childCount := node.ChildCount()
	for i := uint(0); i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if child.Kind() == "identifier" {
			names = append(names, child.Utf8Text(source))
		}
	}
	// In Go, the last identifier is often the type — but only if there are 2+ identifiers
	// e.g., `w http.ResponseWriter` has 1 identifier (w) + qualified_type
	// e.g., `x, y int` has 3 children: x, y, int — but int is type
	return names
}

// ── Svelte script extraction ────────────────────────────────────────────────

// For .svelte files, extract just the <script> content and parse as TS
func extractSvelteScript(content []byte) []byte {
	text := string(content)
	// Find <script ...> tag
	scriptStart := strings.Index(text, "<script")
	if scriptStart < 0 {
		return nil
	}
	// Find end of opening tag
	tagEnd := strings.Index(text[scriptStart:], ">")
	if tagEnd < 0 {
		return nil
	}
	scriptContentStart := scriptStart + tagEnd + 1

	// Find </script>
	scriptEnd := strings.Index(text[scriptContentStart:], "</script>")
	if scriptEnd < 0 {
		return nil
	}

	return []byte(text[scriptContentStart : scriptContentStart+scriptEnd])
}

// ── Public API ──────────────────────────────────────────────────────────────

// ExtractSymbolsTreeSitter extracts symbols using tree-sitter for the given extension.
// Returns nil if the language is not supported by tree-sitter.
func ExtractSymbolsTreeSitter(ext string, content []byte) []Symbol {
	return ExtractSymbolsTreeSitterWithOptions(ext, content, ExtractionOptions{})
}

// ExtractSymbolsTreeSitterWithOptions extracts symbols using tree-sitter with optional range metadata.
func ExtractSymbolsTreeSitterWithOptions(ext string, content []byte, opts ExtractionOptions) []Symbol {
	langName, ok := tsExtMap[ext]
	if !ok {
		return nil
	}

	// For Svelte, extract script block first
	if ext == ".svelte" {
		script := extractSvelteScript(content)
		if script == nil {
			return nil
		}
		content = script
	}

	return extractTreeSitter(content, langName, opts.IncludeRanges)
}

// HasTreeSitter returns true if tree-sitter extraction is available for this extension.
func HasTreeSitter(ext string) bool {
	langName, ok := tsExtMap[ext]
	if !ok {
		return false
	}
	languages := getTSLanguages()
	_, has := languages[langName]
	return has
}
