package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectAddListAndPathTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, err := st.AddProject(ctx, "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := st.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "demo" {
		t.Fatalf("unexpected projects: %#v", ps)
	}
	if _, err := st.FileContent(ctx, p, "../outside"); err == nil {
		t.Fatal("expected traversal to fail")
	}
	content, err := st.FileContent(ctx, p, "a.go")
	if err != nil || content != "package a\n" {
		t.Fatalf("unexpected file read %q err=%v", content, err)
	}
}
