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
	ID      string `json:"id"`
	Host    string `json:"host"`
	Label   string `json:"label"`
	Created string `json:"created"`
	Revoked string `json:"revoked,omitempty"`
}

var (
	ErrTokenUnknown = errors.New("routerstore: host token unknown")
	ErrTokenRevoked = errors.New("routerstore: host token revoked")
	ErrHostMismatch = errors.New("routerstore: token is not for this host")
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

// ListHostTokens returns every token record, revoked ones included.
func (s *SQLiteStore) ListHostTokens() ([]HostToken, error) {
	rows, err := s.db.Query(`SELECT token_id,host,label,created,revoked FROM host_tokens ORDER BY created, token_id`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: ListHostTokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []HostToken
	for rows.Next() {
		var t HostToken
		if err := rows.Scan(&t.ID, &t.Host, &t.Label, &t.Created, &t.Revoked); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
