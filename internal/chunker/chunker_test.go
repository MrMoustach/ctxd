package chunker

import (
	"strings"
	"testing"
)

func TestChunkLineNumbers(t *testing.T) {
	var lines []string
	for i := 0; i < 201; i++ {
		lines = append(lines, "line")
	}
	chunks := Chunk(strings.Join(lines, "\n"), "go")
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 160 {
		t.Fatalf("bad first chunk range: %#v", chunks[0])
	}
	if chunks[1].StartLine != 161 {
		t.Fatalf("bad second chunk start: %#v", chunks[1])
	}
}
