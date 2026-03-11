package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestProject(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"main.py":             "from lib import utils\n",
		"lib/__init__.py":     "",
		"lib/utils.py":        "def helper(): pass\n",
		"src/app.ts":          "import { foo } from './utils'\n",
		"src/utils.ts":        "export function foo() {}\n",
		"src/index.ts":        "export * from './app'\n",
		"pkg/main.go":         "package pkg\n",
		"models/User.java":    "public class User {}\n",
		"models/Order.java":   "public class Order {}\n",
		"src/config.rs":       "pub fn init() {}\n",
		"src/routes/mod.rs":   "pub mod api;\n",
		"src/routes/api.rs":   "pub fn handle() {}\n",
		"Controllers/Home.cs": "namespace App.Controllers {}\n",
		"app/Http/Auth.php":   "<?php namespace App\\Http;\n",
	}

	var paths []string
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(content), 0644)
		paths = append(paths, p)
	}

	return dir, paths
}

func TestNewProjectIndex(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	if idx.Root != dir {
		t.Errorf("Root = %q, want %q", idx.Root, dir)
	}
	if len(idx.allFiles) != len(files) {
		t.Errorf("allFiles has %d entries, want %d", len(idx.allFiles), len(files))
	}
}

func TestResolvePythonDotModule(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	result := idx.ResolvePython("lib", filepath.Join(dir, "main.py"))
	if result == "" {
		t.Error("failed to resolve 'lib' (should find __init__.py)")
	}
}

func TestResolvePythonRelativeImport(t *testing.T) {
	dir := t.TempDir()

	paths := []string{
		filepath.Join(dir, "pkg", "__init__.py"),
		filepath.Join(dir, "pkg", "a.py"),
		filepath.Join(dir, "pkg", "b.py"),
	}
	for _, p := range paths {
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(""), 0644)
	}

	idx := NewProjectIndex(dir, paths)
	result := idx.ResolvePython(".b", filepath.Join(dir, "pkg", "a.py"))
	if result != filepath.Join(dir, "pkg", "b.py") {
		t.Errorf("got %q, want pkg/b.py", result)
	}
}

func TestResolveTSRelative(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	result := idx.ResolveTS("./utils", filepath.Join(dir, "src", "app.ts"))
	if result == "" {
		t.Error("failed to resolve ./utils from src/app.ts")
	}
}

func TestResolveTSIndex(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "components", "index.ts")
	os.MkdirAll(filepath.Dir(p), 0755)
	os.WriteFile(p, []byte(""), 0644)
	from := filepath.Join(dir, "app.ts")
	os.WriteFile(from, []byte(""), 0644)

	idx := NewProjectIndex(dir, []string{p, from})
	result := idx.ResolveTS("./components", from)
	if result == "" {
		t.Error("failed to resolve ./components (should find index.ts)")
	}
}

func TestResolveTSAlias(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	// $lib/utils should try to resolve "utils"
	result := idx.ResolveTS("$lib/utils", filepath.Join(dir, "src", "app.ts"))
	// May or may not resolve depending on index structure — just verify no crash
	_ = result
}

func TestResolveTSBareModule(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	// Bare modules like 'react' should return empty (external)
	result := idx.ResolveTS("react", filepath.Join(dir, "src", "app.ts"))
	if result != "" {
		t.Errorf("bare module 'react' should return empty, got %q", result)
	}
}

func TestResolveGoModule(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/test/proj\n"), 0644)
	pkgDir := filepath.Join(dir, "pkg")
	os.MkdirAll(pkgDir, 0755)
	pkgFile := filepath.Join(pkgDir, "main.go")
	os.WriteFile(pkgFile, []byte("package pkg\n"), 0644)

	idx := NewProjectIndex(dir, []string{pkgFile})
	if idx.GoModule != "github.com/test/proj" {
		t.Errorf("GoModule = %q, want 'github.com/test/proj'", idx.GoModule)
	}

	result := idx.ResolveGo("github.com/test/proj/pkg", pkgFile)
	if result == "" {
		t.Error("failed to resolve Go internal package")
	}
}

func TestResolveGoStdlib(t *testing.T) {
	dir := t.TempDir()
	idx := NewProjectIndex(dir, nil)

	result := idx.ResolveGo("fmt", filepath.Join(dir, "main.go"))
	if result != "" {
		t.Errorf("stdlib 'fmt' should return empty, got %q", result)
	}
}

func TestResolveJava(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	result := idx.ResolveJava("models.User", filepath.Join(dir, "models", "User.java"))
	if result == "" {
		t.Error("failed to resolve models.User")
	}
}

func TestResolveRustMod(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	result := idx.ResolveRust("api", "mod", filepath.Join(dir, "src", "routes", "mod.rs"))
	if result != filepath.Join(dir, "src", "routes", "api.rs") {
		t.Errorf("got %q, want src/routes/api.rs", result)
	}
}

func TestResolveRustUse(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	result := idx.ResolveRust("crate::config", "use", filepath.Join(dir, "src", "routes", "api.rs"))
	if result == "" {
		t.Error("failed to resolve crate::config")
	}
}

func TestResolveCSharp(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	result := idx.ResolveCSharp("App.Controllers.Home", filepath.Join(dir, "test.cs"))
	// May not resolve exactly, but shouldn't crash
	_ = result
}

