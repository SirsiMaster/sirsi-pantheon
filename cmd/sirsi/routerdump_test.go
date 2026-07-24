package main

import (
	"sort"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// TestDumpRecord pins the H4-boundary export contract: exactly the nine agreed
// fields, body sourced from Instructions, and NO internal state (wake fields,
// Result) leaking into the fabric feeder.
func TestDumpRecord(t *testing.T) {
	it := work.Item{
		ID: "20260724-x", From: "a", To: "b", Type: "review", Title: "T",
		Status: "open", Opened: "2026-07-24T00:00:00Z", Closed: "",
		Instructions: "the ask", Result: "SECRET",
		WakeStatus: "armed", WakeError: "leak-me",
	}
	rec := dumpRecord(it)

	keys := make([]string, 0, len(rec))
	for k := range rec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"body", "closed", "from", "id", "opened", "status", "title", "to", "type"}
	if len(keys) != len(want) {
		t.Fatalf("field set = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("field set = %v, want %v", keys, want)
		}
	}
	if rec["body"] != "the ask" {
		t.Errorf("body = %q, want Instructions", rec["body"])
	}
	// No internal state may leak.
	for k, v := range rec {
		if v == "SECRET" || v == "armed" || v == "leak-me" {
			t.Errorf("internal state leaked via %q = %q", k, v)
		}
	}
}
