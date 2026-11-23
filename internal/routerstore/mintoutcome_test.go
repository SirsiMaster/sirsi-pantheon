package routerstore

import (
	"errors"
	"strings"
	"testing"
)

// Failed scoped mints report a structured, nonsecret outcome (synthetic values only).
// Runs on whichever backend openBackendStore selects (SQLite; Postgres when SIRSI_TEST_PG_DSN is set) —
// both share mintScopedTx.
func TestMintScopedTxOutcomes(t *testing.T) {
	const tok, id = "synthetic-token-AAAA", "synthetic-id-1"
	cases := []struct {
		name     string
		f        mintFaults
		want     []error
		notWant  []error
		rowCount int
	}{
		{"rolled back", mintFaults{afterInsert: func() error { return errors.New("boom " + tok) }}, []error{ErrMintRolledBack}, []error{ErrMintRollbackFailed, ErrMintCommitUncertain}, 0},
		{"rollback failed", mintFaults{afterInsert: func() error { return errors.New("boom") }, rollback: func(tx *txHandle) error { _ = tx.Rollback(); return errors.New("rb " + hashToken(tok)) }},
			[]error{ErrMintRollbackFailed}, []error{ErrMintRolledBack}, 0},
		{"commit uncertain", mintFaults{commit: func(tx *txHandle) error { _ = tx.Rollback(); return errors.New("net " + tok) }}, []error{ErrMintCommitUncertain}, []error{ErrMintRolledBack, ErrMintPanic}, 0},
		{"panic", mintFaults{afterInsert: func() error { panic("secret " + tok) }}, []error{ErrMintPanic, ErrMintRolledBack}, []error{ErrMintCommitUncertain}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := openBackendStore(t, t.TempDir()+"/r.db")
			err := s.mintScopedTx(id, tok, "h", "l", "2026-01-01T00:00:00Z", []string{"a"}, c.f)
			var me *MintError
			if !errors.As(err, &me) || me.AttemptID != id {
				t.Fatalf("err = %v, want *MintError with attempt id", err)
			}
			for _, w := range c.want {
				if !errors.Is(err, w) {
					t.Errorf("errors.Is(%v) false: %v", w, err)
				}
			}
			for _, w := range c.notWant {
				if errors.Is(err, w) {
					t.Errorf("errors.Is(%v) true: %v", w, err)
				}
			}
			if m := err.Error(); strings.Contains(m, tok) || strings.Contains(m, hashToken(tok)) || strings.Contains(m, "secret") {
				t.Errorf("error leaks secret material: %q", m)
			}
			var n int
			if e := s.db.QueryRow(`SELECT count(*) FROM host_tokens`).Scan(&n); e != nil || n != c.rowCount {
				t.Errorf("host_tokens rows = %d (%v), want %d", n, e, c.rowCount)
			}
		})
	}
}

// Negative control: the real path still commits, with no error.
func TestMintScopedTxSuccess(t *testing.T) {
	s := openBackendStore(t, t.TempDir()+"/r.db")
	tok, rec, err := s.MintScopedHostToken("h", "l", []string{"a"})
	if err != nil || tok == "" || rec.ID == "" {
		t.Fatalf("mint: %v", err)
	}
	if _, err := s.LookupHostToken(tok); err != nil {
		t.Fatal(err)
	}
}
