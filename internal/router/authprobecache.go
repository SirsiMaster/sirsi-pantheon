package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Auth probe caching (owner priority 2026-08-06).
//
// `node-status` is meant to be a status read — every sibling verb (`router
// ledger`, `thread list`, `router status`) answers in ~10ms. It measured at
// 10-14 SECONDS, because DefaultAuthProbe's claude branch spawns
// `claude --print "respond with OK"`: a full LLM round-trip billed to the
// owner's account, run synchronously on every call. Confirmed directly:
// `claude --print` took 10,881ms against 104ms for `claude --version` — 105x.
//
// FIRST ATTEMPT AT THIS FIX WAS WRONG, and the record stays because the
// mistake is the instructive part: an in-memory, package-level cache. Every
// `sirsi` invocation is a SEPARATE PROCESS — the CLI has no daemon — so a
// map living in process memory is born empty and dies with the process on
// EVERY call. Verified end-to-end with three consecutive real invocations:
// 23s, then 14s, then 13s. No improvement, because there was never a second
// read of the same map. Unit tests all passed anyway, because they called
// CachedAuthProbe twice within one test process — they never exercised the
// actual usage pattern of one process per call, so they proved the caching
// LOGIC works without proving the FIX works.
//
// The cache therefore has to survive the process, i.e. live on disk, exactly
// like threads.json (threads.go) and every other piece of cross-invocation
// router state. Same atomic-write pattern: temp file + rename, never a
// partial write visible to a concurrent reader.
//
// Auth state does not change on the timescale of a poll loop — a login or
// logout is a deliberate operator action — so caching it costs nothing that
// matters to what the field means. AuthOK still means "the CLI was confirmed
// authenticated"; it is confirmed on a cadence a human would call "recently",
// not on literally every read.

// authProbeCacheTTL bounds how stale a cached auth result may be before the
// next call pays for a fresh probe.
const authProbeCacheTTL = 5 * time.Minute

const authProbeCacheFilename = "auth-probe-cache.json"

type authProbeResult struct {
	AuthOK     bool      `json:"auth_ok"`
	NeedsLogin bool      `json:"needs_login"`
	Detail     string    `json:"detail,omitempty"`
	At         time.Time `json:"at"`
}

// authProbeCacheMu guards the ON-DISK file against concurrent read-modify-
// write races WITHIN this process (Rule A21: node-status can be invoked
// concurrently, e.g. an interactive call racing a background poll). It does
// NOT protect against a concurrent SEPARATE process — the atomic
// temp-file-then-rename write pattern is what makes a torn write impossible
// across processes; the mutex only prevents two goroutines in this process
// from interleaving their own read-modify-write.
var authProbeCacheMu sync.Mutex

// authProbeCachePathFn is injectable (Rule A16) so tests never read or write
// the operator's real ~/.sirsi/auth-probe-cache.json.
var authProbeCachePathFn = defaultAuthProbeCachePath

func defaultAuthProbeCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "" // caller treats "" as cache-unavailable, never as a path to write
	}
	return filepath.Join(home, ".sirsi", authProbeCacheFilename)
}

// SetAuthProbeCachePath overrides the cache file location and returns a
// restore function. Test-only seam.
func SetAuthProbeCachePath(path string) (restore func()) {
	old := authProbeCachePathFn
	authProbeCachePathFn = func() string { return path }
	return func() { authProbeCachePathFn = old }
}

func loadAuthProbeCache(path string) map[string]authProbeResult {
	empty := map[string]authProbeResult{}
	if path == "" {
		return empty
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return empty // missing file is the normal cold-start state, not an error
	}
	var m map[string]authProbeResult
	if err := json.Unmarshal(data, &m); err != nil {
		return empty // a corrupt cache file must degrade to "re-probe", never crash a status read
	}
	if m == nil {
		return empty
	}
	return m
}

func saveAuthProbeCache(path string, m map[string]authProbeResult) {
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".auth-probe-cache.json-*")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	// Cache-only best-effort: a failed write here means the next process pays
	// for a fresh probe, never a corrupt read. Silently accepting that is
	// correct — this file is disposable in a way threads.json is not.
	_ = os.Rename(tmpPath, path)
}

// CachedAuthProbe wraps an AuthProbeFunc with the disk-backed TTL cache. The
// wrapped probe still runs synchronously on a cache miss or expiry — this
// amortizes latency across process invocations, it does not hide it: the
// first call after authProbeCacheTTL elapses is exactly as slow as before.
func CachedAuthProbe(probe AuthProbeFunc) AuthProbeFunc {
	return func(cliPath, agentType string) (bool, bool, string) {
		key := cliPath + "\x00" + agentType
		now := time.Now()
		path := authProbeCachePathFn()

		authProbeCacheMu.Lock()
		cache := loadAuthProbeCache(path)
		if r, ok := cache[key]; ok && now.Sub(r.At) < authProbeCacheTTL {
			authProbeCacheMu.Unlock()
			return r.AuthOK, r.NeedsLogin, r.Detail
		}
		authProbeCacheMu.Unlock()

		// The actual probe runs OUTSIDE the lock. It can take 11 seconds; holding
		// the mutex across that would block every other goroutine in THIS process
		// from even checking the cache, for no reason — they are not writing.
		authOK, needsLogin, detail := probe(cliPath, agentType)

		authProbeCacheMu.Lock()
		// Reload rather than reuse the earlier `cache` var: another goroutine (or
		// another process, via the file) may have written a result for a
		// DIFFERENT key while this probe was in flight, and blindly overwriting
		// with the stale in-memory map would lose that entry.
		cache = loadAuthProbeCache(path)
		cache[key] = authProbeResult{AuthOK: authOK, NeedsLogin: needsLogin, Detail: detail, At: now}
		saveAuthProbeCache(path, cache)
		authProbeCacheMu.Unlock()

		return authOK, needsLogin, detail
	}
}

// ResetAuthProbeCache deletes the cache file. Exported for tests that need a
// clean cache between cases — without it, a test asserting a fresh probe ran
// could pass for the wrong reason (reading a previous case's cached result).
func ResetAuthProbeCache() {
	path := authProbeCachePathFn()
	if path == "" {
		return
	}
	authProbeCacheMu.Lock()
	defer authProbeCacheMu.Unlock()
	_ = os.Remove(path)
}
