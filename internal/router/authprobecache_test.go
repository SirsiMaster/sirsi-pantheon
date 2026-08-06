package router

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func withTestCacheFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth-probe-cache.json")
	restore := SetAuthProbeCachePath(path)
	t.Cleanup(restore)
	return path
}

// THE regression the FIRST version of this cache shipped with, and the reason
// it exists as its own test rather than being folded into the count-based one
// below. An in-memory cache passes a same-process "called twice, hit once"
// test perfectly while doing NOTHING for the real usage pattern: every `sirsi`
// invocation is a separate process. This test simulates that by constructing
// a FRESH CachedAuthProbe wrapper per call — nothing in Go process memory
// survives between them — which is exactly what killed the in-memory version
// and exactly what a same-process loop cannot catch.
func TestCachedAuthProbe_SurvivesAcrossSeparateProcessInvocations(t *testing.T) {
	withTestCacheFile(t)
	var calls int32
	slow := func(string, string) (bool, bool, string) {
		atomic.AddInt32(&calls, 1)
		return true, false, ""
	}

	for i := 0; i < 5; i++ {
		// A fresh wrapper each iteration — no shared closure state, no shared
		// in-memory map. The only thing that can make call 2..5 fast is the file.
		CachedAuthProbe(slow)("/usr/bin/claude", "claude")
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("underlying probe called %d times across 5 independent invocations; want 1 — an in-memory-only cache would fail this exact way (each call is a fresh process in reality)", got)
	}
}

func TestCachedAuthProbe_RepeatCallsHitCacheNotTheUnderlyingProbe(t *testing.T) {
	withTestCacheFile(t)
	var calls int32
	slow := func(cliPath, agentType string) (bool, bool, string) {
		atomic.AddInt32(&calls, 1)
		return true, false, ""
	}
	cached := CachedAuthProbe(slow)

	for i := 0; i < 20; i++ {
		cached("/usr/bin/claude", "claude")
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("underlying probe called %d times across 20 reads; want 1", got)
	}
}

func TestCachedAuthProbe_ReturnsTheProbedValue(t *testing.T) {
	withTestCacheFile(t)
	cached := CachedAuthProbe(func(string, string) (bool, bool, string) {
		return false, true, "not authenticated"
	})

	authOK, needsLogin, detail := cached("/usr/bin/claude", "claude")
	if authOK || !needsLogin || detail != "not authenticated" {
		t.Errorf("got (%v,%v,%q), want (false,true,%q)", authOK, needsLogin, detail, "not authenticated")
	}
}

// A logout must become visible within one TTL window.
func TestCachedAuthProbe_ExpiryRefreshes(t *testing.T) {
	path := withTestCacheFile(t)
	var authOK atomic.Bool
	authOK.Store(true)
	var calls int32
	probe := func(string, string) (bool, bool, string) {
		atomic.AddInt32(&calls, 1)
		if authOK.Load() {
			return true, false, ""
		}
		return false, true, "logged out"
	}
	cached := CachedAuthProbe(probe)

	ok, _, _ := cached("/usr/bin/claude", "claude")
	if !ok {
		t.Fatal("expected authenticated on first probe")
	}

	// Back-date the on-disk entry rather than sleeping 5 real minutes.
	authProbeCacheMu.Lock()
	m := loadAuthProbeCache(path)
	for k, v := range m {
		v.At = time.Now().Add(-authProbeCacheTTL - time.Second)
		m[k] = v
	}
	saveAuthProbeCache(path, m)
	authProbeCacheMu.Unlock()

	authOK.Store(false)
	ok, needsLogin, _ := cached("/usr/bin/claude", "claude")
	if ok || !needsLogin {
		t.Errorf("stale cache entry was not refreshed after TTL expiry: authOK=%v needsLogin=%v", ok, needsLogin)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected exactly 2 underlying calls (initial + post-expiry refresh), got %d", got)
	}
}

func TestCachedAuthProbe_KeyedPerCLIAndAgentType(t *testing.T) {
	withTestCacheFile(t)
	cached := CachedAuthProbe(func(cliPath, agentType string) (bool, bool, string) {
		return agentType == "claude", false, agentType
	})

	claudeOK, _, _ := cached("/usr/bin/claude", "claude")
	codexOK, _, _ := cached("/usr/bin/codex", "codex")

	if !claudeOK {
		t.Error("claude entry got the wrong cached value")
	}
	if codexOK {
		t.Error("codex entry returned claude's cached result — cache key collision")
	}
}

func TestCachedAuthProbe_MissingCacheFileIsColdStartNotError(t *testing.T) {
	withTestCacheFile(t) // path set, file never written
	authOK, needsLogin, _ := CachedAuthProbe(func(string, string) (bool, bool, string) {
		return true, false, ""
	})("/usr/bin/claude", "claude")
	if !authOK || needsLogin {
		t.Errorf("a missing cache file was not treated as a normal cold start")
	}
}

// os.UserHomeDir() failing must degrade to "always probe", never panic on a
// nil path.
func TestCachedAuthProbe_UnresolvableCachePathDoesNotPanic(t *testing.T) {
	restore := SetAuthProbeCachePath("")
	defer restore()
	var calls int32
	cached := CachedAuthProbe(func(string, string) (bool, bool, string) {
		atomic.AddInt32(&calls, 1)
		return true, false, ""
	})
	cached("/usr/bin/claude", "claude")
	cached("/usr/bin/claude", "claude")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("with no cache path, every call must re-probe (got %d calls, want 2) — degrading to always-fresh is correct; silently caching nowhere would hide auth changes", got)
	}
}

// Rule A21: node-status can be invoked concurrently within one process.
func TestCachedAuthProbe_ConcurrentAccessIsRaceFree(t *testing.T) {
	withTestCacheFile(t)
	cached := CachedAuthProbe(func(string, string) (bool, bool, string) {
		time.Sleep(time.Millisecond)
		return true, false, ""
	})

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				cached("/usr/bin/claude", "claude")
			}
		}()
	}
	wg.Wait()
}

func TestResetAuthProbeCache_ClearsEntries(t *testing.T) {
	withTestCacheFile(t)
	var calls int32
	cached := CachedAuthProbe(func(string, string) (bool, bool, string) {
		atomic.AddInt32(&calls, 1)
		return true, false, ""
	})
	cached("/usr/bin/claude", "claude")
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("setup: expected one call before reset")
	}

	ResetAuthProbeCache()
	cached("/usr/bin/claude", "claude")

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("after ResetAuthProbeCache, expected a fresh probe (2 total calls), got %d", got)
	}
}

// A corrupt cache file (partial write from a killed process, disk fault) must
// degrade to re-probing, never crash a status read that a human is waiting on.
func TestCachedAuthProbe_CorruptCacheFileDegradesToReprobe(t *testing.T) {
	path := withTestCacheFile(t)
	if err := writeFileForTest(path, "{not json"); err != nil {
		t.Fatal(err)
	}
	authOK, needsLogin, _ := CachedAuthProbe(func(string, string) (bool, bool, string) {
		return true, false, ""
	})("/usr/bin/claude", "claude")
	if !authOK || needsLogin {
		t.Errorf("corrupt cache file was not handled as a cold start")
	}
}
