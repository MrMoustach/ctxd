package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/issam/ctxd/internal/chunker"
	"github.com/issam/ctxd/internal/contextpack"
	"github.com/issam/ctxd/internal/graph"
	"github.com/issam/ctxd/internal/indexer"
	"github.com/issam/ctxd/internal/output"
	"github.com/issam/ctxd/internal/search"
	"github.com/issam/ctxd/internal/store"
	"github.com/issam/ctxd/internal/summary"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func Serve(ctx context.Context, st *store.Store, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10 MB max line
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			continue
		}
		res := response{JSONRPC: "2.0", ID: req.ID}
		v, err := handle(ctx, st, req)
		if err != nil {
			res.Error = map[string]any{"code": -32000, "message": err.Error()}
		} else {
			res.Result = v
		}
		if req.ID != nil {
			if err := enc.Encode(res); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func handle(ctx context.Context, st *store.Store, req request) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "ctx", "version": "0.1.0"}, "capabilities": map[string]any{"tools": map[string]any{}}}, nil
	case "tools/list":
		return map[string]any{"tools": []map[string]any{
			tool("ctxd_project_map", "List registered ctxd projects and basic project metadata", schema(nil, map[string]any{})),
			tool("ctxd_search", "Search indexed code. Use ctxd_context before implementation work when possible.", schema(
				[]string{"project", "query"},
				map[string]any{
					"project":     strProp("project name"),
					"query":       strProp("search query"),
					"limit":       intProp("raw FTS fetch limit (default 20)"),
					"mode":        strProp("output mode: compact (default) | raw | summary"),
					"max_results": intProp("compact/summary: max results (default 5)"),
					"max_files":   intProp("compact/summary: max files (default 3)"),
					"max_lines":   intProp("compact: max lines per result (default 40)"),
					"max_chars":   intProp("compact: max total chars (default 12000)"),
				},
			)),
			tool("ctxd_context", "Build a graph-aware markdown context pack. Call this before implementation work, code changes, or manual repository search.", schema(
				[]string{"project"},
				map[string]any{
					"project":     strProp("project name"),
					"task":        strProp("task description to focus context"),
					"max_tokens":  intProp("max tokens for context pack"),
					"graph":       boolProp("enable graph expansion; defaults on when graph data exists"),
					"graph_depth": intProp("graph expansion depth"),
				},
			)),
			tool("ctxd_read_files", "Read exact project-relative file contents after ctxd_context identifies relevant files", schema(
				[]string{"project", "paths"},
				map[string]any{
					"project": strProp("project name"),
					"paths":   arrProp("project-relative file paths to read"),
				},
			)),
			tool("reindex_project", "Reindex a project", schema(
				[]string{"project"},
				map[string]any{
					"project": strProp("project name"),
					"graph":   boolProp("rebuild graph after indexing; defaults true"),
				},
			)),
			tool("ctxd_graph_rebuild", "Rebuild graph nodes and edges for a project", schema([]string{"project"}, map[string]any{"project": strProp("project name")})),
			tool("ctxd_graph_neighbors", "Inspect related graph symbols/files for a symbol or file", schema([]string{"project", "query"}, map[string]any{"project": strProp("project name"), "query": strProp("symbol or file"), "depth": intProp("graph depth"), "max_nodes": intProp("max nodes to return (default 40)"), "max_edges": intProp("max edges to return (default 80)"), "types": arrProp("optional node types to include"), "include_metadata": boolProp("include metadata_json fields; defaults false")})),
			tool("ctxd_graph_path", "Find a graph path between two symbols or files", schema([]string{"project", "from", "to"}, map[string]any{"project": strProp("project name"), "from": strProp("start symbol or file"), "to": strProp("target symbol or file")})),
			tool("ctxd_graph_stats", "Return graph statistics for a project", schema([]string{"project"}, map[string]any{"project": strProp("project name")})),
			tool("ctxd_graph_report", "Generate .ctxd/GRAPH_REPORT.md for a project", schema([]string{"project"}, map[string]any{"project": strProp("project name")})),
			tool("list_projects", "Alias for ctxd_project_map", schema(nil, map[string]any{})),
			tool("search_code", "Alias for ctxd_search", schema([]string{"project", "query"}, map[string]any{"project": strProp("project name"), "query": strProp("search query"), "limit": intProp("max results")})),
			tool("get_context", "Alias for ctxd_context", schema([]string{"project"}, map[string]any{"project": strProp("project name"), "task": strProp("task"), "max_tokens": intProp("max tokens")})),
			tool("read_files", "Alias for ctxd_read_files", schema([]string{"project", "paths"}, map[string]any{"project": strProp("project name"), "paths": arrProp("paths")})),
		}}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		return callTool(ctx, st, p.Name, p.Arguments)
	default:
		return map[string]any{}, nil
	}
}

