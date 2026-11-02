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
