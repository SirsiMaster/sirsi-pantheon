package routerstore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNewerStoreFailsClosedAgainstOlderBinary(t *testing.T) {
	err := checkSchemaCompatibility(14, 7)
	if err == nil || !strings.Contains(err.Error(), "newer than this binary understands") {
		t.Fatalf("v14 store against v7 binary must fail legibly: %v", err)
	}
}

func TestSharedProductionMigrationRequiresExplicitDeploymentGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "")
	path := filepath.Join(home, ".sirsi", "router.db")
	if !isSharedProductionStore(path) {
		t.Fatal("test path not recognized as shared production store")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations {
		if m.version > 7 {
			break
		}
		if _, err := db.Exec(m.sql); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", m.version)); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "deployment event") {
		t.Fatalf("ordinary binary advanced shared store without gate: %v", err)
	}
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestSharedProductionIdentityGuardRejectsPathAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	live := filepath.Join(home, ".sirsi", "router.db")
	if err := os.MkdirAll(filepath.Dir(live), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias.db")
	if err := os.Symlink(live, alias); err != nil {
		t.Fatal(err)
	}
	if !isSharedProductionStore(alias) {
		t.Fatal("symlink bypassed production-store guard")
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCWD) })
	if !isSharedProductionStore(filepath.Join(".sirsi", "router.db")) {
		t.Fatal("relative path bypassed production-store guard")
	}
	hardlink := filepath.Join(t.TempDir(), "hardlink.db")
	if err := os.Link(live, hardlink); err != nil {
		t.Fatal(err)
	}
	if !isSharedProductionStore(hardlink) {
		t.Fatal("hardlink bypassed production-store guard")
	}
	caseAlias := filepath.Join(home, ".SIRSI", "router.db")
	if _, err := os.Stat(caseAlias); err == nil && !isSharedProductionStore(caseAlias) {
		t.Fatal("case-folded path bypassed production-store guard")
	}
}

func TestSparseDeployedMigrationPathsReachCurrentSchema(t *testing.T) {
	for _, from := range []int{8, 9, 11, 12} {
		t.Run(fmt.Sprintf("v%d", from), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "router.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range migrations {
				if m.version > from {
					break
				}
				if _, err := db.Exec(m.sql); err != nil {
					t.Fatalf("seed migration v%d: %v", m.version, err)
				}
				if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", m.version)); err != nil {
					t.Fatal(err)
				}
			}
			_ = db.Close()
			s, err := Open(path)
			if err != nil {
				t.Fatalf("upgrade from v%d: %v", from, err)
			}
			defer s.Close()
			var got int
			if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != migrations[len(migrations)-1].version {
				t.Fatalf("got v%d", got)
			}
			var waiverRef, leaseUpdated string
			if err := s.db.QueryRow(`SELECT waiver_ref FROM requirements LIMIT 1`).Scan(&waiverRef); err != nil && err != sql.ErrNoRows {
				t.Fatal(err)
			}
			if err := s.db.QueryRow(`SELECT lease_updated FROM items LIMIT 1`).Scan(&leaseUpdated); err != nil && err != sql.ErrNoRows {
				t.Fatal(err)
			}
		})
	}
}
