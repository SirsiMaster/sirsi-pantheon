package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// ── SetWatchdogBridge / GetWatchdogBridge ────────────────────────────────

func TestSetGetWatchdogBridge(t *testing.T) {
	// Initially: save and restore whatever exists
	old := GetWatchdogBridge()
	defer SetWatchdogBridge(old)

	SetWatchdogBridge(nil)
	if GetWatchdogBridge() != nil {
		t.Error("Bridge should be nil after setting nil")
	}

	// Create a real bridge with a very short config
	ctx, cancel := context.WithCancel(context.Background())
	cfg := guard.DefaultBridgeConfig()
	cfg.WatchConfig.Interval = 100 * time.Millisecond
	bridge := guard.StartBridge(ctx, cfg)
	defer cancel()

	SetWatchdogBridge(bridge)

	got := GetWatchdogBridge()
	if got == nil {
		t.Fatal("Bridge should not be nil after setting")
	}
	if got != bridge {
		t.Error("GetWatchdogBridge should return the same bridge")
	}
}

// ── handleWatchdogResource ──────────────────────────────────────────────

func TestHandleWatchdogResource_NoBridge(t *testing.T) {
	old := GetWatchdogBridge()
	defer SetWatchdogBridge(old)
	SetWatchdogBridge(nil)

	rc, err := handleWatchdogResource()
	if err != nil {
		t.Fatalf("handleWatchdogResource: %v", err)
	}
	if rc == nil {
		t.Fatal("Expected non-nil ResourceContent")
	}
	if rc.URI != "anubis://watchdog-alerts" {
		t.Errorf("URI = %q, want anubis://watchdog-alerts", rc.URI)
	}
	if rc.MimeType != "application/json" {
		t.Errorf("MimeType = %q", rc.MimeType)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(rc.Text), &data); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
	if data["active"] != false {
		t.Error("Should report active=false when no bridge")
	}
}

func TestHandleWatchdogResource_WithBridge(t *testing.T) {
	old := GetWatchdogBridge()
	defer SetWatchdogBridge(old)

	ctx, cancel := context.WithCancel(context.Background())
	cfg := guard.DefaultBridgeConfig()
	cfg.WatchConfig.Interval = 100 * time.Millisecond
	bridge := guard.StartBridge(ctx, cfg)
	defer cancel()

	SetWatchdogBridge(bridge)

	rc, err := handleWatchdogResource()
	if err != nil {
		t.Fatalf("handleWatchdogResource: %v", err)
	}
	if rc == nil {
		t.Fatal("Expected non-nil ResourceContent")
	}
	if !strings.Contains(rc.Text, "active") {
		t.Logf("Watchdog response: %s", rc.Text)
	}
}

// ── handleBrainResource ─────────────────────────────────────────────────

func TestHandleBrainResource(t *testing.T) {
	rc, err := handleBrainResource()
	if err != nil {
		t.Fatalf("handleBrainResource: %v", err)
	}
	if rc == nil {
		t.Fatal("Expected non-nil ResourceContent")
	}
	if rc.URI != "anubis://brain-status" {
		t.Errorf("URI = %q", rc.URI)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(rc.Text), &data); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
}

// ── handleHealthCheck with bridge paths ──────────────────────────────────

func TestHandleHealthCheck_WithBridge(t *testing.T) {
	old := GetWatchdogBridge()
	defer SetWatchdogBridge(old)

	ctx, cancel := context.WithCancel(context.Background())
	cfg := guard.DefaultBridgeConfig()
	cfg.WatchConfig.Interval = 100 * time.Millisecond
	bridge := guard.StartBridge(ctx, cfg)
	defer cancel()

	SetWatchdogBridge(bridge)

	result, err := handleHealthCheck(nil)
	if err != nil {
		t.Fatalf("handleHealthCheck: %v", err)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "Watchdog: active") {
		t.Errorf("Should show active watchdog; got:\n%s", text)
	}
}

func TestHandleHealthCheck_NoBridge(t *testing.T) {
	old := GetWatchdogBridge()
	defer SetWatchdogBridge(old)
	SetWatchdogBridge(nil)

	result, err := handleHealthCheck(nil)
	if err != nil {
		t.Fatalf("handleHealthCheck: %v", err)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "Watchdog: dormant") {
		t.Errorf("Should show dormant watchdog; got:\n%s", text)
	}
}
