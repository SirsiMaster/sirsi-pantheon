// Package servicedrain keeps the legacy root-relay startup hold separate from
// a modern producer's drain acknowledgement. No old-work completion is inferred.
package servicedrain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const MaintenanceMarkerName = ".sirsi-root-maintenance.json"
const MaintenancePrepareName = ".sirsi-root-maintenance.prepare.json"
const RelayDrainTimeout = 15 * time.Second

// Written before the private journal's first prepared phase. Its occupied name
// (even a torn body) is startup hold, never permission for ordinary recovery.
// Intent contains the non-secret complete controller status for reconstruction.
type MaintenancePreparation struct {
	Marker MaintenanceMarker `json:"marker"`
	Intent json.RawMessage   `json:"intent"`
}

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
	mu          sync.Mutex
	status      MaintenanceReady
	activeState string
	started     chan struct{}
	verify      func(MaintenanceReady) error
	observe     func() (int, error)
	drain       func(context.Context) error
	resume      func()
	drainVerify func(MaintenanceReady) error
}

func NewStartupMaintenance(pid int, started string, marker MaintenanceMarker, preserved int, verify func(MaintenanceReady) error, observe func() (int, error)) (*StartupMaintenance, error) {
	if marker.Validate() != nil || pid <= 1 || started == "" || preserved < 0 || verify == nil {
		return nil, errors.New("service maintenance: exact startup identity/custody required")
	}
	var generation [16]byte
	if _, err := rand.Read(generation[:]); err != nil {
		return nil, err
	}
	return &StartupMaintenance{status: MaintenanceReady{1, pid, started, hex.EncodeToString(generation[:]), 1, "maintenance_ready_not_drained", marker, preserved}, activeState: "active_with_old_outcomes_unknown", started: make(chan struct{}), verify: verify, observe: observe}, nil
}

// NewActiveMaintenance gives an ordinary root-relay startup the same existing
// PID/start/root-bound endpoint without publishing a durable transaction
// marker. Its marker-shaped identity is ephemeral CAS metadata, not authority.
func NewActiveMaintenance(pid int, started string, rootDevice, rootInode uint64) (*StartupMaintenance, error) {
	if pid <= 1 || started == "" || rootInode == 0 {
		return nil, errors.New("service maintenance: active producer identity/root required")
	}
	var id, generation [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	if _, err := rand.Read(generation[:]); err != nil {
		return nil, err
	}
	identity := sha256.Sum256([]byte(fmt.Sprintf("root-relay-active-control-v1\x00%d\x00%s\x00%d\x00%d", pid, started, rootDevice, rootInode)))
	marker := MaintenanceMarker{Schema: 1, ID: hex.EncodeToString(id[:]), PlanSHA256: hex.EncodeToString(identity[:]), RootDevice: rootDevice, RootInode: rootInode}
	g := &StartupMaintenance{
		status:      MaintenanceReady{1, pid, started, hex.EncodeToString(generation[:]), 2, "relay_active", marker, 0},
		activeState: "relay_active", started: make(chan struct{}),
	}
	close(g.started)
	return g, nil
}

// SetDrainHandlers installs the same-process pause, resume and identity
// revalidation callbacks before publishing the retained maintenance endpoint.
func (g *StartupMaintenance) SetDrainHandlers(drain func(context.Context) error, resume func(), verify func(MaintenanceReady) error) error {
	if drain == nil || resume == nil || verify == nil {
		return errors.New("service maintenance: complete relay drain handlers required")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.drain != nil || g.status.State == "relay_drain_pending" || g.status.State == "relay_drain_acknowledged" {
		return errors.New("service maintenance: relay drain handlers already installed or busy")
	}
	g.drain, g.resume, g.drainVerify = drain, resume, verify
	return nil
}

func (g *StartupMaintenance) Snapshot() (MaintenanceReady, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.observe != nil && g.status.State == "maintenance_ready_not_drained" {
		n, err := g.observe()
		if err != nil {
			return g.status, err
		}
		if n < 0 || n > MaxCustodyEnvelopes {
			return g.status, errors.New("service maintenance: custody observation invalid")
		}
		if n != g.status.Preserved {
			g.status.Preserved = n
			g.status.Version++
		}
	}
	return g.status, nil
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
	if err := g.verify(expected); err != nil {
		return g.status, err
	}
	g.status.State = g.activeState
	g.status.Version++
	close(g.started)
	return g.status, nil
}

func (g *StartupMaintenance) PrepareDrain(ctx context.Context, expected MaintenanceReady) (MaintenanceReady, error) {
	g.mu.Lock()
	if g.status != expected || expected.State != g.activeState || g.drain == nil {
		status := g.status
		g.mu.Unlock()
		return status, errors.New("service maintenance: active producer generation/identity mismatch")
	}
	if _, ok := ctx.Deadline(); !ok {
		status := g.status
		g.mu.Unlock()
		return status, errors.New("service maintenance: bounded producer drain required")
	}
	if err := g.drainVerify(expected); err != nil {
		status := g.status
		g.mu.Unlock()
		return status, err
	}
	g.status.State = "relay_drain_pending"
	g.status.Version++
	pending := g.status
	drain, resume, verify := g.drain, g.resume, g.drainVerify
	g.mu.Unlock()

	err := drain(ctx)
	if err == nil {
		err = verify(expected)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != pending {
		if err == nil {
			resume()
		}
		return g.status, errors.New("service maintenance: drain generation changed before acknowledgment")
	}
	if err != nil {
		resume()
		g.status.State = g.activeState
		g.status.Version++
		return g.status, err
	}
	g.status.State = "relay_drain_acknowledged"
	g.status.Version++
	return g.status, nil
}

func (g *StartupMaintenance) ResumeDrain(expected MaintenanceReady) (MaintenanceReady, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != expected || expected.State != "relay_drain_acknowledged" || g.resume == nil {
		return g.status, errors.New("service maintenance: exact prepared-drain generation required")
	}
	g.resume()
	g.status.State = g.activeState
	g.status.Version++
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
