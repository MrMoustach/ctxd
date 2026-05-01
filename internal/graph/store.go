package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/issam/ctxd/internal/store"
)

type edgeCache struct {
	Imports []string `json:"i,omitempty"`
	Calls   []Call   `json:"c,omitempty"`
	Routes  []Route  `json:"r,omitempty"`
	Uses    []Use    `json:"u,omitempty"`
}

func Rebuild(ctx context.Context, st *store.Store, project store.Project) (Stats, error) {
	files, err := indexedFiles(ctx, st, project.ID)
	if err != nil {
		return Stats{}, err
	}

	hasGraph := project.GraphBuiltAt != "" && HasGraphData(ctx, st, project.ID)

	type workItem struct {
		file    store.FileRecord
		changed bool
	}
	items := make([]workItem, len(files))
	anyChanged := !hasGraph
	for i, f := range files {
		changed := !hasGraph || f.IndexedAt > project.GraphBuiltAt
		items[i] = workItem{f, changed}
		if changed {
			anyChanged = true
		}
	}
	if !anyChanged {
		return ProjectStats(ctx, st, project)
	}

	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		return Stats{}, err
	}

	// Always rebuild all edges; only delete nodes for changed files.
	if _, err := tx.ExecContext(ctx, `DELETE FROM graph_edges WHERE project_id=?`, project.ID); err != nil {
		tx.Rollback()
		return Stats{}, err
	}
	for _, w := range items {
		if w.changed {
			if _, err := tx.ExecContext(ctx, `DELETE FROM graph_nodes WHERE project_id=? AND file_path=?`, project.ID, w.file.Path); err != nil {
				tx.Rollback()
				return Stats{}, err
			}
		}
	}

	nodesByKey := map[string]Node{}
	edgesByPath := map[string]edgeCache{}

	for _, w := range items {
		if !w.changed {
			// Load existing nodes from DB (not deleted).
			rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE project_id=? AND file_path=?`, project.ID, w.file.Path)
			if err != nil {
				tx.Rollback()
				return Stats{}, err
			}
			for rows.Next() {
				var n Node
				if err := rows.Scan(&n.ID, &n.ProjectID, &n.Type, &n.Name, &n.QualifiedName, &n.FilePath, &n.StartLine, &n.EndLine, &n.MetadataJSON); err != nil {
					rows.Close()
					tx.Rollback()
					return Stats{}, err
				}
				nodesByKey[nodeKey(n)] = n
			}
			rows.Close()

			// Load edge data from parse cache.
			var cachedHash, cachedData string
			cacheHit := st.DB.QueryRowContext(ctx, `SELECT hash,data FROM graph_parsed WHERE project_id=? AND file_id=?`, project.ID, w.file.ID).Scan(&cachedHash, &cachedData) == nil && cachedHash == w.file.Hash
			if cacheHit {
				var ec edgeCache
				if json.Unmarshal([]byte(cachedData), &ec) == nil {
					edgesByPath[w.file.Path] = ec
					continue
				}
			}
			// Cache miss: re-read and parse once to warm cache.
			b, rerr := os.ReadFile(w.file.AbsPath)
			if rerr != nil {
				continue
			}
			pf := ParseFile(w.file.Path, w.file.Language, string(b))
			ec := edgeCache{Imports: pf.Imports, Calls: pf.Calls, Routes: pf.Routes, Uses: pf.Uses}
			edgesByPath[w.file.Path] = ec
			if data, jerr := json.Marshal(ec); jerr == nil {
				_, _ = tx.ExecContext(ctx, `INSERT INTO graph_parsed(project_id,file_id,hash,data) VALUES(?,?,?,?) ON CONFLICT(project_id,file_id) DO UPDATE SET hash=excluded.hash,data=excluded.data`, project.ID, w.file.ID, w.file.Hash, string(data))
			}
		} else {
			// Parse changed file, insert fresh nodes.
			b, rerr := os.ReadFile(w.file.AbsPath)
			if rerr != nil {
				continue
			}
			pf := ParseFile(w.file.Path, w.file.Language, string(b))
			for _, n := range pf.Nodes {
				id, ierr := insertNodeTx(ctx, tx, project.ID, n)
				if ierr != nil {
					tx.Rollback()
					return Stats{}, ierr
				}
				n.ID = id
				nodesByKey[nodeKey(n)] = n
			}
			ec := edgeCache{Imports: pf.Imports, Calls: pf.Calls, Routes: pf.Routes, Uses: pf.Uses}
			edgesByPath[w.file.Path] = ec
			if data, jerr := json.Marshal(ec); jerr == nil {
				_, _ = tx.ExecContext(ctx, `INSERT INTO graph_parsed(project_id,file_id,hash,data) VALUES(?,?,?,?) ON CONFLICT(project_id,file_id) DO UPDATE SET hash=excluded.hash,data=excluded.data`, project.ID, w.file.ID, w.file.Hash, string(data))
			}
		}
	}

	// Rebuild defines edges for all non-file nodes already in nodesByKey.
	// (Route nodes from changed files are inserted below and get their defines edge there.)
	for _, n := range nodesByKey {
		if n.Type != "file" {
			if fileNode, ok := nodesByKey["file:"+n.FilePath]; ok {
				_ = insertEdgeTx(ctx, tx, project.ID, fileNode.ID, n.ID, "defines", 1, "")
			}
		}
	}

	// Rebuild all cross-file edges.
	for filePath, ec := range edgesByPath {
		fileNode, ok := nodesByKey["file:"+filePath]
		if !ok {
			continue
		}
		for _, imported := range ec.Imports {
			if to := resolveImport(nodesByKey, imported); to.ID != 0 {
				_ = insertEdgeTx(ctx, tx, project.ID, fileNode.ID, to.ID, "imports", 0.8, meta("import", imported))
			}
		}
		for _, r := range ec.Routes {
			routeNode := Node{Type: "route", Name: r.Method + " " + r.URI, QualifiedName: r.Method + " " + r.URI, FilePath: filePath, StartLine: r.Line, EndLine: r.Line, MetadataJSON: meta("route", r.Raw)}
			rk := nodeKey(routeNode)
			if existing, exists := nodesByKey[rk]; exists {
				routeNode = existing
			} else {
				id, ierr := insertNodeTx(ctx, tx, project.ID, routeNode)
				if ierr != nil {
					tx.Rollback()
					return Stats{}, ierr
				}
				routeNode.ID = id
				nodesByKey[rk] = routeNode
				_ = insertEdgeTx(ctx, tx, project.ID, fileNode.ID, routeNode.ID, "defines", 1, "")
			}
			target := r.Controller + "." + r.Action
			if to := resolveSymbol(nodesByKey, target, r.Action); to.ID != 0 {
				_ = insertEdgeTx(ctx, tx, project.ID, routeNode.ID, to.ID, "route_to", 1, meta("controller", target))
			}
		}
		for _, c := range ec.Calls {
			from := resolveSymbol(nodesByKey, c.FromName, "")
			if from.ID == 0 {
				from = fileNode
			}
			if to := resolveSymbol(nodesByKey, c.ToName, c.ToName); to.ID != 0 && to.ID != from.ID {
				_ = insertEdgeTx(ctx, tx, project.ID, from.ID, to.ID, "calls", 0.65, meta("call", c.Raw))
			}
		}
		for _, u := range ec.Uses {
			from := resolveSymbol(nodesByKey, u.FromName, "")
			if from.ID == 0 {
				from = fileNode
			}
			if to := resolveSymbol(nodesByKey, u.ToName, u.ToName); to.ID != 0 && to.ID != from.ID {
				_ = insertEdgeTx(ctx, tx, project.ID, from.ID, to.ID, u.Type, 0.8, meta("raw", u.Raw))
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return Stats{}, err
	}
	_ = st.SetGraphBuiltAt(ctx, project.ID, time.Now())
	return ProjectStats(ctx, st, project)
}

func HasGraphData(ctx context.Context, st *store.Store, projectID int64) bool {
	var n int
	_ = st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_nodes WHERE project_id=?`, projectID).Scan(&n)
	return n > 0
}

