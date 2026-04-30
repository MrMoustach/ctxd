package summary

import (
	"fmt"
	"regexp"
	"strings"
)

// FileSummary holds rule-extracted metadata about a source file.
type FileSummary struct {
	Path       string
	Type       string
	Namespace  string
	ClassName  string
	ClassKind  string // class / interface / trait
	Methods    []Method
	Purpose    string   // from class-level docblock
	MatchedFor []string // terms from the query found in the file
}

// Method is a function/method signature extracted from source.
type Method struct {
	Visibility string
	Name       string
	Signature  string
}

var (
	phpNamespaceRE = regexp.MustCompile(`(?m)^\s*namespace\s+([\w\\]+)\s*;`)
	phpClassRE     = regexp.MustCompile(`(?m)^\s*(?:abstract\s+|final\s+)?(class|interface|trait)\s+(\w+)`)
	phpMethodRE    = regexp.MustCompile(`(?m)^\s*(public|protected|private)(\s+static)?\s+function\s+(\w+)(\s*\([^)]*\))(\s*:\s*[\w\\?|[\]]+)?`)
	phpDocRE       = regexp.MustCompile(`(?s)/\*\*(.*?)\*/`)
	goPackageRE    = regexp.MustCompile(`(?m)^package\s+(\w+)`)
	goFuncRE       = regexp.MustCompile(`(?m)^func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(([^)]*)\)`)
)

// Summarize extracts a structural summary from file content.
// matchTerms are query terms used to populate the "matched because" section.
func Summarize(path, content, language string, matchTerms []string) FileSummary {
	s := FileSummary{Path: path}
	switch language {
	case "php":
		summarizePHP(&s, path, content)
	case "go":
		summarizeGo(&s, content)
	default:
		s.Type = "file"
	}
	lower := strings.ToLower(content)
	for _, t := range matchTerms {
		if strings.Contains(lower, strings.ToLower(t)) {
			s.MatchedFor = append(s.MatchedFor, "contains "+t)
		}
	}
	return s
}

func summarizePHP(s *FileSummary, path, content string) {
	if m := phpNamespaceRE.FindStringSubmatch(content); len(m) > 1 {
		s.Namespace = m[1]
	}
	if m := phpClassRE.FindStringSubmatch(content); len(m) > 2 {
		s.ClassKind = m[1]
		s.ClassName = m[2]
	}
	s.Type = detectLaravelType(path, s.ClassName)

	if s.ClassName != "" {
		if idx := phpClassRE.FindStringIndex(content); idx != nil {
			docs := phpDocRE.FindAllString(content[:idx[0]], -1)
			if len(docs) > 0 {
				s.Purpose = cleanDocblock(docs[len(docs)-1])
			}
		}
	}

	for _, m := range phpMethodRE.FindAllStringSubmatch(content, -1) {
		visibility := m[1]
		name := m[3]
		params := m[4]
		ret := strings.TrimSpace(m[5])
		sig := name + params
		if ret != "" {
			sig += ": " + strings.TrimPrefix(strings.TrimSpace(ret), ":")
		}
		sig = strings.TrimSpace(sig)
		s.Methods = append(s.Methods, Method{Visibility: visibility, Name: name, Signature: sig})
	}
}

func summarizeGo(s *FileSummary, content string) {
	s.Type = "Go"
	if m := goPackageRE.FindStringSubmatch(content); len(m) > 1 {
		s.Namespace = m[1]
	}
	for _, m := range goFuncRE.FindAllStringSubmatch(content, -1) {
		name := m[1]
		params := m[2]
		vis := "private"
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			vis = "public"
		}
		s.Methods = append(s.Methods, Method{
			Visibility: vis,
			Name:       name,
			Signature:  name + "(" + params + ")",
		})
	}
}

func detectLaravelType(path, className string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".blade.php"):
		return "Blade View"
	case strings.Contains(lower, "/controllers/") || strings.HasSuffix(lower, "controller.php"):
		return "Controller"
	case strings.Contains(lower, "/services/") || strings.HasSuffix(lower, "service.php"):
		return "Service"
	case strings.Contains(lower, "/models/") || strings.HasSuffix(lower, "model.php"):
		return "Model"
	case strings.Contains(lower, "/repositories/") || strings.HasSuffix(lower, "repository.php"):
		return "Repository"
	case strings.Contains(lower, "/middleware/") || strings.HasSuffix(lower, "middleware.php"):
		return "Middleware"
	case strings.Contains(lower, "/jobs/") || strings.HasSuffix(lower, "job.php"):
		return "Job"
	case strings.Contains(lower, "/events/") || strings.HasSuffix(lower, "event.php"):
		return "Event"
	case strings.Contains(lower, "/listeners/") || strings.HasSuffix(lower, "listener.php"):
		return "Listener"
	case strings.Contains(lower, "/migrations/") || strings.Contains(lower, "migrations/"):
		return "Migration"
	case strings.Contains(lower, "/routes/") || strings.HasPrefix(lower, "routes/") || strings.HasSuffix(lower, "routes.php"):
		return "Route file"
	case strings.Contains(lower, "/tests/") || strings.HasPrefix(lower, "tests/") || strings.HasPrefix(strings.ToLower(className), "test"):
		return "Test"
	}
	if className != "" {
		return "PHP Class"
	}
	return "PHP"
}

func cleanDocblock(doc string) string {
	doc = strings.TrimPrefix(doc, "/**")
	doc = strings.TrimSuffix(doc, "*/")
	var lines []string
	for _, line := range strings.Split(doc, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "@") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	// Return first meaningful sentence (up to 200 chars).
	joined := strings.Join(lines, " ")
	if len(joined) > 200 {
		joined = joined[:200] + "..."
	}
	return joined
}

// Format renders a single FileSummary as human-readable text.
func Format(s FileSummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "File: %s\n", s.Path)
	fmt.Fprintf(&sb, "Type: %s\n", s.Type)
	if s.Namespace != "" {
		fmt.Fprintf(&sb, "Namespace: %s\n", s.Namespace)
	}
	if s.ClassName != "" {
		kind := s.ClassKind
		if kind == "" {
			kind = "class"
		}
		fmt.Fprintf(&sb, "%s: %s\n", capitalize(kind), s.ClassName)
	}
	if s.Purpose != "" {
		sb.WriteString("\nPurpose:\n")
		sb.WriteString(s.Purpose)
		sb.WriteString("\n")
	}
	if len(s.Methods) > 0 {
		sb.WriteString("\nImportant symbols:\n")
		for _, m := range s.Methods {
			fmt.Fprintf(&sb, "- %s\n", m.Signature)
		}
	}
	if len(s.MatchedFor) > 0 {
		sb.WriteString("\nMatched because:\n")
		for _, r := range s.MatchedFor {
			fmt.Fprintf(&sb, "- %s\n", r)
		}
	}
	return sb.String()
}

// FormatAll renders multiple summaries separated by dividers.
func FormatAll(summaries []FileSummary) string {
	var sb strings.Builder
	for i, s := range summaries {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		sb.WriteString(Format(s))
	}
	return sb.String()
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
