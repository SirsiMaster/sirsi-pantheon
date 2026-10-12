package routerstore

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// This opt-in fixture forwards through the existing host relay to the real
// canonical service. Only the fixture's own edge is faulted; shared relays and
// cloud service are never stopped. No token is read into the fixture.
type liveRetryEdge struct {
	mu                  sync.Mutex
	down                bool
	upstream            http.RoundTripper
	failures, forwarded int
}

func (e *liveRetryEdge) RoundTrip(r *http.Request) (*http.Response, error) {
	e.mu.Lock()
	if strings.HasSuffix(r.URL.Path, "/SendGuarded") {
		if e.down {
			e.failures++
			e.mu.Unlock()
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		}
		e.forwarded++
	}
	e.mu.Unlock()
	return e.upstream.RoundTrip(r)
}

func TestLiveCanonicalSpoolRetryAndDuplicate(t *testing.T) {
	if os.Getenv("SIRSI_ROUTER_LIVE_PROOF") != "1" {
		t.Skip("explicit live coordination-fixture opt-in required")
	}
	spool := SpoolDir(os.Getenv("SIRSI_ROUTER_URL"))
	if spool == "" {
		t.Fatal("requires existing canonical host spool")
	}
	t.Setenv("SIRSI_AGENT_ID", "sirsi-software-admin")
	t.Setenv("HOME", t.TempDir()) // isolate fixture session cache
	edge := &liveRetryEdge{down: true, upstream: newSpoolTransport(spool, "sirsi-software-admin")}
	privateSpool := t.TempDir()
	rl := &Relay{Spool: privateSpool, Base: "http://spool", Token: "not-a-service-token", Client: &http.Client{Transport: edge}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rl.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("fixture relay failed to stop")
		}
	})
	rs := NewRemoteStore("spool://"+privateSpool, "")
	marker := fmt.Sprintf("SSA-LIVE-RETRY-ONLY-%d", time.Now().UnixNano())
	req := SendReq{From: "sirsi-software-admin", To: "sirsi-software-admin", Type: "decision", Title: marker, SubjectKey: marker, Instructions: "Coordination-only retry fixture; no product work. SSA will close this exact fixture after readback."}
	_, _, err := rs.SendGuarded(req)
	if err == nil || !strings.Contains(err.Error(), "QUEUED_FOR_RETRY") {
		t.Fatalf("expected durable queue, got %v", err)
	}
	held, err := filepath.Glob(filepath.Join(privateSpool, "sirsi-software-admin", "outbox", "*.json"))
	if err != nil || len(held) != 1 {
		t.Fatalf("durable outbox: %v %v", held, err)
	}
	edge.mu.Lock()
	if edge.failures < 2 || edge.forwarded != 0 {
		t.Errorf("failure boundary: failures=%d forwarded=%d", edge.failures, edge.forwarded)
	}
	edge.down = false
	edge.mu.Unlock()
	// Production retry backoff is unchanged (30 seconds). Wait for the fixture
	// to recover naturally, rather than forcing or shortening shared behavior.
	var item Item
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := rs.Inbox("sirsi-software-admin")
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.Title == marker {
				item = row
				break
			}
		}
		if item.ID != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if item.ID == "" {
		t.Fatal("durably queued fixture never recovered")
	}
	t.Cleanup(func() {
		row, err := rs.Get(item.ID)
		if err != nil {
			t.Errorf("fixture cleanup readback %s: %v", item.ID, err)
			return
		}
		if row.Status == "closed" {
			return
		}
		if err := rs.CloseItem(item.ID, "Coordination-only live retry fixture cleanup"); err != nil {
			t.Errorf("fixture item cleanup %s: %v", item.ID, err)
		}
	})
	if item.Instructions != req.Instructions {
		t.Fatal("recovered payload differs")
	}
	id, deduped, err := rs.SendGuarded(req)
	if err != nil || !deduped || id != item.ID {
		t.Fatalf("duplicate must return exact original ID %s: %s dedup=%t err=%v", item.ID, id, deduped, err)
	}
	if err := rs.CloseItem(id, "Live controlled pre-send failure, durable outbox recovery and exact-ID duplicate control PASS"); err != nil {
		t.Fatal(err)
	}
	closed, err := rs.Get(id)
	if err != nil || closed.Status != "closed" {
		t.Fatalf("terminal readback: %+v %v", closed, err)
	}
	edge.mu.Lock()
	failures, forwarded := edge.failures, edge.forwarded
	edge.mu.Unlock()
	t.Logf("LIVE_PROOF id=%s failures_before_forward=%d successful_send_attempts=%d dedup_same_id=true status=%s", id, failures, forwarded, closed.Status)
}
