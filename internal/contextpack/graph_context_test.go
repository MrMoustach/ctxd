package contextpack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/issam/ctxd/internal/graph"
	"github.com/issam/ctxd/internal/indexer"
	"github.com/issam/ctxd/internal/store"
)

func TestContextPackIncludesGraphExpansion(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "handler.go"), []byte("package demo\nfunc HandlePayment(){ SaveInvoice() }\n"), 0o644)
	os.WriteFile(filepath.Join(root, "invoice.go"), []byte("package demo\nfunc SaveInvoice(){ println(\"ledger\") }\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, _ := st.AddProject(ctx, "demo", root)
	if _, err := indexer.IndexProject(ctx, st, p); err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Rebuild(ctx, st, p); err != nil {
		t.Fatal(err)
	}
	md, _, err := BuildWithOptions(ctx, st, p, "HandlePayment", Options{MaxTokens: 2000, Graph: true, GraphDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "## Graph-Expanded Related Files") || !strings.Contains(md, "SaveInvoice") {
		t.Fatalf("missing graph expansion:\n%s", md)
	}
}
