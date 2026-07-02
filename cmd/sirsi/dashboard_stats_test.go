package main

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// withFakeCapacity injects a deterministic NodeCapacity (Rule A16/A21) and
// restores the real sampler afterwards.
func withFakeCapacity(t *testing.T, nc guard.NodeCapacity) {
	t.Helper()
	orig := sampleNodeCapacityFn
	sampleNodeCapacityFn = func() guard.NodeCapacity { return nc }
	t.Cleanup(func() { sampleNodeCapacityFn = orig })
}

const gb = int64(1) << 30

// TestCollectDashRAM_RealFields is the data-honesty regression test: GET
// /api/stats used to return total_ram/used_ram/free_ram as zeros while
// ram_percent was real — live data rendering zeros. All four fields must now
// derive consistently from the guard NodeCapacity sample.
func TestCollectDashRAM_RealFields(t *testing.T) {
	withFakeCapacity(t, guard.NodeCapacity{TotalRAM: 48 * gb, FreeRAM: 16 * gb})

	stats := map[string]interface{}{}
	collectDashRAM(stats)

	if got := stats["total_ram"].(int64); got != 48*gb {
		t.Errorf("total_ram = %d, want %d", got, 48*gb)
	}
	if got := stats["used_ram"].(int64); got != 32*gb {
		t.Errorf("used_ram = %d, want %d", got, 32*gb)
	}
	if got := stats["free_ram"].(int64); got != 16*gb {
		t.Errorf("free_ram = %d, want %d", got, 16*gb)
	}
	pct := stats["ram_percent"].(float64)
	if math.Abs(pct-66.666) > 0.1 {
		t.Errorf("ram_percent = %.3f, want ~66.667 (used/total from the SAME sample)", pct)
	}
	if stats["ram_pressure"] != "medium" {
		t.Errorf("ram_pressure = %v, want medium at ~67%%", stats["ram_pressure"])
	}
}

// TestCollectDashRAM_NegativeUsedClamped guards the used = total - free
// derivation against a free sample larger than total (racy vm_stat reads).
func TestCollectDashRAM_NegativeUsedClamped(t *testing.T) {
	withFakeCapacity(t, guard.NodeCapacity{TotalRAM: 8 * gb, FreeRAM: 9 * gb})

	stats := map[string]interface{}{}
	collectDashRAM(stats)

	if got := stats["used_ram"].(int64); got != 0 {
		t.Errorf("used_ram = %d, want 0 (clamped)", got)
	}
	if stats["ram_pressure"] != "low" {
		t.Errorf("ram_pressure = %v, want low at 0%%", stats["ram_pressure"])
	}
}

// TestCollectDashboardStats_ContractRoundTrip proves the producer bytes decode
// into the dashboard's typed StatsResponse contract (E3) with the RAM fields
// populated — the exact boundary /api/stats serves.
func TestCollectDashboardStats_ContractRoundTrip(t *testing.T) {
	withFakeCapacity(t, guard.NodeCapacity{TotalRAM: 64 * gb, FreeRAM: 40 * gb})

	data, err := json.Marshal(collectDashboardStats())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var typed dashboard.StatsResponse
	if err := json.Unmarshal(data, &typed); err != nil {
		t.Fatalf("bytes do not decode into StatsResponse: %v", err)
	}
	if typed.TotalRAM != 64*gb || typed.UsedRAM != 24*gb || typed.FreeRAM != 40*gb {
		t.Errorf("contract RAM fields = %d/%d/%d, want 64/24/40 GB",
			typed.TotalRAM, typed.UsedRAM, typed.FreeRAM)
	}
}

// TestCollectDashRAM_UnknownHardwareStaysHonest: when hardware detection
// fails (TotalRAM == 0), the RAM byte fields must stay absent/zero — never
// fabricated — while the legacy vm_stat path may still fill ram_percent.
func TestCollectDashRAM_UnknownHardwareStaysHonest(t *testing.T) {
	withFakeCapacity(t, guard.NodeCapacity{})

	stats := map[string]interface{}{}
	collectDashRAM(stats)

	if v, ok := stats["total_ram"]; ok && v.(int64) != 0 {
		t.Errorf("total_ram = %v, want unset/0 when hardware is unknown", v)
	}
	if v, ok := stats["used_ram"]; ok && v.(int64) != 0 {
		t.Errorf("used_ram = %v, want unset/0 when hardware is unknown", v)
	}
}
