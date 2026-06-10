package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func TestFindSpotlightStorm(t *testing.T) {
	findings := []guard.DiagnosticFinding{
		{Check: "RAM Pressure", Severity: guard.SeverityOK},
		{Check: "Spotlight Storm", Severity: guard.SeverityWarn, Message: "busy", Detail: "mds_stores 40%"},
		{Check: "Disk Space", Severity: guard.SeverityOK},
	}
	got := findSpotlightStorm(findings)
	if got == nil {
		t.Fatal("expected to find the Spotlight Storm finding")
	}
	if got.Severity != guard.SeverityWarn || detailOf(got) != "mds_stores 40%" {
		t.Errorf("wrong finding returned: %+v", got)
	}

	// Absent → nil, and detailOf(nil) is empty (no panic).
	if findSpotlightStorm(findings[:1]) != nil {
		t.Error("expected nil when no Spotlight Storm finding present")
	}
	if detailOf(nil) != "" {
		t.Error("detailOf(nil) should be empty")
	}
}

func TestDefaultDevDir(t *testing.T) {
	if !strings.HasSuffix(defaultDevDir(), "Development") {
		t.Errorf("defaultDevDir() = %q, want it to end in Development", defaultDevDir())
	}
}
