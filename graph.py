"""Import graph builder and dependency analyzer.

Parses import statements from source files, resolves them to actual files
on disk, and builds a directed dependency graph. Supports dependency queries,
reverse-dependency (dependents) queries, and transitive impact analysis.

Supported languages: Python, TypeScript/JavaScript/Svelte, Go, Java, Kotlin,
Rust, C#, PHP.
"""

from __future__ import annotations

import re
from collections import defaultdict, deque
from pathlib import Path
from typing import Optional

# ── Import extraction ─────────────────────────────────────────────────────────

# Python: regex-based import parsing (avoids ast.parse which is ~45ms/file)
_PY_IMPORT_RE = re.compile(
    r'''^\s*import\s+([A-Za-z_][\w.]*(?:\s*,\s*[A-Za-z_][\w.]*)*)''',
    re.MULTILINE,
)
_PY_FROM_IMPORT_RE = re.compile(
    r'''^\s*from\s+(\.{0,3}[A-Za-z_]?[\w.]*)\s+import\s+(.+)''',
    re.MULTILINE,
)


def _extract_imports_python(content: str) -> list[dict]:
    """Extract imports from Python source using regex."""
    imports: list[dict] = []
    seen: set[tuple[str, int]] = set()

    for m in _PY_IMPORT_RE.finditer(content):
        line = content[:m.start()].count("\n") + 1
        # Skip "import" inside "from X import Y" (already captured below)
        prefix = content[:m.start()].rstrip()
        if prefix.endswith("from"):
            continue
        for module in m.group(1).split(","):
            module = module.strip().split(" as ")[0].strip()
            if module and (module, line) not in seen:
                seen.add((module, line))
                imports.append({"module": module, "kind": "import", "line": line})

    for m in _PY_FROM_IMPORT_RE.finditer(content):
        line = content[:m.start()].count("\n") + 1
        module = m.group(1).strip()
        names_str = m.group(2).strip().rstrip("\\")
        # Handle multi-line imports with parens
        if "(" in names_str and ")" not in names_str:
            # Find closing paren
            rest_start = m.end()
            paren_end = content.find(")", rest_start)
            if paren_end != -1:
                names_str = names_str + content[rest_start:paren_end]
        names_str = names_str.strip("() \t")
        names = [n.strip().split(" as ")[0].strip() for n in names_str.split(",") if n.strip()]
        if module and (module, line) not in seen:
            seen.add((module, line))
            imports.append({"module": module, "names": names, "kind": "from", "line": line})

    return imports


# TS/JS/Svelte: regex-based import parsing
_TS_IMPORT_RE = re.compile(
    r'''(?:^|\n)\s*import\s+'''
    r'''(?:'''
    r'''(?:type\s+)?'''
    r'''(?:\{[^}]*\}|[A-Za-z_$][A-Za-z0-9_$]*|\*)'''
    r'''(?:\s*,\s*(?:\{[^}]*\}|\*))?'''
    r'''(?:\s+from\s+)?'''
    r''')?'''
    r'''['"]([^'"]+)['"]''',
    re.MULTILINE,
)
_TS_REQUIRE_RE = re.compile(r'''require\s*\(\s*['"]([^'"]+)['"]\s*\)''')
_TS_DYNAMIC_IMPORT_RE = re.compile(r'''import\s*\(\s*['"]([^'"]+)['"]\s*\)''')
_TS_EXPORT_FROM_RE = re.compile(
    r'''(?:^|\n)\s*export\s+(?:type\s+)?(?:\{[^}]*\}|\*(?:\s+as\s+\w+)?)\s+from\s+['"]([^'"]+)['"]''',
    re.MULTILINE,
)


def _extract_imports_ts(content: str) -> list[dict]:
    """Extract imports from TS/JS/Svelte source."""
    imports: list[dict] = []
    seen: set[str] = set()
    for pattern, kind in [
        (_TS_IMPORT_RE, "import"),
        (_TS_REQUIRE_RE, "require"),
        (_TS_DYNAMIC_IMPORT_RE, "dynamic"),
        (_TS_EXPORT_FROM_RE, "re-export"),
    ]:
        for m in pattern.finditer(content):
            module = m.group(1)
            if module not in seen:
                seen.add(module)
                line = content[:m.start()].count("\n") + 1
                imports.append({"module": module, "kind": kind, "line": line})
    return imports


