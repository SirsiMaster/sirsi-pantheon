// Package guard — watchdog.go
//
// Sekhmet Watchdog: Lightweight, goroutine-based CPU/memory pressure monitor.
//
// Design principles:
//   - NEVER block the caller — Watch() launches a background goroutine and returns immediately
//   - NEVER fork processes in a tight loop — sample via native syscalls where possible
//   - Self-throttle — if the watchdog itself is consuming too much, back off
//   - Channel-based alerts — non-blocking sends, bounded buffer, no callback storms
//   - Core-aware — respects runtime.NumCPU() and pins monitor to a single OS thread
//
// Architecture:
//
//	┌────────────┐      ┌───────────┐      ┌──────────┐
//	│  Sampler   │─────▶│  Analyzer │─────▶│  Alerts  │──▶ consumer
//	│ (1 thread) │      │(goroutine)│      │(chan, 16) │
//	└────────────┘      └───────────┘      └──────────┘
//	     ▲                    │
//	     │   backoff if       │
//	     └── self CPU > 5% ───┘
package guard

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Injectable sampler for testability.
var sampleTopCPUFn = defaultSampleTopCPU

// WatchConfig configures the Sekhmet watchdog.
type WatchConfig struct {
	Interval     time.Duration // Polling interval (default: 5s)
	CPUThreshold float64       // Alert threshold per-process (default: 80.0%)
	SustainCount int           // Consecutive checks before alert (default: 3)
	MaxAlerts    int           // Stop after N alerts (0 = unlimited)
	SampleSize   int           // Top-N processes to sample (default: 15)
	SelfBudget   float64       // Max CPU% the watchdog itself should use (default: 5.0)
}

// DefaultWatchConfig returns sensible defaults.
func DefaultWatchConfig() WatchConfig {
	return WatchConfig{
		Interval:     5 * time.Second,
		CPUThreshold: 80.0,
		SustainCount: 3,
		MaxAlerts:    0,
		SampleSize:   15,
		SelfBudget:   5.0,
	}
}

// WatchAlert is emitted when a process sustains CPU > threshold.
type WatchAlert struct {
	Process    ProcessInfo
	CPUPercent float64
	Duration   time.Duration
	Timestamp  time.Time
}

// Watchdog is a running Sekhmet monitor instance.
type Watchdog struct {
	cfg     WatchConfig
	ctx     context.Context
	cancel  context.CancelFunc
	alerts  chan WatchAlert
	stopped chan struct{}
	running atomic.Bool

	// Metrics
	mu          sync.RWMutex
	totalPolls  int64
	totalAlerts int64
	backoffs    int64
	lastPoll    time.Time
}

// StartWatch creates and starts a Sekhmet watchdog on a background goroutine.
// Returns a *Watchdog handle. Consume alerts via watchdog.Alerts().
// The watchdog runs until ctx is cancelled or watchdog.Stop() is called.
func StartWatch(ctx context.Context, cfg WatchConfig) *Watchdog {
	applyDefaults(&cfg)

	wCtx, cancel := context.WithCancel(ctx)
	w := &Watchdog{
		cfg:     cfg,
		ctx:     wCtx,
		cancel:  cancel,
		alerts:  make(chan WatchAlert, 16), // Bounded buffer — never blocks producer
		stopped: make(chan struct{}),
	}
	w.running.Store(true)

	// Launch monitor on a dedicated goroutine
	go w.run()

	return w
}

// Alerts returns the read-only alert channel. Consume this in your main loop.
func (w *Watchdog) Alerts() <-chan WatchAlert {
	return w.alerts
}

// Stop gracefully shuts down the watchdog.
func (w *Watchdog) Stop() {
	w.cancel()
	<-w.stopped // Wait for clean exit
}

// IsRunning returns true if the watchdog is still active.
func (w *Watchdog) IsRunning() bool {
	return w.running.Load()
}

// Stats returns watchdog metrics.
func (w *Watchdog) Stats() (polls, alerts, backoffs int64) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.totalPolls, w.totalAlerts, w.backoffs
}