func Nodes(ctx context.Context, st *store.Store, projectID int64) ([]Node, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE project_id=? ORDER BY file_path,start_line,type,name`, projectID)
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

func Edges(ctx context.Context, st *store.Store, projectID int64) ([]Edge, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,from_node_id,to_node_id,type,confidence,COALESCE(metadata_json,'') FROM graph_edges WHERE project_id=? ORDER BY type,id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.FromNodeID, &e.ToNodeID, &e.Type, &e.Confidence, &e.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func Neighbors(ctx context.Context, st *store.Store, project store.Project, q string, depth int) ([]Node, []Edge, error) {
	if depth <= 0 {
		depth = 1
	}
	start, err := FindNode(ctx, st, project.ID, q)
	if err != nil {
		return nil, nil, err
	}
	seen := map[int64]bool{start.ID: true}
	frontier := []int64{start.ID}
	var nodes []Node
	var edges []Edge
	nodes = append(nodes, start)
	for d := 0; d < depth; d++ {
		next := []int64{}
		for _, id := range frontier {
			rows, err := st.DB.QueryContext(ctx, `SELECT e.id,e.project_id,e.from_node_id,e.to_node_id,e.type,e.confidence,COALESCE(e.metadata_json,''), n.id,n.project_id,n.type,n.name,COALESCE(n.qualified_name,''),n.file_path,COALESCE(n.start_line,0),COALESCE(n.end_line,0),COALESCE(n.metadata_json,'') FROM graph_edges e JOIN graph_nodes n ON n.id=CASE WHEN e.from_node_id=? THEN e.to_node_id ELSE e.from_node_id END WHERE e.project_id=? AND (e.from_node_id=? OR e.to_node_id=?)`, id, project.ID, id, id)
			if err != nil {
				return nil, nil, err
			}
			for rows.Next() {
				var e Edge
				var n Node
				if err := rows.Scan(&e.ID, &e.ProjectID, &e.FromNodeID, &e.ToNodeID, &e.Type, &e.Confidence, &e.MetadataJSON, &n.ID, &n.ProjectID, &n.Type, &n.Name, &n.QualifiedName, &n.FilePath, &n.StartLine, &n.EndLine, &n.MetadataJSON); err != nil {
					rows.Close()
					return nil, nil, err
				}
				edges = append(edges, e)
				if !seen[n.ID] {
					seen[n.ID] = true
					nodes = append(nodes, n)
					next = append(next, n.ID)
				}
			}
			rows.Close()
		}
		frontier = next
	}
	return nodes, dedupeEdges(edges), nil
}

func Path(ctx context.Context, st *store.Store, project store.Project, fromQ, toQ string) ([]Node, []Edge, error) {
	from, err := FindNode(ctx, st, project.ID, fromQ)
	if err != nil {
		return nil, nil, err
	}
	to, err := FindNode(ctx, st, project.ID, toQ)
	if err != nil {
		return nil, nil, err
	}
	type step struct{ id int64 }
	queue := []step{{from.ID}}
	prev := map[int64]int64{from.ID: 0}
	prevEdge := map[int64]Edge{}
	for len(queue) > 0 && len(prev) < 1000 {
		cur := queue[0]
		queue = queue[1:]
		if cur.id == to.ID {
			break
		}
		rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,from_node_id,to_node_id,type,confidence,COALESCE(metadata_json,'') FROM graph_edges WHERE project_id=? AND (from_node_id=? OR to_node_id=?)`, project.ID, cur.id, cur.id)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var e Edge
			if err := rows.Scan(&e.ID, &e.ProjectID, &e.FromNodeID, &e.ToNodeID, &e.Type, &e.Confidence, &e.MetadataJSON); err != nil {
				rows.Close()
				return nil, nil, err
			}
			nid := e.ToNodeID
			if nid == cur.id {
				nid = e.FromNodeID
			}
			if _, ok := prev[nid]; !ok {
				prev[nid] = cur.id
				prevEdge[nid] = e
				queue = append(queue, step{nid})
			}
		}
		rows.Close()
	}
	if _, ok := prev[to.ID]; !ok {
		return nil, nil, fmt.Errorf("no graph path from %q to %q", fromQ, toQ)
	}
	var ids []int64
	var edges []Edge
	for id := to.ID; id != 0; id = prev[id] {
		ids = append(ids, id)
		if e, ok := prevEdge[id]; ok {
			edges = append(edges, e)
		}
	}
	reverseIDs(ids)
	reverseEdges(edges)
	nodes, err := nodesByIDs(ctx, st, ids)
	return nodes, edges, err
}

