package install

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/issam/ctxd/internal/config"

	_ "modernc.org/sqlite"
)

type status int

const (
	pass status = iota
	warn
	fail
)

func (s status) String() string {
	switch s {
	case pass:
		return "PASS"
	case warn:
		return "WARN"
	default:
		return "FAIL"
	}
}

type checkResult struct {
	label  string
	st     status
	detail string
}

// Doctor prints the health of all ctxd integrations to w.
func Doctor(binPath string, w io.Writer) error {
	results := []checkResult{
		checkBinary(binPath),
		checkMCPServe(binPath),
		checkGraphTables(),
		checkProjectGraphData(),
		checkCodexTOML(),
		checkVSCodeMCP(),
		checkMCPJSON(),
		checkInstructions("CLAUDE.md"),
		checkInstructions("AGENTS.md"),
		checkClaudeCLI(),
	}
	for _, agent := range []string{"claude", "codex"} {
		for _, path := range GlobalPaths(agent) {
			results = append(results, checkInstructionsLabeled(agent+" global", path))
		}
	}

	width := 0
	for _, r := range results {
		if len(r.label) > width {
			width = len(r.label)
		}
	}

	anyFail := false
	for _, r := range results {
		fmt.Fprintf(w, "%-6s  %-*s  %s\n", r.st, width, r.label, r.detail)
		if r.st == fail {
			anyFail = true
		}
	}
	if anyFail {
		return fmt.Errorf("one or more checks failed")
	}
	return nil
}

func checkBinary(binPath string) checkResult {
	resolved, err := filepath.EvalSymlinks(binPath)
	if err != nil {
		return checkResult{"binary path", fail, fmt.Sprintf("cannot resolve: %v", err)}
	}
	if _, err := os.Stat(resolved); err != nil {
		return checkResult{"binary path", fail, fmt.Sprintf("not found: %s", resolved)}
	}
	return checkResult{"binary path", pass, resolved}
}

func checkMCPServe(binPath string) checkResult {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "serve", "--mcp")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return checkResult{"serve --mcp", fail, "timed out"}
	}
	if err != nil {
		return checkResult{"serve --mcp", fail, strings.TrimSpace(string(out))}
	}
	return checkResult{"serve --mcp", pass, "starts over stdio"}
}

func checkGraphTables() checkResult {
	dbPath, err := config.DefaultDBPath()
	if err != nil {
		return checkResult{"graph tables", fail, err.Error()}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return checkResult{"graph tables", fail, err.Error()}
	}
	defer db.Close()
	for _, name := range []string{"graph_nodes", "graph_edges"} {
		var found string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&found)
		if err != nil {
			return checkResult{"graph tables", fail, fmt.Sprintf("%s missing (run: ctxd init)", name)}
		}
	}
	return checkResult{"graph tables", pass, "present"}
}

func checkProjectGraphData() checkResult {
	dbPath, err := config.DefaultDBPath()
	if err != nil {
		return checkResult{"project graph data", warn, err.Error()}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return checkResult{"project graph data", warn, err.Error()}
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM graph_nodes`).Scan(&count); err != nil {
		return checkResult{"project graph data", warn, "not available (run: ctxd graph build PROJECT)"}
	}
	if count == 0 {
		return checkResult{"project graph data", warn, "none found (run: ctxd graph build PROJECT)"}
	}
	return checkResult{"project graph data", pass, fmt.Sprintf("%d graph nodes", count)}
}

func checkCodexTOML() checkResult {
	const path = ".codex/config.toml"
	data, err := os.ReadFile(path)
	if err != nil {
		return checkResult{path, warn, "not found (run: ctxd install codex)"}
	}
	if !strings.Contains(string(data), "[mcp_servers.ctxd]") {
		return checkResult{path, fail, "ctxd not configured (run: ctxd install codex)"}
	}
	return checkResult{path, pass, "ctxd configured"}
}

func checkVSCodeMCP() checkResult {
	const path = ".vscode/mcp.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return checkResult{path, warn, "not found (run: ctxd install copilot)"}
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return checkResult{path, fail, fmt.Sprintf("invalid JSON: %v", err)}
	}
	if servers, ok := v["servers"].(map[string]any); ok {
		if _, ok := servers["ctxd"]; ok {
			return checkResult{path, pass, "ctxd configured"}
		}
	}
	return checkResult{path, fail, "ctxd not configured (run: ctxd install copilot)"}
}

func checkMCPJSON() checkResult {
	const path = ".mcp.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return checkResult{path, warn, "not found (run: ctxd install antigravity)"}
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return checkResult{path, fail, fmt.Sprintf("invalid JSON: %v", err)}
	}
	if servers, ok := v["mcpServers"].(map[string]any); ok {
		if _, ok := servers["ctxd"]; ok {
			return checkResult{path, pass, "ctxd configured"}
		}
	}
	return checkResult{path, fail, "ctxd not configured (run: ctxd install antigravity)"}
}

func checkInstructions(path string) checkResult {
	return checkInstructionsLabeled(path, path)
}

func checkInstructionsLabeled(label, path string) checkResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return checkResult{label, warn, fmt.Sprintf("not found: %s (run: ctxd install)", path)}
	}
	if !strings.Contains(string(data), policyMarker) {
		return checkResult{label, fail, fmt.Sprintf("ctxd policy missing in %s (run: ctxd install)", path)}
	}
	return checkResult{label, pass, "ctxd policy present"}
}

func checkClaudeCLI() checkResult {
	path, err := exec.LookPath("claude")
	if err != nil {
		return checkResult{"claude CLI", warn, "not found in PATH"}
	}
	return checkResult{"claude CLI", pass, path}
}
