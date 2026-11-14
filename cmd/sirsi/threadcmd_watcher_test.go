//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// A bounded in-memory custody provider: these tests never probe, spawn, signal
// or kill a process and never need a caller-supplied PID to retire a watcher.
type memoryRouterWatcherFiles struct {
	data     map[string][]byte
	readErr  error
	writeErr error
	writes   int
}

func (f *memoryRouterWatcherFiles) read(leaf string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.data[leaf]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (f *memoryRouterWatcherFiles) write(leaf string, data []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes++
	f.data[leaf] = append([]byte(nil), data...)
	return nil
}

func (f *memoryRouterWatcherFiles) Close() error { return nil }

func memoryRouterWatcher(t *testing.T) (*routerWatcherCustody, *memoryRouterWatcherFiles) {
	t.Helper()
	leaf, err := routerWatcherLeaf("thr-fixture")
	if err != nil {
		t.Fatal(err)
	}
	record := routerWatcherRecord{Version: 1, Kind: "sirsi-thread-watch-router", ThreadID: "thr-fixture",
		AgentID: "fixture", RouterRoot: "/fixture/router", PID: 12345, Nonce: strings.Repeat("a", 64)}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	files := &memoryRouterWatcherFiles{data: map[string][]byte{leaf: data}}
	return &routerWatcherCustody{files: files, record: record, leaf: leaf}, files
}

func TestRouterWatcherExactCooperativeStop(t *testing.T) {
	c, files := memoryRouterWatcher(t)
	if c.retired() {
		t.Fatal("own unrequested watcher must stay armed")
	}
	requested, err := requestRouterWatcherStopFiles(files, c.leaf, c.record.ThreadID)
	if err != nil || !requested || !c.retired() {
		t.Fatalf("exact incarnation stop: requested=%v err=%v retired=%v", requested, err, c.retired())
	}
	if !bytes.Equal(files.data[c.leaf], files.data[c.leaf+".stop"]) {
		t.Fatal("stop must retain exact whole publication, not just its PID")
	}
}

func TestRouterWatcherRetirementDecision(t *testing.T) {
	for _, tc := range []struct {
		name             string
		setup            func(*routerWatcherCustody, *memoryRouterWatcherFiles)
		ancestorErr      error
		retired, failure bool
	}{
		{name: "active"},
		{name: "exact stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			f.data[c.leaf+".stop"] = append([]byte(nil), f.data[c.leaf]...)
		}, retired: true},
		{name: "missing publication", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) { delete(f.data, c.leaf) }, retired: true, failure: true},
		{name: "malformed stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) { f.data[c.leaf+".stop"] = []byte("invalid") }, retired: true, failure: true},
		{name: "stale stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			stale := c.record
			stale.Nonce = strings.Repeat("b", 64)
			f.data[c.leaf+".stop"], _ = json.Marshal(stale)
		}},
		{name: "successor publication", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			next := c.record
			next.Nonce = strings.Repeat("b", 64)
			f.data[c.leaf], _ = json.Marshal(next)
		}, retired: true, failure: true},
		{name: "ancestor changed despite exact stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			f.data[c.leaf+".stop"] = append([]byte(nil), f.data[c.leaf]...)
		}, ancestorErr: errors.New("replaced"), retired: true, failure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, f := memoryRouterWatcher(t)
			if tc.setup != nil {
				tc.setup(c, f)
			}
			retired, err := routerWatcherRetirementDecision(func() error { return tc.ancestorErr }, c)
			if retired != tc.retired || (err != nil) != tc.failure {
				t.Fatalf("retired=%v err=%v; want retired=%v failure=%v", retired, err, tc.retired, tc.failure)
			}
		})
	}
}

func TestRouterWatcherRolloverAndStaleStop(t *testing.T) {
	old, files := memoryRouterWatcher(t)
	lateRequest := append([]byte(nil), files.data[old.leaf]...)
	next := old.record
	next.Nonce = strings.Repeat("b", 64) // even the SAME PID is a new incarnation
	data, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	files.data[old.leaf] = data
	files.data[old.leaf+".stop"] = lateRequest
	successor := &routerWatcherCustody{files: files, record: next, leaf: old.leaf}
	if !old.retired() || successor.retired() {
		t.Fatal("rollover must retire old incarnation without applying its late stop to successor")
	}
	if err := old.Close(); err != nil || !bytes.Equal(files.data[old.leaf], data) {
		t.Fatal("old cleanup must not unlink or overwrite successor publication")
	}
}

