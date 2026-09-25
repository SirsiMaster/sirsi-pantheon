package routerstore

// ADR-062 §3/§4 (rs-11): per-host bearer tokens that can be minted, listed
// and revoked individually. Only the SHA-256 of a token is stored; the
// plaintext is returned exactly once by MintHostToken. Revocation takes
// effect on the host's next request. These methods are server-side only —
// never served on the wire — and are driven by `sirsi router token …` run on
// the service host against its own backend.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// HostToken is the stored record (never the plaintext).
type HostToken struct {
	ID        string `json:"id"`
	Host      string `json:"host"`
	Label     string `json:"label"`
	Created   string `json:"created"`
	Revoked   string `json:"revoked,omitempty"`
	MachineID string `json:"machine_id,omitempty"` // ADR-067: adopted stable id, "" until adopted
}

var (
	ErrTokenUnknown = errors.New("routerstore: host token unknown")
	ErrTokenRevoked = errors.New("routerstore: host token revoked")
	ErrHostMismatch = errors.New("routerstore: token is not for this host")
	// ErrMachineIDClaimed: the machine id is already adopted by a different,
	// still-live host token (ADR-067 §3.2 exclusivity — one machine id per live
	// token). Revoking the holding token frees the id.
	ErrMachineIDClaimed = errors.New("routerstore: machine id already adopted by another live host token")
	// ErrMachineIDAdopted: this host's token already adopted a DIFFERENT machine
	// id. Adoption is one-way (recorded once, ADR-067 §3.1); a new id needs a
	// fresh token, not a silent re-point.
	ErrMachineIDAdopted = errors.New("routerstore: host token already adopted a different machine id")
	// ErrNoTokenForHost: no live host token exists for the authenticated host, so
	// there is nothing to adopt onto (the caller must hold a token to adopt).
	ErrNoTokenForHost = errors.New("routerstore: no live host token for this host")
	// ErrAmbiguousHostToken: more than one live token shares this host, so an
	// adoption cannot pick which one to bind — a fleet anomaly, not a normal
	// state (canon: one token per physical machine). Revoke the duplicate first.
	ErrAmbiguousHostToken = errors.New("routerstore: more than one live host token for this host")
)

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// MintHostToken creates a token for host and returns the plaintext once.
func (s *SQLiteStore) MintHostToken(host, label string) (string, HostToken, error) {
	if host == "" {
		return "", HostToken{}, errors.New("routerstore: MintHostToken: host is required")
	}
	id, err := randomHex(8)
	if err != nil {
		return "", HostToken{}, err
	}
	tok, err := randomHex(32)
	if err != nil {
		return "", HostToken{}, err
	}
	now := s.clock().Format(time.RFC3339)
	if _, err := s.exec(`INSERT INTO host_tokens(token_id,token_hash,host,label,created,revoked) VALUES(?,?,?,?,?,'')`,
		id, hashToken(tok), host, label, now); err != nil {
		return "", HostToken{}, fmt.Errorf("routerstore: MintHostToken: %w", err)
	}
	return tok, HostToken{ID: id, Host: host, Label: label, Created: now}, nil
}

// LookupHostToken resolves a presented plaintext token to its record.
func (s *SQLiteStore) LookupHostToken(plaintext string) (HostToken, error) {
	var t HostToken
	err := s.db.QueryRow(`SELECT token_id,host,label,created,revoked FROM host_tokens WHERE token_hash=?`, hashToken(plaintext)).
		Scan(&t.ID, &t.Host, &t.Label, &t.Created, &t.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return HostToken{}, ErrTokenUnknown
	}
	if err != nil {
		return HostToken{}, fmt.Errorf("routerstore: LookupHostToken: %w", err)
	}
	if t.Revoked != "" {
		return HostToken{}, ErrTokenRevoked
	}
	return t, nil
}

// RevokeHostToken revokes by id; also revokes every session minted under that host
// so a stolen session key dies with its token.
func (s *SQLiteStore) RevokeHostToken(id string) error {
	now := s.clock().Format(time.RFC3339)
	var host string
	if err := s.db.QueryRow(`SELECT host FROM host_tokens WHERE token_id=? AND revoked=''`, id).Scan(&host); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenUnknown
		}
		return fmt.Errorf("routerstore: RevokeHostToken: %w", err)
	}
	if _, err := s.exec(`UPDATE host_tokens SET revoked=? WHERE token_id=?`, now, id); err != nil {
		return fmt.Errorf("routerstore: RevokeHostToken: %w", err)
	}
	if _, err := s.exec(`UPDATE sessions SET revoked=? WHERE host=? AND revoked=''`, now, host); err != nil {
		return fmt.Errorf("routerstore: RevokeHostToken: revoke sessions: %w", err)
	}
	return nil
}

