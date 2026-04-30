package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const codexSectionHeader = "[mcp_servers.ctxd]"

// mergeCodexTOML ensures [mcp_servers.ctxd] section exists in path.
// Appends it if absent; replaces only that section if already present.
func mergeCodexTOML(path, binPath string) error {
	content := ""
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	section := fmt.Sprintf("\n%s\ncommand = %q\nargs = [\"serve\", \"--mcp\"]\n", codexSectionHeader, binPath)
	if strings.Contains(content, codexSectionHeader) {
		content = replaceTOMLSection(content, codexSectionHeader, strings.TrimPrefix(section, "\n"))
		return atomicWrite(path, []byte(content))
	}
	if !strings.HasSuffix(content, "\n") && content != "" {
		content += "\n"
	}
	return atomicWrite(path, []byte(content+section))
}

func replaceTOMLSection(content, header, replacement string) string {
	start := strings.Index(content, header)
	if start < 0 {
		return content
	}
	lineStart := start
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}
	end := start + len(header)
	for end < len(content) {
		next := strings.IndexByte(content[end:], '\n')
		if next < 0 {
			end = len(content)
			break
		}
		line := content[end+next+1:]
		if strings.HasPrefix(line, "[") {
			end = end + next + 1
			break
		}
		end = end + next + 1
	}
	if !strings.HasSuffix(replacement, "\n") {
		replacement += "\n"
	}
	return content[:lineStart] + replacement + content[end:]
}
