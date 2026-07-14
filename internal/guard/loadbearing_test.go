package guard

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func writeGemmaPidfile(t *testing.T, pid int) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".sirsi", "gemma-server.pid"), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	return tmp
}

// The live gemma broker PID is recognized as load-bearing; a dead/absent PID is
// not (a stale pidfile must never protect a reused PID).
func TestLoadBearingRecognizesLiveBroker(t *testing.T) {
	writeGemmaPidfile(t, os.Getpid()) // this test process stands in for the broker
	if !IsLoadBearing(os.Getpid()) {
		t.Fatal("the live gemma broker PID must be recognized as load-bearing")
	}
	if IsLoadBearing(2147480000) {
		t.Error("a dead/absent PID must not be load-bearing")
	}
}

// FindRunaway must NEVER select the load-bearing broker, even when it is the
// single largest memory user (it runs as "Python", so name-based protection
// misses it). It returns the next non-protected process instead.
func TestFindRunawaySkipsLoadBearingBroker(t *testing.T) {
	writeGemmaPidfile(t, os.Getpid())
	s := MemSample{Top: []MemProc{
		{PID: os.Getpid(), Name: "Python", RSS: 25 << 30}, // the broker — biggest RSS
		{PID: 987654, Name: "SomeApp", RSS: 2 << 30},
	}}
	r := FindRunaway(s)
	if r == nil {
		t.Fatal("expected a runaway (the non-broker process), got nil")
	}
	if r.PID == os.Getpid() {
		t.Fatal("SAFETY: FindRunaway selected the load-bearing gemma broker")
	}
	if r.PID != 987654 {
		t.Errorf("expected the next non-protected proc (987654), got %+v", r)
	}
}
