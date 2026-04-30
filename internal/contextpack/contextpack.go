package contextpack

import (
	"context"
	"fmt"
	"strings"

	"github.com/issam/ctxd/internal/chunker"
	"github.com/issam/ctxd/internal/graph"
	"github.com/issam/ctxd/internal/search"
	"github.com/issam/ctxd/internal/store"
)

func Build(ctx context.Context, st *store.Store, project store.Project, task string, maxTokens int) (string, []string, error) {
	return BuildWithOptions(ctx, st, project, task, Options{MaxTokens: maxTokens, Graph: graph.HasGraphData(ctx, st, project.ID), GraphDepth: 1})
}

type Options struct {
	MaxTokens  int
	Graph      bool
	GraphDepth int
}

func BuildWithOptions(ctx context.Context, st *store.Store, project store.Project, task string, opts Options) (string, []string, error) {
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 12000
	}
	results, err := search.Search(ctx, st, project, task, 20)
	if err != nil {
		return "", nil, err
	}
	var related []graph.Related
	var edges []graph.Edge
	if opts.Graph && graph.HasGraphData(ctx, st, project.ID) {
		related, edges, _ = graph.ExpandFromSearch(ctx, st, project, task, results, opts.GraphDepth)
	}
	var b strings.Builder
	used := 0
	add := func(s string) bool {
		t := chunker.EstimateTokens(s)
		if used+t > maxTokens {
			return false
		}
		used += t
		b.WriteString(s)
		return true
	}
	add(fmt.Sprintf("# ctxd Context Pack\n\n## Task\n%s\n\n## Project\n%s\n\n## Summary\nRelevant files were selected from local FTS matches, symbol matches, and graph relationships when available.\n\n", task, project.Name))
	if warn := graphStalenessWarning(ctx, st, project); warn != "" {
		add(warn)
	}
	add("## Direct Matches\n\n")
	seen := map[string]bool{}
	var matchedPaths []string
	for _, r := range results {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		matchedPaths = append(matchedPaths, r.Path)
		section := fmt.Sprintf("### %s\nReason: %s.\nLines: %d-%d\n\n```txt\n%s\n```\n\n", r.Path, r.Reason, r.StartLine, r.EndLine, r.Snippet)
		if !add(section) {
			break
		}
	}
	seenRelated := map[string]bool{}
	add("## Graph-Expanded Related Files\n\n")
	if len(related) == 0 {
		add("- None\n\n")
	} else {
		wrote := false
		for _, rr := range related {
			n := rr.Node
			if seenRelated[n.FilePath] || seen[n.FilePath] {
				continue
			}
			seenRelated[n.FilePath] = true
			matchedPaths = append(matchedPaths, n.FilePath)
			wrote = true
			if !add(fmt.Sprintf("- %s (via %s `%s`, score %.2f)\n", n.FilePath, n.Type, n.Name, rr.Score)) {
				break
			}
		}
		if !wrote {
			add("- None beyond direct matches\n")
		}
		add("\n")
	}
	add("## Relevant Symbols\n\n")
	if len(related) == 0 {
		add("- None\n\n")
	} else {
		count := 0
		for _, rr := range related {
			n := rr.Node
			if n.Type == "file" {
				continue
			}
			if !add(fmt.Sprintf("- `%s` %s in %s:%d (%s)\n", n.Name, n.Type, n.FilePath, n.StartLine, rr.Reason)) {
				break
			}
			count++
			if count >= 20 {
				break
			}
		}
		add("\n")
	}
	add("## Call/Import Relationships\n\n")
	if len(edges) == 0 {
		add("- None\n\n")
	} else {
		for i, e := range edges {
			if i >= 30 {
				break
			}
			if !add(fmt.Sprintf("- %s: %d -> %d\n", e.Type, e.FromNodeID, e.ToNodeID)) {
				break
			}
		}
		add("\n")
	}
	add("## Files/Snippets\n\n")
	if len(seenRelated) == 0 {
		add("- See direct matches above.\n\n")
	} else {
		for path := range seenRelated {
			if seen[path] {
				continue
			}
			content, err := st.FileContent(ctx, project, path)
			if err != nil {
				continue
			}
			snip := firstLines(content, 80)
			if !add(fmt.Sprintf("### %s\nReason: graph-expanded related file.\n\n```txt\n%s\n```\n\n", path, snip)) {
				break
			}
		}
	}
	add("## Implementation Notes\n\n- Reuse existing code paths found above before adding new behavior.\n- Add tests around changed indexing, search, and MCP behavior.\n")
	return b.String(), matchedPaths, nil
}

func firstLines(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > max {
		lines = lines[:max]
	}
	return strings.Join(lines, "\n")
}

func graphStalenessWarning(ctx context.Context, st *store.Store, project store.Project) string {
	if project.GraphBuiltAt == "" {
		return "> **WARNING: Graph not built.** Call `ctxd_graph_rebuild` then `reindex_project` before implementation work.\n\n"
	}
	var changed int
	_ = st.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE project_id=? AND mtime > ?`,
		project.ID, project.GraphBuiltAt,
	).Scan(&changed)
	if changed > 0 {
		return fmt.Sprintf("> **WARNING: Graph is stale** (%d file(s) modified since last build). Call `ctxd_graph_rebuild` then `reindex_project` before implementation work.\n\n", changed)
	}
	return ""
}
