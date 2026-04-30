package ignore

import (
	"os"
	"path/filepath"
	"strings"
)

type Matcher struct {
	root     string
	patterns []string
}

var defaults = []string{".git", "node_modules", "vendor", "dist", "build", ".next", ".nuxt", ".cache", "coverage", "storage/logs", ".env", "*.lock", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "composer.lock", "*.min.js", "*.map", "*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.svg", "*.pdf", "*.zip", "*.tar", "*.gz"}

func New(root string) Matcher {
	m := Matcher{root: root, patterns: append([]string{}, defaults...)}
	if b, err := os.ReadFile(filepath.Join(root, ".gitignore")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
				continue
			}
			m.patterns = append(m.patterns, strings.Trim(line, "/"))
		}
	}
	return m
}

func (m Matcher) Ignored(rel string, isDir bool) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	base := filepath.Base(rel)
	for _, p := range m.patterns {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if strings.Contains(p, "*") {
			if ok, _ := filepath.Match(p, base); ok {
				return true
			}
			continue
		}
		if rel == p || base == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
		if isDir && strings.HasSuffix(p, "/") && strings.TrimSuffix(p, "/") == base {
			return true
		}
	}
	return false
}
