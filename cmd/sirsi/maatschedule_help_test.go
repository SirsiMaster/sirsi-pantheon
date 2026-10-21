package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestReserveDefaultsToSharedBuildNotQuiet(t *testing.T) {
	f := maatReserveCmd.Flags().Lookup("regime")
	if f == nil || f.DefValue != "build" {
		t.Fatalf("ordinary reservations must default to shared build, got %+v", f)
	}
}

// A runner that trusts the help text must get the real contract: defer is the
// refusal exit code and clear is 0 (Mercury's runner deferred every job on the
// old wording, which said "Exit 0 if a runner must defer").
func TestShouldDeferHelpStatesTheExitContract(t *testing.T) {
	h := maatShouldDeferCmd.Short
	if !strings.Contains(h, "Exit "+strconv.Itoa(admissionRefusedExit)+" if a runner must defer") || !strings.Contains(h, "exit 0 if clear") {
		t.Fatalf("help must state defer=%d, clear=0; got %q", admissionRefusedExit, h)
	}
}
