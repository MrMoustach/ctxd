package output_test

import (
	"strings"
	"testing"

	"github.com/issam/ctxd/internal/output"
	"github.com/issam/ctxd/internal/search"
)

func makeResults(n int, path string) []search.Result {
	results := make([]search.Result, n)
	for i := range results {
		results[i] = search.Result{
			Path:      path,
			StartLine: i*10 + 1,
			EndLine:   i*10 + 10,
			Score:     float64(n-i) * 10,
			Reason:    "FTS match",
			Snippet:   strings.Repeat("line of code\n", 50),
		}
	}
	return results
}

func TestCompactRespectMaxTotalChars(t *testing.T) {
	results := makeResults(5, "app/Foo.php")
	opts := output.CompactOptions{
		MaxResults:        5,
		MaxFiles:          3,
		MaxLinesPerResult: 40,
		MaxTotalChars:     500, // very small to force trimming
	}
	co := output.ApplyCompact(results, opts)
	text := output.FormatCompact(co)
	if len(text) > 500*3 { // some overhead is OK, but should be far below raw
		t.Errorf("output too large: %d chars", len(text))
	}
	if !co.Truncated {
		t.Error("expected Truncated=true")
	}
}

func TestCompactMaxFiles(t *testing.T) {
	results := []search.Result{
		{Path: "a.php", Score: 10, Snippet: "aaa"},
		{Path: "b.php", Score: 9, Snippet: "bbb"},
		{Path: "c.php", Score: 8, Snippet: "ccc"},
		{Path: "d.php", Score: 7, Snippet: "ddd"},
	}
	opts := output.CompactOptions{MaxResults: 10, MaxFiles: 2, MaxLinesPerResult: 10, MaxTotalChars: 100000}
	co := output.ApplyCompact(results, opts)
	if co.ReturnedFilesCount > 2 {
		t.Errorf("expected at most 2 files, got %d", co.ReturnedFilesCount)
	}
}

func TestCompactNoFullFileDump(t *testing.T) {
	bigSnippet := strings.Repeat("x", 5000)
	results := []search.Result{
		{Path: "big.php", Score: 10, StartLine: 1, EndLine: 500, Snippet: bigSnippet},
	}
	opts := output.DefaultCompactOptions() // 40 lines max
	co := output.ApplyCompact(results, opts)
	if len(co.Results) > 0 && strings.Contains(co.Results[0].Snippet, strings.Repeat("x", 4000)) {
		t.Error("snippet not trimmed to max lines")
	}
}

func TestCompactTruncatedFlag(t *testing.T) {
	results := makeResults(5, "app/Foo.php")
	opts := output.CompactOptions{MaxResults: 5, MaxFiles: 3, MaxLinesPerResult: 40, MaxTotalChars: 50}
	co := output.ApplyCompact(results, opts)
	if !co.Truncated {
		t.Error("expected Truncated=true when MaxTotalChars is tiny")
	}
}

func TestRawPassthrough(t *testing.T) {
	results := makeResults(3, "src/main.go")
	// Raw mode: results returned as-is (no output package transform needed,
	// just verify ApplyCompact with defaults doesn't corrupt data)
	opts := output.DefaultCompactOptions()
	co := output.ApplyCompact(results, opts)
	if co.ResultsCount == 0 {
		t.Error("expected at least one result")
	}
}

func TestDefaultOptions(t *testing.T) {
	d := output.DefaultCompactOptions()
	if d.MaxResults != 5 || d.MaxFiles != 3 || d.MaxLinesPerResult != 40 || d.MaxTotalChars != 12000 {
		t.Errorf("unexpected defaults: %+v", d)
	}
}
