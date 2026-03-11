package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	json := fs.Bool("json", false, "JSON output")
	r := fs.Bool("r", false, "Recursive")

	args := []string{"file1.py", "--json", "file2.py", "-r"}
	positional := parseFlags(args, fs)

	if !*json {
		t.Error("expected --json to be true")
	}
	if !*r {
		t.Error("expected -r to be true")
	}
	if len(positional) != 2 || positional[0] != "file1.py" || positional[1] != "file2.py" {
		t.Errorf("positional = %v, want [file1.py file2.py]", positional)
	}
}

func TestParseFlagsWithValue(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	root := fs.String("root", "", "Project root")

	args := []string{"--root", "/tmp/project", "file.py"}
	positional := parseFlags(args, fs)

	if *root != "/tmp/project" {
		t.Errorf("root = %q, want '/tmp/project'", *root)
	}
	if len(positional) != 1 || positional[0] != "file.py" {
		t.Errorf("positional = %v, want [file.py]", positional)
	}
}

func TestParseFlagsDoubleDash(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("json", false, "")

	args := []string{"--", "--json", "file.py"}
	positional := parseFlags(args, fs)

	if len(positional) != 2 || positional[0] != "--json" || positional[1] != "file.py" {
		t.Errorf("positional = %v, want [--json file.py]", positional)
	}
}

func TestParseFlagsEqualsSyntax(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	root := fs.String("root", "", "")

	args := []string{"--root=/tmp/project", "file.py"}
	positional := parseFlags(args, fs)

	if *root != "/tmp/project" {
		t.Errorf("root = %q, want '/tmp/project'", *root)
	}
	if len(positional) != 1 {
		t.Errorf("positional = %v, want [file.py]", positional)
	}
}

func TestCollectFilesSingle(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "test.py")
	os.WriteFile(pyFile, []byte("# python"), 0644)
	txtFile := filepath.Join(dir, "readme.txt")
	os.WriteFile(txtFile, []byte("text"), 0644)

	files := collectFiles([]string{pyFile}, false)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
}

func TestCollectFilesDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.py"), []byte("# a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.js"), []byte("// b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644)

	files := collectFiles([]string{dir}, false)
	if len(files) != 2 {
		t.Errorf("got %d files, want 2 (py + js)", len(files))
	}
}

func TestCollectFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "a.py"), []byte("# a"), 0644)
	os.WriteFile(filepath.Join(sub, "b.py"), []byte("# b"), 0644)

	files := collectFiles([]string{dir}, true)
	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
}

func TestCollectFilesSkipsDirs(t *testing.T) {
	dir := t.TempDir()
	nm := filepath.Join(dir, "node_modules")
	os.MkdirAll(nm, 0755)
	os.WriteFile(filepath.Join(nm, "dep.js"), []byte("// dep"), 0644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("// app"), 0644)

	files := collectFiles([]string{dir}, true)
	if len(files) != 1 {
		t.Errorf("got %d files, want 1 (should skip node_modules)", len(files))
	}
}

func TestCollectFilesSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".hidden")
	os.MkdirAll(hidden, 0755)
	os.WriteFile(filepath.Join(hidden, "secret.py"), []byte("# secret"), 0644)
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("# app"), 0644)

	files := collectFiles([]string{dir}, true)
	if len(files) != 1 {
		t.Errorf("got %d files, want 1 (should skip hidden dirs)", len(files))
	}
}

func TestCollectFilesNonexistent(t *testing.T) {
	files := collectFiles([]string{"/nonexistent/path"}, false)
	if len(files) != 0 {
		t.Errorf("got %d files for nonexistent path, want 0", len(files))
	}
}

func TestCollectFilesUnsupportedExt(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# readme"), 0644)

	files := collectFiles([]string{filepath.Join(dir, "readme.md")}, false)
	if len(files) != 0 {
		t.Errorf("got %d files for .md, want 0", len(files))
	}
}

func TestFindProjectRoot(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0755)

	root := findProjectRoot(sub)
	if root != dir {
		t.Errorf("findProjectRoot = %q, want %q", root, dir)
	}
}

func TestFindProjectRootFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	f := filepath.Join(dir, "main.py")
	os.WriteFile(f, []byte(""), 0644)

	root := findProjectRoot(f)
	if root != dir {
		t.Errorf("findProjectRoot = %q, want %q", root, dir)
	}
}

func TestFindProjectRootNoMarker(t *testing.T) {
	dir := t.TempDir()
	root := findProjectRoot(dir)
	// Should return the dir itself when no marker found
	if root == "" {
		t.Error("findProjectRoot returned empty string")
	}
}
