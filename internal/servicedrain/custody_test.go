package servicedrain

import (
	"fmt"
	"testing"
)

func TestCustodyGlobalEnvelopeAndAuditDimensions(t *testing.T) {
	var preservation, readiness CustodyBudget
	// The exact LR01 counterexample crosses 65536 directory entries without
	// exceeding 65536 envelopes. Both production paths consume this same budget.
	for lane := 0; lane < 2; lane++ {
		for n := 0; n < 16385; n++ {
			for _, prefix := range []string{"audit-", "envelope-"} {
				name := fmt.Sprintf("%s%064x", prefix, lane*16385+n)
				if err := preservation.Observe(name); err != nil {
					t.Fatal(err)
				}
				if err := readiness.Observe(name); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if preservation != readiness || readiness.Envelopes != 32770 || readiness.Entries != 65540 {
		t.Fatal(preservation, readiness)
	}
	for n := 32770; n < MaxCustodyEnvelopes; n++ {
		for _, prefix := range []string{"audit-", "envelope-"} {
			if err := readiness.Observe(fmt.Sprintf("%s%064x", prefix, n)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := readiness.Observe(fmt.Sprintf("envelope-%064x", MaxCustodyEnvelopes)); err == nil {
		t.Fatal("global envelope limit not enforced")
	}
}
