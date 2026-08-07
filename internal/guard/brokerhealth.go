package guard

// brokerhealth.go — reads the local-model broker's REAL memory allocation from
// its own /health endpoint (PRD R6, sirsi-pantheon-fabric
// docs/prd/SNE_HETEROGENEOUS_COMPUTE.md). Measured 2026-08-06: broker RSS
// 4.2 GB vs mlx_active_bytes 24.9-31.4 GB — a 27 GB blind spot that RSS-based
// (and, to a lesser degree, phys_footprint-based) checks cannot see, because
// Metal/GPU unified-memory allocation is not fully reflected in either. The
// broker's own /health endpoint already returns the true number
// (sirsi-inference's server.go handleHealth); every guard that judges the
// broker's OWN memory footprint reads it from here, not from RSS. Scoped to
// the one load-bearing broker PID (Rule A35) — every other process keeps
// being sized by phys_footprint/RSS exactly as before.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// brokerHealthDefaultPort mirrors the SNE contract port used elsewhere in this
// repo (cmd/sirsi/gemma_serve.go, internal/dashboard/ask.go) — duplicated
// rather than imported to avoid a cross-package cycle (both of those already
// import guard).
const brokerHealthDefaultPort = 8477

// brokerHealthTimeout bounds the /health probe. Loopback-only; a healthy
// broker answers in milliseconds, so this stays short enough that a dead
// broker never stalls a guard pass.
var brokerHealthTimeout = 2 * time.Second

// brokerHealthPort reads the port the broker is actually serving on, falling
// back to the default when the contract file is absent/unreadable.
func brokerHealthPort() int {
	home, err := os.UserHomeDir()
	if err != nil {
		return brokerHealthDefaultPort
	}
	b, err := os.ReadFile(filepath.Join(home, ".sirsi", "gemma-server.port"))
	if err != nil {
		return brokerHealthDefaultPort
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || p <= 0 || p > 65535 {
		return brokerHealthDefaultPort
	}
	return p
}

// BrokerHealth is the subset of the broker's /health JSON that memory guards
// need. Field names match sirsi-inference's server.go handleHealth verbatim.
type BrokerHealth struct {
	MLXActiveBytes int64 `json:"mlx_active_bytes"`
	MLXPeakBytes   int64 `json:"mlx_peak_bytes"`
}

// fetchBrokerHealthFn is the injectable HTTP seam (Rule A16/A21) so guards
// are testable without a live broker.
var (
	brokerHealthMu      sync.RWMutex
	fetchBrokerHealthFn = defaultFetchBrokerHealth
)

func getFetchBrokerHealthFn() func() (BrokerHealth, error) {
	brokerHealthMu.RLock()
	defer brokerHealthMu.RUnlock()
	return fetchBrokerHealthFn
}

func setFetchBrokerHealthFn(fn func() (BrokerHealth, error)) {
	brokerHealthMu.Lock()
	defer brokerHealthMu.Unlock()
	fetchBrokerHealthFn = fn
}

func defaultFetchBrokerHealth() (BrokerHealth, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/health", brokerHealthPort())
	cl := &http.Client{Timeout: brokerHealthTimeout}
	resp, err := cl.Get(url)
	if err != nil {
		return BrokerHealth{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return BrokerHealth{}, fmt.Errorf("broker /health returned %s", resp.Status)
	}
	var h BrokerHealth
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return BrokerHealth{}, err
	}
	return h, nil
}

// BrokerMLXActiveBytes returns the broker's true current Metal allocation and
// whether the read succeeded. Callers MUST fail gracefully on !ok (broker
// down/unreachable) — never invent a number.
func BrokerMLXActiveBytes() (int64, bool) {
	h, err := getFetchBrokerHealthFn()()
	if err != nil {
		return 0, false
	}
	return h.MLXActiveBytes, true
}

// applyBrokerTruth overrides footprint/peak for a load-bearing broker PID with
// the real Metal allocation read from /health, when reachable. loadBearing is
// passed in (rather than recomputed) so a per-process census loop reads the
// pidfiles once, not once per process. Every non-broker PID passes through
// unchanged.
func applyBrokerTruth(loadBearing map[int]bool, pid int, footprint, peak int64) (int64, int64) {
	if !loadBearing[pid] {
		return footprint, peak
	}
	h, err := getFetchBrokerHealthFn()()
	if err != nil {
		// Broker unreachable — fall back to footprint/RSS. Fails safe: never a
		// worse blind spot than before this change existed.
		return footprint, peak
	}
	newFootprint := footprint
	if h.MLXActiveBytes > 0 {
		newFootprint = h.MLXActiveBytes
	}
	newPeak := peak
	if h.MLXPeakBytes > newPeak {
		newPeak = h.MLXPeakBytes
	}
	return newFootprint, newPeak
}