func tool(name, desc string, schema map[string]any) map[string]any {
	return map[string]any{"name": name, "description": desc, "inputSchema": schema}
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func arrProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}
func schema(required []string, props map[string]any) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "required": required, "properties": props}
}

func callTool(ctx context.Context, st *store.Store, name string, raw json.RawMessage) (any, error) {
	switch name {
	case "list_projects", "ctxd_project_map":
		ps, err := st.Projects(ctx)
		result := content(map[string]any{"projects": ps})
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, err
	case "search_code", "ctxd_search":
		var a struct {
			Project    string `json:"project"`
			Query      string `json:"query"`
			Limit      int    `json:"limit"`
			Mode       string `json:"mode"`
			MaxResults int    `json:"max_results"`
			MaxFiles   int    `json:"max_files"`
			MaxLines   int    `json:"max_lines"`
			MaxChars   int    `json:"max_chars"`
		}
		json.Unmarshal(raw, &a)
		if a.Limit <= 0 {
			a.Limit = 20
		}
		if a.Mode == "" {
			a.Mode = output.ModeCompact
		}
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		rs, err := search.Search(ctx, st, p, a.Query, a.Limit)
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, r := range rs {
			paths = append(paths, r.Path)
		}
		var result any
		if a.Mode == output.ModeRaw {
			result = content(map[string]any{"mode": output.ModeRaw, "results": rs})
		} else if a.Mode == output.ModeSummary {
			opts := output.CompactOptions{
				MaxResults: a.MaxResults,
				MaxFiles:   a.MaxFiles,
			}
			co := output.ApplyCompact(rs, opts)
			seen := map[string]bool{}
			terms := search.Terms(a.Query)
			var summaries []summary.FileSummary
			for _, r := range co.Results {
				if seen[r.Path] {
					continue
				}
				seen[r.Path] = true
				c, err := st.FileContent(ctx, p, r.Path)
				if err != nil {
					continue
				}
				summaries = append(summaries, summary.Summarize(r.Path, c, output.LangFromPath(r.Path), terms))
			}
			result = content(map[string]any{"mode": output.ModeSummary, "summaries": summaries})
		} else {
			opts := output.CompactOptions{
				MaxResults:        a.MaxResults,
				MaxFiles:          a.MaxFiles,
				MaxLinesPerResult: a.MaxLines,
				MaxTotalChars:     a.MaxChars,
			}
			co := output.ApplyCompact(rs, opts)
			result = content(co)
		}
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{
			Tool:          name,
			Project:       a.Project,
			CalledAt:      time.Now(),
			TokensWithout: fileTokens(p.RootPath, paths),
			TokensActual:  responseTokens(result),
		})
		return result, nil
	case "get_context", "ctxd_context":
		var a struct {
			Project    string `json:"project"`
			Task       string `json:"task"`
			MaxTokens  int    `json:"max_tokens"`
			Graph      bool   `json:"graph"`
			GraphDepth int    `json:"graph_depth"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		graphEnabled := a.Graph || graph.HasGraphData(ctx, st, p.ID)
		md, paths, err := contextpack.BuildWithOptions(ctx, st, p, a.Task, contextpack.Options{MaxTokens: a.MaxTokens, Graph: graphEnabled, GraphDepth: a.GraphDepth})
		tokenEstimate := chunker.EstimateTokens(md)
		maxTokens := a.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 12000
		}
		result := content(map[string]any{"markdown": md, "paths": paths, "token_estimate": tokenEstimate, "truncated": tokenEstimate >= maxTokens})
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{
			Tool:          name,
			Project:       a.Project,
			CalledAt:      time.Now(),
			TokensWithout: fileTokens(p.RootPath, paths),
			TokensActual:  responseTokens(result),
		})
		return result, err
	case "read_files", "ctxd_read_files":
		var a struct {
			Project string   `json:"project"`
			Paths   []string `json:"paths"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		var files []map[string]string
		for _, path := range a.Paths {
			c, err := st.FileContent(ctx, p, path)
			if err != nil {
				return nil, err
			}
			files = append(files, map[string]string{"path": path, "content": c})
		}
		result := content(map[string]any{"files": files})
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, Project: a.Project, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, nil
	case "reindex_project":
		var a struct {
			Project string `json:"project"`
			Graph   *bool  `json:"graph"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		r, err := indexer.IndexProject(ctx, st, p)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{"status": "ok", "indexed_files": r.IndexedFiles, "indexed_chunks": r.IndexedChunks}
		if a.Graph == nil || *a.Graph {
			stats, err := graph.Rebuild(ctx, st, p)
			if err != nil {
				return nil, err
			}
			payload["graph_rebuilt"] = true
			payload["graph_files"] = stats.Files
			payload["graph_symbols"] = stats.Symbols
			payload["graph_edges"] = stats.Edges
		} else {
			payload["graph_rebuilt"] = false
		}
		result := content(payload)
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, Project: a.Project, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, nil
	case "ctxd_graph_rebuild":
		var a struct {
			Project string `json:"project"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		stats, err := graph.Rebuild(ctx, st, p)
		result := content(map[string]any{"status": "ok", "graph_rebuilt": true, "graph_files": stats.Files, "graph_symbols": stats.Symbols, "graph_edges": stats.Edges})
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, Project: a.Project, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, err
	case "ctxd_graph_neighbors":
		var a struct {
			Project         string   `json:"project"`
			Query           string   `json:"query"`
			Depth           int      `json:"depth"`
			MaxNodes        int      `json:"max_nodes"`
			MaxEdges        int      `json:"max_edges"`
			Types           []string `json:"types"`
			IncludeMetadata bool     `json:"include_metadata"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		nodes, edges, err := graph.Neighbors(ctx, st, p, a.Query, a.Depth)
		nodes, edges, truncated := compactGraphNeighbors(nodes, edges, a.MaxNodes, a.MaxEdges, a.Types, a.IncludeMetadata)
		result := content(map[string]any{"nodes": nodes, "edges": edges, "truncated": truncated})
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, Project: a.Project, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, err
	case "ctxd_graph_path":
		var a struct {
			Project string `json:"project"`
			From    string `json:"from"`
			To      string `json:"to"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		nodes, edges, err := graph.Path(ctx, st, p, a.From, a.To)
		result := content(map[string]any{"nodes": nodes, "edges": edges})
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, Project: a.Project, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, err
	case "ctxd_graph_stats":
		var a struct {
			Project string `json:"project"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		stats, err := graph.ProjectStats(ctx, st, p)
		result := content(stats)
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, Project: a.Project, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, err
	case "ctxd_graph_report":
		var a struct {
			Project string `json:"project"`
		}
		json.Unmarshal(raw, &a)
		p, err := st.ProjectByName(ctx, a.Project)
		if err != nil {
			return nil, err
		}
		path, err := graph.WriteReport(ctx, st, p)
		result := content(map[string]any{"path": path})
		tok := responseTokens(result)
		_ = st.AddAnalytics(ctx, store.AnalyticsRecord{Tool: name, Project: a.Project, CalledAt: time.Now(), TokensWithout: tok, TokensActual: tok})
		return result, err
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func fileTokens(rootPath string, paths []string) int {
	seen := map[string]bool{}
	total := 0
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		b, err := os.ReadFile(filepath.Join(rootPath, p))
		if err != nil {
			continue
		}
		total += chunker.EstimateTokens(string(b))
	}
	return total
}

func responseTokens(v any) int {
	b, _ := json.Marshal(v)
	return chunker.EstimateTokens(string(b))
}

func compactGraphNeighbors(nodes []graph.Node, edges []graph.Edge, maxNodes, maxEdges int, types []string, includeMetadata bool) ([]graph.Node, []graph.Edge, bool) {
	if maxNodes <= 0 {
		maxNodes = 40
	}
	if maxEdges <= 0 {
		maxEdges = 80
	}
	allowedTypes := map[string]bool{}
	for _, typ := range types {
		allowedTypes[typ] = true
	}
	filterByType := len(allowedTypes) > 0
	keptIDs := map[int64]bool{}
	var outNodes []graph.Node
	truncated := false
	for _, n := range nodes {
		if filterByType && !allowedTypes[n.Type] {
			continue
		}
		if !includeMetadata {
			n.MetadataJSON = ""
		}
		if len(outNodes) >= maxNodes {
			truncated = true
			break
		}
		keptIDs[n.ID] = true
		outNodes = append(outNodes, n)
	}
	var outEdges []graph.Edge
	for _, e := range edges {
		if !keptIDs[e.FromNodeID] || !keptIDs[e.ToNodeID] {
			truncated = true
			continue
		}
		if !includeMetadata {
			e.MetadataJSON = ""
		}
		if len(outEdges) >= maxEdges {
			truncated = true
			break
		}
		outEdges = append(outEdges, e)
	}
	return outNodes, outEdges, truncated
}

func content(v any) any {
	b, _ := json.Marshal(v)
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}}
}