# Go: import "path" or import ( "path1"\n"path2" )
_GO_IMPORT_SINGLE_RE = re.compile(
    r'''^\s*import\s+(?:\w+\s+)?["` ]([^"` ]+)["` ]''',
    re.MULTILINE,
)
_GO_IMPORT_BLOCK_RE = re.compile(
    r'''import\s*\((.*?)\)''',
    re.DOTALL,
)
_GO_IMPORT_LINE_RE = re.compile(
    r'''(?:\w+\s+)?["` ]([^"` ]+)["` ]''',
)


def _extract_imports_go(content: str) -> list[dict]:
    """Extract imports from Go source."""
    imports: list[dict] = []
    seen: set[str] = set()

    # Single imports
    for m in _GO_IMPORT_SINGLE_RE.finditer(content):
        pkg = m.group(1)
        if pkg not in seen:
            seen.add(pkg)
            line = content[:m.start()].count("\n") + 1
            imports.append({"module": pkg, "kind": "import", "line": line})

    # Block imports
    for block_m in _GO_IMPORT_BLOCK_RE.finditer(content):
        block_start = content[:block_m.start()].count("\n") + 1
        block_content = block_m.group(1)
        for i, block_line in enumerate(block_content.splitlines()):
            line_m = _GO_IMPORT_LINE_RE.search(block_line)
            if line_m:
                pkg = line_m.group(1)
                if pkg not in seen:
                    seen.add(pkg)
                    imports.append({"module": pkg, "kind": "import", "line": block_start + i})

    return imports


# Java: import com.foo.Bar; or import static com.foo.Bar.method;
_JAVA_IMPORT_RE = re.compile(
    r'''^\s*import\s+(?:static\s+)?([A-Za-z_][\w.]*(?:\.\*)?)\s*;''',
    re.MULTILINE,
)
_JAVA_PACKAGE_RE = re.compile(
    r'''^\s*package\s+([A-Za-z_][\w.]*)\s*;''',
    re.MULTILINE,
)


def _extract_imports_java(content: str) -> list[dict]:
    """Extract imports from Java source."""
    imports: list[dict] = []
    for m in _JAVA_IMPORT_RE.finditer(content):
        line = content[:m.start()].count("\n") + 1
        imports.append({"module": m.group(1), "kind": "import", "line": line})
    return imports


# Kotlin: import com.foo.Bar (no semicolon required)
_KOTLIN_IMPORT_RE = re.compile(
    r'''^\s*import\s+([A-Za-z_][\w.]*(?:\.\*)?)\s*$''',
    re.MULTILINE,
)


def _extract_imports_kotlin(content: str) -> list[dict]:
    """Extract imports from Kotlin source."""
    imports: list[dict] = []
    for m in _KOTLIN_IMPORT_RE.finditer(content):
        line = content[:m.start()].count("\n") + 1
        imports.append({"module": m.group(1), "kind": "import", "line": line})
    return imports


# Rust: use crate::foo::bar; mod foo;
_RUST_USE_RE = re.compile(
    r'''^\s*(?:pub\s+)?use\s+((?:crate|self|super|[a-z_]\w*)(?:::\w+)*(?:::\{[^}]+\}|::\*)?)\s*;''',
    re.MULTILINE,
)
_RUST_MOD_RE = re.compile(
    r'''^\s*(?:pub\s+)?mod\s+(\w+)\s*;''',
    re.MULTILINE,
)


def _extract_imports_rust(content: str) -> list[dict]:
    """Extract imports from Rust source."""
    imports: list[dict] = []
    for m in _RUST_USE_RE.finditer(content):
        line = content[:m.start()].count("\n") + 1
        imports.append({"module": m.group(1), "kind": "use", "line": line})
    for m in _RUST_MOD_RE.finditer(content):
        line = content[:m.start()].count("\n") + 1
        imports.append({"module": m.group(1), "kind": "mod", "line": line})
    return imports


# C#: using System.IO; using static System.Math;
_CSHARP_USING_RE = re.compile(
    r'''^\s*using\s+(?:static\s+)?([A-Za-z_][\w.]*)\s*;''',
    re.MULTILINE,
)
_CSHARP_NAMESPACE_RE = re.compile(
    r'''^\s*namespace\s+([A-Za-z_][\w.]*)\s*[{;]''',
    re.MULTILINE,
)


