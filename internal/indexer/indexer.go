package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/issam/ctxd/internal/chunker"
	"github.com/issam/ctxd/internal/ignore"
	"github.com/issam/ctxd/internal/store"
	"github.com/issam/ctxd/internal/symbols"
)

type Result struct {
	IndexedFiles  int `json:"indexed_files"`
	IndexedChunks int `json:"indexed_chunks"`
	ChangedFiles  int `json:"changed_files"`
}

func IndexProject(ctx context.Context, st *store.Store, project store.Project) (Result, error) {
	m := ignore.New(project.RootPath)
	var res Result
	err := filepath.WalkDir(project.RootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(project.RootPath, path)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		if m.Ignored(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		lang := Language(path)
		if lang == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1_500_000 {
			return err
		}
		currMtime := info.ModTime().UTC().Format(time.RFC3339)
		var storedMtime, storedHash string
		_ = st.DB.QueryRowContext(ctx, `SELECT mtime, hash FROM files WHERE project_id=? AND path=?`, project.ID, rel).Scan(&storedMtime, &storedHash)
		if storedMtime != "" && storedMtime == currMtime {
			res.IndexedFiles++
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(b) || strings.ContainsRune(string(b), 0) {
			return nil
		}
		sum := sha256.Sum256(b)
		hash := hex.EncodeToString(sum[:])
		if storedHash != "" && storedHash == hash {
			_, _ = st.DB.ExecContext(ctx, `UPDATE files SET mtime=? WHERE project_id=? AND path=?`, currMtime, project.ID, rel)
			res.IndexedFiles++
			return nil
		}
		now := time.Now().UTC().Format(time.RFC3339)
		fileID, err := st.UpsertFile(ctx, store.FileRecord{ProjectID: project.ID, Path: rel, AbsPath: path, Language: lang, SizeBytes: info.Size(), MTime: currMtime, Hash: hash, IndexedAt: now})
		if err != nil {
			return err
		}
		text := string(b)
		for _, ch := range chunker.Chunk(text, lang) {
			if _, err := st.InsertChunk(ctx, store.Chunk{FileID: fileID, ProjectID: project.ID, Path: rel, StartLine: ch.StartLine, EndLine: ch.EndLine, Content: ch.Content, TokenEstimate: ch.TokenEstimate, Kind: ch.Kind}); err != nil {
				return err
			}
			res.IndexedChunks++
		}
		for _, sym := range symbols.Extract(text, lang) {
			_ = st.InsertSymbol(ctx, project.ID, fileID, store.Symbol{Name: sym.Name, Kind: sym.Kind, Line: sym.Line, Signature: sym.Signature})
		}
		for _, im := range symbols.ExtractImports(text) {
			_ = st.InsertImport(ctx, project.ID, fileID, im.Path, im.Raw)
		}
		res.IndexedFiles++
		res.ChangedFiles++
		return nil
	})
	return res, err
}

func Language(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".php":
		return "php"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".jsx":
		return "javascriptreact"
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yml", ".yaml":
		return "yaml"
	case ".env":
		return "env"
	default:
		return ""
	}
}
