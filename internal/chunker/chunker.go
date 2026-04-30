package chunker

import "strings"

type Segment struct {
	StartLine     int
	EndLine       int
	Content       string
	TokenEstimate int
	Kind          string
}

func Chunk(content, language string) []Segment {
	lines := strings.Split(content, "\n")
	var out []Segment
	start := 0
	for start < len(lines) {
		end := start
		chars := 0
		for end < len(lines) && end-start < 160 && chars < 18000 {
			chars += len(lines[end]) + 1
			end++
		}
		if end == start {
			end++
		}
		text := strings.Join(lines[start:end], "\n")
		out = append(out, Segment{StartLine: start + 1, EndLine: end, Content: text, TokenEstimate: EstimateTokens(text), Kind: "code"})
		start = end
	}
	return out
}

func EstimateTokens(s string) int {
	n := len([]rune(s)) / 4
	if n < 1 && s != "" {
		return 1
	}
	return n
}
