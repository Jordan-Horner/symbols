# symbols

A fast, polyglot source code intelligence CLI. Extract symbols, parse imports, trace dependencies, and analyze impact — all from the command line.

No language server required. No build step for your projects. Just point it at your code.

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![MCPAmpel](https://img.shields.io/endpoint?url=https://mcpampel.com/badge/Jordan-Horner/symbols.json)](https://mcpampel.com/repo/Jordan-Horner/symbols)

<a href="https://glama.ai/mcp/servers/Jordan-Horner/symbols">
  <img width="380" height="200" src="https://glama.ai/mcp/servers/Jordan-Horner/symbols/badge" alt="symbols MCP server" />
</a>

## Table of contents

- [Why this exists](#why-this-exists)
- [How it saves context](#how-it-saves-context)
- [What it does](#what-it-does)
- [Install](#install)
- [Language support](#language-support)
- [Usage](#usage)
  - [Symbol extraction](#symbol-extraction)
  - [Import parsing](#import-parsing)
  - [Dependency queries](#dependency-queries)
  - [Impact analysis](#impact-analysis)
  - [Project graph summary](#project-graph-summary)
  - [JSON output](#json-output)
  - [Shorthand](#shorthand)
  - [Symbol search](#symbol-search)
- [MCP server](#mcp-server)
- [How it works](#how-it-works)
- [Project root detection](#project-root-detection)
- [Limitations](#limitations)
- [License](#license)

## Why this exists

`symbols` is for the moments when you need to understand a codebase quickly without opening 30 files first.

Common pain points it targets:

- You are about to change a file and need to know blast radius immediately.
- You are onboarding to an unfamiliar repo and need a map, not a scavenger hunt.
- You are reviewing a PR and want concrete dependency and ownership signals.
- You are using AI coding tools and need reliable, structured project context on demand.

Instead of manually reconstructing context from editor tabs, grep output, and memory, `symbols` gives you the structural view in one step.

## How it saves context

`symbols` saves context in two practical ways:

1. It externalizes code structure into fast, repeatable queries (`list`, `deps`, `dependents`, `impact`, `graph`, `search`) so you do not have to rebuild mental maps every session.
2. It exposes the same model through MCP (`syms mcp`) so agents and tools can fetch fresh project facts directly, rather than relying on stale chat history or guessed file relationships.

Net effect:

- less re-reading
- fewer "what will this break?" surprises
- faster onboarding and safer refactors
- more useful AI assistance because context is retrieved, not improvised
- lower cost from fewer exploratory engineering cycles and reduced AI token spend on repo re-discovery

## What it does

```
syms list server.py           # functions, classes, constants, variables
syms imports server.py        # parsed import statements
syms deps server.py           # files this file imports from
syms dependents server.py     # files that import this file
syms impact server.py         # full impact analysis (direct + transitive)
syms graph .                  # project-wide dependency summary
syms search User              # find symbols by name across a project
syms mcp                      # run as MCP server for AI tools
```

## Install

### Option 1: Build from source

```sh
git clone https://github.com/Jordan-Horner/symbols.git
cd symbols
go build -o syms .
sudo mv syms /usr/local/bin/
```

**Requirements:** Go 1.26+

### Option 2: Direct installation (Linux/macOS)

```sh
# Install directly to /usr/local/bin
curl -L https://github.com/Jordan-Horner/symbols/releases/latest/download/syms-$(uname -s)-$(uname -m) -o /usr/local/bin/syms
chmod +x /usr/local/bin/syms
```

### Option 3: Homebrew (macOS)

```sh
brew tap Jordan-Horner/tap
brew install syms
```

### Verify installation

```sh
syms --version
```

## Language support

Symbol extraction uses tree-sitter for full AST parsing (function signatures with parameters, classes, types, constants). Import parsing and dependency resolution use regex.

| Language | Symbols | Import parsing | Dependency resolution |
|---|---|---|---|
| Python | tree-sitter (functions, classes, constants, variables) | regex | Relative + absolute imports |
| TypeScript | tree-sitter | regex | `tsconfig.json` path aliases, relative paths, `index.ts` |
| JavaScript | tree-sitter | regex | Same as TypeScript (also reads `jsconfig.json`) |
| Svelte | tree-sitter (script block) | regex | Same as TypeScript |
| Go | tree-sitter | regex | `go.mod` module prefix, package directories |
| Java | tree-sitter | regex | Dot-to-slash, `src/main/java` prefix |
| Kotlin | tree-sitter | regex | Same as Java + `.kt` |
| Rust | tree-sitter | regex | `crate`/`self`/`super`, `mod.rs` |
| C# | tree-sitter | regex | Namespace-to-path, class name fallback |
| PHP | tree-sitter | regex | PSR-4 conventions, `require`/`include` |
| C/C++ | tree-sitter | — | — |
| Ruby | tree-sitter | — | — |
| Scala | tree-sitter | — | — |
| Bash | tree-sitter | — | — |

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

# Pretty JSON output (human-readable)
syms list --json --pretty app.py

# Optional: include precise symbol ranges
syms list --json --ranges app.py

# Count symbols per file
syms list --count src/

# Filter by symbol kind (repeatable or comma-separated)
syms list --filter class src/
syms list --filter class,function src/
syms list --filter class --filter function src/
```

**Output:**

```
### `app.py` — 245 lines

  constant VERSION  # line 1
  constant API_URL  # line 3
  variable app  # line 5
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

# Optional: pretty-print JSON for humans
syms graph --json --pretty .

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

### Symbol search

```sh
# Find symbols by name (fuzzy: exact > prefix > contains)
syms search User

# JSON output
syms search --json handle

# Search in a specific project
syms search --root /path/to/project Config

# Search only specific symbol kinds
syms search --filter class User

# Optional: include precise symbol ranges in search results
syms search --json --ranges User
```

**Output:**

```
Found 3 symbols matching "User":

  class User  models.py:1
  class UserProfile  models.py:5
  function get_user(id)  api/handlers.py:12
```

## MCP server

Run `syms` as an MCP server for AI tool integration (e.g. Claude Code):

```sh
syms mcp
```

Exposes all functionality as MCP tools over stdio (JSON-RPC 2.0):

| Tool | Description |
|---|---|
| `syms_list` | Extract symbols from files |
| `syms_imports` | Parse import statements |
| `syms_deps` | File dependencies |
| `syms_dependents` | Reverse dependencies |
| `syms_impact` | Impact analysis |
| `syms_search` | Search symbols by name |
| `syms_graph` | Project dependency graph |

`syms_list` and `syms_search` accept optional `kinds: string[]` arguments to filter symbol kinds.
`syms_list` and `syms_search` also accept optional `include_ranges: boolean` for start/end line+column metadata.
Tool results are returned in `structuredContent` (not JSON text blobs in `content[].text`).

### Claude Code setup

After installing `syms`, configure it as an MCP server:

**Project-level** (recommended for teams):

Create `.mcp.json` in your project root:

```json
{
  "mcpServers": {
    "symbols": {
      "command": "syms",
      "args": ["mcp"]
    }
  }
}
```

Commit this file so your team gets the symbols server automatically.

**Global (all projects)**:

Create or edit `~/.mcp.json`:

```json
{
  "mcpServers": {
    "symbols": {
      "command": "syms",
      "args": ["mcp"]
    }
  }
}
```

After configuration:
1. Restart Claude Code
2. When prompted, approve the `symbols` MCP server
3. Claude Code will now have access to code intelligence tools in all your projects

## How it works

**Symbol extraction** uses tree-sitter for full AST parsing. Each language has a compiled grammar (linked statically into the binary) that produces a syntax tree. The tool walks the tree to extract top-level declarations with names, kinds, line numbers, and function parameters. For Python, module-level assignments are also extracted as constants (UPPER_CASE) or variables.

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

- **Convention-based resolution** — dependency resolution uses file path conventions, not compiler/build system integration. TypeScript/JavaScript `paths` from `tsconfig.json`/`jsconfig.json` are supported (including `extends`), but webpack/vite aliases defined outside tsconfig are not.
- **File-level granularity** — dependencies are traced at the file level (import graph), not at the function or symbol level. There is no call graph.
- **C/C++ includes** — `#include` parsing and header resolution are not yet implemented. Symbol extraction works, but dependency tracing does not.
- **Ruby/Scala/Bash** — symbol extraction works via tree-sitter, but import parsing and dependency resolution are not implemented.
- **Dynamic imports** — Python's `importlib.import_module()`, JavaScript's computed `require()`, and similar dynamic patterns are not detected.
- **Monorepo boundaries** — the tool resolves imports within a single project root. Cross-package imports in monorepos may not resolve correctly.

## License

[MIT](LICENSE)