def _extract_imports_csharp(content: str) -> list[dict]:
    """Extract imports from C# source."""
    imports: list[dict] = []
    for m in _CSHARP_USING_RE.finditer(content):
        line = content[:m.start()].count("\n") + 1
        imports.append({"module": m.group(1), "kind": "using", "line": line})
    return imports


# PHP: use App\Models\User; require/include 'file.php';
_PHP_USE_RE = re.compile(
    r'''^\s*use\s+([A-Za-z_\\][\w\\]*(?:\s+as\s+\w+)?)\s*;''',
    re.MULTILINE,
)
_PHP_REQUIRE_RE = re.compile(
    r'''(?:require|include)(?:_once)?\s*[( ]?\s*['"]([^'"]+)['"]\s*[) ]?\s*;''',
)
_PHP_NAMESPACE_RE = re.compile(
    r'''^\s*namespace\s+([A-Za-z_\\][\w\\]*)\s*;''',
    re.MULTILINE,
)


def _extract_imports_php(content: str) -> list[dict]:
    """Extract imports from PHP source."""
    imports: list[dict] = []
    seen: set[str] = set()
    for m in _PHP_USE_RE.finditer(content):
        module = m.group(1).split(" as ")[0].strip()
        if module not in seen:
            seen.add(module)
            line = content[:m.start()].count("\n") + 1
            imports.append({"module": module, "kind": "use", "line": line})
    for m in _PHP_REQUIRE_RE.finditer(content):
        path = m.group(1)
        if path not in seen:
            seen.add(path)
            line = content[:m.start()].count("\n") + 1
            imports.append({"module": path, "kind": "require", "line": line})
    return imports


# ── Extension → extractor dispatch ────────────────────────────────────────────

_IMPORT_EXTRACTORS: dict[str, callable] = {}

def _register_extractor(exts: list[str], fn: callable) -> None:
    for ext in exts:
        _IMPORT_EXTRACTORS[ext] = fn

_register_extractor([".py"], _extract_imports_python)
_register_extractor([".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"], _extract_imports_ts)
_register_extractor([".go"], _extract_imports_go)
_register_extractor([".java"], _extract_imports_java)
_register_extractor([".kt", ".kts"], _extract_imports_kotlin)
_register_extractor([".rs"], _extract_imports_rust)
_register_extractor([".cs"], _extract_imports_csharp)
_register_extractor([".php"], _extract_imports_php)


def extract_imports(file_path: str, content: str | None = None) -> dict:
    """Extract imports from a single file.

    If *content* is provided, skips the file read (used by build_graph to
    avoid double-reading).
    """
    path = Path(file_path)
    ext = path.suffix.lower()

    if content is None:
        try:
            content = path.read_text(encoding="utf-8", errors="replace")
        except OSError as e:
            return {"file": str(path), "error": str(e), "imports": []}

    extractor = _IMPORT_EXTRACTORS.get(ext)
    if extractor:
        imports = extractor(content)
    else:
        return {"file": str(path), "imports": [], "note": "import extraction not supported"}

    return {"file": str(path), "imports": imports}


# ── Import resolution ─────────────────────────────────────────────────────────

