package graph

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	classRE      = regexp.MustCompile(`\b(class|interface|trait)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	phpFuncRE    = regexp.MustCompile(`\bfunction\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goFuncRE     = regexp.MustCompile(`\bfunc\s+(?:\(([^)]*)\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goTypeRE     = regexp.MustCompile(`\btype\s+([A-Za-z_][A-Za-z0-9_]*)\s+(struct|interface)`)
	jsClassRE    = regexp.MustCompile(`\b(class|interface|type)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	jsFuncRE     = regexp.MustCompile(`\b(?:function\s+|const\s+|let\s+|var\s+)([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:=|\()`)
	methodRE     = regexp.MustCompile(`^\s*(?:public|protected|private|static|async|\*)*\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^;]*\)\s*\{?`)
	importRE     = regexp.MustCompile(`(?m)^\s*(?:use|import|require|from)\s+["']?([^"';\s]+)`)
	callRE       = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*(?:::|->|\.)?\s*([A-Za-z_][A-Za-z0-9_]*)?\s*\(`)
	routeArrayRE = regexp.MustCompile(`Route::([a-zA-Z]+)\s*\(\s*['"]([^'"]+)['"]\s*,\s*\[\s*([A-Za-z_\\][A-Za-z0-9_\\]*)::class\s*,\s*['"]([A-Za-z_][A-Za-z0-9_]*)['"]`)
	routeAtRE    = regexp.MustCompile(`Route::([a-zA-Z]+)\s*\(\s*['"]([^'"]+)['"]\s*,\s*['"]([A-Za-z_\\][A-Za-z0-9_\\]*)@([A-Za-z_][A-Za-z0-9_]*)['"]`)
	modelRE      = regexp.MustCompile(`\b([A-Z][A-Za-z0-9_]*)::(?:query|where|find|create|update|first|get)\s*\(`)
	appRE        = regexp.MustCompile(`\bapp\s*\(\s*([A-Za-z_\\][A-Za-z0-9_\\]*)::class\s*\)`)
	dispatchRE   = regexp.MustCompile(`\b(?:dispatch\s*\(\s*new\s+|([A-Za-z_][A-Za-z0-9_]*)::dispatch\s*\()([A-Za-z_][A-Za-z0-9_]*)?`)
	extendsRE    = regexp.MustCompile(`\bextends\s+([A-Za-z_\\][A-Za-z0-9_\\]*)`)
	implementsRE = regexp.MustCompile(`\bimplements\s+([A-Za-z_\\][A-Za-z0-9_\\, ]+)`)
)

func ParseFile(path, lang, content string) ParsedFile {
	p := ParsedFile{Path: path, Lang: lang, Content: content}
	p.Nodes = append(p.Nodes, Node{Type: "file", Name: filepath.Base(path), QualifiedName: path, FilePath: path, StartLine: 1, EndLine: lineCount(content)})
	switch normalizeLang(lang) {
	case "php":
		parsePHP(&p)
	case "go":
		parseGo(&p)
	case "javascript", "typescript":
		parseJS(&p)
	default:
		parseFallback(&p)
	}
	for _, m := range importRE.FindAllStringSubmatch(content, -1) {
		p.Imports = append(p.Imports, strings.Trim(m[1], `"'`))
	}
	parseCalls(&p)
	if strings.HasPrefix(path, "routes/") && strings.HasSuffix(path, ".php") {
		parseLaravelRoutes(&p)
	}
	parseLaravelUses(&p)
	return p
}

func parsePHP(p *ParsedFile) {
	lines := strings.Split(p.Content, "\n")
	var currentClass string
	for i, line := range lines {
		if m := classRE.FindStringSubmatch(line); len(m) > 0 {
			typ := m[1]
			name := m[2]
			p.Nodes = append(p.Nodes, classifyNode(typ, name, p.Path, i+1, line))
			currentClass = name
			if e := extendsRE.FindStringSubmatch(line); len(e) > 0 {
				p.Uses = append(p.Uses, Use{FromName: name, ToName: shortName(e[1]), Type: "extends", Line: i + 1, Raw: strings.TrimSpace(line)})
			}
			if im := implementsRE.FindStringSubmatch(line); len(im) > 0 {
				for _, part := range strings.Split(im[1], ",") {
					p.Uses = append(p.Uses, Use{FromName: name, ToName: shortName(part), Type: "implements", Line: i + 1, Raw: strings.TrimSpace(line)})
				}
			}
		}
		if m := phpFuncRE.FindStringSubmatch(line); len(m) > 0 {
			typ := "function"
			qn := m[1]
			if currentClass != "" {
				typ = "method"
				qn = currentClass + "." + m[1]
			}
			p.Nodes = append(p.Nodes, Node{Type: typ, Name: m[1], QualifiedName: qn, FilePath: p.Path, StartLine: i + 1, EndLine: i + 1, MetadataJSON: meta("signature", strings.TrimSpace(line))})
		}
	}
}

func parseGo(p *ParsedFile) {
	lines := strings.Split(p.Content, "\n")
	for i, line := range lines {
		if m := goTypeRE.FindStringSubmatch(line); len(m) > 0 {
			typ := "class"
			if m[2] == "interface" {
				typ = "interface"
			}
			p.Nodes = append(p.Nodes, Node{Type: typ, Name: m[1], QualifiedName: m[1], FilePath: p.Path, StartLine: i + 1, EndLine: i + 1, MetadataJSON: meta("signature", strings.TrimSpace(line))})
		}
		if m := goFuncRE.FindStringSubmatch(line); len(m) > 0 {
			name := m[2]
			qn := name
			typ := "function"
			if m[1] != "" {
				typ = "method"
				qn = receiverName(m[1]) + "." + name
			}
			p.Nodes = append(p.Nodes, Node{Type: typ, Name: name, QualifiedName: qn, FilePath: p.Path, StartLine: i + 1, EndLine: i + 1, MetadataJSON: meta("signature", strings.TrimSpace(line))})
		}
	}
}

func parseJS(p *ParsedFile) {
	lines := strings.Split(p.Content, "\n")
	var currentClass string
	for i, line := range lines {
		if m := jsClassRE.FindStringSubmatch(line); len(m) > 0 {
			typ := "class"
			if m[1] == "interface" {
				typ = "interface"
			}
			p.Nodes = append(p.Nodes, Node{Type: typ, Name: m[2], QualifiedName: m[2], FilePath: p.Path, StartLine: i + 1, EndLine: i + 1, MetadataJSON: meta("signature", strings.TrimSpace(line))})
			currentClass = m[2]
		}
		if m := jsFuncRE.FindStringSubmatch(line); len(m) > 0 {
			p.Nodes = append(p.Nodes, Node{Type: "function", Name: m[1], QualifiedName: m[1], FilePath: p.Path, StartLine: i + 1, EndLine: i + 1, MetadataJSON: meta("signature", strings.TrimSpace(line))})
			continue
		}
		if currentClass != "" {
			if m := methodRE.FindStringSubmatch(line); len(m) > 0 && !keyword(m[1]) {
				p.Nodes = append(p.Nodes, Node{Type: "method", Name: m[1], QualifiedName: currentClass + "." + m[1], FilePath: p.Path, StartLine: i + 1, EndLine: i + 1, MetadataJSON: meta("signature", strings.TrimSpace(line))})
			}
		}
	}
}

func parseFallback(p *ParsedFile) {
	lines := strings.Split(p.Content, "\n")
	for i, line := range lines {
		for _, re := range []*regexp.Regexp{phpFuncRE, jsFuncRE, goFuncRE} {
			if m := re.FindStringSubmatch(line); len(m) > 0 {
				p.Nodes = append(p.Nodes, Node{Type: "function", Name: m[len(m)-1], QualifiedName: m[len(m)-1], FilePath: p.Path, StartLine: i + 1, EndLine: i + 1, MetadataJSON: meta("signature", strings.TrimSpace(line))})
				break
			}
		}
	}
}

func parseCalls(p *ParsedFile) {
	lines := strings.Split(p.Content, "\n")
	current := ""
	for i, line := range lines {
		for _, n := range p.Nodes {
			if n.Type != "file" && n.StartLine == i+1 {
				current = n.QualifiedName
				if current == "" {
					current = n.Name
				}
			}
		}
		for _, m := range callRE.FindAllStringSubmatch(line, -1) {
			name := m[1]
			if len(m) > 2 && m[2] != "" {
				name = m[2]
			}
			if keyword(name) || name == current || strings.EqualFold(name, "function") {
				continue
			}
			p.Calls = append(p.Calls, Call{FromName: current, ToName: name, Line: i + 1, Raw: strings.TrimSpace(line)})
		}
	}
}

func parseLaravelRoutes(p *ParsedFile) {
	lines := strings.Split(p.Content, "\n")
	for i, line := range lines {
		if m := routeArrayRE.FindStringSubmatch(line); len(m) > 0 {
			p.Routes = append(p.Routes, Route{Method: strings.ToUpper(m[1]), URI: m[2], Controller: shortName(m[3]), Action: m[4], Line: i + 1, Raw: strings.TrimSpace(line)})
		}
		if m := routeAtRE.FindStringSubmatch(line); len(m) > 0 {
			p.Routes = append(p.Routes, Route{Method: strings.ToUpper(m[1]), URI: m[2], Controller: shortName(m[3]), Action: m[4], Line: i + 1, Raw: strings.TrimSpace(line)})
		}
	}
}

func parseLaravelUses(p *ParsedFile) {
	lines := strings.Split(p.Content, "\n")
	for i, line := range lines {
		from := nearestSymbol(p.Nodes, i+1)
		for _, m := range modelRE.FindAllStringSubmatch(line, -1) {
			p.Uses = append(p.Uses, Use{FromName: from, ToName: m[1], Type: "uses", Line: i + 1, Raw: strings.TrimSpace(line)})
		}
		for _, m := range appRE.FindAllStringSubmatch(line, -1) {
			p.Uses = append(p.Uses, Use{FromName: from, ToName: shortName(m[1]), Type: "depends_on", Line: i + 1, Raw: strings.TrimSpace(line)})
		}
		for _, m := range dispatchRE.FindAllStringSubmatch(line, -1) {
			job := m[1]
			if job == "" && len(m) > 2 {
				job = m[2]
			}
			if job != "" {
				p.Uses = append(p.Uses, Use{FromName: from, ToName: job, Type: "depends_on", Line: i + 1, Raw: strings.TrimSpace(line)})
			}
		}
	}
}

func classifyNode(typ, name, path string, line int, raw string) Node {
	if typ == "class" {
		nameLower := strings.ToLower(name)
		switch {
		case strings.HasSuffix(name, "Controller"):
			typ = "service"
		case strings.HasSuffix(name, "Job"):
			typ = "job"
		case strings.HasSuffix(name, "Command") || strings.Contains(strings.ToLower(path), "/console/commands/"):
			typ = "command"
		case strings.HasSuffix(name, "Service"):
			typ = "service"
		case strings.HasSuffix(name, "Test") || strings.Contains(nameLower, "test") || strings.Contains(strings.ToLower(path), "test"):
			typ = "test"
		case strings.Contains(path, "app/Models/") || strings.Contains(path, "Models/"):
			typ = "model"
		}
	}
	return Node{Type: typ, Name: name, QualifiedName: name, FilePath: path, StartLine: line, EndLine: line, MetadataJSON: meta("signature", strings.TrimSpace(raw))}
}

func normalizeLang(lang string) string {
	switch lang {
	case "typescriptreact":
		return "typescript"
	case "javascriptreact":
		return "javascript"
	default:
		return lang
	}
}

func shortName(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `\`))
	if i := strings.LastIndex(s, `\`); i >= 0 {
		return s[i+1:]
	}
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

func receiverName(s string) string {
	fields := strings.Fields(strings.ReplaceAll(s, "*", " "))
	if len(fields) == 0 {
		return ""
	}
	return shortName(fields[len(fields)-1])
}

func nearestSymbol(nodes []Node, line int) string {
	best := ""
	bestLine := 0
	for _, n := range nodes {
		if n.Type == "file" || n.StartLine > line || n.StartLine < bestLine {
			continue
		}
		best = n.QualifiedName
		if best == "" {
			best = n.Name
		}
		bestLine = n.StartLine
	}
	return best
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func keyword(s string) bool {
	switch s {
	case "if", "for", "switch", "return", "while", "catch", "foreach", "function", "class", "new", "make", "array":
		return true
	default:
		return false
	}
}

func meta(k, v string) string {
	if v == "" {
		return ""
	}
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return `{"` + k + `":"` + v + `"}`
}
