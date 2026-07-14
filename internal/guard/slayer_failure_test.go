package guard

import (
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// SlayWith is a process-kill module; Rule A16 requires its FAILURE path be
// tested. The existing slayer tests use a Mock whose Kill always succeeded, so
// the result.Failed++/Errors branch (slayer.go) was never exercised. With Kill
// error-injection the failed kill is now covered: SlayWith aggregates the failure
// into the result (not a top-level error) and records it.
func TestSlayWith_KillFailureRecorded(t *testing.T) {
	m := &platform.Mock{
		CommandResults: map[string]string{
			"ps -axo pid,rss,vsz,%cpu,user,command": "  PID   RSS   VSZ  %CPU USER     COMM\n 9999 51200 81920 1.5 user node",
		},
		KillErr: errors.New("operation not permitted"),
	}
	result, err := SlayWith(m, SlayNode, false)
	if err != nil {
		t.Fatalf("SlayWith aggregates kill failures into the result, not a top-level err: %v", err)
	}
	if len(m.KillCalls) < 1 {
		t.Fatalf("setup: SlayNode should have attempted a kill (KillCalls empty) — result=%+v", result)
	}
	if result.Failed < 1 {
		t.Fatalf("a failed kill must increment result.Failed, got %+v", result)
	}
	if len(result.Errors) < 1 {
		t.Fatal("a failed kill must be recorded in result.Errors")
	}
}
