package main

import (
	"strings"
	"testing"
)

func TestExtractSymbolsPython(t *testing.T) {
	content := `
VERSION = "1.0"
API_URL = "https://example.com"
logger = logging.getLogger(__name__)

def greet(name):
    pass

async def fetch_data(url):
    pass

class MyClass:
    def method(self):
        pass

class AnotherClass(Base):
    pass
`
	symbols := extractSymbolsPython(content)

	want := []struct {
		name string
		kind string
	}{
		{"VERSION", "constant"},
		{"API_URL", "constant"},
		{"logger", "variable"},
		{"greet", "def"},
		{"fetch_data", "async def"},
		{"MyClass", "class"},
		{"AnotherClass", "class"},
	}

	if len(symbols) != len(want) {
		t.Fatalf("got %d symbols, want %d: %+v", len(symbols), len(want), symbols)
	}
	for i, w := range want {
		if symbols[i].Name != w.name {
			t.Errorf("symbol[%d].Name = %q, want %q", i, symbols[i].Name, w.name)
		}
		if symbols[i].Kind != w.kind {
			t.Errorf("symbol[%d].Kind = %q, want %q", i, symbols[i].Kind, w.kind)
		}
	}
}

func TestExtractSymbolsPythonConstants(t *testing.T) {
	content := `MAX_RETRIES = 3
DB_URL = "postgres://localhost"
x: int = 42
items: list[str] = []
`
	symbols := extractSymbolsPython(content)
	want := map[string]string{
		"MAX_RETRIES": "constant",
		"DB_URL":      "constant",
		"x":           "variable",
		"items":       "variable",
	}
	if len(symbols) != len(want) {
		t.Fatalf("got %d symbols, want %d: %+v", len(symbols), len(want), symbols)
	}
	for _, s := range symbols {
		if wantKind, ok := want[s.Name]; ok {
			if s.Kind != wantKind {
				t.Errorf("%s: kind = %q, want %q", s.Name, s.Kind, wantKind)
			}
		} else {
			t.Errorf("unexpected symbol %q", s.Name)
		}
	}
}

func TestExtractSymbolsPythonSkipsPrivate(t *testing.T) {
	content := `_private = 1
__dunder__ = 2
PUBLIC = 3
`
	symbols := extractSymbolsPython(content)
	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}
	if names["_private"] || names["__dunder__"] {
		t.Errorf("should skip private/dunder assignments: %+v", symbols)
	}
	if !names["PUBLIC"] {
		t.Error("should include PUBLIC")
	}
}

func TestExtractSymbolsPythonSkipsComplexAssign(t *testing.T) {
	content := `obj.attr = 1
data["key"] = 2
VALID = True
`
	symbols := extractSymbolsPython(content)
	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}
	if names["obj"] || names["data"] {
		t.Error("should skip attribute/subscript assignments")
	}
	if !names["VALID"] {
		t.Error("should include VALID")
	}
}

func TestExtractSymbolsPythonSkipsIndented(t *testing.T) {
	content := `class Foo:
    def bar(self):
        pass
    def baz(self):
        pass
`
	symbols := extractSymbolsPython(content)
	if len(symbols) != 1 {
		t.Fatalf("expected 1 top-level symbol, got %d: %+v", len(symbols), symbols)
	}
	if symbols[0].Name != "Foo" {
		t.Errorf("got %q, want %q", symbols[0].Name, "Foo")
	}
}

func TestExtractSymbolsTS(t *testing.T) {
	content := `
export function greet(name: string): void {}
export default class App {}
interface Config {}
type ID = string
export const VERSION = "1.0"
enum Status { Active, Inactive }
`
	symbols := extractSymbolsTS(content)

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, want := range []string{"greet", "App", "Config", "ID", "VERSION", "Status"} {
		if !names[want] {
			t.Errorf("missing symbol %q in %+v", want, symbols)
		}
	}
}

func TestExtractSymbolsGo(t *testing.T) {
	content := `package main

func main() {}

func (s *Server) Start() error { return nil }

type Server struct {}

type Handler interface {}
`
	symbols := extractSymbolsGo(content)

	want := map[string]string{
		"main":    "function",
		"Start":   "function",
		"Server":  "type",
		"Handler": "type",
	}

	if len(symbols) != len(want) {
		t.Fatalf("got %d symbols, want %d: %+v", len(symbols), len(want), symbols)
	}
	for _, s := range symbols {
		if wantKind, ok := want[s.Name]; ok {
			if s.Kind != wantKind {
				t.Errorf("%s: kind = %q, want %q", s.Name, s.Kind, wantKind)
			}
		} else {
			t.Errorf("unexpected symbol %q", s.Name)
		}
	}
}