func FindNode(ctx context.Context, st *store.Store, projectID int64, q string) (Node, error) {
	var n Node
	err := st.DB.QueryRowContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE project_id=? AND (name=? OR qualified_name=? OR file_path=?) ORDER BY CASE WHEN name=? OR qualified_name=? THEN 0 ELSE 1 END LIMIT 1`, projectID, q, q, q, q, q).Scan(&n.ID, &n.ProjectID, &n.Type, &n.Name, &n.QualifiedName, &n.FilePath, &n.StartLine, &n.EndLine, &n.MetadataJSON)
	if err == nil {
		return n, nil
	}
	if err != sql.ErrNoRows {
		return Node{}, err
	}
	like := "%" + q + "%"
	err = st.DB.QueryRowContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE project_id=? AND (name LIKE ? OR qualified_name LIKE ? OR file_path LIKE ?) ORDER BY type='file' DESC, length(name) LIMIT 1`, projectID, like, like, like).Scan(&n.ID, &n.ProjectID, &n.Type, &n.Name, &n.QualifiedName, &n.FilePath, &n.StartLine, &n.EndLine, &n.MetadataJSON)
	if err == sql.ErrNoRows {
		return Node{}, fmt.Errorf("graph node %q not found", q)
	}
	return n, err
}

