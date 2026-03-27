package guard

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// ── AlertRing Tests ─────────────────────────────────────────────────────

func TestNewAlertRing(t *testing.T) {
	r := NewAlertRing(AlertRingSize)
	if r == nil {
		t.Fatal("NewAlertRing returned nil")
	}
	current, lifetime := r.Stats()
	if current != 0 || lifetime != 0 {
		t.Errorf("Fresh ring should have 0/0, got %d/%d", current, lifetime)
	}
}

func TestAlertRing_AddAndGetAll(t *testing.T) {
	r := NewAlertRing(AlertRingSize)

	// Add 3 alerts
	for i := 0; i < 3; i++ {
		r.Add(AlertEntry{
			ProcessName: "test-process",
			PID:         1000 + i,
			CPUPercent:  float64(80 + i*10),
			Severity:    "warning",
			Timestamp:   time.Now(),
		})
	}

	current, _ := r.Stats()
	if current != 3 {
		t.Errorf("Expected 3 current, got %d", current)
	}

	// GetAll — should return newest first
	all := r.GetAll()
	if len(all) != 3 {
		t.Fatalf("Expected 3 alerts, got %d", len(all))
	}
	if all[0].PID != 1002 {
		t.Errorf("Most recent should be PID 1002, got %d", all[0].PID)
	}
	if all[1].PID != 1001 {
		t.Errorf("Second most recent should be PID 1001, got %d", all[1].PID)
	}
}

func TestAlertRing_GetAllEdgeCases(t *testing.T) {
	r := NewAlertRing(AlertRingSize)

	// Empty ring
	if got := r.GetAll(); len(got) != 0 {
		t.Errorf("Empty ring GetAll should return empty, got %d", len(got))
	}

	// Single entry
	r.Add(AlertEntry{PID: 1, Timestamp: time.Now()})
	got := r.GetAll()
	if len(got) != 1 {
		t.Errorf("Expected 1 alert, got %d", len(got))
	}
}

func TestAlertRing_Overflow(t *testing.T) {
	r := NewAlertRing(AlertRingSize)

	// Add more than AlertRingSize
	for i := 0; i < AlertRingSize+10; i++ {
		r.Add(AlertEntry{PID: i, ProcessName: "overflow-test", Timestamp: time.Now()})
	}

	// The ring should have at most AlertRingSize entries
	all := r.GetAll()
	if len(all) > AlertRingSize {
		t.Errorf("Expected at most %d after overflow, got %d", AlertRingSize, len(all))
	}

	// Most recent should be the last pushed
	if all[0].PID != AlertRingSize+9 {
		t.Errorf("Most recent should be PID %d, got %d", AlertRingSize+9, all[0].PID)
	}
}

func TestAlertRing_Concurrent(t *testing.T) {
	r := NewAlertRing(AlertRingSize)
	var wg sync.WaitGroup

	// 10 goroutines each adding 100 alerts
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				r.Add(AlertEntry{
					PID:         gid*1000 + i,
					ProcessName: "concurrent",
					Timestamp:   time.Now(),
				})
			}
		}(g)
	}

	// Concurrent readers
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				r.GetAll()
				r.Stats()
			}
		}()
	}

	wg.Wait()

	// Ring should still be operational (no panic, no race)
	all := r.GetAll()
	if len(all) == 0 {
		t.Error("Expected some alerts after concurrent pushes")
	}
}

// ── DefaultBridgeConfig Tests ───────────────────────────────────────────

func TestDefaultBridgeConfig(t *testing.T) {
	cfg := DefaultBridgeConfig()
	if cfg.BufferSize != 50 {
		t.Errorf("Expected buffer size 50, got %d", cfg.BufferSize)
	}
	if cfg.WatchConfig.CPUThreshold != 80.0 {
		t.Errorf("Expected 80.0 CPU threshold, got %.1f", cfg.WatchConfig.CPUThreshold)
	}
	if cfg.WatchConfig.SustainCount != 3 {
		t.Errorf("Expected sustain count 3, got %d", cfg.WatchConfig.SustainCount)
	}
}

// ── Bridge Lifecycle Tests ──────────────────────────────────────────────