func TestRouterWatcherRejectsUnverifiableRecords(t *testing.T) {
	for _, change := range []string{"numeric", "thread", "kind", "nonce", "pid", "oversize"} {
		t.Run(change, func(t *testing.T) {
			c, files := memoryRouterWatcher(t)
			record := c.record
			switch change {
			case "thread":
				record.ThreadID = "thr-foreign"
			case "kind":
				record.Kind = "sleep"
			case "nonce":
				record.Nonce = ""
			case "pid":
				record.PID = -1
			}
			data, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if change == "numeric" {
				data = []byte("12345\n")
			}
			if change == "oversize" {
				data = bytes.Repeat([]byte(" "), 4097)
			}
			files.data[c.leaf] = data
			requested, err := requestRouterWatcherStopFiles(files, c.leaf, c.record.ThreadID)
			if err == nil || requested || files.writes != 0 || !bytes.Equal(data, files.data[c.leaf]) {
				t.Fatalf("unverifiable record must remain unchanged, no request/signal: requested=%v writes=%d err=%v", requested, files.writes, err)
			}
		})
	}
}

func TestRouterWatcherIOFailureIsNotSuccessfulStop(t *testing.T) {
	for _, operation := range []string{"read", "write"} {
		t.Run(operation, func(t *testing.T) {
			c, files := memoryRouterWatcher(t)
			if operation == "read" {
				files.readErr = os.ErrPermission
			} else {
				files.writeErr = io.ErrShortWrite
			}
			if requested, err := requestRouterWatcherStopFiles(files, c.leaf, c.record.ThreadID); err == nil || requested {
				t.Fatalf("failed %s cannot confirm stop: requested=%v err=%v", operation, requested, err)
			}
		})
	}
	c, files := memoryRouterWatcher(t)
	files.readErr = os.ErrPermission
	if !c.retired() {
		t.Fatal("watcher must self-retire on custody loss")
	}
}

func TestRouterWatcherLegacyDiagnosticOnly(t *testing.T) {
	calls := 0
	err := legacyRouterWatcherDiagnostic("thr-fixture", func(path string) (os.FileInfo, error) {
		calls++
		if path != "/tmp/sirsi-router-watch-thr-fixture.pid" {
			t.Fatalf("unexpected diagnostic path %q", path)
		}
		return nil, nil // entry exists; no reading or cleanup API is supplied
	})
	if err == nil || !strings.Contains(err.Error(), "may still be running") || calls != 1 {
		t.Fatalf("legacy presence must remain visible: calls=%d err=%v", calls, err)
	}
	if err := legacyRouterWatcherDiagnostic("../foreign", func(string) (os.FileInfo, error) {
		t.Fatal("invalid ID must not reach filesystem")
		return nil, nil
	}); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := legacyRouterWatcherDiagnostic("thr-fixture", func(string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}); !errors.Is(err, os.ErrPermission) {
		t.Fatal("diagnostic inspection error hidden")
	}
}

func TestRouterWatcherDispatchGateCancellationAndDrain(t *testing.T) {
	c, files := memoryRouterWatcher(t)
	var gate routerWatcherDispatchGate
	ctx, cancel := context.WithCancel(context.Background())
	if !gate.begin(ctx, c) {
		t.Fatal("own armed callback denied")
	}
	cancel()
	if gate.begin(ctx, c) {
		t.Fatal("pending callback entered after context cancellation")
	}
	// An admitted callback remains visible until it returns; no consumer-killer
	// exists in this gate. Checking admission after stop also covers queued timers.
	if requested, err := requestRouterWatcherStopFiles(files, c.leaf, c.record.ThreadID); !requested || err != nil {
		t.Fatal(err)
	}
	if gate.begin(context.Background(), c) {
		t.Fatal("pending callback entered after matching stop")
	}
	gate.end()
	var output bytes.Buffer
	gate.drain(&output)
	fresh, _ := memoryRouterWatcher(t)
	if gate.begin(context.Background(), fresh) {
		t.Fatal("closed gate admitted callback")
	}
}

type routerWatcherDrainNotice struct{ notice chan struct{} }

func (w routerWatcherDrainNotice) Write(data []byte) (int, error) {
	select {
	case w.notice <- struct{}{}:
	default:
	}
	return len(data), nil
}

func TestRouterWatcherDrainWaitsForInFlightCallback(t *testing.T) {
	c, _ := memoryRouterWatcher(t)
	var gate routerWatcherDispatchGate
	if !gate.begin(context.Background(), c) {
		t.Fatal("own callback not admitted")
	}
	notice := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		gate.drain(routerWatcherDrainNotice{notice: notice})
		close(done)
	}()
	select {
	case <-notice:
	case <-time.After(2 * time.Second):
		gate.end()
		t.Fatal("in-flight callback uncertainty was not surfaced")
	}
	select {
	case <-done:
		t.Fatal("watcher claimed drain before callback returned")
	default:
	}
	if gate.begin(context.Background(), c) {
		t.Fatal("draining watcher admitted a new callback")
	}
	gate.end() // callback returns ordinarily; no consumer interruption
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("returned callback did not release drain")
	}
}

