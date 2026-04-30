package install

import (
	"fmt"
	"io"
)

// InstallCopilot writes ctxd to .vscode/mcp.json for GitHub Copilot / VS Code.
func InstallCopilot(binPath string, w io.Writer) error {
	overlay := map[string]any{
		"servers": map[string]any{
			"ctxd": map[string]any{
				"type":    "stdio",
				"command": binPath,
				"args":    []string{"serve", "--mcp"},
			},
		},
	}
	if err := mergeJSON(".vscode/mcp.json", overlay); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote ctxd to .vscode/mcp.json\n")
	return nil
}
