package contextpack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/issam/ctxd/internal/indexer"
	"github.com/issam/ctxd/internal/store"
)

func TestContextPackRespectsBudgetShape(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "checkout.go"), []byte("package checkout\nfunc Pay(){ println(\"payment checkout invoice\") }\n"), 0o644)
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
	md, _, err := Build(ctx, st, p, "payment checkout", 300)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "# ctxd Context Pack") || !strings.Contains(md, "checkout.go") {
		t.Fatalf("bad context pack:\n%s", md)
	}
}
