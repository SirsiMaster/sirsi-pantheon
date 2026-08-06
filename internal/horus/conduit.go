package horus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NodeConduit is Horus's unified per-node conduit — the single flow through which
// router items and observability data pass on this machine. There is exactly ONE
// NodeConduit per physical/virtual node. Multiple processes on the same node share
// the same conduit identity (keyed on hostname + conduit dir).
//
// Hierarchy: Ra (fleet aggregator) ← Horus/NodeConduit (one per node) ← Anubis ← SNE
//
// Router items and observability are not separate concerns: both travel through
// the conduit so Ra sees a unified stream when it aggregates across nodes.
type NodeConduit struct {
	mu       sync.RWMutex
	identity ConduitIdentity
	dir      string
}

// ConduitIdentity is the stable, serialized identity written to disk.
// Other processes on the same node read this to discover the active conduit.
type ConduitIdentity struct {
	Hostname    string    `json:"hostname"`
	NodeID      string    `json:"node_id"` // hostname (extensible to UUID later)
	StartedAt   time.Time `json:"started_at"`
	SchemaVer   string    `json:"schema_version"`    // "1.0.0"
	TelemetryOn bool      `json:"telemetry_enabled"` // user opt-in to Horus→Ra reporting
}

// ConduitReport is the unified payload Horus sends upward to Ra.
// Router items + observability are one struct — Ra aggregates these across nodes.
type ConduitReport struct {
	Identity    ConduitIdentity    `json:"identity"`
	ReportedAt  time.Time          `json:"reported_at"`
	RouterItems int                `json:"router_items_pending"`  // unread items on this node
	WorkReport  *WorkstationReport `json:"workstation,omitempty"` // observability snapshot
}

const conduitSchemaVersion = "1.0.0"

// OpenConduit returns the NodeConduit for this machine, creating the identity
// file if it does not exist. Idempotent: multiple callers get the same conduit.
func OpenConduit() (*NodeConduit, error) {
	dir, err := conduitDir()
	if err != nil {
		return nil, fmt.Errorf("horus conduit: %w", err)
	}

	c := &NodeConduit{dir: dir}
	if err := c.loadOrCreate(); err != nil {
		return nil, fmt.Errorf("horus conduit: %w", err)
	}
	return c, nil
}

// Identity returns the stable identity of this node's conduit.
func (c *NodeConduit) Identity() ConduitIdentity {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.identity
}

// SetTelemetry updates the user's opt-in for Horus→Ra reporting and persists it.
// Anubis telemetry opt-in is the user's Horus reporting to Sirsi's Ra.
func (c *NodeConduit) SetTelemetry(enabled bool) error {
	c.mu.Lock()
	c.identity.TelemetryOn = enabled
	c.mu.Unlock()
	return c.persist()
}

// BuildReport assembles a ConduitReport from current node state.
// This is what Horus sends to Ra: router queue depth + workstation observability.
func (c *NodeConduit) BuildReport(routerPending int, ws *WorkstationReport) ConduitReport {
	c.mu.RLock()
	id := c.identity
	c.mu.RUnlock()

	return ConduitReport{
		Identity:    id,
		ReportedAt:  time.Now(),
		RouterItems: routerPending,
		WorkReport:  ws,
	}
}

func (c *NodeConduit) loadOrCreate() error {
	identityPath := filepath.Join(c.dir, "conduit.json")

	data, err := os.ReadFile(identityPath)
	if err == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		return json.Unmarshal(data, &c.identity)
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("read conduit identity: %w", err)
	}

	hostname, _ := os.Hostname()
	c.mu.Lock()
	c.identity = ConduitIdentity{
		Hostname:  hostname,
		NodeID:    hostname, // ponytail: hostname as ID; extend to UUID when multi-NIC nodes need disambiguation
		StartedAt: time.Now(),
		SchemaVer: conduitSchemaVersion,
	}
	c.mu.Unlock()

	return c.persist()
}

func (c *NodeConduit) persist() error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("create conduit dir: %w", err)
	}

	c.mu.RLock()
	data, err := json.MarshalIndent(c.identity, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal conduit identity: %w", err)
	}

	return os.WriteFile(filepath.Join(c.dir, "conduit.json"), data, 0o600)
}

func conduitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "sirsi", "horus"), nil
}
