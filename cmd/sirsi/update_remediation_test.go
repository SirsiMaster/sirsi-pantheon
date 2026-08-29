package main

import "testing"

func TestBinaryDriftRemediationUsesPantheonSignedAppUpdater(t *testing.T) {
	command, description := remediationFor("binary-drift")
	if command != "sirsi update --app" {
		t.Fatalf("binary drift remediation = %q, want signed app updater", command)
	}
	if description == "" {
		t.Fatal("binary drift remediation requires an owner-readable description")
	}
}
