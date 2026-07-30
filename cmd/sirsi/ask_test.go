package main

import (
	"strings"
	"testing"
)

func TestAskRejectsJSONFixBeforeDiagnosis(t *testing.T) {
	oldJSON, oldFix := askJSON, askFix
	askJSON, askFix = true, true
	t.Cleanup(func() {
		askJSON, askFix = oldJSON, oldFix
	})

	err := runAsk(nil, []string{"what is wrong?"})
	if err == nil {
		t.Fatal("--json --fix must not silently return diagnostic JSON without performing the requested repair")
	}
	if !strings.Contains(err.Error(), "--json and --fix cannot be combined") {
		t.Fatalf("error = %q, want explicit incompatible-flags diagnosis", err)
	}
}
