package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeJSONEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	overlay := map[string]any{
		"mcpServers": map[string]any{
			"ctxd": map[string]any{"type": "stdio", "command": "/bin/ctxd"},
		},
	}
	if err := mergeJSON(path, overlay); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	servers := got["mcpServers"].(map[string]any)
	ctxd := servers["ctxd"].(map[string]any)
	if ctxd["command"] != "/bin/ctxd" {
		t.Fatalf("unexpected command: %v", ctxd["command"])
	}
}

func TestMergeJSONPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	existing := `{"mcpServers":{"other":{"type":"stdio","command":"/bin/other"}}}`
	os.WriteFile(path, []byte(existing), 0o644)

	overlay := map[string]any{
		"mcpServers": map[string]any{
			"ctxd": map[string]any{"type": "stdio", "command": "/bin/ctxd"},
		},
	}
	if err := mergeJSON(path, overlay); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var got map[string]any
	json.Unmarshal(data, &got)
	servers := got["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Fatal("existing 'other' server was removed")
	}
	if _, ok := servers["ctxd"]; !ok {
		t.Fatal("new 'ctxd' server was not added")
	}
}

func TestMergeJSONIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	overlay := map[string]any{
		"mcpServers": map[string]any{
			"ctxd": map[string]any{"type": "stdio", "command": "/bin/ctxd"},
		},
	}
	if err := mergeJSON(path, overlay); err != nil {
		t.Fatal(err)
	}
	if err := mergeJSON(path, overlay); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var got map[string]any
	json.Unmarshal(data, &got)
	servers := got["mcpServers"].(map[string]any)
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
}

func TestMergeJSONCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "test.json")

	if err := mergeJSON(path, map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
