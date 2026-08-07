package guard

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var errUnreachable = errors.New("broker unreachable (test stub)")

// TestBrokerMLXActiveBytes_ParsesHealth pins the /health contract: the field
// read is mlx_active_bytes, from the port named by ~/.sirsi/gemma-server.port.
func TestBrokerMLXActiveBytes_ParsesHealth(t *testing.T) {
	const wantBytes = int64(31_400_000_000) // ~31.4 GB — the measured 2026-08-06 peak
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BrokerHealth{MLXActiveBytes: wantBytes, MLXPeakBytes: wantBytes + 1})
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte(port), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := BrokerMLXActiveBytes()
	if !ok {
		t.Fatal("BrokerMLXActiveBytes: ok = false, want true")
	}
	if got != wantBytes {
		t.Errorf("BrokerMLXActiveBytes = %d, want %d", got, wantBytes)
	}
}

// TestBrokerMLXActiveBytes_Unreachable proves the broker-down/unreachable case
// fails gracefully (ok=false), never inventing a number.
func TestBrokerMLXActiveBytes_Unreachable(t *testing.T) {
	// A listener that is opened then immediately closed guarantees "connection
	// refused" on the port without depending on any real service being absent.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte(strconv.Itoa(port)), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := BrokerMLXActiveBytes()
	if ok {
		t.Errorf("BrokerMLXActiveBytes on a closed port = (%d, true), want ok=false", got)
	}
}

// TestApplyBrokerTruth_ScopedToLoadBearingPID is the R6 regression guard
// (PRD SNE_HETEROGENEOUS_COMPUTE.md): applyBrokerTruth overrides footprint for
// the load-bearing broker PID ONLY, using the real Metal allocation — every
// other process's footprint passes through untouched (Rule A35 — scoped to
// the one PID the claim is actually about).
func TestApplyBrokerTruth_ScopedToLoadBearingPID(t *testing.T) {
	old := getFetchBrokerHealthFn()
	t.Cleanup(func() { setFetchBrokerHealthFn(old) })

	// Measured 2026-08-06: RSS/footprint read 4.2 GB while mlx_active_bytes was
	// 24.9-31.4 GB. Use the high end as the "true" number /health reports.
	const measuredFootprintGB = int64(4_200_000_000)
	const measuredMLXBytes = int64(31_400_000_000)
	setFetchBrokerHealthFn(func() (BrokerHealth, error) {
		return BrokerHealth{MLXActiveBytes: measuredMLXBytes, MLXPeakBytes: measuredMLXBytes + 1000}, nil
	})

	loadBearing := map[int]bool{4242: true}

	// The broker PID: footprint/peak are overridden with the /health numbers.
	fp, peak := applyBrokerTruth(loadBearing, 4242, measuredFootprintGB, measuredFootprintGB)
	if fp != measuredMLXBytes {
		t.Errorf("broker footprint = %d, want mlx_active_bytes %d (the 27GB gap must be closed)", fp, measuredMLXBytes)
	}
	if peak != measuredMLXBytes+1000 {
		t.Errorf("broker peak = %d, want mlx_peak_bytes %d", peak, measuredMLXBytes+1000)
	}

	// A different, non-load-bearing PID: passes through unchanged, even though
	// the same (mocked) /health endpoint is reachable — this is what proves the
	// override is scoped to the one PID, not a blanket replacement.
	otherFP, otherPeak := applyBrokerTruth(loadBearing, 9999, 500, 600)
	if otherFP != 500 || otherPeak != 600 {
		t.Errorf("non-broker PID footprint/peak = %d/%d, want unchanged 500/600", otherFP, otherPeak)
	}
}

// TestApplyBrokerTruth_FailsSafeWhenUnreachable proves that when /health
// cannot be reached, applyBrokerTruth falls back to the caller-supplied
// footprint/peak rather than zeroing them out — never a worse blind spot than
// before this change existed.
func TestApplyBrokerTruth_FailsSafeWhenUnreachable(t *testing.T) {
	old := getFetchBrokerHealthFn()
	t.Cleanup(func() { setFetchBrokerHealthFn(old) })
	setFetchBrokerHealthFn(func() (BrokerHealth, error) { return BrokerHealth{}, errUnreachable })

	loadBearing := map[int]bool{4242: true}
	fp, peak := applyBrokerTruth(loadBearing, 4242, 4_200_000_000, 5_000_000_000)
	if fp != 4_200_000_000 || peak != 5_000_000_000 {
		t.Errorf("footprint/peak = %d/%d, want unchanged fallback values when /health is unreachable", fp, peak)
	}
}
