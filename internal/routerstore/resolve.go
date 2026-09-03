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
//  1. SIRSI_ROUTER_URL set → the router service (RemoteStore, bearer token from
//     SIRSI_ROUTER_TOKEN). A missing token is refused loudly, never a silent
//     fallback to a local file.
//  2. SIRSI_ROUTER_DB set → that SQLite path (tests, sandboxes).
//  3. neither → ~/.sirsi/router.db (Anubis default).
//
// The parent directory is created for the local cases: a fresh HOME has no
// ~/.sirsi yet and SQLite cannot create a file in a missing directory.
func Resolve() (Store, error) {
	if u := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_URL")); u != "" {
		tok := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_TOKEN"))
		if tok == "" {
			// Never fall back to a local file: a node that believes it is on the
			// service must not write a local ledger (split-brain, ADR-062 §1).
			return nil, fmt.Errorf("routerstore: SIRSI_ROUTER_URL=%q is set but SIRSI_ROUTER_TOKEN is empty", u)
		}
		return NewRemoteStore(u, tok), nil
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