class ProjectIndex:
    """Index of all source files in a project, for resolving imports to files."""

    def __init__(self, root: str, files: list[str]):
        self.root = Path(root).resolve()
        self._by_module: dict[str, str] = {}       # "engine.routing" → abs path
        self._by_stem: dict[str, list[str]] = defaultdict(list)
        self._by_relpath: dict[str, str] = {}       # "engine/routing.py" → abs path
        self._by_slash_path: dict[str, str] = {}    # "engine/routing" → abs path (no ext)
        self._all_files: set[str] = set()

        # Go module name (from go.mod)
        self._go_module: str = ""
        go_mod = self.root / "go.mod"
        if go_mod.exists():
            try:
                for line in go_mod.read_text().splitlines():
                    if line.startswith("module "):
                        self._go_module = line.split(None, 1)[1].strip()
                        break
            except OSError:
                pass

        for f in files:
            abs_path = Path(f).resolve()
            self._all_files.add(str(abs_path))
            try:
                rel = abs_path.relative_to(self.root)
            except ValueError:
                continue

            rel_str = str(rel)
            self._by_relpath[rel_str] = str(abs_path)

            parts = list(rel.parts)
            if parts:
                stem = parts[-1]
                ext = ""
                for e in (".py", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte",
                          ".go", ".java", ".kt", ".kts", ".rs", ".cs", ".php",
                          ".c", ".h", ".cpp", ".cc", ".cxx", ".hpp",
                          ".rb", ".swift", ".scala", ".sh", ".bash"):
                    if stem.endswith(e):
                        ext = e
                        stem = stem[: -len(e)]
                        break
                parts_no_ext = list(parts)
                parts_no_ext[-1] = stem

                # Dot-separated module key: engine.routing
                module_key = ".".join(parts_no_ext)
                self._by_module[module_key] = str(abs_path)

                # Slash-separated path key: engine/routing
                slash_key = "/".join(parts_no_ext)
                self._by_slash_path[slash_key] = str(abs_path)

                # Python __init__ / TS index shortcuts
                if stem == "__init__":
                    pkg_key = ".".join(parts_no_ext[:-1])
                    if pkg_key:
                        self._by_module[pkg_key] = str(abs_path)
                if stem == "index":
                    dir_key_dot = ".".join(parts_no_ext[:-1])
                    dir_key_slash = "/".join(parts_no_ext[:-1])
                    if dir_key_dot:
                        self._by_module[dir_key_dot] = str(abs_path)
                    if dir_key_slash:
                        self._by_slash_path[dir_key_slash] = str(abs_path)

                # Rust mod.rs acts like __init__
                if stem == "mod" and ext == ".rs":
                    pkg_key = ".".join(parts_no_ext[:-1])
                    slash_pkg = "/".join(parts_no_ext[:-1])
                    if pkg_key:
                        self._by_module[pkg_key] = str(abs_path)
                    if slash_pkg:
                        self._by_slash_path[slash_pkg] = str(abs_path)

                # Bare stem for fuzzy fallback
                self._by_stem[stem].append(str(abs_path))

    # ── Python ────────────────────────────────────────────────────────────

    def resolve_python_import(self, module: str, from_file: str) -> Optional[str]:
        if module.startswith("."):
            dots = len(module) - len(module.lstrip("."))
            remainder = module[dots:]
            from_path = Path(from_file).resolve()
            base = from_path.parent
            for _ in range(dots - 1):
                base = base.parent
            if remainder:
                candidate = base / remainder.replace(".", "/") / "__init__.py"
                if candidate.exists():
                    return str(candidate)
                candidate = base / (remainder.replace(".", "/") + ".py")
                if candidate.exists():
                    return str(candidate)
            return None

        if module in self._by_module:
            return self._by_module[module]
        parts = module.split(".")
        for i in range(len(parts) - 1, 0, -1):
            prefix = ".".join(parts[:i])
            if prefix in self._by_module:
                return self._by_module[prefix]
        return None

    # ── TypeScript / JavaScript ───────────────────────────────────────────

    def resolve_ts_import(self, specifier: str, from_file: str) -> Optional[str]:
        if not specifier.startswith(".") and not specifier.startswith("/"):
            cleaned = re.sub(r'^[$@][^/]*/', '', specifier)
            if cleaned != specifier:
                module_key = cleaned.replace("/", ".")
                if module_key in self._by_module:
                    return self._by_module[module_key]
                if cleaned in self._by_slash_path:
                    return self._by_slash_path[cleaned]
            return None

        from_path = Path(from_file).resolve()
        base = from_path.parent
        if specifier.startswith("/"):
            base = self.root
            specifier = specifier[1:]

        target = (base / specifier).resolve()
        ts_exts = [".ts", ".tsx", ".js", ".jsx", ".mjs", ".svelte",
                   "/index.ts", "/index.js", "/index.tsx"]

        if str(target) in self._all_files:
            return str(target)
        for ext in ts_exts:
            candidate = str(target) + ext
            if candidate in self._all_files:
                return candidate
        return None

    # ── Go ────────────────────────────────────────────────────────────────

    def resolve_go_import(self, pkg: str, from_file: str) -> Optional[str]:
        """Resolve a Go import path to a directory (represented by any .go file in it)."""
        # Strip the module prefix to get a relative path within the project
        rel_path = ""
        if self._go_module and pkg.startswith(self._go_module):
            rel_path = pkg[len(self._go_module):].lstrip("/")
        elif not pkg.startswith(".") and "/" not in pkg:
            # Standard library package (fmt, os, etc.) — skip
            return None
        else:
            # Try as a relative path within the project
            rel_path = pkg

        if not rel_path:
            # Module root — look for .go files in root
            rel_path = "."

        # Find any .go file in the target directory
        target_dir = self.root / rel_path
        if target_dir.is_dir():
            for f in sorted(self._all_files):
                fp = Path(f)
                if fp.suffix == ".go" and fp.parent == target_dir:
                    return f
        # Also check the slash path index
        if rel_path in self._by_slash_path:
            return self._by_slash_path[rel_path]
        return None

    # ── Java ──────────────────────────────────────────────────────────────

    def resolve_java_import(self, module: str, from_file: str) -> Optional[str]:
        """Resolve a Java import (com.foo.Bar) to a .java file."""
        # Strip wildcard
        module_clean = module.rstrip(".*")

        # Try direct module key match: com.foo.Bar → com.foo.Bar
        if module_clean in self._by_module:
            return self._by_module[module_clean]

        # Java convention: com.foo.Bar → com/foo/Bar.java
        # But projects often have src/main/java/ prefix
        slash_path = module_clean.replace(".", "/")
        if slash_path in self._by_slash_path:
            return self._by_slash_path[slash_path]

        # Try with common source roots stripped from index
        for prefix in ("src/main/java/", "src/", "java/", "app/src/main/java/"):
            candidate = prefix + slash_path
            if candidate in self._by_slash_path:
                return self._by_slash_path[candidate]

        # Fuzzy: try matching the last component (class name) against stems
        class_name = module_clean.rsplit(".", 1)[-1]
        candidates = self._by_stem.get(class_name, [])
        if len(candidates) == 1:
            return candidates[0]

        return None

    # ── Kotlin ────────────────────────────────────────────────────────────

    def resolve_kotlin_import(self, module: str, from_file: str) -> Optional[str]:
        """Resolve a Kotlin import. Same conventions as Java plus .kt files."""
        module_clean = module.rstrip(".*")

        if module_clean in self._by_module:
            return self._by_module[module_clean]

        slash_path = module_clean.replace(".", "/")
        if slash_path in self._by_slash_path:
            return self._by_slash_path[slash_path]

        for prefix in ("src/main/kotlin/", "src/main/java/", "src/", "app/src/main/kotlin/"):
            candidate = prefix + slash_path
            if candidate in self._by_slash_path:
                return self._by_slash_path[candidate]

        class_name = module_clean.rsplit(".", 1)[-1]
        candidates = self._by_stem.get(class_name, [])
        if len(candidates) == 1:
            return candidates[0]

        return None

    # ── Rust ──────────────────────────────────────────────────────────────

    def resolve_rust_import(self, module: str, kind: str, from_file: str) -> Optional[str]:
        """Resolve a Rust use/mod statement to a .rs file."""
        from_path = Path(from_file).resolve()

        if kind == "mod":
            # mod foo; → look for foo.rs or foo/mod.rs next to current file
            parent = from_path.parent
            # If current file is main.rs or lib.rs, look in src/
            candidate = parent / f"{module}.rs"
            if str(candidate) in self._all_files:
                return str(candidate)
            candidate = parent / module / "mod.rs"
            if str(candidate) in self._all_files:
                return str(candidate)
            return None

        # use statement: strip the tree part ({...}), just resolve the path prefix
        # use crate::foo::bar → src/foo/bar.rs or src/foo/bar/mod.rs
        path = module.split("::{")[0]  # strip glob/group
        path = path.rstrip("::*")     # strip wildcard
        segments = path.split("::")

        if not segments:
            return None

        # Determine base
        if segments[0] == "crate":
            segments = segments[1:]
            # Find src/ directory
            base = self.root / "src"
            if not base.is_dir():
                base = self.root
        elif segments[0] == "super":
            base = from_path.parent.parent
            segments = segments[1:]
        elif segments[0] == "self":
            base = from_path.parent
            segments = segments[1:]
        else:
            # External crate — can't resolve
            return None

        if not segments:
            return None

        rel = "/".join(segments)

        # Try: base/rel.rs
        candidate = base / (rel + ".rs")
        if str(candidate) in self._all_files:
            return str(candidate)
        # Try: base/rel/mod.rs
        candidate = base / rel / "mod.rs"
        if str(candidate) in self._all_files:
            return str(candidate)

        # Try progressively shorter prefixes (use crate::foo::bar::Baz → foo/bar.rs)
        for i in range(len(segments) - 1, 0, -1):
            rel = "/".join(segments[:i])
            candidate = base / (rel + ".rs")
            if str(candidate) in self._all_files:
                return str(candidate)
            candidate = base / rel / "mod.rs"
            if str(candidate) in self._all_files:
                return str(candidate)

        return None

    # ── C# ────────────────────────────────────────────────────────────────

    def resolve_csharp_import(self, namespace: str, from_file: str) -> Optional[str]:
        """Resolve a C# using directive to a .cs file."""
        # C# namespaces don't strictly map to file paths, but conventionally
        # MyApp.Models.User → Models/User.cs or MyApp/Models/User.cs

        if namespace in self._by_module:
            return self._by_module[namespace]

        slash_path = namespace.replace(".", "/")
        if slash_path in self._by_slash_path:
            return self._by_slash_path[slash_path]

        # Try stripping common root namespace prefixes
        parts = namespace.split(".")
        for start in range(1, len(parts)):
            sub = ".".join(parts[start:])
            if sub in self._by_module:
                return self._by_module[sub]
            sub_slash = "/".join(parts[start:])
            if sub_slash in self._by_slash_path:
                return self._by_slash_path[sub_slash]

        # Fuzzy: last component is usually the class name
        class_name = parts[-1]
        candidates = self._by_stem.get(class_name, [])
        if len(candidates) == 1:
            return candidates[0]

        return None

    # ── PHP ───────────────────────────────────────────────────────────────

    def resolve_php_import(self, module: str, kind: str, from_file: str) -> Optional[str]:
        """Resolve a PHP use/require statement to a .php file."""
        if kind == "require":
            # Direct file path — resolve relative to the importing file
            from_path = Path(from_file).resolve()
            candidate = (from_path.parent / module).resolve()
            if str(candidate) in self._all_files:
                return str(candidate)
            # Also try relative to project root
            candidate = (self.root / module).resolve()
            if str(candidate) in self._all_files:
                return str(candidate)
            return None

        # PSR-4 style: App\Models\User → app/Models/User.php or src/Models/User.php
        slash_path = module.replace("\\", "/")

        # Direct slash path match
        if slash_path in self._by_slash_path:
            return self._by_slash_path[slash_path]

        # Try with common PSR-4 prefix mappings
        parts = slash_path.split("/")
        # Strip first namespace component and try common source dirs
        if len(parts) > 1:
            remainder = "/".join(parts[1:])
            for prefix in ("src/", "app/", "lib/", ""):
                candidate = prefix + remainder
                if candidate in self._by_slash_path:
                    return self._by_slash_path[candidate]

        # Full module path with dots
        module_dot = module.replace("\\", ".")
        if module_dot in self._by_module:
            return self._by_module[module_dot]

        # Fuzzy: class name
        class_name = parts[-1]
        candidates = self._by_stem.get(class_name, [])
        php_candidates = [c for c in candidates if c.endswith(".php")]
        if len(php_candidates) == 1:
            return php_candidates[0]

        return None


