package router

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

type refreshPublicationNotice struct {
	agentID        string
	root           string
	watchOnly      chan struct{}
	heartbeat      chan struct{}
	barrierErr     chan error
	watchOnce      sync.Once
	heartbeatCount atomic.Int32
	retried        chan struct{}
}

func (n *refreshPublicationNotice) Write(p []byte) (int, error) {
	line := string(p)
	if strings.Contains(line, "wake-loop "+n.agentID+": WATCH-ONLY: consumer configuration unavailable:") {
		n.watchOnce.Do(func() {
			err := os.Chmod(n.root, 0o500)
			n.barrierErr <- err
			close(n.watchOnly)
		})
	}
	if strings.Contains(line, "wake-loop "+n.agentID+": heartbeat FAILED; new dispatch held") &&
		strings.Contains(strings.ToLower(line), "permission denied") {
		switch n.heartbeatCount.Add(1) {
		case 1:
			close(n.heartbeat)
		case 2:
			close(n.retried)
		}
	}
	return len(p), nil
}

func TestWakeLoopRefreshPublicationFailureHoldsObserver(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permission barrier is unavailable when running as root")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv(routercfg.StoreWakeEnv, "0")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	const agentID = "refresh-publication-observer"
	oldLoad := getLoadAvgFn()
	SetLoadAvgFn(func() (float64, bool) { return 0, true })
	defer SetLoadAvgFn(oldLoad)

	cfg := AgentConfig{ID: agentID, Type: "worker", Workstream: "refresh-publication-fixture"}
	root := wakeTestRoot(t, cfg)
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	originalMode := info.Mode().Perm()
	t.Setenv("SIRSI_AGENT_ID", "")
	t.Setenv("SIRSI_THREAD_ID", "")
	notice := &refreshPublicationNotice{
		agentID: agentID, root: root,
		watchOnly: make(chan struct{}),
		heartbeat: make(chan struct{}), retried: make(chan struct{}), barrierErr: make(chan error, 1),
	}
	previousOutput := log.Writer()
	log.SetOutput(notice)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		done <- RunWakeLoop(ctx, root, agentID, 5*time.Millisecond)
	}()
	defer func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			t.Error("fixture observer did not terminate after cancellation")
		}
		if err := os.Chmod(root, originalMode); err != nil {
			t.Errorf("restore fixture router root permissions: %v", err)
		}
		log.SetOutput(previousOutput)
	}()

	// The callback installs the barrier after the ordinary heartbeat succeeds
	// and immediately before the refreshed invalid configuration is published.
	select {
	case <-notice.watchOnly:
	case err := <-done:
		t.Fatalf("observer exited during consumer refresh: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("WATCH-ONLY refresh was not observed")
	}
	if err := <-notice.barrierErr; err != nil {
		t.Fatalf("install router-root permission barrier in refresh callback: %v", err)
	}
	select {
	case <-notice.heartbeat:
	case err := <-done:
		t.Fatalf("observer exited instead of holding dispatch after publication failure: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("canonical heartbeat failure with permission denied was not observed")
	}
	select {
	case <-notice.retried:
	case err := <-done:
		t.Fatalf("observer exited instead of retrying after publication failure: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("observer did not retry under the publication write barrier")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("observer cancellation after publication retry: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("observer did not finish after cancellation")
	}
}
