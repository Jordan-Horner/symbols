package main

import (
	"testing"
)

func TestExtractImportsPython(t *testing.T) {
	content := `
import os
import sys, json
from pathlib import Path
from collections import OrderedDict, defaultdict
from . import sibling
from ..parent import helper
from typing import (
    List,
    Dict,
    Optional
)
import numpy as np
`
	imports := extractImportsPython(content)

	modules := make(map[string]bool)
	for _, imp := range imports {
		modules[imp.Module] = true
	}

	for _, want := range []string{"os", "sys", "json", "pathlib", ".", "..parent", "typing", "numpy"} {
		if !modules[want] {
			t.Errorf("missing import %q, got: %+v", want, imports)
		}
	}
}

func TestExtractImportsPythonFromNames(t *testing.T) {
	content := `from os.path import join, exists`
	imports := extractImportsPython(content)
	if len(imports) != 1 {
		t.Fatalf("got %d imports, want 1", len(imports))
	}
	if len(imports[0].Names) != 2 {
		t.Errorf("got %d names, want 2: %+v", len(imports[0].Names), imports[0].Names)
	}
}

func TestExtractImportsTS(t *testing.T) {
	content := `
import React from 'react'
import { useState, useEffect } from 'react'
import type { Config } from './types'
import utils from '../utils'
const fs = require('fs')
export { foo } from './foo'
const mod = await import('./lazy')
`
	imports := extractImportsTS(content)

	modules := make(map[string]bool)
	for _, imp := range imports {
		modules[imp.Module] = true
	}

	for _, want := range []string{"react", "./types", "../utils", "fs", "./foo", "./lazy"} {
		if !modules[want] {
			t.Errorf("missing import %q, got modules: %v", want, modules)
		}
	}
}

func TestExtractImportsTSKinds(t *testing.T) {
	content := `
import { a } from 'mod-a'
const b = require('mod-b')
const c = await import('mod-c')
export { d } from 'mod-d'
`
	imports := extractImportsTS(content)

	kindMap := make(map[string]string)
	for _, imp := range imports {
		kindMap[imp.Module] = imp.Kind
	}

	if kindMap["mod-a"] != "import" {
		t.Errorf("mod-a kind = %q, want 'import'", kindMap["mod-a"])
	}
	if kindMap["mod-b"] != "require" {
		t.Errorf("mod-b kind = %q, want 'require'", kindMap["mod-b"])
	}
	if kindMap["mod-c"] != "dynamic" {
		t.Errorf("mod-c kind = %q, want 'dynamic'", kindMap["mod-c"])
	}
	if kindMap["mod-d"] != "re-export" {
		t.Errorf("mod-d kind = %q, want 're-export'", kindMap["mod-d"])
	}
}

func TestExtractImportsGo(t *testing.T) {
	content := `package main

import "fmt"
import (
	"os"
	"path/filepath"
	mylib "github.com/user/lib"
)
`
	imports := extractImportsGo(content)

	modules := make(map[string]bool)
	for _, imp := range imports {
		modules[imp.Module] = true
	}

	for _, want := range []string{"fmt", "os", "path/filepath", "github.com/user/lib"} {
		if !modules[want] {
			t.Errorf("missing import %q", want)
		}
	}
}

func TestExtractImportsJava(t *testing.T) {
	content := `
import java.util.List;
import java.util.Map;
import static org.junit.Assert.*;
`
	imports := extractImportsJava(content)
	if len(imports) != 3 {
		t.Fatalf("got %d imports, want 3: %+v", len(imports), imports)
	}
}

func TestExtractImportsKotlin(t *testing.T) {
	content := `
import kotlin.collections.List
import com.example.MyClass
`
	imports := extractImportsKotlin(content)
	if len(imports) != 2 {
		t.Fatalf("got %d imports, want 2: %+v", len(imports), imports)
	}
}

