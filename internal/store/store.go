package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

type Project struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	RootPath     string `json:"root_path"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	GraphBuiltAt string `json:"graph_built_at,omitempty"`
}

type FileRecord struct {
	ID        int64
	ProjectID int64
	Path      string
	AbsPath   string
	Language  string
	SizeBytes int64
	MTime     string
	Hash      string
	IndexedAt string
}

type Chunk struct {
	ID            int64
	FileID        int64
	ProjectID     int64
	Path          string
	StartLine     int
	EndLine       int
	Content       string
	TokenEstimate int
	Kind          string
}

type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Line      int    `json:"line"`
	Signature string `json:"signature"`
	Path      string `json:"path,omitempty"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.Migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS projects (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, root_path TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS files (id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, path TEXT NOT NULL, abs_path TEXT NOT NULL, language TEXT, size_bytes INTEGER NOT NULL, mtime TEXT NOT NULL, hash TEXT NOT NULL, indexed_at TEXT NOT NULL, UNIQUE(project_id, path), FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS chunks (id INTEGER PRIMARY KEY AUTOINCREMENT, file_id INTEGER NOT NULL, project_id INTEGER NOT NULL, start_line INTEGER NOT NULL, end_line INTEGER NOT NULL, content TEXT NOT NULL, token_estimate INTEGER NOT NULL, kind TEXT DEFAULT 'code', FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE, FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(content, file_path UNINDEXED)`,
		`CREATE TABLE IF NOT EXISTS symbols (id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, file_id INTEGER NOT NULL, name TEXT NOT NULL, kind TEXT NOT NULL, line INTEGER NOT NULL, signature TEXT, FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE, FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS imports (id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, file_id INTEGER NOT NULL, imported_path TEXT NOT NULL, raw TEXT NOT NULL, FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE, FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS graph_nodes (id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, type TEXT NOT NULL, name TEXT NOT NULL, qualified_name TEXT, file_path TEXT NOT NULL, start_line INTEGER, end_line INTEGER, metadata_json TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS graph_edges (id INTEGER PRIMARY KEY AUTOINCREMENT, project_id INTEGER NOT NULL, from_node_id INTEGER NOT NULL, to_node_id INTEGER NOT NULL, type TEXT NOT NULL, confidence REAL DEFAULT 1.0, metadata_json TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE, FOREIGN KEY(from_node_id) REFERENCES graph_nodes(id) ON DELETE CASCADE, FOREIGN KEY(to_node_id) REFERENCES graph_nodes(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_project_type ON graph_nodes(project_id, type)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_project_name ON graph_nodes(project_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_nodes_project_file ON graph_nodes(project_id, file_path)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_project_from ON graph_edges(project_id, from_node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_project_to ON graph_edges(project_id, to_node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_graph_edges_project_type ON graph_edges(project_id, type)`,
		`CREATE TABLE IF NOT EXISTS analytics (id INTEGER PRIMARY KEY AUTOINCREMENT, tool TEXT NOT NULL, project TEXT NOT NULL DEFAULT '', called_at TEXT NOT NULL, tokens_without INTEGER NOT NULL, tokens_actual INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS graph_parsed (project_id INTEGER NOT NULL, file_id INTEGER NOT NULL, hash TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(project_id,file_id), FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE, FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE)`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	// Best-effort schema upgrades for existing databases.
	_, _ = s.DB.ExecContext(ctx, `ALTER TABLE projects ADD COLUMN graph_built_at TEXT`)
	_, _ = s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS graph_parsed (project_id INTEGER NOT NULL, file_id INTEGER NOT NULL, hash TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(project_id,file_id), FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE, FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE)`)
	return nil
}

func (s *Store) AddProject(ctx context.Context, name, root string) (Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Project{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO projects(name, root_path, created_at, updated_at) VALUES(?,?,?,?) ON CONFLICT(name) DO UPDATE SET root_path=excluded.root_path, updated_at=excluded.updated_at`, name, abs, now, now)
	if err != nil {
		return Project{}, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		p, err := s.ProjectByName(ctx, name)
		return p, err
	}
	return Project{ID: id, Name: name, RootPath: abs, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) Projects(ctx context.Context) ([]Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,root_path,created_at,updated_at,COALESCE(graph_built_at,'') FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.CreatedAt, &p.UpdatedAt, &p.GraphBuiltAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ProjectByName(ctx context.Context, name string) (Project, error) {
	var p Project
	err := s.DB.QueryRowContext(ctx, `SELECT id,name,root_path,created_at,updated_at,COALESCE(graph_built_at,'') FROM projects WHERE name=?`, name).Scan(&p.ID, &p.Name, &p.RootPath, &p.CreatedAt, &p.UpdatedAt, &p.GraphBuiltAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("project %q not found", name)
	}
	return p, err
}

func (s *Store) ProjectByPath(ctx context.Context, absPath string) (Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,root_path,created_at,updated_at,COALESCE(graph_built_at,'') FROM projects ORDER BY length(root_path) DESC`)
	if err != nil {
		return Project{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.CreatedAt, &p.UpdatedAt, &p.GraphBuiltAt); err != nil {
			return Project{}, err
		}
		if p.RootPath == absPath || strings.HasPrefix(absPath, p.RootPath+string(filepath.Separator)) {
			return p, nil
		}
	}
	if err := rows.Err(); err != nil {
		return Project{}, err
	}
	return Project{}, fmt.Errorf("no project registered at path %q", absPath)
}

func (s *Store) SetGraphBuiltAt(ctx context.Context, projectID int64, t time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE projects SET graph_built_at=? WHERE id=?`, t.UTC().Format(time.RFC3339), projectID)
	return err
}

func (s *Store) UpsertFile(ctx context.Context, f FileRecord) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var oldID int64
	_ = tx.QueryRowContext(ctx, `SELECT id FROM files WHERE project_id=? AND path=?`, f.ProjectID, f.Path).Scan(&oldID)
	if oldID != 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE rowid IN (SELECT id FROM chunks WHERE file_id=?)`, oldID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE file_id=?`, oldID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM symbols WHERE file_id=?`, oldID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM imports WHERE file_id=?`, oldID); err != nil {
			return 0, err
		}
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO files(project_id,path,abs_path,language,size_bytes,mtime,hash,indexed_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_id,path) DO UPDATE SET abs_path=excluded.abs_path, language=excluded.language, size_bytes=excluded.size_bytes, mtime=excluded.mtime, hash=excluded.hash, indexed_at=excluded.indexed_at`, f.ProjectID, f.Path, f.AbsPath, f.Language, f.SizeBytes, f.MTime, f.Hash, f.IndexedAt)
	if err != nil {
		return 0, err
	}
	id := oldID
	if id == 0 {
		id, _ = res.LastInsertId()
	}
	return id, tx.Commit()
}

func (s *Store) InsertChunk(ctx context.Context, c Chunk) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO chunks(file_id,project_id,start_line,end_line,content,token_estimate,kind) VALUES(?,?,?,?,?,?,?)`, c.FileID, c.ProjectID, c.StartLine, c.EndLine, c.Content, c.TokenEstimate, c.Kind)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO chunks_fts(rowid,content,file_path) VALUES(?,?,?)`, id, c.Content, c.Path)
	return id, err
}

func (s *Store) InsertSymbol(ctx context.Context, projectID, fileID int64, sym Symbol) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO symbols(project_id,file_id,name,kind,line,signature) VALUES(?,?,?,?,?,?)`, projectID, fileID, sym.Name, sym.Kind, sym.Line, sym.Signature)
	return err
}

func (s *Store) InsertImport(ctx context.Context, projectID, fileID int64, imported, raw string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO imports(project_id,file_id,imported_path,raw) VALUES(?,?,?,?)`, projectID, fileID, imported, raw)
	return err
}

type AnalyticsRecord struct {
	ID            int64
	Tool          string
	Project       string
	CalledAt      time.Time
	TokensWithout int
	TokensActual  int
}

type AnalyticsFilter struct {
	Tool      string
	Project   string
	Since     time.Time
	Until     time.Time
	MinTokens int
	MaxTokens int
}

func (s *Store) AddAnalytics(ctx context.Context, r AnalyticsRecord) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO analytics(tool,project,called_at,tokens_without,tokens_actual) VALUES(?,?,?,?,?)`,
		r.Tool, r.Project, r.CalledAt.UTC().Format(time.RFC3339), r.TokensWithout, r.TokensActual)
	return err
}

func (s *Store) QueryAnalytics(ctx context.Context, f AnalyticsFilter) ([]AnalyticsRecord, error) {
	q := `SELECT id,tool,project,called_at,tokens_without,tokens_actual FROM analytics WHERE 1=1`
	var args []any
	if f.Tool != "" {
		q += ` AND tool=?`
		args = append(args, f.Tool)
	}
	if f.Project != "" {
		q += ` AND project=?`
		args = append(args, f.Project)
	}
	if !f.Since.IsZero() {
		q += ` AND called_at >= ?`
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		q += ` AND called_at <= ?`
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}
	if f.MinTokens > 0 {
		q += ` AND tokens_actual >= ?`
		args = append(args, f.MinTokens)
	}
	if f.MaxTokens > 0 {
		q += ` AND tokens_actual <= ?`
		args = append(args, f.MaxTokens)
	}
	q += ` ORDER BY called_at DESC`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AnalyticsRecord
	for rows.Next() {
		var r AnalyticsRecord
		var calledAt string
		if err := rows.Scan(&r.ID, &r.Tool, &r.Project, &calledAt, &r.TokensWithout, &r.TokensActual); err != nil {
			return nil, err
		}
		r.CalledAt, _ = time.Parse(time.RFC3339, calledAt)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) AnalyticsTools(ctx context.Context) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT tool FROM analytics ORDER BY tool`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) FileContent(ctx context.Context, project Project, rel string) (string, error) {
	clean := filepath.Clean(rel)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || len(clean) >= 3 && clean[:3] == "../" {
		return "", fmt.Errorf("invalid project-relative path %q", rel)
	}
	abs := filepath.Join(project.RootPath, clean)
	root, _ := filepath.EvalSymlinks(project.RootPath)
	target, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	r, err := filepath.Rel(root, target)
	if err != nil || r == ".." || len(r) >= 3 && r[:3] == "../" {
		return "", fmt.Errorf("path escapes project root")
	}
	b, err := os.ReadFile(target)
	return string(b), err
}