func TestStartBridge_LifecycleWithAlerts(t *testing.T) {
	// Mock the sampler to produce high-CPU alerts
	old := sampleTopCPUFn
	sampleTopCPUFn = func(topN int) ([]ProcessInfo, error) {
		return []ProcessInfo{
			{PID: 42, Name: "Plugin Host", CPUPercent: 103.9, RSS: 512 * 1024 * 1024},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	var received []AlertEntry
	var mu sync.Mutex

	bridge := StartBridge(ctx, BridgeConfig{
		BufferSize: 50,
		WatchConfig: WatchConfig{
			Interval:     50 * time.Millisecond,
			CPUThreshold: 80.0,
			SustainCount: 2,
			SampleSize:   5,
			SelfBudget:   50.0,
		},
		OnAlert: func(entry AlertEntry) {
			mu.Lock()
			received = append(received, entry)
			mu.Unlock()
		},
	})

	// Wait for some alerts to flow through
	time.Sleep(400 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond) // drain goroutines before restoring
	sampleTopCPUFn = old

	// Verify ring buffer has alerts
	current, _ := bridge.Ring().Stats()
	if current == 0 {
		t.Error("Expected alerts in ring buffer after sustained CPU spike")
	}

	// Verify callback was called
	mu.Lock()
	rcvCount := len(received)
	mu.Unlock()
	if rcvCount == 0 {
		t.Error("OnAlert callback was never called")
	}
}

func TestStartBridge_CriticalSeverity(t *testing.T) {
	old := sampleTopCPUFn
	sampleTopCPUFn = func(topN int) ([]ProcessInfo, error) {
		return []ProcessInfo{
			{PID: 99, Name: "runaway", CPUPercent: 200.0, RSS: 1024 * 1024},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	var gotCritical bool
	var mu sync.Mutex

	_ = StartBridge(ctx, BridgeConfig{
		BufferSize: 50,
		WatchConfig: WatchConfig{
			Interval:     50 * time.Millisecond,
			CPUThreshold: 80.0,
			SustainCount: 2,
			SampleSize:   5,
			SelfBudget:   50.0,
		},
		OnAlert: func(entry AlertEntry) {
			mu.Lock()
			if entry.Severity == "critical" {
				gotCritical = true
			}
			mu.Unlock()
		},
	})

	time.Sleep(400 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond) // drain goroutines before restoring
	sampleTopCPUFn = old

	mu.Lock()
	if !gotCritical {
		t.Error("Expected 'critical' severity for 200% CPU")
	}
	mu.Unlock()
}

func TestStartBridge_DefaultBufferSize(t *testing.T) {
	old := sampleTopCPUFn
	sampleTopCPUFn = func(topN int) ([]ProcessInfo, error) {
		return nil, nil // no processes
	}

	ctx, cancel := context.WithCancel(context.Background())

	bridge := StartBridge(ctx, BridgeConfig{
		BufferSize: 50,
		WatchConfig: WatchConfig{
			Interval:     50 * time.Millisecond,
			CPUThreshold: 80.0,
			SustainCount: 3,
			SampleSize:   5,
			SelfBudget:   50.0,
		},
	})

	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond) // drain goroutines before restoring
	sampleTopCPUFn = old

	// Should still work (no panic, clean shutdown)
	current, _ := bridge.Ring().Stats()
	t.Logf("Bridge with defaults: %d alerts", current)
}

// ── Status & JSON Tests ─────────────────────────────────────────────────

func TestBridge_StatusJSON(t *testing.T) {
	old := sampleTopCPUFn
	sampleTopCPUFn = func(topN int) ([]ProcessInfo, error) {
		return []ProcessInfo{
			{PID: 42, Name: "vscode", CPUPercent: 95.0, RSS: 256 * 1024 * 1024},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	bridge := StartBridge(ctx, BridgeConfig{
		BufferSize: 50,
		WatchConfig: WatchConfig{
			Interval:     50 * time.Millisecond,
			CPUThreshold: 80.0,
			SustainCount: 2,
			SampleSize:   5,
			SelfBudget:   50.0,
		},
	})

	time.Sleep(300 * time.Millisecond)

	// Get Status struct
	status := bridge.Status()

	// Get JSON
	jsonStr, err := bridge.StatusJSON()
	if err != nil {
		t.Fatalf("StatusJSON error: %v", err)
	}
	if jsonStr == "" {
		t.Error("StatusJSON returned empty string")
	}

	// Verify valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("StatusJSON produced invalid JSON: %v", err)
	}

	cancel()
	time.Sleep(100 * time.Millisecond) // drain goroutines before restoring
	sampleTopCPUFn = old

	t.Logf("StatusJSON: buffered=%d, lifetime=%d, polls=%d",
		status.BufferedCount, status.LifetimeAlerts, status.WatchdogPolls)
}

func TestBridge_WatchdogAccessor(t *testing.T) {
	old := sampleTopCPUFn
	sampleTopCPUFn = func(topN int) ([]ProcessInfo, error) {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	bridge := StartBridge(ctx, BridgeConfig{
		BufferSize: 50,
		WatchConfig: WatchConfig{
			Interval:     100 * time.Millisecond,
			CPUThreshold: 80.0,
			SustainCount: 3,
			SampleSize:   5,
			SelfBudget:   50.0,
		},
	})

	if bridge.Ring() == nil {
		t.Error("Ring accessor should not be nil")
	}

	cancel()
	time.Sleep(100 * time.Millisecond) // drain goroutines before restoring
	sampleTopCPUFn = old
}