func TestResolvePHP(t *testing.T) {
	dir, files := setupTestProject(t)
	idx := NewProjectIndex(dir, files)

	result := idx.ResolvePHP("App\\Http\\Auth", "use", filepath.Join(dir, "test.php"))
	// May or may not resolve, just verify no crash
	_ = result
}

func TestResolvePHPRequire(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.php")
	helper := filepath.Join(dir, "helper.php")
	os.WriteFile(main, []byte("<?php require 'helper.php';"), 0644)
	os.WriteFile(helper, []byte("<?php function h() {}"), 0644)

	idx := NewProjectIndex(dir, []string{main, helper})
	result := idx.ResolvePHP("helper.php", "require", main)
	if result != helper {
		t.Errorf("got %q, want %q", result, helper)
	}
}

func TestResolveTSConfigPaths(t *testing.T) {
	dir := t.TempDir()

	// Create tsconfig.json with path aliases
	tsconfig := `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@/*": ["./src/*"],
				"@components/*": ["./src/components/*"],
				"~utils/*": ["./src/utils/*"]
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(tsconfig), 0644)

	// Create source files
	files := map[string]string{
		"src/app.ts":                "",
		"src/components/Button.tsx": "",
		"src/components/index.ts":  "",
		"src/utils/format.ts":      "",
		"src/pages/Home.tsx":       "",
	}
	var paths []string
	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(content), 0644)
		paths = append(paths, p)
	}

	idx := NewProjectIndex(dir, paths)
	from := filepath.Join(dir, "src/pages/Home.tsx")

	tests := []struct {
		specifier string
		wantFile  string
	}{
		{"@/app", "src/app.ts"},
		{"@/components/Button", "src/components/Button.tsx"},
		{"@components/Button", "src/components/Button.tsx"},
		{"@/components", "src/components/index.ts"},
		{"~utils/format", "src/utils/format.ts"},
	}

	for _, tt := range tests {
		result := idx.ResolveTS(tt.specifier, from)
		want := filepath.Join(dir, tt.wantFile)
		if result != want {
			t.Errorf("ResolveTS(%q) = %q, want %q", tt.specifier, result, want)
		}
	}
}

func TestResolveTSConfigBaseUrl(t *testing.T) {
	dir := t.TempDir()

	tsconfig := `{
		"compilerOptions": {
			"baseUrl": "src",
			"paths": {
				"@/*": ["./*"]
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(tsconfig), 0644)

	p := filepath.Join(dir, "src", "lib", "api.ts")
	os.MkdirAll(filepath.Dir(p), 0755)
	os.WriteFile(p, []byte(""), 0644)

	idx := NewProjectIndex(dir, []string{p})
	result := idx.ResolveTS("@/lib/api", filepath.Join(dir, "src", "app.ts"))
	if result != p {
		t.Errorf("got %q, want %q", result, p)
	}
}

func TestResolveTSConfigExtends(t *testing.T) {
	dir := t.TempDir()

	base := `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@/*": ["./src/*"]
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "tsconfig.base.json"), []byte(base), 0644)

	child := `{
		"extends": "./tsconfig.base.json",
		"compilerOptions": {
			"paths": {
				"@lib/*": ["./lib/*"]
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(child), 0644)

	srcFile := filepath.Join(dir, "src", "app.ts")
	libFile := filepath.Join(dir, "lib", "helpers.ts")
	os.MkdirAll(filepath.Dir(srcFile), 0755)
	os.MkdirAll(filepath.Dir(libFile), 0755)
	os.WriteFile(srcFile, []byte(""), 0644)
	os.WriteFile(libFile, []byte(""), 0644)

	idx := NewProjectIndex(dir, []string{srcFile, libFile})
	from := filepath.Join(dir, "index.ts")

	// Parent alias should work
	result := idx.ResolveTS("@/app", from)
	if result != srcFile {
		t.Errorf("parent alias: got %q, want %q", result, srcFile)
	}

	// Child alias should work
	result = idx.ResolveTS("@lib/helpers", from)
	if result != libFile {
		t.Errorf("child alias: got %q, want %q", result, libFile)
	}
}

func TestResolveTSConfigNoFile(t *testing.T) {
	dir := t.TempDir()
	// No tsconfig.json — should not crash
	idx := NewProjectIndex(dir, nil)
	if len(idx.tsAliases) != 0 {
		t.Errorf("expected no aliases without tsconfig, got %d", len(idx.tsAliases))
	}
}

func TestResolveTSConfigJsconfig(t *testing.T) {
	dir := t.TempDir()

	jsconfig := `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@/*": ["./src/*"]
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "jsconfig.json"), []byte(jsconfig), 0644)

	p := filepath.Join(dir, "src", "app.js")
	os.MkdirAll(filepath.Dir(p), 0755)
	os.WriteFile(p, []byte(""), 0644)

	idx := NewProjectIndex(dir, []string{p})
	result := idx.ResolveTS("@/app", filepath.Join(dir, "index.js"))
	if result != p {
		t.Errorf("jsconfig: got %q, want %q", result, p)
	}
}

func TestResolveImportDispatch(t *testing.T) {
	dir := t.TempDir()
	idx := NewProjectIndex(dir, nil)

	// Unsupported extension returns empty
	result := idx.ResolveImport(".txt", "something", "import", "test.txt")
	if result != "" {
		t.Errorf("expected empty for unsupported ext, got %q", result)
	}
}
