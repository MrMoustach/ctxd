package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// InstallClaude registers ctxd with Claude Code via the CLI if available,
// otherwise writes a project-level .mcp.json fallback.
func InstallClaude(binPath string, w io.Writer) error {
	if _, err := exec.LookPath("claude"); err == nil {
		return installViaCLI(binPath, w)
	}
	fmt.Fprintln(w, "claude CLI not found; writing .mcp.json fallback")
	return installMCPJSON(binPath, w)
}

func installViaCLI(binPath string, w io.Writer) error {
	cfg := map[string]any{
		"type":    "stdio",
		"command": binPath,
		"args":    []string{"serve", "--mcp"},
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	cmd := exec.Command("claude", "mcp", "add-json", "ctxd", string(cfgJSON))
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "already exists") {
			fmt.Fprintf(w, "ctxd already registered with Claude CLI\n")
			return nil
		}
		return fmt.Errorf("claude mcp add-json failed: %w\n%s", err, outStr)
	}
	fmt.Fprintf(w, "registered ctxd with Claude CLI\n")
	return nil
}

func installMCPJSON(binPath string, w io.Writer) error {
	overlay := map[string]any{
		"mcpServers": map[string]any{
			"ctxd": map[string]any{
				"type":    "stdio",
				"command": binPath,
				"args":    []string{"serve", "--mcp"},
			},
		},
	}
	if err := mergeJSON(".mcp.json", overlay); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote ctxd to .mcp.json\n")
	return nil
}
