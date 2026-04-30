package output

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/issam/ctxd/internal/search"
	"github.com/issam/ctxd/internal/symbols"
)

const (
	ModeCompact = "compact"
	ModeRaw     = "raw"
	ModeSummary = "summary"
)

// CompactOptions controls trimming in compact mode.
type CompactOptions struct {
	MaxResults        int
	MaxFiles          int
	MaxLinesPerResult int
	MaxTotalChars     int
}

func DefaultCompactOptions() CompactOptions {
	return CompactOptions{
		MaxResults:        5,
		MaxFiles:          3,
		MaxLinesPerResult: 40,
		MaxTotalChars:     12000,
	}
}

type CompactResult struct {
	Path      string   `json:"path"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Score     float64  `json:"score"`
	Reason    string   `json:"reason"`
	Symbols   []string `json:"symbols"`
	Snippet   string   `json:"snippet"`
}

type CompactOutput struct {
	Mode               string          `json:"mode"`
	ResultsCount       int             `json:"results_count"`
	ReturnedFilesCount int             `json:"returned_files_count"`
	Truncated          bool            `json:"truncated"`
	MaxTotalChars      int             `json:"max_total_chars"`
	Results            []CompactResult `json:"results"`
}

// ApplyCompact converts raw search results into a size-bounded compact output.
// Lower-ranked results are trimmed first when MaxTotalChars is exceeded.
func ApplyCompact(results []search.Result, opts CompactOptions) CompactOutput {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 5
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 3
	}
	if opts.MaxLinesPerResult <= 0 {
		opts.MaxLinesPerResult = 40
	}
	if opts.MaxTotalChars <= 0 {
		opts.MaxTotalChars = 12000
	}

	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	// Enforce max files limit, preserving rank order.
	seenFiles := map[string]bool{}
	var capped []search.Result
	for _, r := range results {
		if !seenFiles[r.Path] {
			if len(seenFiles) >= opts.MaxFiles {
				continue
			}
			seenFiles[r.Path] = true
		}
		capped = append(capped, r)
	}

	var out []CompactResult
	fileSet := map[string]bool{}
	totalChars := 0
	truncated := false

	for _, r := range capped {
		snippet := trimLines(r.Snippet, opts.MaxLinesPerResult)
		syms := extractSymbolNames(r.Path, snippet)
		cr := CompactResult{
			Path:      r.Path,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
			Score:     r.Score,
			Reason:    r.Reason,
			Symbols:   syms,
			Snippet:   snippet,
		}
		est := estimateChars(cr)
		if totalChars+est > opts.MaxTotalChars && len(out) > 0 {
			truncated = true
			break
		}
		totalChars += est
		out = append(out, cr)
		fileSet[r.Path] = true
	}

	return CompactOutput{
		Mode:               ModeCompact,
		ResultsCount:       len(out),
		ReturnedFilesCount: len(fileSet),
		Truncated:          truncated,
		MaxTotalChars:      opts.MaxTotalChars,
		Results:            out,
	}
}

// FormatCompact renders a CompactOutput as human-readable text.
func FormatCompact(co CompactOutput) string {
	var sb strings.Builder
	if co.Truncated {
		fmt.Fprintf(&sb, "[trimmed: showing %d results from %d files — lower-ranked results dropped to stay within %d chars]\n\n",
			co.ResultsCount, co.ReturnedFilesCount, co.MaxTotalChars)
	}
	for _, r := range co.Results {
		fmt.Fprintf(&sb, "%s:%d-%d\nScore: %.1f\nReason: %s\n", r.Path, r.StartLine, r.EndLine, r.Score, r.Reason)
		if len(r.Symbols) > 0 {
			sb.WriteString("\nSymbols:\n")
			for _, s := range r.Symbols {
				fmt.Fprintf(&sb, "- %s\n", s)
			}
		}
		sb.WriteString("\nSnippet:\n")
		sb.WriteString(r.Snippet)
		sb.WriteString("\n\n")
	}
	if co.Truncated {
		sb.WriteString("[lower-ranked results were trimmed — use --mode raw or expand for full output]\n")
	}
	return sb.String()
}

func trimLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		s = strings.Join(lines[:maxLines], "\n") + "\n..."
	}
	// Also cap by chars: ~80 chars per line is reasonable.
	maxChars := maxLines * 80
	if maxChars < 800 {
		maxChars = 800
	}
	if len(s) > maxChars {
		s = s[:maxChars] + "\n..."
	}
	return s
}

func extractSymbolNames(path, snippet string) []string {
	lang := LangFromPath(path)
	syms := symbols.Extract(snippet, lang)
	seen := map[string]bool{}
	var out []string
	for _, s := range syms {
		label := s.Kind + " " + s.Name
		if !seen[label] {
			seen[label] = true
			out = append(out, label)
		}
	}
	return out
}

// LangFromPath returns the language identifier for a file extension.
func LangFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".php":
		return "php"
	case ".js", ".mjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".jsx":
		return "javascriptreact"
	}
	return ""
}

func estimateChars(cr CompactResult) int {
	n := len(cr.Path) + len(cr.Reason) + len(cr.Snippet) + 100
	for _, s := range cr.Symbols {
		n += len(s) + 3
	}
	return n
}
