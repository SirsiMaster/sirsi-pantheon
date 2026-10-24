// Package servicedrain keeps the legacy root-relay startup hold separate from
// a modern producer's drain acknowledgement. No old-work completion is inferred.
package servicedrain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
)

const MaintenanceMarkerName = ".sirsi-root-maintenance.json"

type MaintenanceMarker struct {
	Schema     int    `json:"schema"`
	ID         string `json:"id"`
	PlanSHA256 string `json:"plan_sha256"`
	RootDevice uint64 `json:"root_device"`
	RootInode  uint64 `json:"root_inode"`
}

func validHex(s string, n int) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == n && hex.EncodeToString(b) == s
}

func (m MaintenanceMarker) Validate() error {
	if m.Schema != 1 || !validHex(m.ID, 16) || !validHex(m.PlanSHA256, 32) || m.RootInode == 0 {
		return errors.New("service maintenance: invalid exact transaction/root binding")
	}
	return nil
}

type MaintenanceReady struct {
	Schema     int               `json:"schema"`
	PID        int               `json:"pid"`
	Started    string            `json:"started"`
	Generation string            `json:"generation"`
	Version    uint64            `json:"version"`
	State      string            `json:"state"`
	Marker     MaintenanceMarker `json:"marker"`
	Preserved  int               `json:"preserved_envelopes"`
}

// StartupMaintenance is born closed. It never uses an expiration to release,
// and never converts preserved envelopes into an empty/drained certificate.
type StartupMaintenance struct {
	mu      sync.Mutex
	status  MaintenanceReady
	started chan struct{}
	verify  func() error
}

func NewStartupMaintenance(pid int, started string, marker MaintenanceMarker, preserved int, verify func() error) (*StartupMaintenance, error) {
	if marker.Validate() != nil || pid <= 1 || started == "" || preserved < 0 || verify == nil {
		return nil, errors.New("service maintenance: exact startup identity/custody required")
	}
	var generation [16]byte
	if _, err := rand.Read(generation[:]); err != nil {
		return nil, err
	}
	return &StartupMaintenance{status: MaintenanceReady{1, pid, started, hex.EncodeToString(generation[:]), 1, "maintenance_ready_not_drained", marker, preserved}, started: make(chan struct{}), verify: verify}, nil
}

func (g *StartupMaintenance) Status() MaintenanceReady {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status
}

func (g *StartupMaintenance) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.started:
		return nil
	}
}

func (g *StartupMaintenance) Activate(expected MaintenanceReady) (MaintenanceReady, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != expected || expected.State != "maintenance_ready_not_drained" {
		return g.status, errors.New("service maintenance: stale/mixed activation refused")
	}
	if err := g.verify(); err != nil {
		return g.status, err
	}
	g.status.State = "active_with_old_outcomes_unknown"
	g.status.Version++
	close(g.started)
	return g.status, nil
}

// Strict canonical encoding rejects duplicate keys, case aliases, missing
// fields and trailing input without a permissive normalization round-trip.
func DecodeMaintenance[T any](raw []byte) (T, error) {
	var value T
	if len(raw) == 0 || len(raw) > 8192 || raw[len(raw)-1] != '\n' {
		return value, errors.New("service maintenance: bounded complete canonical object required")
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	canonical, err := json.Marshal(value)
	if err != nil || string(append(canonical, '\n')) != string(raw) {
		return value, errors.New("service maintenance: exact closed canonical fields required")
	}
	return value, nil
}
