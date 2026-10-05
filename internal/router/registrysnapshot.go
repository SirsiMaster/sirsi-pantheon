package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Origin-pinned registry (A37, ADR-072 P5, ledger task identity-from-origin).
//
// The registry is read from agents.json in the router root, which is the working
// tree of a SHARED checkout. Whatever branch and uncommitted edits another session
// left there silently became the fabric's identity: a codex lane flipped to
// wake=none and read WATCH_ONLY, sends failed "identity is not fully declared", and
// every fix was a hand copy. A host that opts in with `registry sync` reads a
// snapshot of origin/main instead: the record exists only on origin, and a local
// working-tree edit cannot change who the lanes are.

// ErrRegistryPinned is returned when something tries to write the registry while the
// host reads the origin-pinned snapshot: identity changes go through a PR.
var ErrRegistryPinned = errors.New("registry is origin-pinned on this host: change .agents/idea-router/agents.json through a PR to main, then `sirsi router registry sync` (or `registry unpin` to read the working tree again)")

const registrySnapshotMaxAge = 72 * time.Hour

type registryMeta struct {
	RouterRoot string `json:"router_root"` // the checkout this snapshot belongs to; any other root reads its own tree
	SHA256     string `json:"sha256"`
	Commit     string `json:"source_commit"`
	FetchedAt  string `json:"fetched_at"`
}

var snapshotDirOverride struct {
	sync.RWMutex
	dir string
}

// snapshotDir is ~/.sirsi/registry (overridable for tests).
func snapshotDir() string {
	snapshotDirOverride.RLock()
	d := snapshotDirOverride.dir
	snapshotDirOverride.RUnlock()
	if d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sirsi", "registry")
}

// setSnapshotDirForTest points the snapshot at a temp dir.
func setSnapshotDirForTest(d string) {
	snapshotDirOverride.Lock()
	snapshotDirOverride.dir = d
	snapshotDirOverride.Unlock()
}

func snapshotPaths() (data, meta string) {
	d := snapshotDir()
	if d == "" {
		return "", ""
	}
	return filepath.Join(d, "agents.json"), filepath.Join(d, "agents.meta.json")
}

var staleLogged sync.Once

// registrySource says which agents.json this host reads: the origin snapshot when
// one exists and is fresh, else the working tree (the pre-pinning behavior).
func registrySource(routerRoot string) (path string, pinned bool) {
	working := filepath.Join(routerRoot, "agents.json")
	dp, mp := snapshotPaths()
	if dp == "" {
		return working, false
	}
	if _, err := os.Stat(dp); err != nil {
		return working, false
	}
	var m registryMeta
	if b, err := os.ReadFile(mp); err == nil && json.Unmarshal(b, &m) == nil {
		// The snapshot belongs to one checkout. A different router root (a test's temp
		// dir, another clone) reads its own agents.json.
		if m.RouterRoot != "" && m.RouterRoot != filepath.Clean(routerRoot) {
			return working, false
		}
		if t, perr := time.Parse(time.RFC3339, m.FetchedAt); perr == nil && time.Since(t) > registrySnapshotMaxAge {
			staleLogged.Do(func() {
				log.Printf("registry: origin snapshot is %s old (max %s); reading the working tree until `sirsi router registry sync`", time.Since(t).Round(time.Hour), registrySnapshotMaxAge)
			})
			return working, false
		}
	}
	return dp, true
}

// GitRunner runs git and returns stdout; injectable (Rule A16).
type GitRunner func(dir string, args ...string) (string, error)

func execGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

// SyncRegistrySnapshot fetches origin/main and stores agents.json from it as this
// host's pinned registry. It validates the JSON before replacing anything and
// writes atomically, so a failed or partial sync leaves the previous source intact.
func SyncRegistrySnapshot(routerRoot string, run GitRunner) (registryMeta, error) {
	if run == nil {
		run = execGit
	}
	repo := filepath.Clean(filepath.Join(routerRoot, "..", ".."))
	if _, err := run(repo, "fetch", "-q", "origin", "main"); err != nil {
		return registryMeta{}, fmt.Errorf("git fetch origin main: %w", err)
	}
	commit, err := run(repo, "rev-parse", "origin/main")
	if err != nil {
		return registryMeta{}, fmt.Errorf("resolve origin/main: %w", err)
	}
	rel := filepath.ToSlash(filepath.Join(".agents", "idea-router", "agents.json"))
	data, err := run(repo, "show", "origin/main:"+rel)
	if err != nil {
		return registryMeta{}, fmt.Errorf("read %s from origin/main: %w", rel, err)
	}
	var reg Registry
	if err := json.Unmarshal([]byte(data), &reg); err != nil || len(reg.Agents) == 0 {
		return registryMeta{}, fmt.Errorf("origin/main agents.json is not a usable registry (parse error %v, %d agents): snapshot not replaced", err, len(reg.Agents))
	}
	dp, mp := snapshotPaths()
	if dp == "" {
		return registryMeta{}, errors.New("no home directory for the registry snapshot")
	}
	if err := os.MkdirAll(filepath.Dir(dp), 0o755); err != nil {
		return registryMeta{}, err
	}
	sum := sha256.Sum256([]byte(data))
	m := registryMeta{RouterRoot: filepath.Clean(routerRoot), SHA256: hex.EncodeToString(sum[:]), Commit: strings.TrimSpace(commit), FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeFileAtomic(dp, []byte(data)); err != nil {
		return registryMeta{}, err
	}
	mb, _ := json.Marshal(m)
	return m, writeFileAtomic(mp, mb)
}

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// UnpinRegistry removes the snapshot so the host reads the working tree again.
func UnpinRegistry() error {
	dp, mp := snapshotPaths()
	if dp == "" {
		return nil
	}
	_ = os.Remove(mp)
	if err := os.Remove(dp); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RegistryPinStatus describes which source this host reads.
func RegistryPinStatus(routerRoot string) (path string, pinned bool, meta *registryMeta) {
	path, pinned = registrySource(routerRoot)
	if pinned {
		_, mp := snapshotPaths()
		var m registryMeta
		if b, err := os.ReadFile(mp); err == nil && json.Unmarshal(b, &m) == nil {
			meta = &m
		}
	}
	return path, pinned, meta
}
