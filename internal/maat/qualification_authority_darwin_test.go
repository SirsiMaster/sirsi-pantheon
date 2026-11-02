//go:build darwin

package maat

import (
	"testing"
)

func TestCanonicalInstalledAuthorityRefusesNoncanonicalGoTestExecutable(t *testing.T) {
	authority, err := VerifyCurrentInstalledLifecycleAuthority()
	if err == nil {
		t.Fatal("ordinary go test executable unexpectedly received canonical installed-engine authority")
	}
	if authority.Valid() {
		t.Fatal("noncanonical go test executable received a valid installed-engine authority")
	}
	inventory, err := readLifecycleAcceptanceInventory()
	if err != nil || len(inventory.Policy) != 0 || len(inventory.Evaluation) != 0 || len(inventory.Outcome) != 0 {
		t.Fatalf("unexpected empirical installed evidence inventory: %+v err=%v", inventory, err)
	}
	// Positive exact-pin acceptance is exercised through the inert package-private
	// verifier seam in TestInstalledVerifierAcceptsSelectedSampleHeadAndRejectsOriginDrift.
	// This Darwin-only check must never treat the temporary go test binary as the
	// canonical installed engine.
}
