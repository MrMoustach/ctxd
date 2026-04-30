package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/issam/ctxd/internal/graph"
	"github.com/issam/ctxd/internal/indexer"
	"github.com/issam/ctxd/internal/output"
	"github.com/issam/ctxd/internal/store"
)

func TestMCPToolsListAndCall(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc HandlePayment(){ println(\"invoice\") }\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, _ := st.AddProject(ctx, "demo", root)
	indexer.IndexProject(ctx, st, p)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_code","arguments":{"project":"demo","query":"payment","limit":3}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := Serve(ctx, st, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "search_code") || !strings.Contains(got, "pay.go") {
		t.Fatalf("unexpected MCP output: %s", got)
	}
}

func TestMCPSearchDefaultsToCompact(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc HandlePayment(){ println(\"invoice\") }\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, _ := st.AddProject(ctx, "demo", root)
	indexer.IndexProject(ctx, st, p)

	// No mode specified — should default to compact.
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_code","arguments":{"project":"demo","query":"payment"}}}` + "\n"
	var out bytes.Buffer
	if err := Serve(ctx, st, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}

	// Parse the outer MCP envelope.
	var env struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if len(env.Result.Content) == 0 {
		t.Fatal("empty content")
	}
	// Parse the inner compact output.
	var co output.CompactOutput
	if err := json.Unmarshal([]byte(env.Result.Content[0].Text), &co); err != nil {
		t.Fatalf("inner JSON not CompactOutput: %v\n%s", err, env.Result.Content[0].Text)
	}
	if co.Mode != output.ModeCompact {
		t.Errorf("expected mode=%q, got %q", output.ModeCompact, co.Mode)
	}
	// MaxTotalChars must be set (defaults applied inside ApplyCompact).
	if co.MaxTotalChars <= 0 {
		t.Errorf("expected MaxTotalChars > 0, got %d", co.MaxTotalChars)
	}
}

func TestMCPSearchRawMode(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc HandlePayment(){ println(\"invoice\") }\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, _ := st.AddProject(ctx, "demo", root)
	indexer.IndexProject(ctx, st, p)

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_code","arguments":{"project":"demo","query":"payment","mode":"raw"}}}` + "\n"
	var out bytes.Buffer
	if err := Serve(ctx, st, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "pay.go") {
		t.Fatalf("raw mode missing file: %s", out.String())
	}
}

func TestMCPSearchSummaryMode(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc HandlePayment(){ println(\"invoice\") }\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, _ := st.AddProject(ctx, "demo", root)
	indexer.IndexProject(ctx, st, p)

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ctxd_search","arguments":{"project":"demo","query":"payment","mode":"summary"}}}` + "\n"
	payload := callMCP(t, st, input)
	if payload["mode"] != output.ModeSummary {
		t.Fatalf("expected mode=%q, got %#v", output.ModeSummary, payload["mode"])
	}
	if _, ok := payload["summaries"].([]any); !ok {
		t.Fatalf("expected summaries array, got %#v", payload["summaries"])
	}
}

func TestMCPContextReturnsMetadata(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc HandlePayment(){ println(\"invoice\") }\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, _ := st.AddProject(ctx, "demo", root)
	indexer.IndexProject(ctx, st, p)

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ctxd_context","arguments":{"project":"demo","task":"payment","max_tokens":4000}}}` + "\n"
	payload := callMCP(t, st, input)
	if payload["markdown"] == "" {
		t.Fatalf("expected markdown, got %#v", payload)
	}
	if _, ok := payload["paths"].([]any); !ok {
		t.Fatalf("expected paths array, got %#v", payload["paths"])
	}
	if _, ok := payload["token_estimate"].(float64); !ok {
		t.Fatalf("expected token_estimate, got %#v", payload["token_estimate"])
	}
	if _, ok := payload["truncated"].(bool); !ok {
		t.Fatalf("expected truncated bool, got %#v", payload["truncated"])
	}
}

func TestMCPReindexRebuildsGraphByDefault(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc HandlePayment(){ Save() }\nfunc Save(){}\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if _, err := st.AddProject(ctx, "demo", root); err != nil {
		t.Fatal(err)
	}

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"reindex_project","arguments":{"project":"demo"}}}` + "\n"
	payload := callMCP(t, st, input)
	if payload["graph_rebuilt"] != true {
		t.Fatalf("expected graph_rebuilt=true, got %#v", payload)
	}
	if payload["graph_files"].(float64) == 0 {
		t.Fatalf("expected graph files, got %#v", payload)
	}
	project, err := st.ProjectByName(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	stats, err := graph.ProjectStats(ctx, st, project)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NodesByType["file"] == 0 {
		t.Fatalf("expected graph file nodes, got %#v", stats.NodesByType)
	}
}

func TestMCPGraphRebuildToolAndNeighborsCompaction(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc HandlePayment(){ Save() }\nfunc Save(){}\n"), 0o644)
	st, err := store.Open(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	p, _ := st.AddProject(ctx, "demo", root)
	indexer.IndexProject(ctx, st, p)

	rebuild := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ctxd_graph_rebuild","arguments":{"project":"demo"}}}` + "\n"
	payload := callMCP(t, st, rebuild)
	if payload["graph_rebuilt"] != true {
		t.Fatalf("expected graph rebuild payload, got %#v", payload)
	}

	neighbors := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ctxd_graph_neighbors","arguments":{"project":"demo","query":"pay.go","max_nodes":1,"max_edges":1}}}` + "\n"
	payload = callMCP(t, st, neighbors)
	if payload["truncated"] != true {
		t.Fatalf("expected truncated neighbors, got %#v", payload)
	}
	nodes, ok := payload["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("expected one node, got %#v", payload["nodes"])
	}
	node := nodes[0].(map[string]any)
	if _, ok := node["metadata_json"]; ok {
		t.Fatalf("expected metadata_json to be omitted by default, got %#v", node)
	}
}

func callMCP(t *testing.T, st *store.Store, input string) map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(context.Background(), st, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if env.Error != nil {
		t.Fatalf("MCP error: %#v", env.Error)
	}
	if len(env.Result.Content) == 0 {
		t.Fatal("empty content")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(env.Result.Content[0].Text), &payload); err != nil {
		t.Fatalf("bad payload: %v\n%s", err, env.Result.Content[0].Text)
	}
	return payload
}
