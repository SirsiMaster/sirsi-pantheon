package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatCoverageProgressPrefixDoesNotInventTotal(t *testing.T) {
	if got := maatCoverageProgressPrefix(maat.PackageProgress{Current: 91, Total: 0}); got != "  [91]" {
		t.Fatalf("unknown-total prefix = %q, want ordinal only", got)
	}
	if got := maatCoverageProgressPrefix(maat.PackageProgress{Current: 2, Total: 7}); got != "  [2/7]" {
		t.Fatalf("known-total prefix = %q, want exact total", got)
	}
}
