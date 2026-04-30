package search

import (
	"context"
	"database/sql"
	"regexp"
	"sort"
	"strings"

	"github.com/issam/ctxd/internal/store"
)

type Result struct {
	Path      string  `json:"path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float64 `json:"score"`
	Reason    string  `json:"reason"`
	Snippet   string  `json:"snippet"`
}

var synonyms = map[string][]string{
	"payment":  {"paid", "transaction", "wallet", "invoice", "billing", "balance"},
	"checkout": {"check_out", "departure", "end_date"},
	"anomaly":  {"issue", "ticket", "missing", "unresolved", "not_done"},
	"sync":     {"synchronize", "import", "fetch", "pull", "push"},
}

var wordRE = regexp.MustCompile(`[A-Za-z0-9_]+`)

func Search(ctx context.Context, st *store.Store, project store.Project, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 10
	}
	terms := Terms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	fts := strings.Join(terms, " OR ")
	rows, err := st.DB.QueryContext(ctx, `SELECT f.path,c.start_line,c.end_line,c.content,bm25(chunks_fts) AS rank FROM chunks_fts JOIN chunks c ON c.id=chunks_fts.rowid JOIN files f ON f.id=c.file_id WHERE c.project_id=? AND chunks_fts MATCH ? ORDER BY rank LIMIT ?`, project.ID, fts, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var r Result
		var rank sql.NullFloat64
		if err := rows.Scan(&r.Path, &r.StartLine, &r.EndLine, &r.Snippet, &rank); err != nil {
			return nil, err
		}
		r.Score = score(query, terms, r.Path, r.Snippet, rank.Float64)
		r.Reason = reason(query, terms, r.Path, r.Snippet)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func Terms(q string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range wordRE.FindAllString(strings.ToLower(q), -1) {
		if len(t) < 2 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		for _, s := range synonyms[t] {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func score(query string, terms []string, path, snippet string, bm25 float64) float64 {
	lowPath, lowSnip, lowQ := strings.ToLower(path), strings.ToLower(snippet), strings.ToLower(query)
	score := -bm25
	for _, t := range terms {
		if strings.Contains(lowPath, t) {
			score += 2
		}
		if strings.Contains(filepathBase(lowPath), t) {
			score += 2
		}
		if strings.Contains(lowSnip, t) {
			score += 0.3
		}
	}
	if lowQ != "" && strings.Contains(lowSnip, lowQ) {
		score += 2.5
	}
	return score
}

func reason(query string, terms []string, path, snippet string) string {
	var parts []string
	lowPath, lowSnip := strings.ToLower(path), strings.ToLower(snippet)
	for _, t := range terms {
		if strings.Contains(lowPath, t) {
			parts = append(parts, "path match")
			break
		}
	}
	for _, t := range terms {
		if strings.Contains(lowSnip, t) {
			parts = append(parts, "FTS match")
			break
		}
	}
	if strings.Contains(lowSnip, strings.ToLower(query)) {
		parts = append(parts, "exact phrase")
	}
	if len(parts) == 0 {
		return "FTS rank"
	}
	return strings.Join(parts, " + ")
}

func filepathBase(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return p
	}
	return p[i+1:]
}
