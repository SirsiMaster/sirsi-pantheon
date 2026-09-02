package routerstore

import (
	"errors"
	"testing"
	"time"
)

// rs-11: per-host tokens. Each rejection sits beside its positive control.

func TestHostTokenMintLookupRevoke(t *testing.T) {
	h := newIdentityHarness(t)
	plain, rec, mintErr := h.backend.MintHostToken("m1", "backup mac")
	if mintErr != nil || plain == "" || rec.ID == "" {
		t.Fatalf("mint: %v %+v", mintErr, rec)
	}
	if got, err := h.backend.LookupHostToken(plain); err != nil || got.Host != "m1" {
		t.Fatalf("positive control: lookup minted token: %+v %v", got, err)
	}
	if _, err := h.backend.LookupHostToken("not-a-token"); !errors.Is(err, ErrTokenUnknown) {
		t.Fatalf("unknown token: want ErrTokenUnknown, got %v", err)
	}
	if err := h.backend.RevokeHostToken(rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.backend.LookupHostToken(plain); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("revoked token: want ErrTokenRevoked, got %v", err)
	}
	list, err := h.backend.ListHostTokens()
	if err != nil || len(list) != 1 || list[0].Revoked == "" {
		t.Fatalf("list after revoke: %+v %v", list, err)
	}
}

func TestHostTokenAuthorizesOnlyItsOwnHost(t *testing.T) {
	h := newIdentityHarness(t)
	plain, _, err := h.backend.MintHostToken("m1", "")
	if err != nil {
		t.Fatal(err)
	}
	// A node on m1 presenting m1's token: sessions mint, signed calls work.
	rs := NewRemoteStore(h.srv.URL, plain)
	rs.sessionDir = ""
	rs.host, rs.agent = "m1", "claude-m1"
	rs.now = func() time.Time { return h.now }
	if _, err := rs.Inbox("claude-m1"); err != nil {
		t.Fatalf("positive control: per-host token on its own host: %v", err)
	}
	// The same token claiming to be another host is refused at MintSession.
	impostor := NewRemoteStore(h.srv.URL, plain)
	impostor.sessionDir = ""
	impostor.host, impostor.agent = "m5", "claude-m5"
	impostor.now = func() time.Time { return h.now }
	if _, err := impostor.Inbox("claude-m5"); err == nil {
		t.Fatal("m1's token must not mint a session for host m5")
	}
}

func TestHostTokenRevocationKillsItsSessionsOnNextRequest(t *testing.T) {
	h := newIdentityHarness(t)
	plain, rec, mintErr := h.backend.MintHostToken("m1", "")
	if mintErr != nil {
		t.Fatal(mintErr)
	}
	rs := NewRemoteStore(h.srv.URL, plain)
	rs.sessionDir = ""
	rs.host, rs.agent = "m1", "claude-m1"
	rs.now = func() time.Time { return h.now }
	if _, err := rs.Inbox("claude-m1"); err != nil {
		t.Fatal("positive control:", err)
	}
	if err := h.backend.RevokeHostToken(rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rs.Inbox("claude-m1"); err == nil {
		t.Fatal("after token revocation the host's next request must fail")
	}
	// And the other host is unaffected (goal G6 revocation rehearsal shape).
	other, _, err := h.backend.MintHostToken("m5", "")
	if err != nil {
		t.Fatal(err)
	}
	o := NewRemoteStore(h.srv.URL, other)
	o.sessionDir = ""
	o.host, o.agent = "m5", "claude-m5"
	o.now = func() time.Time { return h.now }
	if _, err := o.Inbox("claude-m5"); err != nil {
		t.Fatalf("unrelated host must be unaffected by m1's revocation: %v", err)
	}
}
