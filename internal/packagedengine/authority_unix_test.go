//go:build darwin || linux

package packagedengine

import (
	"context"
	"reflect"
	"testing"
)

func TestVerificationCommandUsesFixedInlineTeamRequirement(t *testing.T) {
	path := "/usr/local/libexec/sirsi-pantheon/sirsi"
	cmd := VerificationCommand(context.Background(), path)
	want := []string{"/usr/bin/codesign", "--verify", "--strict", "-R", `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`, path}
	if cmd.Path != "/usr/bin/codesign" || !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("verification argv = %q, want %q", cmd.Args, want)
	}
}
