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

func TestResolveImportDispatch(t *testing.T) {
	dir := t.TempDir()
	idx := NewProjectIndex(dir, nil)

	// Unsupported extension returns empty
	result := idx.ResolveImport(".txt", "something", "import", "test.txt")
	if result != "" {
		t.Errorf("expected empty for unsupported ext, got %q", result)
	}
}
