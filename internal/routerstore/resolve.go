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
//  3. host cut over (~/.sirsi/router-service.env present) and no URL → refused:
//     a process started without the service env must not read a frozen copy
//     as live or create an empty ledger. Rollback of a node is a deliberate
//     procedure (docs/runbooks/router-service-tokens-and-rollback.md), not an
//     unset variable.
//  4. otherwise → ~/.sirsi/router.db (Anubis default).
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
	p, merr := cutOverMarker()
	if merr != nil {
		return nil, merr
	}
	if p != "" {
		// This host has been cut over to the router service (the cut-over wrote
		// ~/.sirsi/router-service.env). A process that starts without the service
		// env — a GUI app, a plist with no EnvironmentVariables, an old shell — must
		// not fall back to the local file: it would read a frozen copy as if live,
		// or create an empty ledger and split the fabric (observed 2026-09-10).
		return nil, fmt.Errorf("routerstore: this host is cut over to the router service (%s exists) but SIRSI_ROUTER_URL is unset — run `source ~/.zshenv` or start the process with the service env; the local file is not a ledger here", p)
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

// cutOverMarker returns the path of the per-host service env file when it
// exists (written by scripts/router-service/cutover-m5.sh step 6), "" when it
// does not, and an error for any other stat failure: an unreadable or invalid
// marker must never silently authorize the local fallback. SIRSI_ROUTER_DB set
// explicitly (tests, sandboxes) bypasses the check: that is a deliberate local
// store, not a fallback.
func cutOverMarker() (string, error) {
	if strings.TrimSpace(os.Getenv("SIRSI_ROUTER_DB")) != "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	p := filepath.Join(home, ".sirsi", "router-service.env")
	switch _, err := os.Stat(p); {
	case err == nil:
		return p, nil
	case os.IsNotExist(err):
		return "", nil
	default:
		return "", fmt.Errorf("routerstore: cannot read cut-over marker %s: %w (refusing the local file until it is readable)", p, err)
	}
}
