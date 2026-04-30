package graph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/issam/ctxd/internal/indexer"
	"github.com/issam/ctxd/internal/store"
)

func TestGraphExtractionAndEdges(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "routes", "web.php"), `<?php
use App\Http\Controllers\UserController;
Route::get('/users', [UserController::class, 'index']);
`)
	mustWrite(t, filepath.Join(root, "app", "Http", "Controllers", "UserController.php"), `<?php
class UserController {
  public function index() {
    User::query();
    app(UserService::class)->list();
  }
}
`)
	mustWrite(t, filepath.Join(root, "app", "Models", "User.php"), `<?php class User {}`)
	mustWrite(t, filepath.Join(root, "app", "Services", "UserService.php"), `<?php class UserService { public function list() {} }`)
	st := newIndexedStore(t, root)
	p, err := st.ProjectByName(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	stats, err := Rebuild(context.Background(), st, p)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NodesByType["route"] != 1 {
		t.Fatalf("expected route node, got %+v", stats.NodesByType)
	}
	if stats.EdgesByType["route_to"] != 1 || stats.EdgesByType["uses"] == 0 || stats.EdgesByType["depends_on"] == 0 {
		t.Fatalf("expected route/model/service edges, got %+v", stats.EdgesByType)
	}
}

func TestGraphExpansionAndPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), `package demo
func Handle() { Save() }
`)
	mustWrite(t, filepath.Join(root, "b.go"), `package demo
func Save() {}
`)
	st := newIndexedStore(t, root)
	p, _ := st.ProjectByName(context.Background(), "demo")
	if _, err := Rebuild(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	nodes, edges, err := Neighbors(context.Background(), st, p, "Handle", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) < 2 || len(edges) == 0 {
		t.Fatalf("expected graph neighbors, nodes=%d edges=%d", len(nodes), len(edges))
	}
	path, pathEdges, err := Path(context.Background(), st, p, "Handle", "Save")
	if err != nil {
		t.Fatal(err)
	}
	if len(path) < 2 || len(pathEdges) == 0 {
		t.Fatalf("expected graph path, nodes=%d edges=%d", len(path), len(pathEdges))
	}
}

func TestGraphReportWarnsWhenGraphFileNodesAreStale(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), `package demo
func Handle() {}
`)
	st := newIndexedStore(t, root)
	p, _ := st.ProjectByName(context.Background(), "demo")
	if _, err := Rebuild(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "b.go"), `package demo
func Save() {}
`)
	if _, err := indexer.IndexProject(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	path, err := WriteReport(context.Background(), st, p)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Warning: graph has") {
		t.Fatalf("expected stale graph warning, got:\n%s", data)
	}
}

func newIndexedStore(t *testing.T, root string) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p, err := st.AddProject(context.Background(), "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.IndexProject(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	return st
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
