package routerstore

import "testing"

func TestLedgerSchemaVersionServed(t *testing.T) {
	// Postgres: the version OpenPostgres verified is recorded and returned.
	if v, err := (&SQLiteStore{d: postgresDialect, schemaVersion: postgresSchemaVersion}).LedgerSchemaVersion(); err != nil || v != postgresSchemaVersion {
		t.Fatalf("recorded version = %d, %v; want %d", v, err, postgresSchemaVersion)
	}
	// A backend that recorded nothing refuses rather than reporting 0.
	if v, err := (&SQLiteStore{d: postgresDialect}).LedgerSchemaVersion(); err == nil {
		t.Fatalf("unrecorded backend returned %d, nil; want an error", v)
	}
	// Over the wire, through a lane-scoped token (laneAPI + Rule-of-Ra exemption).
	f := newScopedFixture(t, []string{"ra"})
	v, err := f.client("ra").LedgerSchemaVersion()
	if err != nil {
		t.Fatalf("LedgerSchemaVersion over the wire: %v", err)
	}
	if v != MaxSupportedSchemaVersion() {
		t.Fatalf("wire version = %d, want %d", v, MaxSupportedSchemaVersion())
	}
}