func TestRouterWatcherObservationArmsBeforeCheckingCustody(t *testing.T) {
	paths, err := routerWatcherCustodyPaths("/private/home")
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range []int{0, 1, len(paths) - 1} {
		t.Run(fmt.Sprint(gap), func(t *testing.T) {
			armed, stopped, checks := 0, false, 0
			retired, err := armRouterWatcherObservation(paths, func(path string) error {
				if path != paths[armed] {
					t.Fatalf("unexpected subscription order %q", path)
				}
				if armed == gap {
					stopped = true
				}
				armed++
				return nil
			}, func() (bool, error) {
				checks++
				if armed != len(paths) {
					t.Fatal("checked custody before watches were installed")
				}
				return stopped, nil
			})
			if err != nil || !retired || checks != 1 {
				t.Fatalf("setup gap: retired=%v checks=%d err=%v", retired, checks, err)
			}
		})
	}
	checks := 0
	_, err = armRouterWatcherObservation(paths, func(string) error { return os.ErrPermission }, func() (bool, error) { checks++; return false, nil })
	if !errors.Is(err, os.ErrPermission) || checks != 0 {
		t.Fatalf("failed subscription must refuse: checks=%d err=%v", checks, err)
	}
}

func TestRouterWatcherObservationSetupRetirementReason(t *testing.T) {
	for _, tc := range []struct {
		name             string
		setup            func(*routerWatcherCustody, *memoryRouterWatcherFiles)
		ancestorErr      error
		retired, failure bool
	}{
		{name: "active"},
		{name: "exact stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			f.data[c.leaf+".stop"] = append([]byte(nil), f.data[c.leaf]...)
		}, retired: true},
		{name: "stale stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			stale := c.record
			stale.Nonce = strings.Repeat("b", 64)
			f.data[c.leaf+".stop"], _ = json.Marshal(stale)
		}},
		{name: "missing publication", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) { delete(f.data, c.leaf) }, retired: true, failure: true},
		{name: "malformed publication", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) { f.data[c.leaf] = []byte("invalid") }, retired: true, failure: true},
		{name: "successor publication", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			next := c.record
			next.Nonce = strings.Repeat("b", 64)
			f.data[c.leaf], _ = json.Marshal(next)
		}, retired: true, failure: true},
		{name: "malformed stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) { f.data[c.leaf+".stop"] = []byte("invalid") }, retired: true, failure: true},
		{name: "ancestor changed despite exact stop", setup: func(c *routerWatcherCustody, f *memoryRouterWatcherFiles) {
			f.data[c.leaf+".stop"] = append([]byte(nil), f.data[c.leaf]...)
		}, ancestorErr: errors.New("replaced"), retired: true, failure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, files := memoryRouterWatcher(t)
			armed := false
			retired, err := armRouterWatcherObservation([]string{"/fixture/custody"}, func(string) error {
				// The request/replacement occurs after publication, while arming.
				if tc.setup != nil {
					tc.setup(c, files)
				}
				armed = true
				return nil
			}, func() (bool, error) {
				if !armed {
					t.Fatal("retirement checked before subscription")
				}
				return routerWatcherRetirementDecision(func() error { return tc.ancestorErr }, c)
			})
			if retired != tc.retired || (err != nil) != tc.failure {
				t.Fatalf("retired=%v err=%v; want retired=%v failure=%v", retired, err, tc.retired, tc.failure)
			}
		})
	}
}

func TestRouterWatcherCustodyEventsIncludeReplacementAndStop(t *testing.T) {
	c, files := memoryRouterWatcher(t)
	if c.retired() {
		t.Fatal("initial custody must be active")
	}
	newRecord := c.record
	newRecord.Nonce = strings.Repeat("b", 64)
	data, err := json.Marshal(newRecord)
	if err != nil {
		t.Fatal(err)
	}
	files.data[c.leaf] = data
	if !c.retired() {
		t.Fatal("atomic replacement must retire previous incarnation")
	}
}

func TestRouterWatcherObservationLossIsTerminal(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		open bool
	}{
		{"overflow", fsnotify.ErrEventOverflow, true},
		{"backend failure", os.ErrPermission, true},
		{"closed error stream", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := routerWatcherObservationError(tc.err, tc.open); err == nil {
				t.Fatal("observation loss must terminate without polling fallback")
			}
		})
	}
}
