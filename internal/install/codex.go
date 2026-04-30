package install

import (
	"fmt"
	"io"
)

// InstallCodex writes [mcp_servers.ctxd] to .codex/config.toml.
func InstallCodex(binPath string, w io.Writer) error {
	if err := mergeCodexTOML(".codex/config.toml", binPath); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote ctxd to .codex/config.toml\n")
	return nil
}
