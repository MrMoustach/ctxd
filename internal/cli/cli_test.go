package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddAcceptsNameAfterPath(t *testing.T) {
	t.Setenv("CTX_DB", filepath.Join(t.TempDir(), "ctx.db"))
	root := t.TempDir()
	var out, errOut bytes.Buffer
	err := Run([]string{"add", root, "--name", "demo"}, &out, &errOut)
	if err != nil {
		t.Fatalf("add failed: %v stderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "added demo") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestAddAcceptsNameBeforePath(t *testing.T) {
	t.Setenv("CTX_DB", filepath.Join(t.TempDir(), "ctx.db"))
	root := t.TempDir()
	var out, errOut bytes.Buffer
	err := Run([]string{"add", "--name", "demo", root}, &out, &errOut)
	if err != nil {
		t.Fatalf("add failed: %v stderr=%s", err, errOut.String())
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatal(statErr)
	}
}
