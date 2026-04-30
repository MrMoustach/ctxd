# ctxd Specification

`ctx` is a portable local code-context engine for developers and AI coding agents. It indexes local projects, searches code without LLM calls, and exposes context through a CLI and MCP stdio server.

## V1 Scope

- Single Go CLI binary named `ctx`
- Commands: `init`, `add`, `projects`, `index`, `search`, `context`, `serve --mcp`
- SQLite storage with FTS5
- Project registration
- File scanning with `.gitignore` and default ignore rules
- Line-preserving chunking
- Lightweight symbol and import extraction
- Ranked FTS search
- Markdown context pack generation
- MCP tools: `list_projects`, `search_code`, `get_context`, `read_files`, `reindex_project`
- No LLM calls, Ollama, or required embeddings

## Safety

- Never index `.env`, build artifacts, dependencies, binary/media/archive files, PDFs, or lock files by default.
- Respect root `.gitignore`.
- Prevent path traversal when reading files through MCP or CLI internals.

The full user-provided product brief is represented by this repository's implementation, README, and tests.