# ── Resolver dispatch ─────────────────────────────────────────────────────────

_RESOLVER_EXTS: dict[str, str] = {}
for _ext in (".py",):
    _RESOLVER_EXTS[_ext] = "python"
for _ext in (".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"):
    _RESOLVER_EXTS[_ext] = "ts"
for _ext in (".go",):
    _RESOLVER_EXTS[_ext] = "go"
for _ext in (".java",):
    _RESOLVER_EXTS[_ext] = "java"
for _ext in (".kt", ".kts"):
    _RESOLVER_EXTS[_ext] = "kotlin"
for _ext in (".rs",):
    _RESOLVER_EXTS[_ext] = "rust"
for _ext in (".cs",):
    _RESOLVER_EXTS[_ext] = "csharp"
for _ext in (".php",):
    _RESOLVER_EXTS[_ext] = "php"


def _resolve_import(
    index: ProjectIndex, ext: str, module: str, kind: str, from_file: str,
) -> Optional[str]:
    """Dispatch import resolution to the right language resolver."""
    lang = _RESOLVER_EXTS.get(ext)
    if lang == "python":
        return index.resolve_python_import(module, from_file)
    if lang == "ts":
        return index.resolve_ts_import(module, from_file)
    if lang == "go":
        return index.resolve_go_import(module, from_file)
    if lang == "java":
        return index.resolve_java_import(module, from_file)
    if lang == "kotlin":
        return index.resolve_kotlin_import(module, from_file)
    if lang == "rust":
        return index.resolve_rust_import(module, kind, from_file)
    if lang == "csharp":
        return index.resolve_csharp_import(module, from_file)
    if lang == "php":
        return index.resolve_php_import(module, kind, from_file)
    return None


