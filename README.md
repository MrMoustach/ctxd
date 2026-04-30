# ctxd

`ctxd` is a portable local code-context engine for developers and AI coding agents. It indexes projects into SQLite FTS5, builds a structural code graph, and returns ranked snippets or graph-expanded markdown context packs.

It stays local: no LLM calls, no embeddings, no cloud APIs, and no background service requirement beyond optional MCP stdio.

## Install

macOS/Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/MrMoustach/ctxd/main/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/MrMoustach/ctxd/main/install.ps1 | iex
```

Install a specific version by setting `CTXD_VERSION`, for example:

```bash
curl -fsSL https://raw.githubusercontent.com/MrMoustach/ctxd/main/install.sh | CTXD_VERSION=v0.1.0 sh
```

```powershell
$env:CTXD_VERSION = "v0.1.0"; irm https://raw.githubusercontent.com/MrMoustach/ctxd/main/install.ps1 | iex
```

The installers download the matching GitHub Release archive, verify it against `checksums.txt`, and install `ctxd` into:

- macOS/Linux: `/usr/local/bin` when writable, otherwise `$HOME/.local/bin`
- Windows: `%LOCALAPPDATA%\ctxd\bin`

Override the install directory with `CTXD_INSTALL_DIR`.

Manual downloads are available from GitHub Releases:

```txt
https://github.com/MrMoustach/ctxd/releases
```

Build from source:

```bash
go build -o ctxd .
```

Set `CTX_DB=/path/to/ctx.db` to choose a database location. By default, `ctxd` stores data under the user config directory.

## Quick Start

```bash
ctxd init
ctxd add /path/to/project --name pms
ctxd index pms
ctxd graph build pms
ctxd context pms "implement checkout anomaly detection" --graph-depth 2 --max-tokens 12000
```

`ctxd index` keeps the FTS, file, chunk, symbol, and import indexes current. `ctxd graph build` rebuilds graph nodes and edges for the project without deleting FTS data.

## Commands

```bash
ctxd setup /path/to/project --name pms --agents claude,codex
ctxd init
ctxd add /path/to/project --name pms
ctxd projects
ctxd index pms
ctxd search pms "where is payment handled?" --limit 10
ctxd context pms "implement checkout anomaly detection" --graph-depth 2 --max-tokens 12000
ctxd serve --mcp
ctxd install all
ctxd doctor
```

Most human-facing commands support `--json` where useful.

## Graph

The graph layer adds:

- File and symbol nodes
- Import, define, call, route, use, inheritance, and dependency edges
- Laravel-aware route, controller, model, service, job, command, and test extraction
- Graph expansion for context retrieval
- Markdown, JSON, and self-contained HTML graph outputs

Supported source languages include PHP, JavaScript, TypeScript, and Go, with fallback symbol extraction for other indexed text languages.

```bash
ctxd graph build pms
ctxd graph stats pms
ctxd graph report pms
ctxd graph export pms --format json
ctxd graph export pms --format html
ctxd graph neighbors pms UserController
ctxd graph path pms "GET /users" UserController.index
```

Generated graph files are written under the project:

- `.ctxd/GRAPH_REPORT.md`
- `.ctxd/graph.json`
- `.ctxd/graph.html`

## Context Packs

`ctxd context` uses FTS seeds first, then graph data when available. Graph expansion is enabled by default if the project has graph nodes.

```bash
ctxd context pms "change reservation cancellation flow"
ctxd context pms "change reservation cancellation flow" --graph
ctxd context pms "change reservation cancellation flow" --graph-depth 2 --max-tokens 8000
```

Context output includes direct matches, graph-expanded related files, relevant symbols, call/import relationships, and snippets.

## MCP

Start the MCP stdio server:

```bash
ctxd serve --mcp
```

Exposed MCP tools:

- `ctxd_context`
- `ctxd_search`
- `ctxd_read_files`
- `ctxd_project_map`
- `ctxd_graph_neighbors`
- `ctxd_graph_path`
- `ctxd_graph_rebuild`
- `ctxd_graph_stats`
- `ctxd_graph_report`
- `reindex_project`

`reindex_project` refreshes the file/chunk index and rebuilds graph data by default. Pass `graph: false` for an FTS-only refresh. `ctxd_context` returns markdown plus matched paths, a token estimate, and a truncation flag. `ctxd_graph_neighbors` returns compact results by default; use `max_nodes`, `max_edges`, `types`, and `include_metadata` to tune output.

Compatibility aliases are still available for older configs: `list_projects`, `search_code`, `get_context`, and `read_files`.

## Agent Installers

The installer commands register the absolute path of the current `ctxd` binary with MCP config for each agent and update local instruction files.

```bash
ctxd install claude
ctxd install codex
ctxd install copilot
ctxd install antigravity
ctxd install all
```

Targets:

- Claude: uses `claude mcp add-json ctxd ...` when the Claude CLI is available, otherwise falls back to `.mcp.json`
- Codex: writes `.codex/config.toml`
- GitHub Copilot / VS Code: writes `.vscode/mcp.json`
- Antigravity: writes `.mcp.json`

Standard MCP shape:

```json
{
  "mcpServers": {
    "ctxd": {
      "type": "stdio",
      "command": "/absolute/path/to/ctxd",
      "args": ["serve", "--mcp"]
    }
  }
}
```

Codex project config shape:

```toml
[mcp_servers.ctxd]
command = "/absolute/path/to/ctxd"
args = ["serve", "--mcp"]
```

## Doctor

```bash
ctxd doctor
```

## Releasing

Releases are built by GoReleaser when a version tag is pushed:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The GitHub Actions workflow publishes macOS, Linux, and Windows archives plus `checksums.txt`.

Doctor checks the resolved binary path, `serve --mcp`, graph tables, project graph data, MCP config files, local instruction policies, global Claude/Codex instruction files, and Claude CLI availability.

## Ignored By Default

`.git`, `node_modules`, `vendor`, `dist`, `build`, `.next`, `.nuxt`, `.cache`, `coverage`, `storage/logs`, `.env`, lock files, minified JavaScript, maps, images, SVGs, PDFs, and common archives are skipped. Root `.gitignore` rules are also respected.
