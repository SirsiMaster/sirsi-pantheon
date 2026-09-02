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
	if ifErr1 := os.MkdirAll(filepath.Dir(path), 0700); ifErr1 != nil {
		t.Fatal(ifErr1)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations {
		if m.version > 7 {
			break
		}
		if _, ifErr2 := db.Exec(m.sql); ifErr2 != nil {
			t.Fatal(ifErr2)
		}
		if _, ifErr3 := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", m.version)); ifErr3 != nil {
			t.Fatal(ifErr3)
		}
	}
	_ = db.Close()
	if _, ifErr4 := OpenPath(path); ifErr4 == nil || !strings.Contains(ifErr4.Error(), "deployment event") {
		t.Fatalf("ordinary binary advanced shared store without gate: %v", ifErr4)
	}
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	s, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestSharedProductionIdentityGuardRejectsPathAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	live := filepath.Join(home, ".sirsi", "router.db")
	if ifErr5 := os.MkdirAll(filepath.Dir(live), 0700); ifErr5 != nil {
		t.Fatal(ifErr5)
	}
	if ifErr6 := os.WriteFile(live, nil, 0600); ifErr6 != nil {
		t.Fatal(ifErr6)
	}
	alias := filepath.Join(t.TempDir(), "alias.db")
	if ifErr7 := os.Symlink(live, alias); ifErr7 != nil {
		t.Fatal(ifErr7)
	}
	if !isSharedProductionStore(alias) {
		t.Fatal("symlink bypassed production-store guard")
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if ifErr8 := os.Chdir(home); ifErr8 != nil {
		t.Fatal(ifErr8)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCWD) })
	if !isSharedProductionStore(filepath.Join(".sirsi", "router.db")) {
		t.Fatal("relative path bypassed production-store guard")
	}
	hardlink := filepath.Join(t.TempDir(), "hardlink.db")
	if ifErr9 := os.Link(live, hardlink); ifErr9 != nil {
		t.Fatal(ifErr9)
	}
	if !isSharedProductionStore(hardlink) {
		t.Fatal("hardlink bypassed production-store guard")
	}
	caseAlias := filepath.Join(home, ".SIRSI", "router.db")
	if _, ifErr10 := os.Stat(caseAlias); ifErr10 == nil && !isSharedProductionStore(caseAlias) {
		t.Fatal("case-folded path bypassed production-store guard")
	}
}

func TestFreshSharedStoreInitializesWithoutDeploymentOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "")
	path := filepath.Join(home, ".sirsi", "router.db")
	if ifErr11 := os.MkdirAll(filepath.Dir(path), 0700); ifErr11 != nil {
		t.Fatal(ifErr11)
	}
	s, err := OpenPath(path)
	if err != nil {
		t.Fatalf("fresh host must initialize version zero store: %v", err)
	}
	defer s.Close()
}

func TestTaskContinuationTriggerMatchesAuthoritativeDependencyExpression(t *testing.T) {
	if pgTestDSN() != "" {
		t.Skip("introspects SQLite catalog (sqlite_master / PRAGMA); the Postgres schema is verified by scripts/check-pg-schema.sh")
	}
	s := newTestStore(t)
	var triggerSQL string
	if ifErr12 := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='trigger' AND name='wake_continue_after_task'`).Scan(&triggerSQL); ifErr12 != nil {
		t.Fatal(ifErr12)
	}
	if !strings.Contains(triggerSQL, actionableTaskDependency("t")) {
		t.Fatalf("task continuation trigger drifted from authoritative predicate:\n%s", triggerSQL)
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
				if _, ifErr13 := db.Exec(m.sql); ifErr13 != nil {
					t.Fatalf("seed migration v%d: %v", m.version, ifErr13)
				}
				if _, ifErr14 := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", m.version)); ifErr14 != nil {
					t.Fatal(ifErr14)
				}
			}
			_ = db.Close()
			s, err := OpenPath(path)
			if err != nil {
				t.Fatalf("upgrade from v%d: %v", from, err)
			}
			defer s.Close()
			var got int
			if ifErr15 := s.db.QueryRow(`PRAGMA user_version`).Scan(&got); ifErr15 != nil {
				t.Fatal(ifErr15)
			}
			if got != migrations[len(migrations)-1].version {
				t.Fatalf("got v%d", got)
			}
			var waiverRef, leaseUpdated string
			if ifErr16 := s.db.QueryRow(`SELECT waiver_ref FROM requirements LIMIT 1`).Scan(&waiverRef); ifErr16 != nil && ifErr16 != sql.ErrNoRows {
				t.Fatal(ifErr16)
			}
			if ifErr17 := s.db.QueryRow(`SELECT lease_updated FROM items LIMIT 1`).Scan(&leaseUpdated); ifErr17 != nil && ifErr17 != sql.ErrNoRows {
				t.Fatal(ifErr17)
			}
		})
	}
}
