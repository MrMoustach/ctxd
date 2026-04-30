package install

import (
	"fmt"
	"io"
)

// InstallAntigravity writes ctxd to .mcp.json for Antigravity.
func InstallAntigravity(binPath string, w io.Writer) error {
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