func TestExtractImportsRust(t *testing.T) {
	content := `
use std::collections::HashMap;
use crate::config::Settings;
pub use super::utils;
mod helpers;
`
	imports := extractImportsRust(content)

	var useCount, modCount int
	for _, imp := range imports {
		switch imp.Kind {
		case "use":
			useCount++
		case "mod":
			modCount++
		}
	}
	if useCount != 3 {
		t.Errorf("got %d use imports, want 3", useCount)
	}
	if modCount != 1 {
		t.Errorf("got %d mod imports, want 1", modCount)
	}
}

func TestExtractImportsCSharp(t *testing.T) {
	content := `
using System;
using System.Collections.Generic;
using static System.Math;
`
	imports := extractImportsCSharp(content)
	if len(imports) != 3 {
		t.Fatalf("got %d imports, want 3: %+v", len(imports), imports)
	}
}

func TestExtractImportsPHP(t *testing.T) {
	content := `<?php
use App\Models\User;
use App\Services\Auth as AuthService;
require_once 'vendor/autoload.php';
include 'helpers.php';
`
	imports := extractImportsPHP(content)

	kinds := make(map[string]int)
	for _, imp := range imports {
		kinds[imp.Kind]++
	}
	if kinds["use"] != 2 {
		t.Errorf("got %d use imports, want 2", kinds["use"])
	}
	if kinds["require"] != 2 {
		t.Errorf("got %d require imports, want 2", kinds["require"])
	}
}

func TestExtractImportsDispatch(t *testing.T) {
	tests := []struct {
		file    string
		content string
	}{
		{"test.py", "import os"},
		{"test.ts", "import { foo } from './foo'"},
		{"test.go", `import "fmt"`},
		{"test.java", "import java.util.List;"},
		{"test.rs", "use std::io;"},
	}

	for _, tt := range tests {
		r := ExtractImports(tt.file, tt.content)
		if len(r.Imports) == 0 {
			t.Errorf("%s: expected imports, got none", tt.file)
		}
		if r.Error != "" {
			t.Errorf("%s: unexpected error: %s", tt.file, r.Error)
		}
	}
}

func TestExtractImportsUnsupported(t *testing.T) {
	r := ExtractImports("test.txt", "whatever")
	if r.Note == "" {
		t.Error("expected note for unsupported file type")
	}
}

func TestExtractImportsEmpty(t *testing.T) {
	r := ExtractImports("test.py", "")
	if r.Error != "" {
		t.Errorf("unexpected error: %s", r.Error)
	}
	if len(r.Imports) != 0 {
		t.Errorf("expected 0 imports, got %d", len(r.Imports))
	}
}

func TestExtractImportsLineNumbers(t *testing.T) {
	content := "import os\nimport sys\n"
	imports := extractImportsPython(content)
	if len(imports) != 2 {
		t.Fatalf("got %d imports, want 2", len(imports))
	}
	if imports[0].Line != 1 {
		t.Errorf("first import line = %d, want 1", imports[0].Line)
	}
	if imports[1].Line != 2 {
		t.Errorf("second import line = %d, want 2", imports[1].Line)
	}
}

func TestExtractImportsPHPAlias(t *testing.T) {
	content := `<?php
use App\Services\Auth as AuthService;
`
	imports := extractImportsPHP(content)
	if len(imports) != 1 {
		t.Fatalf("got %d imports, want 1", len(imports))
	}
	// Should strip " as AuthService"
	if imports[0].Module != "App\\Services\\Auth" {
		t.Errorf("module = %q, want %q", imports[0].Module, "App\\Services\\Auth")
	}
}

func TestFormatImportsText(t *testing.T) {
	r := ImportResult{
		File: "test.py",
		Imports: []Import{
			{Module: "os", Kind: "import", Line: 1},
			{Module: "pathlib", Kind: "from", Names: []string{"Path"}, Line: 2},
		},
	}
	out := FormatImportsText(r)
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatImportsTextTruncatesNames(t *testing.T) {
	r := ImportResult{
		File: "test.py",
		Imports: []Import{
			{Module: "mod", Kind: "from", Names: []string{"a", "b", "c", "d", "e", "f"}, Line: 1},
		},
	}
	out := FormatImportsText(r)
	if out == "" {
		t.Error("expected non-empty output")
	}
}
