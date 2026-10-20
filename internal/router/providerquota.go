package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Only complete provider diagnostic lines from a failed consumer's output are
// recognized. Quoted prose, item bodies mentioning quota, generic 429s and
// ambiguous reset hints must not become durable provider holds.
func providerQuotaFailure(tail string) bool {
	// Require the whole diagnostic, not a matching line embedded in item/model prose.
	line := strings.ToLower(strings.TrimSpace(tail))
	if strings.ContainsAny(line, "\n\r") {
		return false
	}
	// Observed suffix is informational, never a reset authority.
	if strings.HasPrefix(line, "you've hit your monthly spend limit · raise it at claude.ai/settings/usage?") {
		return true
	}
	switch line {
	case "you've hit your monthly spend limit", "you've hit your monthly spend limit.",
		"error: you've hit your monthly spend limit", "error: you've hit your monthly spend limit.",
		"error: monthly spend limit reached", "monthly spend limit reached":
		return true
	}
	return false
}

func knownExitedFailure(tail string, exitErr error) string {
	k := KnownFailure(tail)
	if k == OutcomeProviderQuota && exitErr == nil {
		return ""
	}
	return k
}

type providerQuotaRecovery struct {
	Generation uint64    `json:"generation"`
	At         time.Time `json:"at"`
	Evidence   string    `json:"evidence"`
}

type providerQuotaRecord struct {
	Generation          uint64                  `json:"generation"`
	AvailableGeneration uint64                  `json:"available_generation,omitempty"`
	Recoveries          []providerQuotaRecovery `json:"recovery_history,omitempty"`
	History             []time.Time             `json:"failure_history,omitempty"`
	BlockedAt           time.Time               `json:"blocked_at"`
	AvailableAt         time.Time               `json:"available_at,omitempty"`
	Evidence            string                  `json:"availability_evidence,omitempty"`
}

func providerQuotaPath(agent string) (string, error) {
	if agent == "" || agent == "." || agent == ".." || strings.ContainsAny(agent, "/\\") {
		return "", fmt.Errorf("invalid quota lane")
	}
	path := dispatchLedgerPath(agent)
	if path == "" {
		return "", fmt.Errorf("dispatch directory unavailable")
	}
	return path + ".quota.json", nil
}
func readProviderQuota(agent string) (providerQuotaRecord, error) {
	var r providerQuotaRecord
	p, e := providerQuotaPath(agent)
	if e != nil {
		return r, e
	}
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return r, nil
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(b, &r)
	if e == nil && r.BlockedAt.IsZero() {
		e = fmt.Errorf("invalid quota record")
	}
	return r, e
}
func writeProviderQuota(agent string, r providerQuotaRecord) error {
	p, e := providerQuotaPath(agent)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".quota-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), p); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(p))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}

// A separate stable lock inode protects rename-based state replacement across
// processes. The lock covers local read/write/fsync only, never consumer work.
func withProviderQuotaLock(agent string, fn func() error) (resultErr error) {
	p, e := providerQuotaPath(agent)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(p+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	// Normal read/fsync/rename operations may briefly contend, but the
	// controller must never wait indefinitely and lose its heartbeat.
	lockDeadline := time.Now().Add(200 * time.Millisecond)
	for {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			break
		}
		if (e != syscall.EWOULDBLOCK && e != syscall.EAGAIN) || !time.Now().Before(lockDeadline) {
			return fmt.Errorf("quota state busy: %w", e)
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer func() {
		if unlockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); resultErr == nil && unlockErr != nil {
			resultErr = fmt.Errorf("unlock provider quota state: %w", unlockErr)
		}
	}()
	return fn()
}

// observeProviderQuotaExit is independent of queue-progress scoring: a
// consumer may have acknowledged work before the provider refuses its next
// request. Keep that real progress, but never use it as proof of availability.
func observeProviderQuotaExit(agent, tail string, exitErr error) (bool, error) {
	if knownExitedFailure(tail, exitErr) != OutcomeProviderQuota {
		return false, nil
	}
	return true, recordProviderQuota(agent)
}

// Check the writable/syncable durable state path BEFORE starting a consumer.
// A persistence outage must not become a restart-and-paid-retry loop.
func checkProviderQuotaStorage(agent string) error {
	return withProviderQuotaLock(agent, func() error {
		p, e := providerQuotaPath(agent)
		if e != nil {
			return e
		}
		f, e := os.CreateTemp(filepath.Dir(p), ".quota-probe-")
		if e != nil {
			return e
		}
		name := f.Name()
		defer os.Remove(name)
		e = f.Sync()
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		return nil
	})
}
func recordProviderQuota(agent string) error {
	return withProviderQuotaLock(agent, func() error {
		r, e := readProviderQuota(agent)
		if e != nil {
			return e
		}
		r.Generation++
		r.BlockedAt = time.Now().UTC()
		r.History = append(r.History, r.BlockedAt)
		return writeProviderQuota(agent, r)
	})
}
func providerQuotaBlocked(agent string) (bool, error) {
	// Read errors are always a hold, including malformed state after restart.
	r, e := readProviderQuota(agent)
	if e != nil {
		return true, e
	}
	return !r.BlockedAt.IsZero() && (r.Generation == 0 || r.AvailableGeneration != r.Generation), nil
}

// RecordProviderAvailability requires the exact failure generation independently
// inspected by the custodian. A stale recovery can NEVER clear a newer failure.
func RecordProviderAvailability(agent, evidence string, generation uint64) error {
	if strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("availability evidence reference required")
	}
	return withProviderQuotaLock(agent, func() error {
		r, e := readProviderQuota(agent)
		if e != nil {
			return e
		}
		if r.BlockedAt.IsZero() || generation == 0 || r.Generation != generation {
			return fmt.Errorf("quota failure generation changed or missing; inspect current state")
		}
		r.AvailableAt = time.Now().UTC()
		r.AvailableGeneration = generation
		r.Evidence = evidence
		r.Recoveries = append(r.Recoveries, providerQuotaRecovery{Generation: generation, At: r.AvailableAt, Evidence: evidence})
		return writeProviderQuota(agent, r)
	})
}
