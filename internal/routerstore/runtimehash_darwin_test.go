package routerstore

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// rtFixture installs a fake executable + counting hasher + temp cache path.
type rtFixture struct {
	exe, cache string
	hashes     atomic.Int32
}

func newRTFixture(t *testing.T, content string) *rtFixture {
	t.Helper()
	dir := t.TempDir()
	f := &rtFixture{exe: filepath.Join(dir, "sirsi"), cache: filepath.Join(dir, "cache", "rh.json")}
	if err := os.WriteFile(f.exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	oe, oh, oc := runtimeSeams()
	setRuntimeSeams(func() (string, error) { return f.exe, nil },
		func(r io.Reader) (string, error) { f.hashes.Add(1); return hashReader(r) },
		func(string) string { return f.cache })
	t.Cleanup(func() { setRuntimeSeams(oe, oh, oc) })
	return f
}

func TestRuntimeHashCacheHitSkipsReadingBinary(t *testing.T) {
	f := newRTFixture(t, "binary-v1")
	first := computeRuntimeHash()
	if f.hashes.Load() != 1 || !isHex64(first) {
		t.Fatalf("first call: hashes=%d hash=%q", f.hashes.Load(), first)
	}
	if second := computeRuntimeHash(); second != first || f.hashes.Load() != 1 {
		t.Fatalf("hit must return same hash without hashing: %q vs %q, hashes=%d", second, first, f.hashes.Load())
	}
}

func TestRuntimeHashCacheFile0600(t *testing.T) {
	f := newRTFixture(t, "binary-v1")
	computeRuntimeHash()
	fi, err := os.Stat(f.cache)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("cache file: err=%v mode=%v", err, fi)
	}
}

func TestRuntimeHashRecomputesWhenBinaryChanges(t *testing.T) {
	cases := map[string]func(t *testing.T, f *rtFixture){
		"same-size rewrite in place": func(t *testing.T, f *rtFixture) {
			if err := os.WriteFile(f.exe, []byte("binary-v2"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"size change": func(t *testing.T, f *rtFixture) {
			if err := os.WriteFile(f.exe, []byte("binary-v1-longer"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"mtime change only": func(t *testing.T, f *rtFixture) {
			future := time.Now().Add(time.Hour)
			if err := os.Chtimes(f.exe, future, future); err != nil {
				t.Fatal(err)
			}
		},
		"rename-over (new inode, same content)": func(t *testing.T, f *rtFixture) {
			tmp := f.exe + ".new"
			if err := os.WriteFile(tmp, []byte("binary-v1"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(tmp, f.exe); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newRTFixture(t, "binary-v1")
			computeRuntimeHash()
			mutate(t, f)
			computeRuntimeHash()
			if f.hashes.Load() != 2 {
				t.Fatalf("binary changed but hashes=%d (want 2: stale cache served)", f.hashes.Load())
			}
		})
	}
}

// Each key field alone must defeat the cache. The mtime row is the one the
// negative control (ignore mtime in the comparison) turns red.
func TestRuntimeCacheKeyEveryFieldMatters(t *testing.T) {
	key := runtimeKey{Path: "/x/sirsi", Size: 10, MtimeNs: 20, CtimeNs: 30, Ino: 40, Dev: 50}
	hash := hashOf(t, "h")
	cache := filepath.Join(t.TempDir(), "rh.json")
	writeRuntimeCache(cache, key, hash)
	if got, ok := readRuntimeCache(cache, key); !ok || got != hash {
		t.Fatalf("exact key must hit: %q %v", got, ok)
	}
	mut := map[string]runtimeKey{
		"path":  func() runtimeKey { k := key; k.Path += "2"; return k }(),
		"size":  func() runtimeKey { k := key; k.Size++; return k }(),
		"mtime": func() runtimeKey { k := key; k.MtimeNs++; return k }(),
		"ctime": func() runtimeKey { k := key; k.CtimeNs++; return k }(),
		"ino":   func() runtimeKey { k := key; k.Ino++; return k }(),
		"dev":   func() runtimeKey { k := key; k.Dev++; return k }(),
	}
	for name, k := range mut {
		if _, ok := readRuntimeCache(cache, k); ok {
			t.Errorf("changing %s must miss", name)
		}
	}
}

func hashOf(t *testing.T, s string) string {
	t.Helper()
	h, err := hashReader(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRuntimeHashCorruptOrUnsafeCacheRecomputes(t *testing.T) {
	cases := map[string]func(t *testing.T, cache string){
		"garbage":        func(t *testing.T, c string) { _ = os.WriteFile(c, []byte("not json"), 0o600) },
		"bad hash":       func(t *testing.T, c string) { _ = os.WriteFile(c, []byte(`{"key":{},"hash":"zz"}`), 0o600) },
		"empty":          func(t *testing.T, c string) { _ = os.WriteFile(c, nil, 0o600) },
		"world-writable": func(t *testing.T, c string) { _ = os.Chmod(c, 0o666) },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			f := newRTFixture(t, "binary-v1")
			want := computeRuntimeHash()
			corrupt(t, f.cache)
			if got := computeRuntimeHash(); got != want || f.hashes.Load() != 2 {
				t.Fatalf("must recompute the true hash: got %q want %q hashes=%d", got, want, f.hashes.Load())
			}
		})
	}
}

func TestRuntimeHashConcurrentWritersNeverCorrupt(t *testing.T) {
	f := newRTFixture(t, "binary-v1")
	want := computeRuntimeHash()
	key, ok := func() (runtimeKey, bool) {
		fi, _ := os.Stat(f.exe)
		return runtimeKeyOf(f.exe, fi)
	}()
	if !ok {
		t.Skip("no file identity")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				writeRuntimeCache(f.cache, key, want)
				if h, ok := readRuntimeCache(f.cache, key); ok && h != want {
					t.Errorf("torn read: %q", h)
				}
			}
		}()
	}
	wg.Wait()
	if h, ok := readRuntimeCache(f.cache, key); !ok || h != want {
		t.Fatalf("final cache invalid: %q %v", h, ok)
	}
	if ents, _ := os.ReadDir(filepath.Dir(f.cache)); len(ents) != 1 {
		t.Fatalf("temp files leaked: %d entries", len(ents))
	}
}
