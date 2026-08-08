package main

import "testing"

func TestDisplayUnknown(t *testing.T) {
	if got := displayUnknown(""); got != "unknown" {
		t.Fatalf("empty identity = %q", got)
	}
	if got := displayUnknown("pantheon"); got != "pantheon" {
		t.Fatalf("identity = %q", got)
	}
}
