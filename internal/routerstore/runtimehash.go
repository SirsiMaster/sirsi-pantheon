package routerstore

// Runtime-hash cache. RuntimeHash used to SHA-256 the whole running binary
// (~40 MB, ~140 ms) in every process. The result is now cached on disk keyed by
// the binary's resolved path plus its file identity, so an unchanged binary
// costs one stat instead of a full read.
//
// What the hash is, and what the cache does NOT change: the hash is a
// CLIENT-ASSERTED label. The running binary computes it about itself, sends it
// at mint (serve.go MintSession) and in X-Sirsi-Runtime on every call, and the
// server only checks that the value equals the one bound at mint
// (serve.go authenticate step 5). The server never re-hashes any binary and the
// header is outside the HMAC. An attacker who can replace the binary can already
// make it report any hash, and one who can write ~/.sirsi can already plant a
// session file, so this cache does not lower that bar. What it must still
// guarantee is that an HONEST binary never reports a stale hash: a replaced
// binary (rename-over: new inode; in-place write: new ctime, which cannot be set
// back from userspace; size/mtime change) fails the key and is re-hashed.
// Any read, parse, permission or key mismatch falls back to hashing.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type runtimeKey struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	MtimeNs int64  `json:"mtime_ns"`
	CtimeNs int64  `json:"ctime_ns"`
	Ino     uint64 `json:"ino"`
	Dev     uint64 `json:"dev"`
}

type runtimeCacheEntry struct {
	Key  runtimeKey `json:"key"`
	Hash string     `json:"hash"`
}

// Injectable seams (Rules A16/A21): mutex-guarded, set only through setRuntimeSeams.
var (
	runtimeSeamMu    sync.RWMutex
	runtimeExeFn     = os.Executable
	runtimeHashFn    = hashReader
	runtimeCachePath = defaultRuntimeCachePath
)

func runtimeSeams() (func() (string, error), func(io.Reader) (string, error), func(string) string) {
	runtimeSeamMu.RLock()
	defer runtimeSeamMu.RUnlock()
	return runtimeExeFn, runtimeHashFn, runtimeCachePath
}

func setRuntimeSeams(exe func() (string, error), hash func(io.Reader) (string, error), cachePath func(string) string) {
	runtimeSeamMu.Lock()
	defer runtimeSeamMu.Unlock()
	runtimeExeFn, runtimeHashFn, runtimeCachePath = exe, hash, cachePath
}

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// defaultRuntimeCachePath is ~/.sirsi/runtime-hash/<sha256(path)[:16]>.json: one
// file per binary path, so different binaries never thrash one entry. Go test
// binaries (*.test) never persist, so test runs leave nothing in the real home.
func defaultRuntimeCachePath(exe string) string {
	if strings.HasSuffix(exe, ".test") {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(exe))
	return filepath.Join(home, ".sirsi", "runtime-hash", hex.EncodeToString(sum[:8])+".json")
}

func runtimeKeyOf(path string, fi os.FileInfo) (runtimeKey, bool) {
	ino, dev, ctime, ok := fileIdentity(fi)
	if !ok {
		return runtimeKey{}, false
	}
	return runtimeKey{Path: path, Size: fi.Size(), MtimeNs: fi.ModTime().UnixNano(), CtimeNs: ctime, Ino: ino, Dev: dev}, true
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// readRuntimeCache returns the cached hash only on an exact key match from a
// file that is not group/world writable.
func readRuntimeCache(cachePath string, want runtimeKey) (string, bool) {
	fi, err := os.Lstat(cachePath)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o022 != 0 {
		return "", false
	}
	b, err := os.ReadFile(cachePath)
	if err != nil {
		return "", false
	}
	var e runtimeCacheEntry
	if json.Unmarshal(b, &e) != nil || e.Key != want || !isHex64(e.Hash) {
		return "", false
	}
	return e.Hash, true
}

// writeRuntimeCache is best effort: temp file (0600) in the same dir, then rename,
// so concurrent processes can never leave a torn file.
func writeRuntimeCache(cachePath string, key runtimeKey, hash string) {
	dir := filepath.Dir(cachePath)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	b, err := json.Marshal(runtimeCacheEntry{Key: key, Hash: hash})
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(tmp.Name(), 0o600) != nil || os.Rename(tmp.Name(), cachePath) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// computeRuntimeHash returns the SHA-256 of the running executable, from cache
// when the binary's identity is unchanged, else by hashing the opened file. The
// key is taken from the same handle that is hashed and re-checked afterwards, so
// a binary modified mid-hash is never cached.
func computeRuntimeHash() string {
	exeFn, hashFn, cachePathFn := runtimeSeams()
	exe, err := exeFn()
	if err != nil {
		return "unknown"
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	cachePath := cachePathFn(exe)
	if cachePath != "" {
		if fi, err := os.Stat(exe); err == nil {
			if k, ok := runtimeKeyOf(exe, fi); ok {
				if h, ok := readRuntimeCache(cachePath, k); ok {
					return h
				}
			}
		}
	}
	f, err := os.Open(exe)
	if err != nil {
		return "unknown"
	}
	defer func() { _ = f.Close() }()
	var before runtimeKey
	haveKey := false
	if fi, err := f.Stat(); err == nil {
		before, haveKey = runtimeKeyOf(exe, fi)
	}
	h, err := hashFn(f)
	if err != nil {
		return "unknown"
	}
	if haveKey && cachePath != "" {
		if fi, err := f.Stat(); err == nil {
			if after, ok := runtimeKeyOf(exe, fi); ok && after == before {
				writeRuntimeCache(cachePath, before, h)
			}
		}
	}
	return h
}