// run is the main monitor loop — runs on its own goroutine.
func (w *Watchdog) run() {
	defer close(w.stopped)
	defer close(w.alerts)
	defer w.running.Store(false)

	// Pin to a single OS thread to avoid scheduler contention.
	// This goroutine does I/O (ps fork) — we don't want it competing
	// with the Go scheduler's P pool on a resource-constrained machine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hotStreak := make(map[int]int)
	firstSeen := make(map[int]time.Time)
	currentInterval := w.cfg.Interval

	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			// Check self-governance via AntiGravity (Rule A1)
			ag := &AntiGravity{MaxMemoryMB: 1536} // 1.5GB soft limit
			_ = ag.CheckSelf()

			start := time.Now()
			procs, err := sampleTopCPUFn(w.cfg.SampleSize)
			if err != nil {
				continue // Transient — retry next tick
			}

			elapsed := time.Since(start)
			w.mu.Lock()
			w.totalPolls++
			w.lastPoll = time.Now()
			w.mu.Unlock()

			// Self-throttle: if sampling itself took > SelfBudget% of interval,
			// double the interval to back off
			selfCost := float64(elapsed) / float64(w.cfg.Interval) * 100
			if selfCost > w.cfg.SelfBudget {
				currentInterval = min64(currentInterval*2, 30*time.Second)
				ticker.Reset(currentInterval)
				w.mu.Lock()
				w.backoffs++
				w.mu.Unlock()
				continue
			} else if currentInterval > w.cfg.Interval {
				// Recover from backoff
				currentInterval = w.cfg.Interval
				ticker.Reset(currentInterval)
			}

			// Analyze — track sustained CPU spikes
			currentHot := make(map[int]bool)
			for _, p := range procs {
				if p.CPUPercent >= w.cfg.CPUThreshold {
					currentHot[p.PID] = true
					hotStreak[p.PID]++

					if _, exists := firstSeen[p.PID]; !exists {
						firstSeen[p.PID] = time.Now()
					}

					if hotStreak[p.PID] >= w.cfg.SustainCount {
						alert := WatchAlert{
							Process:    p,
							CPUPercent: p.CPUPercent,
							Duration:   time.Since(firstSeen[p.PID]),
							Timestamp:  time.Now(),
						}

						// Non-blocking send — drop alert if consumer is slow
						select {
						case w.alerts <- alert:
							w.mu.Lock()
							w.totalAlerts++
							w.mu.Unlock()
						default:
							// Consumer too slow — drop this alert silently
						}

						// Reset streak to avoid spamming
						hotStreak[p.PID] = 0

						if w.cfg.MaxAlerts > 0 {
							w.mu.RLock()
							total := w.totalAlerts
							w.mu.RUnlock()
							if total >= int64(w.cfg.MaxAlerts) {
								return
							}
						}
					}
				}
			}

			// Clean up cooled-down processes
			for pid := range hotStreak {
				if !currentHot[pid] {
					delete(hotStreak, pid)
					delete(firstSeen, pid)
				}
			}
		}
	}
}

// sampleTopCPU returns the top-N processes by CPU usage.
// Uses a single fork to `ps` sorted by CPU descending — one syscall per poll.
func sampleTopCPU(topN int) ([]ProcessInfo, error) {
	return sampleTopCPUFn(topN)
}

func defaultSampleTopCPU(topN int) ([]ProcessInfo, error) {
	if topN <= 0 {
		topN = 15
	}

	out, err := exec.Command("ps", "-arcxo", "pid,rss,%cpu,comm").Output()
	if err != nil {
		return nil, err
	}

	var procs []ProcessInfo
	lines := strings.Split(string(out), "\n")

	for i, line := range lines {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		rss, _ := strconv.ParseInt(fields[1], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		name := strings.Join(fields[3:], " ")

		procs = append(procs, ProcessInfo{
			PID:        pid,
			Name:       name,
			RSS:        rss * 1024,
			CPUPercent: cpu,
		})

		if len(procs) >= topN {
			break
		}
	}

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].CPUPercent > procs[j].CPUPercent
	})

	return procs, nil
}

// FormatAlert formats a WatchAlert for terminal display.
func FormatAlert(a WatchAlert) string {
	return fmt.Sprintf("⚠️  𓁵 SEKHMET ALERT: %s (PID %d) at %.1f%% CPU for %s — using %s RAM",
		a.Process.Name, a.Process.PID, a.CPUPercent,
		a.Duration.Truncate(time.Second), FormatBytes(a.Process.RSS))
}

func applyDefaults(cfg *WatchConfig) {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.CPUThreshold == 0 {
		cfg.CPUThreshold = 80.0
	}
	if cfg.SustainCount == 0 {
		cfg.SustainCount = 3
	}
	if cfg.SampleSize == 0 {
		cfg.SampleSize = 15
	}
	if cfg.SelfBudget == 0 {
		cfg.SelfBudget = 5.0
	}
}

func min64(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// ── Legacy compatibility ──────────────────────────────────────────────
// Watch provides the old blocking API for backward compatibility.
// New code should use StartWatch() instead.
func Watch(ctx context.Context, cfg WatchConfig, onAlert AlertFunc) error {
	w := StartWatch(ctx, cfg)
	defer w.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case alert, ok := <-w.Alerts():
			if !ok {
				return nil // Channel closed — watchdog stopped
			}
			if onAlert != nil {
				onAlert(alert)
			}
		}
	}
}

// AlertFunc is the callback for the legacy Watch() API.
type AlertFunc func(alert WatchAlert)
