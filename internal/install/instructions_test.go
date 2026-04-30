package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateInstructionsIdempotentAndGraphAware(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := UpdateInstructions(path); err != nil {
		t.Fatal(err)
	}
	if err := UpdateInstructions(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if strings.Count(s, policyMarker) != 1 {
		t.Fatalf("policy duplicated:\n%s", s)
	}
	for _, want := range []string{"ctxd_context", "ctxd_graph_neighbors", "ctxd_graph_path", "ctxd_read_files"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in policy:\n%s", want, s)
		}
	}
}
