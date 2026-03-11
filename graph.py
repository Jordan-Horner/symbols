"""Import graph builder and dependency analyzer.

Parses import statements from Python and TS/JS/Svelte files, resolves them
to actual files on disk, and builds a directed dependency graph. Supports
dependency queries, reverse-dependency (dependents) queries, and transitive
impact analysis.
"""

from __future__ import annotations

import ast
import re
from collections import defaultdict, deque
from pathlib import Path
from typing import Optional

# ── Import extraction ─────────────────────────────────────────────────────────

# Python: use ast for reliable import parsing
def _extract_imports_python(content: str) -> list[dict]:
    """Extract imports from Python source. Returns list of import dicts."""
    try:
        tree = ast.parse(content)
    except SyntaxError:
        return []
    imports: list[dict] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                imports.append({
                    "module": alias.name,
                    "names": [alias.asname or alias.name],
                    "line": node.lineno,
                    "kind": "import",
                })
        elif isinstance(node, ast.ImportFrom):
            module = node.module or ""
            level = node.level or 0
            prefix = "." * level
            imports.append({
                "module": prefix + module,
                "names": [a.name for a in (node.names or [])],
                "line": node.lineno,
                "kind": "from",
            })
    return imports


# TS/JS/Svelte: regex-based import parsing
_TS_IMPORT_RE = re.compile(
    r'''(?:^|\n)\s*import\s+'''
    r'''(?:'''
    r'''(?:type\s+)?'''  # optional "type" keyword
    r'''(?:\{[^}]*\}|[A-Za-z_$][A-Za-z0-9_$]*|\*)'''  # named/default/star import
    r'''(?:\s*,\s*(?:\{[^}]*\}|\*))?'''  # optional second clause
    r'''(?:\s+from\s+)?'''
    r''')?'''
    r'''['"]([^'"]+)['"]''',  # module specifier
    re.MULTILINE,
)

_TS_REQUIRE_RE = re.compile(
    r'''require\s*\(\s*['"]([^'"]+)['"]\s*\)''',
)

_TS_DYNAMIC_IMPORT_RE = re.compile(
    r'''import\s*\(\s*['"]([^'"]+)['"]\s*\)''',
)

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
                # Calculate line number
                line = content[:m.start()].count("\n") + 1
                imports.append({
                    "module": module,
                    "kind": kind,
                    "line": line,
                })
    return imports


def extract_imports(file_path: str) -> dict:
    """Extract imports from a single file."""
    path = Path(file_path)
    ext = path.suffix.lower()

    try:
        content = path.read_text(encoding="utf-8", errors="replace")
    except OSError as e:
        return {"file": str(path), "error": str(e), "imports": []}

    if ext == ".py":
        imports = _extract_imports_python(content)
    elif ext in (".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"):
        imports = _extract_imports_ts(content)
    else:
        return {"file": str(path), "imports": [], "note": "import extraction not supported"}

    return {"file": str(path), "imports": imports}


# ── Import resolution ─────────────────────────────────────────────────────────

