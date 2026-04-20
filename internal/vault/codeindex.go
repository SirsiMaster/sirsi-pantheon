package vault

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// CodeChunk is a piece of source code indexed for retrieval.
type CodeChunk struct {
	ID        int64   `json:"id"`
	File      string  `json:"file"`
	StartLine int     `json:"startLine"`
	EndLine   int     `json:"endLine"`
	Kind      string  `json:"kind"`
	Name      string  `json:"name"`
	Content   string  `json:"content"`
	Tokens    int     `json:"tokens"`
	Rank      float64 `json:"rank,omitempty"`
}

// IndexStats holds metrics about the indexing operation.
type IndexStats struct {
	FilesIndexed  int    `json:"filesIndexed"`
	ChunksCreated int    `json:"chunksCreated"`
	TotalTokens   int64  `json:"totalTokens"`
	Duration      string `json:"duration"`
}

// CodeIndex manages the FTS5-backed code search index.
type CodeIndex struct {
	db   *sql.DB
	path string
}

// DefaultCodeIndexPath returns the default code index path.
func DefaultCodeIndexPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sirsi", "vault", "codeindex.db")
}

// OpenCodeIndex opens or creates the code index database.
func OpenCodeIndex(path string) (*CodeIndex, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create code index dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open code index db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if err := initCodeSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init code schema: %w", err)
	}

	return &CodeIndex{db: db, path: path}, nil
}

func initCodeSchema(db *sql.DB) error {
	schema := `
		CREATE VIRTUAL TABLE IF NOT EXISTS code_fts USING fts5(
			file,
			name,
			kind,
			content,
			tokenize = 'porter unicode61'
		);
		CREATE TABLE IF NOT EXISTS code_meta (
			rowid INTEGER PRIMARY KEY,
			start_line INTEGER,
			end_line INTEGER,
			tokens INTEGER DEFAULT 0,
			indexed_at TEXT DEFAULT (datetime('now'))
		);
		CREATE TABLE IF NOT EXISTS code_files (
			path TEXT PRIMARY KEY,
			mod_time INTEGER,
			indexed_at TEXT DEFAULT (datetime('now'))
		);
	`
	_, err := db.Exec(schema)
	return err
}

// sourceExtensions lists file extensions we index.
var sourceExtensions = map[string]bool{
	".go":    true,
	".py":    true,
	".js":    true,
	".ts":    true,
	".tsx":   true,
	".jsx":   true,
	".rs":    true,
	".java":  true,
	".c":     true,
	".h":     true,
	".cpp":   true,
	".rb":    true,
	".swift": true,
	".kt":    true,
	".yaml":  true,
	".yml":   true,
	".toml":  true,
	".sql":   true,
	".sh":    true,
	".md":    true,
}

// skipDirs lists directories to skip during indexing.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"dist":         true,
	".thoth":       true,
	"__pycache__":  true,
	".next":        true,
	"build":        true,
	"target":       true,
}

// IndexDir walks a directory and indexes all recognized source files.
func (ci *CodeIndex) IndexDir(root string) (*IndexStats, error) {
	start := time.Now()
	stats := &IndexStats{}

	goChunker := &GoChunker{}
	genericChunker := &GenericChunker{MaxChunkLines: 50, Overlap: 25}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !sourceExtensions[ext] {
			return nil
		}

		// Skip large files (>500KB).
		if info.Size() > 500*1024 {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(root, path)
		if relPath == "" {
			relPath = path
		}

		var chunker Chunker
		if ext == ".go" {
			chunker = goChunker
		} else {
			chunker = genericChunker
		}

		chunks, err := chunker.Chunk(relPath, content)
		if err != nil {
			return nil
		}

		for _, chunk := range chunks {
			if err := ci.insertChunk(chunk); err != nil {
				return nil // skip individual failures
			}
			stats.ChunksCreated++
		}
		stats.FilesIndexed++

		// Record file mod time for refresh.
		ci.db.Exec(
			"INSERT OR REPLACE INTO code_files(path, mod_time) VALUES (?, ?)",
			relPath, info.ModTime().Unix(),
		)

		return nil
	})

	stats.Duration = time.Since(start).String()
	return stats, err
}

func (ci *CodeIndex) insertChunk(chunk CodeChunk) error {
	res, err := ci.db.Exec(
		"INSERT INTO code_fts(file, name, kind, content) VALUES (?, ?, ?, ?)",
		chunk.File, chunk.Name, chunk.Kind, chunk.Content,
	)
	if err != nil {
		return fmt.Errorf("insert code chunk: %w", err)
	}
	rowid, _ := res.LastInsertId()

	_, err = ci.db.Exec(
		"INSERT INTO code_meta(rowid, start_line, end_line, tokens) VALUES (?, ?, ?, ?)",
		rowid, chunk.StartLine, chunk.EndLine, chunk.Tokens,
	)
	return err
}

// IndexFile indexes a single file.
func (ci *CodeIndex) IndexFile(path string, content []byte) error {
	ext := strings.ToLower(filepath.Ext(path))
	var chunker Chunker
	if ext == ".go" {
		chunker = &GoChunker{}
	} else {
		chunker = &GenericChunker{MaxChunkLines: 50, Overlap: 25}
	}

	chunks, err := chunker.Chunk(path, content)
	if err != nil {
		return fmt.Errorf("chunk file %s: %w", path, err)
	}

	for _, chunk := range chunks {
		if err := ci.insertChunk(chunk); err != nil {
			return err
		}
	}
	return nil
}

// Search performs BM25-ranked full-text search over indexed code.
func (ci *CodeIndex) Search(query string, limit int) ([]CodeChunk, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := ci.db.Query(`
		SELECT f.rowid, f.file, f.name, f.kind,
			snippet(code_fts, 3, '»', '«', '…', 40) as snip,
			m.start_line, m.end_line, m.tokens, rank
		FROM code_fts f
		JOIN code_meta m ON m.rowid = f.rowid
		WHERE code_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("code search: %w", err)
	}
	defer rows.Close()

	var chunks []CodeChunk
	for rows.Next() {
		var c CodeChunk
		if err := rows.Scan(&c.ID, &c.File, &c.Name, &c.Kind, &c.Content, &c.StartLine, &c.EndLine, &c.Tokens, &c.Rank); err != nil {
			return nil, fmt.Errorf("scan code chunk: %w", err)
		}
		chunks = append(chunks, c)
	}

	return chunks, nil
}

// Refresh re-indexes files that have changed since last index.
func (ci *CodeIndex) Refresh(root string) (*IndexStats, error) {
	// Clear existing index and rebuild.
	ci.db.Exec("DELETE FROM code_fts")
	ci.db.Exec("DELETE FROM code_meta")
	ci.db.Exec("DELETE FROM code_files")
	return ci.IndexDir(root)
}

// Stats returns code index statistics.
func (ci *CodeIndex) Stats() (*IndexStats, error) {
	var stats IndexStats
	ci.db.QueryRow("SELECT COUNT(DISTINCT file) FROM code_fts").Scan(&stats.FilesIndexed)
	ci.db.QueryRow("SELECT COUNT(*) FROM code_fts").Scan(&stats.ChunksCreated)
	return &stats, nil
}

// Close closes the code index database.
func (ci *CodeIndex) Close() error {
	return ci.db.Close()
}
