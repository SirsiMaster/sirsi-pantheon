//go:build darwin || linux

package routerstore

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Compare equivalent private idle fixtures, not production CPU or latency.
func TestRelaySingleScanIsolatedIdleCost(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 85; i++ {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("lane-%03d", i), "req"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cpu := func() time.Duration {
		var u syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &u); err != nil {
			t.Fatal(err)
		}
		return time.Duration(u.Utime.Sec+u.Stime.Sec)*time.Second + time.Duration(u.Utime.Usec+u.Stime.Usec)*time.Microsecond
	}
	oldPass := func() {
		_, _ = filepath.Glob(filepath.Join(root, "*", "req", "*.json"))
		lanes, _ := os.ReadDir(root)
		for _, l := range lanes {
			if l.IsDir() {
				_, _ = os.ReadDir(filepath.Join(root, l.Name(), "req"))
			}
		}
	}
	rl := &Relay{Spool: root, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	// A fixed pass count (about 2.4 s of ticks) keeps both fixtures on equal
	// intervals. A wall-clock bound let a loaded host drop a tick from one of
	// them (15 vs 16 on a shared CI Mac), failing a comparison that only logs CPU.
	passes := int(2400 * time.Millisecond / spoolPoll)
	if passes < 1 {
		passes = 1
	}
	measure := func(fn func()) (int, time.Duration, time.Duration) {
		startCPU, start := cpu(), time.Now()
		ticker := time.NewTicker(spoolPoll)
		defer ticker.Stop()
		n := 0
		for n < passes {
			fn()
			n++
			<-ticker.C
		}
		return n, cpu() - startCPU, time.Since(start)
	}
	on, oc, ow := measure(oldPass)
	nn, nc, nw := measure(func() { rl.discoverTracked(context.Background(), nil) })
	t.Logf("isolated idle 85-lane fixture: old passes=%d cpu=%s wall=%s; single-scan passes=%d cpu=%s wall=%s", on, oc, ow, nn, nc, nw)
	if on != nn {
		t.Fatalf("test intervals differ: %d vs %d", on, nn)
	}
}
