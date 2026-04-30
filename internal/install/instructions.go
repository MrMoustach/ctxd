package install

import (
	"os"
	"path/filepath"
	"strings"
)

const policyMarker = "## ctxd Context Policy"

const policySection = `
## ctxd Context Policy

Before answering implementation questions, modifying code, or searching manually through the repository, use the ` + "`ctxd_context`" + ` MCP tool first.

Use:
- ` + "`ctxd_context`" + ` for implementation tasks and graph-expanded context
- ` + "`ctxd_search`" + ` for finding files, symbols, services, routes, models, or logic
- ` + "`ctxd_graph_neighbors`" + ` to inspect related symbols/files
- ` + "`ctxd_graph_path`" + ` to understand how two parts of the system connect
- ` + "`ctxd_read_files`" + ` when exact file contents are needed

Do not rely only on visible editor context unless ctxd is unavailable.

If ` + "`ctxd_context`" + ` output contains a graph staleness warning, call ` + "`ctxd_graph_rebuild`" + ` followed by ` + "`reindex_project`" + ` before proceeding. After large merges, refactors, or file renames, always rebuild: ` + "`ctxd_graph_rebuild`" + ` then ` + "`reindex_project`" + `.
`

// UpdateInstructions appends the ctxd policy section to path if not already present.
func UpdateInstructions(path string) error {
	content := ""
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
	}
	if strings.Contains(content, policyMarker) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return atomicWrite(path, []byte(content+policySection))
}

// GlobalPaths returns the global instruction file paths for the given agent.
// Returns nil for agents with no known global instruction file.
func GlobalPaths(agent string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	switch agent {
	case "claude":
		return []string{filepath.Join(home, ".claude", "CLAUDE.md")}
	case "codex":
		return []string{filepath.Join(home, "AGENTS.md")}
	}
	return nil
}