# ── Dependency graph ──────────────────────────────────────────────────────────

class DepGraph:
    """Directed dependency graph with query methods."""

    def __init__(self, root: str):
        self.root = Path(root).resolve()
        self.edges: dict[str, set[str]] = defaultdict(set)
        self.reverse: dict[str, set[str]] = defaultdict(set)
        self.unresolved: dict[str, list[str]] = defaultdict(list)

    def add_edge(self, from_file: str, to_file: str) -> None:
        self.edges[from_file].add(to_file)
        self.reverse[to_file].add(from_file)

    def add_unresolved(self, from_file: str, specifier: str) -> None:
        self.unresolved[from_file].append(specifier)

    def _rel(self, path: str) -> str:
        try:
            return str(Path(path).relative_to(self.root))
        except ValueError:
            return path

    def deps(self, file_path: str, transitive: bool = False) -> list[str]:
        target = str(Path(file_path).resolve())
        if not transitive:
            return sorted(self.edges.get(target, set()))
        visited: set[str] = set()
        queue = deque([target])
        while queue:
            current = queue.popleft()
            for dep in self.edges.get(current, set()):
                if dep not in visited and dep != target:
                    visited.add(dep)
                    queue.append(dep)
        return sorted(visited)

    def dependents(self, file_path: str, transitive: bool = False) -> list[str]:
        target = str(Path(file_path).resolve())
        if not transitive:
            return sorted(self.reverse.get(target, set()))
        visited: set[str] = set()
        queue = deque([target])
        while queue:
            current = queue.popleft()
            for dep in self.reverse.get(current, set()):
                if dep not in visited and dep != target:
                    visited.add(dep)
                    queue.append(dep)
        return sorted(visited)

    def impact(self, file_path: str) -> dict:
        target = str(Path(file_path).resolve())
        direct = self.reverse.get(target, set())
        transitive = set(self.dependents(file_path, transitive=True))
        indirect = transitive - direct
        return {
            "file": self._rel(target),
            "direct_dependents": len(direct),
            "transitive_dependents": len(transitive),
            "files": {
                "direct": sorted(self._rel(f) for f in direct),
                "indirect": sorted(self._rel(f) for f in indirect),
            },
        }

    def summary(self) -> dict:
        all_files = set(self.edges.keys()) | set(self.reverse.keys())
        hot_spots = sorted(
            ((f, len(deps)) for f, deps in self.reverse.items()),
            key=lambda x: -x[1],
        )[:15]
        heavy = sorted(
            ((f, len(deps)) for f, deps in self.edges.items()),
            key=lambda x: -x[1],
        )[:10]
        circular: list[tuple[str, str]] = []
        for a, a_deps in self.edges.items():
            for b in a_deps:
                if a in self.edges.get(b, set()):
                    pair = tuple(sorted([self._rel(a), self._rel(b)]))
                    if pair not in circular:
                        circular.append(pair)

        total_unresolved = sum(len(v) for v in self.unresolved.values())
        return {
            "total_files": len(all_files),
            "total_edges": sum(len(v) for v in self.edges.values()),
            "unresolved_imports": total_unresolved,
            "hot_spots": [(self._rel(f), n) for f, n in hot_spots],
            "heaviest_importers": [(self._rel(f), n) for f, n in heavy],
            "circular_pairs": circular[:20],
        }


