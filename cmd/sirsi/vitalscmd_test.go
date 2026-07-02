package main

// Tests for the `sirsi vitals` contract (TUI design proof gap V1).
// NOTE: no t.Parallel() anywhere here — these tests pin package-level
// behavior and sibling tests in this package swap globals (repo lessons
// #129/#131).

import (
	"encoding/json"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// TestBuildVitalsReport_Shape pins the JSON contract every surface decodes:
// field names, byte math, pressure vocabulary, and hog trimming.
func TestBuildVitalsReport_Shape(t *testing.T) {
	mem := guard.MemSample{
		TotalRAM:  48 << 30,
		FreeBytes: 16 << 30,
		Top: []guard.MemProc{
			{PID: 101, Name: "/Applications/Chrome.app/Contents/MacOS/Chrome", RSS: 4 << 30},
			{PID: 202, Name: "codex", RSS: 2 << 30},
			{PID: 303, Name: "node", RSS: 1 << 30},
		},
	}

	r := buildVitalsReport(mem, guard.PressureWarn, "kernel-dispatch", 512<<20, 2)

	if r.Command != "sirsi vitals" {
		t.Errorf("command = %q", r.Command)
	}
	if r.TotalBytes != 48<<30 || r.FreeBytes != 16<<30 {
		t.Errorf("total/free = %d/%d", r.TotalBytes, r.FreeBytes)
	}
	if want := int64(32 << 30); r.UsedBytes != want {
		t.Errorf("used = %d, want %d (total - free)", r.UsedBytes, want)
	}
	if r.SwapUsedBytes != 512<<20 {
		t.Errorf("swap = %d", r.SwapUsedBytes)
	}
	if r.Pressure != "warn" || r.PressureSource != "kernel-dispatch" {
		t.Errorf("pressure = %q/%q", r.Pressure, r.PressureSource)
	}
	// topN caps the hogs, and the first name is the basename, not the bundle path.
	if len(r.Top) != 2 {
		t.Fatalf("top = %d entries, want 2", len(r.Top))
	}
	if r.Top[0].Name != "Chrome" || r.Top[0].PID != 101 || r.Top[0].RSSBytes != 4<<30 {
		t.Errorf("top[0] = %+v", r.Top[0])
	}

	// Pin the wire field names — the decoders in TUI/menubar/dashboard depend
	// on these exact keys.
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"command", "total_bytes", "used_bytes", "free_bytes", "swap_used_bytes", "pressure", "pressure_source", "top"} {
		if _, ok := wire[key]; !ok {
			t.Errorf("JSON contract missing key %q", key)
		}
	}
	top, ok := wire["top"].([]interface{})
	if !ok || len(top) == 0 {
		t.Fatalf("top not an array: %v", wire["top"])
	}
	entry := top[0].(map[string]interface{})
	for _, key := range []string{"name", "pid", "rss_bytes"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("top entry missing key %q", key)
		}
	}
}

// TestBuildVitalsReport_Empty confirms a sample with no hogs yields an empty
// (never null) top array and sane zero math — the loading/empty states the
// TUI renders must be decodable.
func TestBuildVitalsReport_Empty(t *testing.T) {
	r := buildVitalsReport(guard.MemSample{}, guard.PressureUnknown, "unknown", 0, 5)
	if r.UsedBytes != 0 {
		t.Errorf("used = %d for empty sample, want 0 (never negative)", r.UsedBytes)
	}
	if r.Top == nil || len(r.Top) != 0 {
		t.Errorf("top must be an empty array, got %v", r.Top)
	}
	b, _ := json.Marshal(r)
	if string(b) == "" || !json.Valid(b) {
		t.Errorf("empty report must marshal to valid JSON")
	}
	// "top":[] not "top":null — dispatchers range over it without nil checks.
	var wire map[string]interface{}
	_ = json.Unmarshal(b, &wire)
	if _, isArray := wire["top"].([]interface{}); !isArray {
		t.Errorf("top must serialize as [] (got %T)", wire["top"])
	}
}

// TestBuildVitalsReport_TopNZeroMeansAll — topN <= 0 includes every sampled hog.
func TestBuildVitalsReport_TopNZeroMeansAll(t *testing.T) {
	mem := guard.MemSample{Top: []guard.MemProc{{PID: 1, Name: "a"}, {PID: 2, Name: "b"}}}
	r := buildVitalsReport(mem, guard.PressureNormal, "bootstrap-snapshot", 0, 0)
	if len(r.Top) != 2 {
		t.Errorf("topN=0 should include all hogs, got %d", len(r.Top))
	}
}
