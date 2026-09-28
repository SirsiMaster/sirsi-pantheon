package main

import "testing"

func TestMaatIsDiscoverableFromTheCanonicalCLI(t *testing.T) {
	if maatCmd.Hidden {
		t.Fatal("Ma'at is hidden from the canonical CLI despite being Pantheon's default local intelligence")
	}
}
