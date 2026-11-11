package verify

import (
	"flag"
	"testing"
)

// Runs last (file name sorts after every other test file). Fails if any deny
// code the package can return was never produced by a test, so a new code
// cannot ship unexercised. Only enforced on a full run (no -run filter).
func TestEveryDenyReasonIsExercised(t *testing.T) {
	if f := flag.Lookup("test.run"); f != nil && f.Value.String() != "" {
		t.Skip("subset run")
	}
	for _, r := range DenyReasons() {
		if _, ok := seenReasons.Load(r); !ok {
			t.Errorf("deny reason %q was never produced by any test", r)
		}
	}
}
