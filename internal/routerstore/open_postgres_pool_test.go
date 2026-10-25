package routerstore

import "testing"

// The router service sizes its Postgres pool from SIRSI_ROUTER_PG_MAX_CONNS;
// anything missing or invalid keeps the ADR-062 per-node default of 4.
func TestPostgresMaxConns(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want int
	}{
		{"", 4},
		{"16", 16},
		{" 8 ", 8},
		{"1", 1},
		{"64", 64},
		{"0", 4},
		{"65", 4},
		{"-3", 4},
		{"lots", 4},
	} {
		t.Setenv("SIRSI_ROUTER_PG_MAX_CONNS", tc.env)
		if got := postgresMaxConns(); got != tc.want {
			t.Errorf("SIRSI_ROUTER_PG_MAX_CONNS=%q: got %d, want %d", tc.env, got, tc.want)
		}
	}
}
