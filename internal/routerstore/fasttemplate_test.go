package routerstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Fresh SQLite stores in this suite are seeded from one already-migrated
// template file instead of replaying every schema migration per test (a cold
// OpenPath costs ~0.5 s under -race, a re-open of a current file ~15 ms, and
// the suite opens hundreds). The template is produced by the real OpenPath
// migrate path, so production code is unchanged, and
// TestFastTemplateMatchesFreshMigration proves the template is
// indistinguishable from a freshly migrated store. Tests that exercise
// migration or open-from-empty call OpenPath directly on a path that does not
// exist and still run the real path.

var fastTemplate struct {
	once sync.Once
	dir  string // set by TestMain
	path string
	err  error
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "routerstore-template-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "routerstore tests: template dir:", err)
		os.Exit(1)
	}
	fastTemplate.dir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// buildFastTemplate migrates a new store once per process and leaves a single
// self-contained file (checkpointed, closed, no -wal/-shm).
func buildFastTemplate() (string, error) {
	path := filepath.Join(fastTemplate.dir, "template.db")
	s, err := OpenPath(path)
	if err != nil {
		return "", fmt.Errorf("template open: %w", err)
	}
	if _, err := s.db.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE);`); err != nil {
		_ = s.Close()
		return "", fmt.Errorf("template checkpoint: %w", err)
	}
	if err := s.Close(); err != nil {
		return "", fmt.Errorf("template close: %w", err)
	}
	for _, side := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + side); err == nil {
			return "", fmt.Errorf("template left %s behind", path+side)
		}
	}
	return path, nil
}

// seedFromTemplate returns the path OpenPath should be given for a store that
// starts current. ":memory:" becomes a temp file, since a file is what can be
// seeded. A path that already exists is left alone: a re-open must see the
// data written earlier.
func seedFromTemplate(t testing.TB, path string) string {
	t.Helper()
	if path == ":memory:" {
		path = filepath.Join(t.TempDir(), "router.db")
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	fastTemplate.once.Do(func() { fastTemplate.path, fastTemplate.err = buildFastTemplate() })
	if fastTemplate.err != nil {
		t.Fatalf("fast template: %v", fastTemplate.err)
	}
	b, err := os.ReadFile(fastTemplate.path)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
	return path
}

// dumpSchemaAndSeed returns a deterministic text dump of everything a
// migration leaves behind: user_version, every schema object, and every table
// row except the two wall-clock stamps written at migration time.
func dumpSchemaAndSeed(t *testing.T, s *SQLiteStore) string {
	t.Helper()
	db := s.db.db
	var out string
	var ver int
	if err := db.QueryRow(`PRAGMA user_version;`).Scan(&ver); err != nil {
		t.Fatal(err)
	}
	out += fmt.Sprintf("user_version=%d\n", ver)
	rows, err := db.Query(`SELECT type, name, tbl_name, COALESCE(sql,'') FROM sqlite_master ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var typ, name, tbl, sqlText string
		if err := rows.Scan(&typ, &name, &tbl, &sqlText); err != nil {
			t.Fatal(err)
		}
		out += fmt.Sprintf("%s|%s|%s|%s\n", typ, name, tbl, sqlText)
		if typ == "table" && len(name) > 0 && name[0] != '_' && (len(name) < 7 || name[:7] != "sqlite_") {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	for _, tbl := range tables {
		q := fmt.Sprintf(`SELECT * FROM %q`, tbl)
		if tbl == "state" {
			// Both keys hold the wall-clock moment of the migration by design.
			q += fmt.Sprintf(` WHERE key NOT IN ('%s','operational_enforcement_since')`, provenanceKey)
		}
		r, err := db.Query(q + ` ORDER BY 1`)
		if err != nil {
			t.Fatalf("dump %s: %v", tbl, err)
		}
		cols, _ := r.Columns()
		for r.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			out += fmt.Sprintf("row %s %v\n", tbl, vals)
		}
		if err := r.Err(); err != nil {
			t.Fatal(err)
		}
		_ = r.Close()
	}
	return out
}

func TestFastTemplateMatchesFreshMigration(t *testing.T) {
	fresh, err := OpenPath(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("fresh migrate: %v", err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	seeded, err := OpenPath(seedFromTemplate(t, filepath.Join(t.TempDir(), "seeded.db")))
	if err != nil {
		t.Fatalf("open seeded: %v", err)
	}
	t.Cleanup(func() { _ = seeded.Close() })
	a := strings.Split(dumpSchemaAndSeed(t, fresh), "\n")
	b := strings.Split(dumpSchemaAndSeed(t, seeded), "\n")
	if len(a) != len(b) {
		t.Fatalf("template has %d dump lines, fresh migration %d", len(b), len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("template differs from a freshly migrated store\n  fresh:    %s\n  template: %s", a[i], b[i])
		}
	}
}
