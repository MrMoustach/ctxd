package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/issam/ctxd/internal/search"
	"github.com/issam/ctxd/internal/store"
)

func TestIndexAndSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "PaymentService.php"), []byte("<?php\nclass PaymentService {\n public function handlePayment() { return 'paid invoice'; }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, err := st.AddProject(ctx, "pms", root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := IndexProject(ctx, st, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.IndexedFiles != 1 {
		t.Fatalf("expected one indexed file, got %#v", res)
	}
	results, err := search.Search(ctx, st, p, "where is payment handled?", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Path != "PaymentService.php" {
		t.Fatalf("unexpected search results: %#v", results)
	}
}
