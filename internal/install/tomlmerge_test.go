package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeCodexTOMLEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "config.toml")

	if err := mergeCodexTOML(path, "/bin/ctxd"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "[mcp_servers.ctxd]") {
		t.Fatal("section header not written")
	}
	if !strings.Contains(s, `"/bin/ctxd"`) {
		t.Fatal("command not written")
	}
}

func TestMergeCodexTOMLPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	existing := "[other_section]\nfoo = \"bar\"\n"
	os.WriteFile(path, []byte(existing), 0o644)

	if err := mergeCodexTOML(path, "/bin/ctxd"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "[other_section]") {
		t.Fatal("existing section was removed")
	}
	if !strings.Contains(s, "[mcp_servers.ctxd]") {
		t.Fatal("new section not appended")
	}
}

func TestMergeCodexTOMLIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := mergeCodexTOML(path, "/bin/ctxd"); err != nil {
		t.Fatal(err)
	}
	if err := mergeCodexTOML(path, "/bin/ctxd"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	count := strings.Count(s, "[mcp_servers.ctxd]")
	if count != 1 {
		t.Fatalf("expected 1 occurrence, got %d", count)
	}
}

func TestMergeCodexTOMLUpdatesExistingCtxdSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	existing := "[mcp_servers.ctxd]\ncommand = \"/old/ctxd\"\nargs = [\"serve\", \"--mcp\"]\n[other]\nvalue = true\n"
	os.WriteFile(path, []byte(existing), 0o644)

	if err := mergeCodexTOML(path, "/new/ctxd"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if strings.Contains(s, "/old/ctxd") || !strings.Contains(s, `"/new/ctxd"`) {
		t.Fatalf("ctxd command was not updated:\n%s", s)
	}
	if !strings.Contains(s, "[other]") {
		t.Fatalf("other section was not preserved:\n%s", s)
	}
}
