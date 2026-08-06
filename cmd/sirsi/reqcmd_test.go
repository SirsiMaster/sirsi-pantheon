package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestReqAuditCommandRecordsExternallyReachableAudit(t *testing.T) {
	db := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	cmd := newReqAuditCmd()
	cmd.SetArgs([]string{"codex-home", "--evidence", "proof://canon-map"})
	cmd.SetOut(new(bytes.Buffer))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	s, err := routerstore.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.RunnableFor("codex-home")
	if err != nil {
		t.Fatal(err)
	}
	if r.RequirementAuditNeeded {
		t.Fatal("CLI audit did not clear audit-needed predicate")
	}
}
