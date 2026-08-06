package routerstore

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaGapDegradedOnlyWhenStoreIsAhead(t *testing.T) {
	cases := []struct {
		store, binary int
		want          bool
	}{{14, 7, true}, {7, 7, false}, {5, 7, false}}
	for _, c := range cases {
		if got := (SchemaGap{StoreVersion: c.store, BinaryVersion: c.binary}).Degraded(); got != c.want {
			t.Errorf("store=%d binary=%d Degraded()=%v want %v", c.store, c.binary, got, c.want)
		}
	}
}

// The banner is the single wording every surface renders, so three surfaces
// cannot describe one condition three ways — the divergence the fleet board
// exists to end.
func TestBannerNamesBothVersionsAndTheCulprit(t *testing.T) {
	b := SchemaGap{StoreVersion: 14, BinaryVersion: 7, MigratedBy: "v14 by abc123 (DIRTY)"}.Banner()
	for _, want := range []string{"PARTIAL", "v14", "v7", "abc123"} {
		if !strings.Contains(b, want) {
			t.Errorf("banner missing %q: %s", want, b)
		}
	}
	if got := (SchemaGap{StoreVersion: 7, BinaryVersion: 7}).Banner(); got != "" {
		t.Errorf("a non-degraded gap must render no banner, got %q", got)
	}
}

// A normally-migrated store must open read-only with no gap, so the compat path
// is safe as the universal read entrypoint rather than a special case surfaces
// have to remember to choose.
func TestOpenReadOnlyOnCurrentSchemaReportsNoGap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.db")
	st, err := Open(path) // creates + migrates to this binary's max
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
	_ = st.Close()

	ro, gap, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer func() { _ = ro.Close() }()
	if gap.Degraded() {
		t.Errorf("freshly migrated store reported degraded: store=%d binary=%d", gap.StoreVersion, gap.BinaryVersion)
	}
}

// Read-only must be enforced by the driver, not by convention. Convention is
// what let an uncommitted build migrate production.
func TestOpenReadOnlyRefusesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
	_ = st.Close()

	ro, _, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer func() { _ = ro.Close() }()

	if _, err := ro.db.Exec(`INSERT INTO state(key,value) VALUES('x','y')`); err == nil {
		t.Error("write succeeded through the read-only handle — the guarantee is convention, not enforcement")
	}
}
