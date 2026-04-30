package install

import (
	"fmt"
	"io"
)

// Run dispatches to the named installer and updates instruction files afterward.
func Run(target, binPath string, w io.Writer) error {
	switch target {
	case "claude":
		if err := InstallClaude(binPath, w); err != nil {
			return err
		}
	case "codex":
		if err := InstallCodex(binPath, w); err != nil {
			return err
		}
	case "copilot":
		if err := InstallCopilot(binPath, w); err != nil {
			return err
		}
	case "antigravity":
		if err := InstallAntigravity(binPath, w); err != nil {
			return err
		}
	case "all":
		failed := 0
		for name, fn := range map[string]func(string, io.Writer) error{
			"claude":      InstallClaude,
			"codex":       InstallCodex,
			"copilot":     InstallCopilot,
			"antigravity": InstallAntigravity,
		} {
			if err := fn(binPath, w); err != nil {
				fmt.Fprintf(w, "  error [%s]: %v\n", name, err)
				failed++
			} else {
				updateGlobalInstructions(name, w)
			}
		}
		updateLocalInstructions(w)
		if failed > 0 {
			return fmt.Errorf("%d installer(s) failed", failed)
		}
		return nil
	default:
		return fmt.Errorf("unknown target %q; choose: claude, codex, copilot, antigravity, all", target)
	}
	updateGlobalInstructions(target, w)
	updateLocalInstructions(w)
	return nil
}

func updateLocalInstructions(w io.Writer) {
	for _, path := range []string{"CLAUDE.md", "AGENTS.md"} {
		if err := UpdateInstructions(path); err != nil {
			fmt.Fprintf(w, "  warning: could not update %s: %v\n", path, err)
		} else {
			fmt.Fprintf(w, "updated %s\n", path)
		}
	}
}

func updateGlobalInstructions(agent string, w io.Writer) {
	for _, path := range GlobalPaths(agent) {
		if err := UpdateInstructions(path); err != nil {
			fmt.Fprintf(w, "  warning: could not update %s: %v\n", path, err)
		} else {
			fmt.Fprintf(w, "updated %s\n", path)
		}
	}
}
