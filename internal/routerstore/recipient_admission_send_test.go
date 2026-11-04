package routerstore

import (
	"errors"
	"testing"
)

// The raw Send RPC must not route around the recipient limit.
func TestRawSendHonorsRecipientLimit(t *testing.T) {
	tightenAdmission(t)
	s := openTestStore(t)
	seedOpen(t, s, "lane", 5)
	if _, err := s.Send("ra", "lane", "one more", "proposal", "x"); !errors.Is(err, ErrRecipientSaturated) {
		t.Fatalf("raw Send to saturated lane: err = %v, want ErrRecipientSaturated", err)
	}
	if _, err := s.Send("routerstore", "lane", "escalation", "decision", "x"); err != nil {
		t.Fatalf("router-internal raw Send must stay exempt: %v", err)
	}
	if _, err := s.Send("ra", "quiet", "fine", "proposal", "x"); err != nil {
		t.Fatalf("raw Send to quiet lane: %v", err)
	}
}

// Only Ra reaches a saturated SSA, on both send paths; every other pair stays limited.
func TestRaReachesSaturatedSSA(t *testing.T) {
	tightenAdmission(t)
	s := openTestStore(t)
	const ssa = "sirsi-software-admin"
	seedOpen(t, s, ssa, 5)
	if _, err := s.Send("ra", ssa, "raw", "proposal", "x"); err != nil {
		t.Fatalf("raw Send ra to saturated SSA: %v", err)
	}
	if _, _, err := s.SendGuarded(SendReq{From: "ra", To: ssa, Title: "guarded", Type: "proposal", Instructions: "x"}); err != nil {
		t.Fatalf("SendGuarded ra to saturated SSA: %v", err)
	}
	if _, err := s.Send("mercury", ssa, "other sender", "proposal", "x"); !errors.Is(err, ErrRecipientSaturated) {
		t.Fatalf("another sender to saturated SSA: err = %v, want ErrRecipientSaturated", err)
	}
	if _, _, err := s.SendGuarded(SendReq{From: "mercury", To: ssa, Title: "other", Type: "proposal", Instructions: "x"}); !errors.Is(err, ErrRecipientSaturated) {
		t.Fatalf("guarded other sender to saturated SSA: err = %v, want ErrRecipientSaturated", err)
	}
}
