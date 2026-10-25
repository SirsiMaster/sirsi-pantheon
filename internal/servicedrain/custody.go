package servicedrain

import (
	"errors"
	"strings"
)

// Dimensions are shared by preservation and successor readiness. One retained
// envelope has one audit record: a 65536-envelope inventory is not a
// 65536-directory-entry inventory. Limits apply to the entire physical root.
const MaxCustodyEnvelopes = 65536
const MaxCustodyAudits = MaxCustodyEnvelopes
const MaxCustodyEntries = MaxCustodyEnvelopes + MaxCustodyAudits
const MaxCustodyLanes = 1024

type CustodyBudget struct{ Envelopes, Audits, Entries int }

func (b *CustodyBudget) Observe(name string) error {
	prefix := "audit-"
	if strings.HasPrefix(name, "envelope-") {
		prefix = "envelope-"
	}
	if !strings.HasPrefix(name, prefix) || !validHex(strings.TrimPrefix(name, prefix), 32) {
		return errors.New("maintenance custody: unknown entry retained")
	}
	b.Entries++
	if prefix == "audit-" {
		b.Audits++
	} else {
		b.Envelopes++
	}
	if b.Entries > MaxCustodyEntries || b.Audits > MaxCustodyAudits || b.Envelopes > MaxCustodyEnvelopes {
		return errors.New("maintenance custody: global dimensioned inventory bound exceeded")
	}
	return nil
}
