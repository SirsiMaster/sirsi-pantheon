package mcp

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brain"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// ── Antigravity IPC Bridge (global accessor) ────────────────────────────

var (
	bridgeMu     sync.RWMutex
	activeBridge *guard.AntigravityBridge
)

// SetWatchdogBridge stores a reference to the active Antigravity bridge
// so MCP resources and tools can query watchdog alerts.
func SetWatchdogBridge(b *guard.AntigravityBridge) {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	activeBridge = b
}

// GetWatchdogBridge returns the active bridge (may be nil).
func GetWatchdogBridge() *guard.AntigravityBridge {
	bridgeMu.RLock()
	defer bridgeMu.RUnlock()
	return activeBridge
}

// registerResources adds all Anubis resources to the MCP server.
func registerResources(s *Server) {
	s.RegisterResource(Resource{
		URI:         "anubis://health-status",
		Name:        "System Health Status",
		Description: "Current infrastructure hygiene status including waste total, ghost count, and health grade.",
		MimeType:    "application/json",
	}, handleHealthResource)

	s.RegisterResource(Resource{
		URI:         "anubis://capabilities",
		Name:        "Anubis Capabilities",
		Description: "List of all Anubis modules, scan rules, and available commands.",
		MimeType:    "application/json",
	}, handleCapabilitiesResource)

	s.RegisterResource(Resource{
		URI:         "anubis://brain-status",
		Name:        "Neural Brain Status",
		Description: "Status of the neural classification brain — installed model, version, and capabilities.",
		MimeType:    "application/json",
	}, handleBrainResource)

	s.RegisterResource(Resource{
		URI:         "anubis://watchdog-alerts",
		Name:        "Isis Watchdog Alerts",
		Description: "Live CPU/memory pressure alerts from the Isis watchdog. Shows recent sustained CPU spikes and IDE starvation events.",
		MimeType:    "application/json",
	}, handleWatchdogResource)
}

// handleHealthResource provides system health as a structured JSON resource.
func handleHealthResource() (*ResourceContent, error) {
	health := map[string]interface{}{
		"platform":        fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		"cpus":            runtime.NumCPU(),
		"brain_installed": brain.IsInstalled(),
		"version":         ServerVersion,
	}

	data, err := json.MarshalIndent(health, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal health: %w", err)
	}

	return &ResourceContent{
		URI:      "anubis://health-status",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}

// handleCapabilitiesResource lists all Anubis capabilities.
func handleCapabilitiesResource() (*ResourceContent, error) {
	capabilities := map[string]interface{}{
		"modules": []map[string]string{
			{"name": "jackal", "codename": "The Hunter", "description": "Scan engine with 64+ rules"},
			{"name": "ka", "codename": "The Spirit", "description": "Ghost app detection"},
			{"name": "guard", "codename": "The Guardian", "description": "RAM audit and process management"},
			{"name": "sight", "codename": "The Sight", "description": "Launch Services and Spotlight repair"},
			{"name": "hapi", "codename": "The Flow", "description": "GPU detection, dedup, APFS snapshots"},
			{"name": "scarab", "codename": "The Transformer", "description": "Network discovery and container audit"},
			{"name": "brain", "codename": "Neural", "description": "On-demand neural classification"},
			{"name": "mcp", "codename": "Context Sanitizer", "description": "MCP server for AI IDE integration"},
		},
		"commands": []string{
			"weigh", "judge", "ka", "guard", "sight", "profile",
			"seba", "hapi", "scarab", "install-brain", "uninstall-brain",
			"mcp", "book-of-the-dead", "initiate",
		},
		"scan_categories": []string{
			"general", "dev", "ai", "vms", "ides", "cloud", "storage",
		},
		"rule_count":   64,
		"global_flags": []string{"--json", "--quiet", "--stealth"},
	}

	data, err := json.MarshalIndent(capabilities, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal capabilities: %w", err)
	}

	return &ResourceContent{
		URI:      "anubis://capabilities",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}

// handleBrainResource provides neural brain status.
func handleBrainResource() (*ResourceContent, error) {
	status, err := brain.GetStatus()
	if err != nil {
		// Return a minimal status even on error
		fallback := map[string]interface{}{
			"installed": false,
			"error":     err.Error(),
		}
		data, _ := json.MarshalIndent(fallback, "", "  ")
		return &ResourceContent{
			URI:      "anubis://brain-status",
			MimeType: "application/json",
			Text:     string(data),
		}, nil
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal brain status: %w", err)
	}

	return &ResourceContent{
		URI:      "anubis://brain-status",
		MimeType: "application/json",
		Text:     string(data),
	}, nil
}

// handleWatchdogResource provides live watchdog alert data from the Antigravity bridge.
func handleWatchdogResource() (*ResourceContent, error) {
	bridge := GetWatchdogBridge()
	if bridge == nil {
		// No bridge active — return dormant status
		status := map[string]interface{}{
			"active":        false,
			"message":       "Isis watchdog not running. Start with 'pantheon guard --watch'.",
			"recent_alerts": []interface{}{},
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		return &ResourceContent{
			URI:      "anubis://watchdog-alerts",
			MimeType: "application/json",
			Text:     string(data),
		}, nil
	}

	statusJSON, err := bridge.StatusJSON()
	if err != nil {
		return nil, fmt.Errorf("bridge status: %w", err)
	}

	return &ResourceContent{
		URI:      "anubis://watchdog-alerts",
		MimeType: "application/json",
		Text:     statusJSON,
	}, nil
}
