package main

import (
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestRouterBoardEnvelopePrefersLiveNodeStatus(t *testing.T) {
	ns := &router.NodeStatus{
		SchemaVersion: router.NodeStatusSchemaVersion,
		TotalPending:  0,
		LiveThreads: []router.ThreadSummary{{
			ThreadID: "thr-live",
			AgentID:  "claude-io",
			Armed:    true,
		}},
	}

	env := routerBoardEnvelope("/tmp/router-board.md", []byte("cached says two waiting"), true, "2026-07-28T20:00:00Z", ns, nil)

	if got := env["source"]; got != "live-node-status" {
		t.Fatalf("source = %v, want live-node-status", got)
	}
	if stale, ok := env["stale"].(bool); !ok || stale {
		t.Fatalf("stale = %v, want false", env["stale"])
	}
	if _, ok := env["content"]; ok {
		t.Fatalf("live envelope must not expose stale cached markdown: %+v", env)
	}
	if got := env["node_status"]; got != ns {
		t.Fatalf("node_status not preserved: %+v", env)
	}
}

func TestRouterBoardEnvelopeMarksCachedFallbackStale(t *testing.T) {
	env := routerBoardEnvelope("/tmp/router-board.md", []byte("cached board"), true, "2026-07-28T20:00:00Z", nil, errors.New("boom"))

	if got := env["source"]; got != "cached-markdown" {
		t.Fatalf("source = %v, want cached-markdown", got)
	}
	if stale, ok := env["stale"].(bool); !ok || !stale {
		t.Fatalf("stale = %v, want true", env["stale"])
	}
	if got := env["content"]; got != "cached board" {
		t.Fatalf("content = %v, want cached board", got)
	}
	if got := env["live_error"]; got != "boom" {
		t.Fatalf("live_error = %v, want boom", got)
	}
}
