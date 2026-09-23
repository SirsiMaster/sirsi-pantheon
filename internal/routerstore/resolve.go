package routerstore

import (
	"bufio"
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
//     fallback to a local file. A spool:// URL needs no token: the relay on this
//     host holds it (ADR-062 20a.1b).
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
		if tok == "" && SpoolDir(u) == "" {
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
		//
		// Self-heal instead of just erroring (2026-09-23: a live Codex session
		// spawned outside any shell that sources ~/.zshenv hit this on the M5 —
		// "the router relay failed and confirmed nothing was sent"; the marker
		// this process just found IS the authoritative pointer the cut-over
		// wrote, so read the URL/trust-group straight out of it and proceed —
		// this is NOT the forbidden fallback: it never touches a local ledger,
		// it only recovers the same pointer `source ~/.zshenv` would have set.
		if url := loadCutOverEnv(p); url != "" {
			os.Setenv("SIRSI_ROUTER_URL", url)
			return Resolve()
		}
		return nil, fmt.Errorf("routerstore: this host is cut over to the router service (%s exists) but SIRSI_ROUTER_URL is unset and could not be recovered from the marker file — run `source ~/.zshenv` or start the process with the service env; the local file is not a ledger here", p)
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

// loadCutOverEnv parses the simple `export KEY='value'` / `export KEY="value"`
// lines a cut-over script writes to path (scripts/router-service/cutover-*.sh)
// and sets each recognized variable in this process's own environment — but
// ONLY when that variable is not already set, so an operator's explicit
// override always wins over the file. Returns the SIRSI_ROUTER_URL value
// found ("" if none), which is all the caller needs to know recovery
// succeeded; SIRSI_RELAY_TRUST_GROUP is set as a side effect for the spool
// client to pick up via its own later os.Getenv (internal/routerstore/spool.go).
// Malformed or unreadable input yields "" — never a partial, misleading state.
func loadCutOverEnv(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	recognized := map[string]bool{"SIRSI_ROUTER_URL": true, "SIRSI_RELAY_TRUST_GROUP": true}
	var url string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimPrefix(line, "export ")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || !recognized[key] {
			continue
		}
		// Extract the value: a quoted string up to its matching close-quote
		// (a trailing shell comment like the real M5 file's
		// `export SIRSI_RELAY_TRUST_GROUP="_sirsipantheon"  # added by claude-io ...`
		// sits safely outside that range and is discarded), or the first
		// whitespace-delimited token when unquoted.
		val = strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '\'' || val[0] == '"') {
			if end := strings.IndexByte(val[1:], val[0]); end >= 0 {
				val = val[1 : 1+end]
			} else {
				val = "" // unterminated quote — malformed, do not guess
			}
		} else if i := strings.IndexAny(val, " \t#"); i >= 0 {
			val = val[:i]
		}
		if val == "" {
			continue
		}
		if strings.TrimSpace(os.Getenv(key)) == "" {
			os.Setenv(key, val)
		}
		if key == "SIRSI_ROUTER_URL" {
			url = strings.TrimSpace(os.Getenv(key))
		}
	}
	return url
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
