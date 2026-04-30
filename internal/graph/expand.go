package graph

import (
	"context"
	"sort"
	"strings"

	"github.com/issam/ctxd/internal/search"
	"github.com/issam/ctxd/internal/store"
)

type Related struct {
	Node     Node
	Score    float64
	Distance int
	Reason   string
}

func ExpandFromSearch(ctx context.Context, st *store.Store, project store.Project, task string, direct []search.Result, depth int) ([]Related, []Edge, error) {
	if depth <= 0 {
		depth = 1
	}
	scores := map[int64]Related{}
	var allEdges []Edge
	terms := search.Terms(task)
	for _, r := range direct {
		seeds, err := seedNodes(ctx, st, project.ID, r.Path, terms)
		if err != nil {
			return nil, nil, err
		}
		for _, seed := range seeds {
			boost := 5.0
			if strings.EqualFold(seed.Name, task) || strings.EqualFold(seed.QualifiedName, task) {
				boost = 10
			}
			putRelated(scores, seed, r.Score+boost, 0, "direct FTS/symbol match")
			nodes, edges, err := Neighbors(ctx, st, project, nodeQuery(seed), depth)
			if err != nil {
				continue
			}
			allEdges = append(allEdges, edges...)
			for _, n := range nodes {
				dist := 1
				if n.ID == seed.ID {
					dist = 0
				}
				score := r.Score + graphDistanceBoost(dist)
				putRelated(scores, n, score, dist, "graph neighbor")
			}
		}
	}
	for _, term := range terms {
		nodes, err := symbolMatches(ctx, st, project.ID, term)
		if err != nil {
			return nil, nil, err
		}
		for _, n := range nodes {
			putRelated(scores, n, 8, 0, "symbol match")
			ns, es, _ := Neighbors(ctx, st, project, nodeQuery(n), depth)
			allEdges = append(allEdges, es...)
			for _, rn := range ns {
				putRelated(scores, rn, 4, 1, "symbol graph neighbor")
			}
		}
	}
	out := make([]Related, 0, len(scores))
	for _, r := range scores {
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 50 {
		out = out[:50]
	}
	return out, dedupeEdges(allEdges), nil
}

func seedNodes(ctx context.Context, st *store.Store, projectID int64, path string, terms []string) ([]Node, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE project_id=? AND file_path=? ORDER BY type='file' DESC,start_line LIMIT 25`, projectID, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Type, &n.Name, &n.QualifiedName, &n.FilePath, &n.StartLine, &n.EndLine, &n.MetadataJSON); err != nil {
			return nil, err
		}
		if n.Type == "file" || matchesTerms(n, terms) {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

func symbolMatches(ctx context.Context, st *store.Store, projectID int64, term string) ([]Node, error) {
	like := "%" + term + "%"
	rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE project_id=? AND type<>'file' AND (lower(name) LIKE lower(?) OR lower(qualified_name) LIKE lower(?)) ORDER BY length(name) LIMIT 10`, projectID, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Type, &n.Name, &n.QualifiedName, &n.FilePath, &n.StartLine, &n.EndLine, &n.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func putRelated(m map[int64]Related, n Node, score float64, dist int, reason string) {
	if old, ok := m[n.ID]; ok && old.Score >= score {
		return
	}
	m[n.ID] = Related{Node: n, Score: score, Distance: dist, Reason: reason}
}

func matchesTerms(n Node, terms []string) bool {
	low := strings.ToLower(n.Name + " " + n.QualifiedName + " " + n.FilePath)
	for _, t := range terms {
		if strings.Contains(low, strings.ToLower(t)) {
			return true
		}
	}
	return false
}

func graphDistanceBoost(d int) float64 {
	switch d {
	case 0:
		return 6
	case 1:
		return 3.5
	default:
		return 2
	}
}

func nodeQuery(n Node) string {
	if n.QualifiedName != "" {
		return n.QualifiedName
	}
	if n.Type == "file" {
		return n.FilePath
	}
	return n.Name
}
