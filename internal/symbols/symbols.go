package symbols

import (
	"regexp"
	"strings"
)

type Symbol struct {
	Name      string
	Kind      string
	Line      int
	Signature string
}

type Import struct {
	Path string
	Raw  string
}

var patterns = map[string][]struct {
	kind string
	re   *regexp.Regexp
}{
	"php": {
		{"class", regexp.MustCompile(`\b(class|interface|trait|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{"function", regexp.MustCompile(`\bfunction\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)},
	},
	"go": {
		{"function", regexp.MustCompile(`\bfunc\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)},
		{"type", regexp.MustCompile(`\btype\s+([A-Za-z_][A-Za-z0-9_]*)\s+(struct|interface)`)},
	},
	"javascript": {
		{"function", regexp.MustCompile(`\bfunction\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)},
		{"class", regexp.MustCompile(`\bclass\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
	},
	"typescript": {
		{"function", regexp.MustCompile(`\bfunction\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)},
		{"class", regexp.MustCompile(`\b(class|interface|type)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
	},
}

func Extract(content, language string) []Symbol {
	ps := patterns[language]
	if language == "typescriptreact" {
		ps = patterns["typescript"]
	}
	if language == "javascriptreact" {
		ps = patterns["javascript"]
	}
	lines := strings.Split(content, "\n")
	var out []Symbol
	for i, line := range lines {
		for _, p := range ps {
			m := p.re.FindStringSubmatch(line)
			if len(m) == 0 {
				continue
			}
			name := m[len(m)-1]
			if p.kind == "class" && len(m) > 2 {
				name = m[2]
			}
			out = append(out, Symbol{Name: name, Kind: p.kind, Line: i + 1, Signature: strings.TrimSpace(line)})
		}
	}
	return out
}

var importRE = regexp.MustCompile(`(?m)^\s*(?:use|import|require|from)\s+["']?([^"';\s]+)`)

func ExtractImports(content string) []Import {
	var out []Import
	for _, m := range importRE.FindAllStringSubmatch(content, -1) {
		out = append(out, Import{Path: m[1], Raw: strings.TrimSpace(m[0])})
	}
	return out
}