def _read_and_extract(f: str) -> tuple[str, str, dict]:
    """Read a file and extract its imports in one pass."""
    abs_f = str(Path(f).resolve())
    ext = Path(f).suffix.lower()
    try:
        content = Path(f).read_text(encoding="utf-8", errors="replace")
    except OSError:
        return abs_f, ext, {"imports": []}
    result = extract_imports(f, content=content)
    return abs_f, ext, result


def build_graph(root: str, files: list[str]) -> DepGraph:
    """Build a dependency graph from a list of source files."""
    from concurrent.futures import ThreadPoolExecutor

    index = ProjectIndex(root, files)
    graph = DepGraph(root)

    # Read + extract imports in parallel (I/O bound)
    with ThreadPoolExecutor() as pool:
        results = list(pool.map(_read_and_extract, files))

    for abs_f, ext, result in results:
        for imp in result.get("imports", []):
            module = imp.get("module", "")
            if not module:
                continue

            kind = imp.get("kind", "")
            resolved = _resolve_import(index, ext, module, kind, abs_f)

            if resolved:
                graph.add_edge(abs_f, resolved)
            else:
                graph.add_unresolved(abs_f, module)

    return graph


# ── Formatting ────────────────────────────────────────────────────────────────

def format_imports_text(result: dict) -> str:
    lines = [f"### `{result['file']}`\n"]
    if result.get("error"):
        lines.append(f"  error: {result['error']}")
        return "\n".join(lines)
    if not result["imports"]:
        lines.append("  (no imports)")
        return "\n".join(lines)
    for imp in result["imports"]:
        module = imp["module"]
        kind = imp.get("kind", "")
        line = imp.get("line", "?")
        names = imp.get("names", [])
        if names and kind == "from":
            names_str = ", ".join(names[:5])
            if len(names) > 5:
                names_str += ", ..."
            lines.append(f"  from {module} import {names_str}  # line {line}")
        else:
            lines.append(f"  {kind} {module}  # line {line}")
    return "\n".join(lines)