// AdoptTokenMachineID records that the live host token for `host` also answers
// to the stable `machineID` (ADR-067, rs-42). The credential is the token
// itself: the SERVER injects host=sess.Host from the authenticated session
// (serve.go, the DeleteThreadCAS pattern), so a caller can only adopt onto a
// host it authenticated as — proof of the OLD identity authorizing the NEW one,
// recorded once, never inferred from string shape (the rejected bridge, §2).
//
// One-way and exclusive: re-adopting the SAME id is idempotent; a DIFFERENT id
// on an already-adopted token is refused (ErrMachineIDAdopted); an id already
// held by another live token is refused (ErrMachineIDClaimed, the §3.2 cap that
// makes the worst case a reversible DoS instead of silent impersonation).
func (s *SQLiteStore) AdoptTokenMachineID(host, machineID string) error {
	if host == "" || machineID == "" {
		return errors.New("routerstore: AdoptTokenMachineID: host and machine id are both required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("routerstore: AdoptTokenMachineID: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The caller's live token(s) for this host. Exactly one is the norm.
	rows, err := tx.Query(`SELECT token_id, machine_id FROM host_tokens WHERE host=? AND revoked=''`, host)
	if err != nil {
		return fmt.Errorf("routerstore: AdoptTokenMachineID: lookup host: %w", err)
	}
	type tok struct{ id, mid string }
	var toks []tok
	for rows.Next() {
		var t tok
		if err := rows.Scan(&t.id, &t.mid); err != nil {
			_ = rows.Close()
			return err
		}
		toks = append(toks, t)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	switch len(toks) {
	case 0:
		return ErrNoTokenForHost
	case 1:
		// normal
	default:
		return ErrAmbiguousHostToken
	}
	switch cur := toks[0].mid; {
	case cur == machineID:
		return nil // idempotent: already adopted to this id
	case cur != "":
		return ErrMachineIDAdopted // one-way: refuse a silent re-point
	}

	// Exclusivity: is this id already held by a DIFFERENT live token?
	var otherHost string
	err = tx.QueryRow(`SELECT host FROM host_tokens WHERE machine_id=? AND revoked='' AND token_id!=?`, machineID, toks[0].id).Scan(&otherHost)
	if err == nil {
		return ErrMachineIDClaimed
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("routerstore: AdoptTokenMachineID: exclusivity check: %w", err)
	}

	if _, err := tx.Exec(`UPDATE host_tokens SET machine_id=? WHERE token_id=?`, machineID, toks[0].id); err != nil {
		// The partial unique index is the backstop against a race that slips past
		// the SELECT above; surface it as the exclusivity refusal, not a raw error.
		return fmt.Errorf("routerstore: AdoptTokenMachineID: %w (%v)", ErrMachineIDClaimed, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("routerstore: AdoptTokenMachineID: commit: %w", err)
	}
	return nil
}

// HostIdentity resolves an identity string to its canonical form for the
// authority checks (ADR-067 §3.3): if a live host token binds `id` (as its host
// OR as its adopted machine id), return that token's adopted machine id;
// otherwise return `id` unchanged. Two identities "resolve together" iff their
// HostIdentity values are equal — which happens only when a live token, adopted
// by an authenticated act, ties them to the same machine. Server-internal only
// (notServed): it is read by threadAuthority and MintSession, never over the wire.
func (s *SQLiteStore) HostIdentity(id string) (string, error) {
	if id == "" {
		return "", nil
	}
	var mid string
	err := s.db.QueryRow(
		`SELECT machine_id FROM host_tokens WHERE revoked='' AND machine_id!='' AND (host=? OR machine_id=?) LIMIT 1`,
		id, id).Scan(&mid)
	if errors.Is(err, sql.ErrNoRows) {
		return id, nil // no adoption touches this identity → it is its own canon
	}
	if err != nil {
		return "", fmt.Errorf("routerstore: HostIdentity: %w", err)
	}
	return mid, nil
}

// ListHostTokens returns every token record, revoked ones included.
func (s *SQLiteStore) ListHostTokens() ([]HostToken, error) {
	rows, err := s.db.Query(`SELECT token_id,host,label,created,revoked,machine_id FROM host_tokens ORDER BY created, token_id`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: ListHostTokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []HostToken
	for rows.Next() {
		var t HostToken
		if err := rows.Scan(&t.ID, &t.Host, &t.Label, &t.Created, &t.Revoked, &t.MachineID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