func insertNodeTx(ctx context.Context, tx *sql.Tx, projectID int64, n Node) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes(project_id,type,name,qualified_name,file_path,start_line,end_line,metadata_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, projectID, n.Type, n.Name, n.QualifiedName, n.FilePath, n.StartLine, n.EndLine, n.MetadataJSON, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func insertEdgeTx(ctx context.Context, tx *sql.Tx, projectID, fromID, toID int64, typ string, confidence float64, meta string) error {
	if fromID == 0 || toID == 0 || fromID == toID {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO graph_edges(project_id,from_node_id,to_node_id,type,confidence,metadata_json) VALUES(?,?,?,?,?,?)`, projectID, fromID, toID, typ, confidence, meta)
	return err
}

func indexedFiles(ctx context.Context, st *store.Store, projectID int64) ([]store.FileRecord, error) {
	rows, err := st.DB.QueryContext(ctx, `SELECT id,project_id,path,abs_path,language,size_bytes,mtime,hash,indexed_at FROM files WHERE project_id=? ORDER BY path`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.FileRecord
	for rows.Next() {
		var f store.FileRecord
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Path, &f.AbsPath, &f.Language, &f.SizeBytes, &f.MTime, &f.Hash, &f.IndexedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func nodeKey(n Node) string {
	if n.Type == "file" {
		return "file:" + n.FilePath
	}
	qn := n.QualifiedName
	if qn == "" {
		qn = n.Name
	}
	return n.Type + ":" + n.FilePath + ":" + qn
}

func resolveSymbol(nodes map[string]Node, qn, name string) Node {
	qn = strings.TrimSpace(qn)
	name = strings.TrimSpace(name)
	for _, n := range nodes {
		if n.Type == "file" {
			continue
		}
		if qn != "" && (n.QualifiedName == qn || n.Name == qn || strings.HasSuffix(n.QualifiedName, "."+qn)) {
			return n
		}
	}
	for _, n := range nodes {
		if n.Type != "file" && name != "" && n.Name == name {
			return n
		}
	}
	return Node{}
}

func resolveImport(nodes map[string]Node, imported string) Node {
	base := strings.TrimSuffix(filepath.Base(imported), filepath.Ext(imported))
	return resolveSymbol(nodes, base, base)
}

func dedupeEdges(edges []Edge) []Edge {
	seen := map[int64]bool{}
	out := []Edge{}
	for _, e := range edges {
		if !seen[e.ID] {
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	return out
}

func nodesByIDs(ctx context.Context, st *store.Store, ids []int64) ([]Node, error) {
	out := []Node{}
	for _, id := range ids {
		var n Node
		err := st.DB.QueryRowContext(ctx, `SELECT id,project_id,type,name,COALESCE(qualified_name,''),file_path,COALESCE(start_line,0),COALESCE(end_line,0),COALESCE(metadata_json,'') FROM graph_nodes WHERE id=?`, id).Scan(&n.ID, &n.ProjectID, &n.Type, &n.Name, &n.QualifiedName, &n.FilePath, &n.StartLine, &n.EndLine, &n.MetadataJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func reverseIDs(ids []int64) {
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
}

func reverseEdges(edges []Edge) {
	for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
		edges[i], edges[j] = edges[j], edges[i]
	}
}

func topConnected(ctx context.Context, st *store.Store, projectID int64, where string, limit int) ([]Connected, error) {
	q := `SELECT n.id,n.name,n.type,n.file_path,COUNT(e.id) c FROM graph_nodes n LEFT JOIN graph_edges e ON e.project_id=n.project_id AND (e.from_node_id=n.id OR e.to_node_id=n.id) WHERE n.project_id=? ` + where + ` GROUP BY n.id,n.name,n.type,n.file_path ORDER BY c DESC,n.name LIMIT ?`
	rows, err := st.DB.QueryContext(ctx, q, projectID, limit)
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

func sortStrings(v []string) {
	sort.Strings(v)
}