def format_deps_text(file_path: str, deps: list[str], root: str, label: str = "depends on") -> str:
    root_path = Path(root).resolve()
    def _rel(p: str) -> str:
        try:
            return str(Path(p).relative_to(root_path))
        except ValueError:
            return p

    rel_file = _rel(str(Path(file_path).resolve()))
    if not deps:
        return f"### `{rel_file}` — {label}: (none)\n"
    lines = [f"### `{rel_file}` — {label}: {len(deps)} files\n"]
    for d in deps:
        lines.append(f"  {_rel(d)}")
    return "\n".join(lines)


def format_impact_text(impact: dict) -> str:
    lines = [
        f"### `{impact['file']}` — impact analysis\n",
        f"  Direct dependents:     {impact['direct_dependents']}",
        f"  Transitive dependents: {impact['transitive_dependents']}",
    ]
    if impact["files"]["direct"]:
        lines.append("\n  Direct:")
        for f in impact["files"]["direct"]:
            lines.append(f"    {f}")
    if impact["files"]["indirect"]:
        lines.append("\n  Indirect (transitive):")
        for f in impact["files"]["indirect"]:
            lines.append(f"    {f}")
    return "\n".join(lines)


def format_summary_text(summary: dict) -> str:
    lines = [
        "Project dependency graph\n",
        f"  Files:              {summary['total_files']}",
        f"  Import edges:       {summary['total_edges']}",
        f"  Unresolved imports: {summary['unresolved_imports']}",
    ]
    if summary["hot_spots"]:
        lines.append("\n  Most depended-on files:")
        for f, n in summary["hot_spots"]:
            lines.append(f"    {f}  ({n} dependents)")
    if summary["heaviest_importers"]:
        lines.append("\n  Heaviest importers:")
        for f, n in summary["heaviest_importers"]:
            lines.append(f"    {f}  ({n} imports)")
    if summary["circular_pairs"]:
        lines.append(f"\n  Circular dependencies ({len(summary['circular_pairs'])}):")
        for a, b in summary["circular_pairs"]:
            lines.append(f"    {a} <-> {b}")
    return "\n".join(lines)