func TestExtractSymbolsRust(t *testing.T) {
	content := `
pub fn serve(port: u16) {}
pub async fn handle() {}
struct Config {}
pub enum Status { Ok, Err }
trait Handler {}
impl Config {}
`
	symbols := extractSymbolsRust(content)

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}
	for _, want := range []string{"serve", "handle", "Config", "Status", "Handler"} {
		if !names[want] {
			t.Errorf("missing symbol %q", want)
		}
	}
}

func TestExtractSymbolsJava(t *testing.T) {
	content := `
public class MyService {
    public void handle() {}
}

public interface Handler {}

enum Status { ACTIVE }
`
	symbols := extractSymbolsJava(content)
	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}
	for _, want := range []string{"MyService", "Handler", "Status"} {
		if !names[want] {
			t.Errorf("missing symbol %q", want)
		}
	}
}

func TestExtractSymbolsCSharp(t *testing.T) {
	content := `
public class MyController {}
internal interface IService {}
public struct Point {}
public enum Color { Red, Blue }
`
	symbols := extractSymbolsCSharp(content)
	if len(symbols) != 4 {
		t.Fatalf("got %d symbols, want 4: %+v", len(symbols), symbols)
	}
}

func TestExtractSymbolsPHP(t *testing.T) {
	content := `<?php
function helper() {}
class UserController {}
public static function index() {}
`
	symbols := extractSymbolsPHP(content)
	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}
	if !names["helper"] || !names["UserController"] || !names["index"] {
		t.Errorf("missing symbols: %+v", symbols)
	}
}

func TestExtractSymbolsBash(t *testing.T) {
	content := `#!/bin/bash
function setup() { echo "setup"; }
cleanup() { echo "cleanup"; }
`
	symbols := extractSymbolsBash(content)
	if len(symbols) != 2 {
		t.Fatalf("got %d symbols, want 2: %+v", len(symbols), symbols)
	}
}

func TestExtractSymbolsDispatch(t *testing.T) {
	tests := []struct {
		file    string
		content string
		wantN   int
	}{
		{"test.py", "def foo(): pass\nclass Bar: pass\nVALUE = 1", 3},
		{"test.go", "package main\nfunc main() {}", 1},
		{"test.rs", "fn main() {}", 1},
		{"test.unknown", "whatever", 0},
	}

	for _, tt := range tests {
		r := ExtractSymbols(tt.file, tt.content)
		if tt.file == "test.unknown" {
			if r.Error == "" {
				t.Errorf("%s: expected error for unsupported extension", tt.file)
			}
			continue
		}
		if len(r.Symbols) != tt.wantN {
			t.Errorf("%s: got %d symbols, want %d", tt.file, len(r.Symbols), tt.wantN)
		}
	}
}

func TestExtractSymbolsLineNumbers(t *testing.T) {
	content := "def first(): pass\n\ndef second(): pass\n"
	symbols := extractSymbolsPython(content)
	if len(symbols) != 2 {
		t.Fatalf("got %d symbols, want 2", len(symbols))
	}
	if symbols[0].Line != 1 {
		t.Errorf("first symbol line = %d, want 1", symbols[0].Line)
	}
	if symbols[1].Line != 3 {
		t.Errorf("second symbol line = %d, want 3", symbols[1].Line)
	}
}

func TestFormatSymbolResult(t *testing.T) {
	r := SymbolResult{
		File:    "test.py",
		Lines:   10,
		Symbols: []Symbol{{Name: "foo", Kind: "def", Line: 1}},
	}
	out := FormatSymbolResult(r)
	if !strings.Contains(out, "test.py") {
		t.Error("output should contain filename")
	}
	if !strings.Contains(out, "foo") {
		t.Error("output should contain symbol name")
	}
}

func TestFormatSymbolResultError(t *testing.T) {
	r := SymbolResult{File: "bad.py", Error: "read failed"}
	out := FormatSymbolResult(r)
	if !strings.Contains(out, "error") {
		t.Error("output should contain error")
	}
}

