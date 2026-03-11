# symbols

A fast, polyglot source code intelligence CLI. Extract symbols, parse imports, trace dependencies, and analyze impact — all from the command line.

No language server required. No build step for your projects. Just point it at your code.

## What it does

```
syms list server.py           # functions, classes, types with line numbers
syms imports server.py        # parsed import statements
syms deps server.py           # files this file imports from
syms dependents server.py     # files that import this file
syms impact server.py         # full impact analysis (direct + transitive)
syms graph .                  # project-wide dependency summary
```

## Install

```sh
git clone https://github.com/Jordan-Horner/symbols.git
cd symbols
go build -o syms .
sudo mv syms /usr/local/bin/
```

**Requirements:** Go 1.26+

## Language support

Symbol extraction uses tree-sitter for full AST parsing (function signatures with parameters, classes, types). Import parsing and dependency resolution use regex.

| Language | Symbols | Import parsing | Dependency resolution |
|---|---|---|---|
| Python | tree-sitter | regex | Relative + absolute imports |
| TypeScript | tree-sitter | regex | Relative paths, `$lib`/`@` aliases, `index.ts` |
| JavaScript | tree-sitter | regex | Same as TypeScript |
| Svelte | tree-sitter | regex | Same as TypeScript |
| Go | tree-sitter | regex | `go.mod` module prefix, package directories |
| Java | tree-sitter | regex | Dot-to-slash, `src/main/java` prefix |
| Kotlin | tree-sitter | regex | Same as Java + `.kt` |
| Rust | tree-sitter | regex | `crate`/`self`/`super`, `mod.rs` |
| C# | tree-sitter | regex | Namespace-to-path, class name fallback |
| PHP | tree-sitter | regex | PSR-4 conventions, `require`/`include` |
| C/C++ | tree-sitter | - | - |
| Ruby | tree-sitter | - | - |
| Scala | tree-sitter | - | - |
| Bash | tree-sitter | - | - |

## Usage

### Symbol extraction

```sh
# Single file
syms list app.py

# Multiple files
syms list src/main.go src/handlers.go

# Recursive directory scan
syms list -r src/

# JSON output (for piping to other tools)
syms list --json app.py
```

**Output:**

```
### `app.py` — 245 lines

  class Application  # line 12
  def __init__(self, config)  # line 15
  async def start(self)  # line 34
  def shutdown(self)  # line 78
```

### Import parsing

```sh
syms imports server.py
```

**Output:**

```
### `server.py`

  from flask import Flask, jsonify  # line 1
  from .models import User, Post  # line 2
  import os  # line 3
```

### Dependency queries

```sh
# Direct dependencies
syms deps src/handlers.go

# Transitive (everything it depends on, recursively)
syms deps -t src/handlers.go

# Who imports this file?
syms dependents src/models.py

# Transitive dependents
syms dependents -t src/models.py
```

### Impact analysis

```sh
syms impact src/core/utils.py
```

**Output:**

```
### `src/core/utils.py` — impact analysis

  Direct dependents:     8
  Transitive dependents: 23

  Direct:
    src/api/handlers.py
    src/core/auth.py
    src/core/db.py
    ...

  Indirect (transitive):
    src/api/routes.py
    src/main.py
    tests/test_auth.py
    ...
```

### Project graph summary

```sh
syms graph .
```

**Output:**

```
Project dependency graph

  Files:              187
  Import edges:       562
  Unresolved imports: 43

  Most depended-on files:
    src/utils.py  (36 dependents)
    src/config.py  (33 dependents)
    src/models.py  (23 dependents)

  Heaviest importers:
    src/app.py  (28 imports)
    src/main.py  (24 imports)

  Circular dependencies (1):
    src/config.py <-> src/runner.py
```

### JSON output

All commands support `--json` for machine-readable output:

```sh
syms impact --json src/utils.py | jq '.direct_dependents'
syms graph --json . | jq '.hot_spots[:5]'

# Full edge map (file → its dependencies)
syms graph --json . | jq '.edges'

# What does a specific file depend on?
syms graph --json . | jq '.edges["src/app.py"]'
```

### Shorthand

The `list` subcommand is the default — you can omit it:

```sh
# These are equivalent:
syms list app.py
syms app.py

# Flags work too:
syms -r src/ --json
```

## How it works

**Symbol extraction** uses tree-sitter for full AST parsing. Each language has a compiled grammar (linked statically into the binary) that produces a syntax tree. The tool walks the tree to extract top-level declarations with names, kinds, line numbers, and function parameters.

**Import parsing** uses regex patterns tuned to each language's import syntax. This is fast and reliable for standard import forms without needing AST parsing.

**Dependency resolution** maps import specifiers to actual files on disk using language-specific conventions:
- Python: module dot-path to file path, relative import resolution
- Go: `go.mod` module name stripping, package-to-directory mapping
- Java/Kotlin: dot-to-slash convention, standard source root prefixes (`src/main/java/`)
- Rust: `crate`/`self`/`super` path resolution, `mod.rs` convention
- C#: namespace-to-path with progressive prefix stripping
- PHP: PSR-4 backslash-to-slash mapping, `require`/`include` path resolution

**Directory scanning** uses early pruning of `.git`, `node_modules`, `dist`, `build`, `vendor`, `target`, and other common non-source directories.

## Project root detection

For `deps`, `dependents`, `impact`, and `graph`, the tool auto-detects the project root by walking up the directory tree looking for `.git`, `package.json`, or `pyproject.toml`. Override with `--root`:

```sh
syms deps src/app.py --root /path/to/project
```

## Limitations

- **Symbol extraction** only captures top-level declarations (not nested functions or local classes)
- **Import resolution** is convention-based, not compiler-backed — path aliases beyond `$lib`/`@` prefix in TS/JS aren't resolved
- **No call graph** — dependencies are file-level (import graph), not function-level
- **C/C++ headers** — `#include` parsing and resolution not yet implemented

## License

MIT
