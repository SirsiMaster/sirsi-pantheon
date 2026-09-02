package routerstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve is the single production constructor for the router ledger
// (ADR-062 §1–§2). Order:
//
//  1. SIRSI_ROUTER_URL set → the router service. Not yet implemented; refused
//     loudly rather than silently falling back to a local file, because a node
//     that believes it is on the service must never write a local ledger.
//  2. SIRSI_ROUTER_DB set → that SQLite path (tests, sandboxes).
//  3. neither → ~/.sirsi/router.db (Anubis default).
//
// The parent directory is created for the local cases: a fresh HOME has no
// ~/.sirsi yet and SQLite cannot create a file in a missing directory.
func Resolve() (Store, error) {
	if u := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_URL")); u != "" {
		// ponytail: HTTP client Store lands in ADR-062 Phase C (rs-09); until then a
		// set URL is a configuration error, not a fallback.
		return nil, fmt.Errorf("routerstore: SIRSI_ROUTER_URL=%q but the remote store client is not implemented yet (ADR-062 rs-09)", u)
	}
	path, err := LocalPath()
	if err != nil {
		return nil, err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("routerstore: create store dir: %w", err)
		}
	}
	return OpenPath(path)
}