func TestFormatSymbolResultEmpty(t *testing.T) {
	r := SymbolResult{File: "empty.py", Lines: 5, Symbols: []Symbol{}}
	out := FormatSymbolResult(r)
	if !strings.Contains(out, "no symbols found") {
		t.Error("output should indicate no symbols found")
	}
}

func TestExtractSymbolsEmptyContent(t *testing.T) {
	r := ExtractSymbols("test.py", "")
	if r.Error != "" {
		t.Errorf("unexpected error: %s", r.Error)
	}
}

func TestExtractSymbolsParallel(t *testing.T) {
	// Can't easily test with real files, but verify it doesn't panic with empty input
	results := ExtractSymbolsParallel(nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// Bug: C/C++ files fell through to extractSymbolsRust which misses functions.
func TestExtractSymbolsC_FallbackExtractsFunctions(t *testing.T) {
	content := `
int add(int a, int b) { return a + b; }
void reset(void) {}
struct Point { int x; int y; };
enum Color { RED, GREEN, BLUE };
`
	symbols := extractSymbolsC(content)
	names := make(map[string]string)
	for _, s := range symbols {
		names[s.Name] = s.Kind
	}
	if names["add"] != "function" {
		t.Errorf("add: got kind %q, want %q", names["add"], "function")
	}
	if names["reset"] != "function" {
		t.Errorf("reset: got kind %q, want %q", names["reset"], "function")
	}
	if names["Point"] != "struct" {
		t.Errorf("Point: got kind %q, want %q", names["Point"], "struct")
	}
	if names["Color"] != "enum" {
		t.Errorf("Color: got kind %q, want %q", names["Color"], "enum")
	}
}

func TestExtractSymbolsC_FallbackDoesNotProduceRustSymbols(t *testing.T) {
	// Verify the C fallback is NOT using the Rust extractor.
	// Rust patterns (fn, pub, trait, impl) should not produce matches on C code.
	content := `
struct Node { int val; };
int process(int x) { return x * 2; }
`
	symbols := extractSymbolsC(content)
	for _, s := range symbols {
		if s.Kind == "impl" || s.Kind == "trait" {
			t.Errorf("got Rust-specific kind %q for symbol %q in C code", s.Kind, s.Name)
		}
	}
}

// Bug: markExportsGo used name[0] (a byte) instead of the first rune,
// causing Unicode uppercase identifiers like Ä to be missed as exported.
func TestMarkExportsGoUnicode(t *testing.T) {
	result := &SymbolResult{
		File: "test.go",
		Symbols: []Symbol{
			{Name: "Ä", Kind: "function"}, // U+00C4, valid Go exported ident
			{Name: "Normal", Kind: "function"},
			{Name: "lower", Kind: "function"},
		},
	}
	markExportsGo(result)

	if result.Symbols[0].Exported == nil || !*result.Symbols[0].Exported {
		t.Error("Ä should be exported (Unicode uppercase)")
	}
	if result.Symbols[1].Exported == nil || !*result.Symbols[1].Exported {
		t.Error("Normal should be exported")
	}
	if result.Symbols[2].Exported == nil || *result.Symbols[2].Exported {
		t.Error("lower should not be exported")
	}
}

func TestMarkExportsTSCommonJSObjectExports(t *testing.T) {
	content := `
function deriveRouteDecision() {}
function deriveRouteState() {}
function internalHelper() {}

module.exports = {
  deriveRouteDecision,
  routeState: deriveRouteState,
};
`
	result := ExtractSymbolsWithOptions("route-decision.cjs", content, ExtractionOptions{})
	MarkExports(&result, content)
	exported := FilterExported([]SymbolResult{result})[0].Symbols

	names := make(map[string]bool)
	for _, s := range exported {
		names[symbolBaseName(s.Name)] = true
	}

	for _, want := range []string{"deriveRouteDecision", "deriveRouteState"} {
		if !names[want] {
			t.Errorf("expected CommonJS export %q in %+v", want, exported)
		}
	}
	if names["internalHelper"] {
		t.Errorf("internalHelper should not be exported: %+v", exported)
	}
}

func TestMarkExportsTSCommonJSNamedAndDefaultExports(t *testing.T) {
	content := `
function runTicketWorkflow() {}
function buildTicketOutput() {}
function privateHelper() {}

exports.buildTicketOutput = buildTicketOutput;
module.exports.runTicketWorkflow = runTicketWorkflow;
`
	result := ExtractSymbolsWithOptions("ticket-output.cjs", content, ExtractionOptions{})
	MarkExports(&result, content)
	exported := FilterExported([]SymbolResult{result})[0].Symbols

	names := make(map[string]bool)
	for _, s := range exported {
		names[symbolBaseName(s.Name)] = true
	}

	for _, want := range []string{"buildTicketOutput", "runTicketWorkflow"} {
		if !names[want] {
			t.Errorf("expected CommonJS named export %q in %+v", want, exported)
		}
	}
	if names["privateHelper"] {
		t.Errorf("privateHelper should not be exported: %+v", exported)
	}

	defaultContent := `
function main() {}
function helper() {}
module.exports = main;
`
	defaultResult := ExtractSymbolsWithOptions("main.cjs", defaultContent, ExtractionOptions{})
	MarkExports(&defaultResult, defaultContent)
	defaultExported := FilterExported([]SymbolResult{defaultResult})[0].Symbols
	defaultNames := make(map[string]bool)
	for _, s := range defaultExported {
		defaultNames[symbolBaseName(s.Name)] = true
	}
	if !defaultNames["main"] {
		t.Errorf("expected CommonJS default export main in %+v", defaultExported)
	}
	if defaultNames["helper"] {
		t.Errorf("helper should not be exported: %+v", defaultExported)
	}
}

func TestExtractSymbolsWithRangesOptIn(t *testing.T) {
	content := "func main() {}\n"

	withRanges := ExtractSymbolsWithOptions("test.go", content, ExtractionOptions{IncludeRanges: true})
	if len(withRanges.Symbols) == 0 {
		t.Fatal("expected symbols")
	}
	s := withRanges.Symbols[0]
	if s.StartLine == nil || s.EndLine == nil || s.StartCol == nil || s.EndCol == nil {
		t.Fatalf("expected range fields, got %+v", s)
	}
}

func TestExtractSymbolsWithoutRangesByDefault(t *testing.T) {
	content := "func main() {}\n"

	withoutRanges := ExtractSymbols("test.go", content)
	if len(withoutRanges.Symbols) == 0 {
		t.Fatal("expected symbols")
	}
	s := withoutRanges.Symbols[0]
	if s.StartLine != nil || s.EndLine != nil || s.StartCol != nil || s.EndCol != nil {
		t.Fatalf("expected no range fields by default, got %+v", s)
	}
}

func TestExtractSymbolsWithRangesLanguageMatrix(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{name: "python", file: "a.py", content: "def foo(x):\n    return x\n"},
		{name: "typescript", file: "a.ts", content: "export function foo(x: number) { return x }\n"},
		{name: "tsx", file: "a.tsx", content: "export function Foo() { return <div/> }\n"},
		{name: "javascript", file: "a.js", content: "function foo(x) { return x }\n"},
		{name: "go", file: "a.go", content: "package main\nfunc main() {}\n"},
		{name: "java", file: "A.java", content: "class A { void m() {} }\n"},
		{name: "kotlin", file: "A.kt", content: "class A { fun m() {} }\n"},
		{name: "rust", file: "a.rs", content: "fn foo() {}\n"},
		{name: "csharp", file: "A.cs", content: "class A { void M() {} }\n"},
		{name: "php", file: "a.php", content: "<?php\nfunction foo() {}\n"},
		{name: "c", file: "a.c", content: "int foo() { return 1; }\n"},
		{name: "cpp", file: "a.cpp", content: "int foo() { return 1; }\n"},
		{name: "ruby", file: "a.rb", content: "class A\nend\n"},
		{name: "scala", file: "A.scala", content: "object A {}\n"},
		{name: "bash", file: "a.sh", content: "foo() { echo hi; }\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ExtractSymbolsWithOptions(tt.file, tt.content, ExtractionOptions{IncludeRanges: true})
			if r.Error != "" {
				t.Fatalf("unexpected error: %s", r.Error)
			}
			if len(r.Symbols) == 0 {
				t.Fatalf("expected at least one symbol, got none for %s", tt.file)
			}
			for _, s := range r.Symbols {
				if s.StartLine == nil || s.EndLine == nil || s.StartCol == nil || s.EndCol == nil {
					t.Fatalf("expected range fields for symbol %+v in %s", s, tt.file)
				}
			}
		})
	}
}
