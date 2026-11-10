package routerstore

import "testing"

// Every table in the current schema is either migrated or deliberately excluded with a reason.
// host_token_scopes (v25) was silently missing from the v18 manifest: a copy would have
// turned every scoped token into an unscoped one.
func TestMigratedTablesCoverSchema(t *testing.T) {
	s := openTestStore(t)
	if s.dialect().name == "postgres" {
		t.Skip("schema listing below reads sqlite_master")
	}
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	migrated := map[string]bool{}
	for _, m := range migratedTables {
		migrated[m.name] = true
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if !migrated[name] && unmigratedTables[name] == "" {
			t.Errorf("table %s is neither in migratedTables nor in unmigratedTables", name)
		}
	}
}

// A scoped mint is one transaction: its token row never exists without its scope rows.
func TestScopedMintIsAtomic(t *testing.T) {
	s := openTestStore(t)
	// A duplicate agent in the (deduplicated) list cannot fail, so force a failure with a dropped table.
	if _, err := s.db.Exec(`DROP TABLE host_token_scopes`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.MintScopedHostToken("h", "l", []string{"a"}); err == nil {
		t.Fatal("mint succeeded without a scope table")
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM host_tokens`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("token rows left behind: %d (%v)", n, err)
	}
}
