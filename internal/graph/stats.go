package graph

import (
	"context"
	"database/sql"

	"github.com/issam/ctxd/internal/store"
)

func ProjectStats(ctx context.Context, st *store.Store, project store.Project) (Stats, error) {
	stats := Stats{
		Project:     project.Name,
		Languages:   map[string]int{},
		NodesByType: map[string]int{},
		EdgesByType: map[string]int{},
	}
	rows, err := st.DB.QueryContext(ctx, `SELECT language,COUNT(*) FROM files WHERE project_id=? GROUP BY language`, project.ID)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var lang string
		var count int
		if err := rows.Scan(&lang, &count); err != nil {
			rows.Close()
			return stats, err
		}
		stats.Languages[lang] = count
		stats.Files += count
	}
	rows.Close()
	rows, err = st.DB.QueryContext(ctx, `SELECT type,COUNT(*) FROM graph_nodes WHERE project_id=? GROUP BY type`, project.ID)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var typ string
		var count int
		if err := rows.Scan(&typ, &count); err != nil {
			rows.Close()
			return stats, err
		}
		stats.NodesByType[typ] = count
		if typ != "file" {
			stats.Symbols += count
		}
		stats.HasGraphData = true
	}
	rows.Close()
	rows, err = st.DB.QueryContext(ctx, `SELECT type,COUNT(*) FROM graph_edges WHERE project_id=? GROUP BY type`, project.ID)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var typ string
		var count int
		if err := rows.Scan(&typ, &count); err != nil {
			rows.Close()
			return stats, err
		}
		stats.EdgesByType[typ] = count
		stats.Edges += count
	}
	rows.Close()
	stats.TopFiles, _ = topConnected(ctx, st, project.ID, `AND n.type='file'`, 10)
	stats.TopSymbols, _ = topConnected(ctx, st, project.ID, `AND n.type<>'file'`, 10)
	stats.HighCoupling, _ = topConnected(ctx, st, project.ID, `AND n.type='file'`, 10)
	stats.CallHotspots, _ = topCalled(ctx, st, project.ID, 10)
	stats.OrphanFiles, _ = orphanFiles(ctx, st, project.ID)
	stats.Routes, _ = nodesByType(ctx, st, project.ID, "route", 50)
	stats.Services, _ = nodesByType(ctx, st, project.ID, "service", 50)
	stats.Models, _ = nodesByType(ctx, st, project.ID, "model", 50)
	stats.Jobs, _ = nodesByType(ctx, st, project.ID, "job", 50)
	stats.Commands, _ = nodesByType(ctx, st, project.ID, "command", 50)
	stats.Tests, _ = nodesByType(ctx, st, project.ID, "test", 50)
	stats.GraphBuiltAt = project.GraphBuiltAt
	return stats, nil
}

func topCalled(ctx context.Context, st *store.Store, projectID int64, limit int) ([]Connected, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT n.id,n.name,n.type,n.file_path,COUNT(e.id) c FROM graph_nodes n JOIN graph_edges e ON e.to_node_id=n.id WHERE n.project_id=? AND e.type='calls' GROUP BY n.id,n.name,n.type,n.file_path ORDER BY c DESC,n.name LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connected
	for rows.Next() {
		var c Connected
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.FilePath, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func nodesByType(ctx context.Context, st *store.Store, projectID int64, typ string, limit int) ([]Node, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE project_id=? AND type=? ORDER BY file_path,start_line LIMIT ?`, projectID, typ, limit)
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

func orphanFiles(ctx context.Context, st *store.Store, projectID int64) ([]string, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT n.file_path,COUNT(e.id) c FROM graph_nodes n LEFT JOIN graph_edges e ON e.project_id=n.project_id AND (e.from_node_id=n.id OR e.to_node_id=n.id) WHERE n.project_id=? AND n.type='file' GROUP BY n.id,n.file_path HAVING c=0 ORDER BY n.file_path LIMIT 50`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var path string
		var c sql.NullInt64
		if err := rows.Scan(&path, &c); err != nil {
			return nil, err
		}
		out = append(out, path)
	}
	return out, rows.Err()
}
