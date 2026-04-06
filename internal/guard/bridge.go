package guard

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
)

// AntigravityBridge provides a thread-safe, bounded ring buffer for MCP resources
// to monitor system health and watchdog alerts.
type AntigravityBridge struct {
	mu          sync.RWMutex
	alerts      *AlertRing
	polls       int64
	alertsTotal int64
	backoffs    int64
	onAlert     func(AlertEntry)
}

// AlertRing is a fixed-size buffer for system alerts.
type AlertRing struct {
	mu      sync.Mutex
	entries []AlertEntry
	head    int
	size    int
}

// AlertEntry is a single resource or forensic alert in the bridge.
type AlertEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
	Severity    string    `json:"severity"`
	PID         int       `json:"pid"`
	ProcessName string    `json:"process_name"`
	CPUPercent  float64   `json:"cpu_percent"`
	RSSHuman    string    `json:"rss_human"`
	Duration    string    `json:"duration"`
	Message     string    `json:"message"`
	Metadata    any       `json:"metadata,omitempty"`
}

// NewAlertRing creates a ring buffer with the specified capacity.
func NewAlertRing(capacity int) *AlertRing {
	return &AlertRing{
		entries: make([]AlertEntry, capacity),
		size:    capacity,
	}
}

// Add appends an alert to the ring.
func (r *AlertRing) Add(alert AlertEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[r.head] = alert
	r.head = (r.head + 1) % r.size
}

// GetAll returns all alerts in the ring, ordered by timestamp.
func (r *AlertRing) GetAll() []AlertEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]AlertEntry, 0, r.size)
	for i := 0; i < r.size; i++ {
		idx := (r.head + r.size - 1 - i) % r.size // Reverse order (newest first)
		if !r.entries[idx].Timestamp.IsZero() {
			result = append(result, r.entries[idx])
		}
	}
	return result
}

// Stats returns buffered and lifetime counts for the ring.
func (r *AlertRing) Stats() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, e := range r.entries {
		if !e.Timestamp.IsZero() {
			count++
		}
	}
	// Note: We don't track lifetime alerts in the ring itself,
	// but the bridge does. For compatibility, we'll return count twice
	// or assume the caller handles the second value separately if needed.
	return count, count
}

// StartBridge initializes and starts the global Antigravity Bridge.
func StartBridge(ctx context.Context, cfg BridgeConfig) *AntigravityBridge {
	b := &AntigravityBridge{
		alerts:  NewAlertRing(cfg.BufferSize),
		onAlert: cfg.OnAlert,
	}

	// Start the watchdog and pipe alerts into the bridge
	w := StartWatch(ctx, cfg.WatchConfig)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case alert, ok := <-w.Alerts():
				if !ok {
					return
				}

				entry := AlertEntry{
					Timestamp:   alert.Timestamp,
					Source:      "Isis",
					Severity:    "warn",
					PID:         alert.Process.PID,
					ProcessName: alert.Process.Name,
					CPUPercent:  alert.CPUPercent,
					RSSHuman:    FormatBytes(alert.Process.RSS),
					Duration:    alert.Duration.Truncate(time.Second).String(),
					Message:     FormatAlert(alert),
				}

				if alert.CPUPercent > 95 {
					entry.Severity = "critical"
				}

				b.AddAlert(entry)
				if b.onAlert != nil {
					b.onAlert(entry)
				}

				// Sync metrics
				polls, alerts, backoffs := w.Stats()
				b.mu.Lock()
				b.polls = polls
				b.alertsTotal = alerts
				b.backoffs = backoffs
				b.mu.Unlock()
			}
		}
	}()

	return b
}

// AddAlert pushes an already-formed alert entry to the bridge.
func (b *AntigravityBridge) AddAlert(entry AlertEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.alerts.Add(entry)
	b.alertsTotal++
	logging.Debug("𓁢 AntigravityBridge: alert cached", "msg", entry.Message)
}

// StatusJSON returns a JSON string of the current bridge summary.
func (b *AntigravityBridge) StatusJSON() (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count, _ := b.alerts.Stats()
	summary := map[string]interface{}{
		"active":          true,
		"buffered_alerts": count,
		"lifetime_alerts": b.alertsTotal,
		"polls":           b.polls,
		"backoffs":        b.backoffs,
		"recent_alerts":   b.alerts.GetAll(),
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Ring returns the underlying alert ring.
func (b *AntigravityBridge) Ring() *AlertRing {
	return b.alerts
}

// Status returns current bridge metrics.
func (b *AntigravityBridge) Status() BridgeStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count, _ := b.alerts.Stats()
	return BridgeStatus{
		BufferedCount:    count,
		LifetimeAlerts:   b.alertsTotal,
		WatchdogPolls:    b.polls,
		WatchdogBackoffs: b.backoffs,
	}
}

// BridgeStatus returns a summary of the bridge state.
type BridgeStatus struct {
	BufferedCount    int
	LifetimeAlerts   int64
	WatchdogPolls    int64
	WatchdogBackoffs int64
}

// BridgeConfig defines bridge parameters.
type BridgeConfig struct {
	BufferSize  int
	WatchConfig WatchConfig
	OnAlert     func(AlertEntry)
}

// DefaultBridgeConfig returns defaults for the bridge.
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		BufferSize:  50,
		WatchConfig: DefaultWatchConfig(),
	}
}

// AlertRingSize is the constant size of the alert ring for tests.
const AlertRingSize = 50