class ProjectIndex:
    """Index of all source files in a project, for resolving imports to files."""

    def __init__(self, root: str, files: list[str]):
        self.root = Path(root).resolve()
        # Map module-style paths to actual file paths
        self._by_module: dict[str, str] = {}  # "engine.routing" → "/abs/path/engine/routing.py"
        self._by_stem: dict[str, list[str]] = defaultdict(list)  # "routing" → [paths...]
        self._by_relpath: dict[str, str] = {}  # "engine/routing.py" → "/abs/path/..."
        self._all_files: set[str] = set()

        for f in files:
            abs_path = Path(f).resolve()
            self._all_files.add(str(abs_path))
            try:
                rel = abs_path.relative_to(self.root)
            except ValueError:
                continue

            rel_str = str(rel)
            self._by_relpath[rel_str] = str(abs_path)

            # Build module-style key: engine/routing.py → engine.routing
            parts = list(rel.parts)
            if parts:
                stem = parts[-1]
                for ext in (".py", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"):
                    if stem.endswith(ext):
                        stem = stem[: -len(ext)]
                        break
                parts[-1] = stem
                module_key = ".".join(parts)
                self._by_module[module_key] = str(abs_path)
                # Also without __init__
                if stem == "__init__":
                    module_key = ".".join(parts[:-1])
                    if module_key:
                        self._by_module[module_key] = str(abs_path)
                # Index by bare stem for fuzzy fallback
                self._by_stem[stem].append(str(abs_path))

                # Also index without index suffix for TS
                if stem == "index":
                    dir_key = ".".join(parts[:-1])
                    if dir_key:
                        self._by_module[dir_key] = str(abs_path)

    def resolve_python_import(self, module: str, from_file: str) -> Optional[str]:
        """Resolve a Python import to an absolute file path."""
        # Handle relative imports
        if module.startswith("."):
            dots = len(module) - len(module.lstrip("."))
            remainder = module[dots:]
            from_path = Path(from_file).resolve()
            base = from_path.parent
            for _ in range(dots - 1):
                base = base.parent
            if remainder:
                # Try as package (directory with __init__.py)
                candidate = base / remainder.replace(".", "/") / "__init__.py"
                if candidate.exists():
                    return str(candidate)
                # Try as module
                candidate = base / (remainder.replace(".", "/") + ".py")
                if candidate.exists():
                    return str(candidate)
            return None

        # Absolute import — check our index
        if module in self._by_module:
            return self._by_module[module]

        # Try progressively shorter prefixes (import engine.routing.foo → engine.routing)
        parts = module.split(".")
        for i in range(len(parts) - 1, 0, -1):
            prefix = ".".join(parts[:i])
            if prefix in self._by_module:
                return self._by_module[prefix]

        return None

    def resolve_ts_import(self, specifier: str, from_file: str) -> Optional[str]:
        """Resolve a TS/JS import specifier to an absolute file path."""
        # Skip bare package imports (npm packages)
        if not specifier.startswith(".") and not specifier.startswith("/"):
            # Could be a path alias — try matching against index
            # e.g. "$lib/stores/board" or "@/components/Foo"
            cleaned = re.sub(r'^[$@][^/]*/', '', specifier)
            if cleaned != specifier:
                # Try to find in index
                module_key = cleaned.replace("/", ".")
                if module_key in self._by_module:
                    return self._by_module[module_key]
            return None

        # Relative import
        from_path = Path(from_file).resolve()
        base = from_path.parent

        if specifier.startswith("/"):
            base = self.root
            specifier = specifier[1:]

        target = (base / specifier).resolve()

        # Try exact path, then with extensions, then as directory/index
        ts_exts = [".ts", ".tsx", ".js", ".jsx", ".mjs", ".svelte", "/index.ts", "/index.js", "/index.tsx"]

        if str(target) in self._all_files:
            return str(target)

        for ext in ts_exts:
            candidate = str(target) + ext
            if candidate in self._all_files:
                return candidate

        return None


# ── Dependency graph ──────────────────────────────────────────────────────────

class DepGraph:
    """Directed dependency graph with query methods."""

    def __init__(self, root: str):
        self.root = Path(root).resolve()
        # file → set of files it imports
        self.edges: dict[str, set[str]] = defaultdict(set)
        # file → set of files that import it
        self.reverse: dict[str, set[str]] = defaultdict(set)
        # file → list of unresolved import specifiers
        self.unresolved: dict[str, list[str]] = defaultdict(list)

    def add_edge(self, from_file: str, to_file: str) -> None:
        self.edges[from_file].add(to_file)
        self.reverse[to_file].add(from_file)

    def add_unresolved(self, from_file: str, specifier: str) -> None:
        self.unresolved[from_file].append(specifier)

    def _rel(self, path: str) -> str:
        """Return path relative to project root."""
        try:
            return str(Path(path).relative_to(self.root))
        except ValueError:
            return path

    def deps(self, file_path: str, transitive: bool = False) -> list[str]:
        """Files that this file depends on (imports from)."""
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
        """Files that depend on this file (import it)."""
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
        """Impact analysis: what's affected if this file changes."""
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
        """Project-wide graph statistics."""
        all_files = set(self.edges.keys()) | set(self.reverse.keys())
        # Most-depended-on files
        hot_spots = sorted(
            ((f, len(deps)) for f, deps in self.reverse.items()),
            key=lambda x: -x[1],
        )[:15]
        # Most-importing files
        heavy = sorted(
            ((f, len(deps)) for f, deps in self.edges.items()),
            key=lambda x: -x[1],
        )[:10]
        # Circular dependency detection (simple: A→B and B→A)
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


def build_graph(root: str, files: list[str]) -> DepGraph:
    """Build a dependency graph from a list of source files."""
    index = ProjectIndex(root, files)
    graph = DepGraph(root)

    for f in files:
        abs_f = str(Path(f).resolve())
        result = extract_imports(f)
        ext = Path(f).suffix.lower()

        for imp in result.get("imports", []):
            module = imp.get("module", "")
            if not module:
                continue

            resolved: Optional[str] = None
            if ext == ".py":
                resolved = index.resolve_python_import(module, f)
            elif ext in (".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"):
                resolved = index.resolve_ts_import(module, f)

            if resolved:
                graph.add_edge(abs_f, resolved)
            else:
                graph.add_unresolved(abs_f, module)

    return graph


# ── Formatting ────────────────────────────────────────────────────────────────

def format_imports_text(result: dict) -> str:
    """Format import extraction result as text."""
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
    """Format a dependency list as text."""
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
    """Format impact analysis as text."""
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
    """Format graph summary as text."""
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
