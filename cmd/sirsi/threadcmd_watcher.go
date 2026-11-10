package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// A record is a cooperative lease, NOT OS liveness or authority to signal PID.
// Only watch-router creates its own random incarnation and retains it in memory.
// A stop request must match that entire incarnation; recycled/caller PIDs cannot
// select a signal target because this protocol has no signal operation at all.
type routerWatcherRecord struct {
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	ThreadID   string `json:"thread_id"`
	AgentID    string `json:"agent_id"`
	RouterRoot string `json:"router_root"`
	PID        int    `json:"pid"`
	Nonce      string `json:"incarnation"`
}

type routerWatcherFiles interface {
	read(string) ([]byte, error)
	write(string, []byte) error
	Close() error
}

type routerWatcherCustody struct {
	files  routerWatcherFiles
	record routerWatcherRecord
	leaf   string
}

func routerWatcherLeaf(threadID string) (string, error) {
	if threadID == "" || len(threadID) > 128 || strings.Trim(threadID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		return "", errors.New("invalid watcher thread identifier")
	}
	key := sha256.Sum256([]byte(threadID))
	return hex.EncodeToString(key[:]) + ".pid", nil
}

func decodeRouterWatcherRecord(data []byte, threadID string) (routerWatcherRecord, error) {
	var record routerWatcherRecord
	if len(data) > 4096 || json.Unmarshal(data, &record) != nil {
		return record, errors.New("unverifiable watcher record; retained without signaling")
	}
	nonce, err := hex.DecodeString(record.Nonce)
	if record.Version != 1 || record.Kind != "sirsi-thread-watch-router" || record.ThreadID != threadID ||
		record.AgentID == "" || record.PID <= 0 || err != nil || len(nonce) != 32 || hex.EncodeToString(nonce) != record.Nonce ||
		!filepath.IsAbs(record.RouterRoot) || filepath.Clean(record.RouterRoot) != record.RouterRoot {
		return record, errors.New("watcher identity/format mismatch; retained without signaling")
	}
	return record, nil
}

func newRouterWatcherCustody(threadID, agentID, routerRoot string) (*routerWatcherCustody, error) {
	if err := legacyRouterWatcherDiagnostic(threadID, os.Lstat); err != nil {
		return nil, err
	}
	return newRouterWatcherCustodyWith(threadID, agentID, routerRoot, os.UserHomeDir, rand.Reader)
}

func newRouterWatcherCustodyWith(threadID, agentID, routerRoot string, home func() (string, error), entropy io.Reader) (*routerWatcherCustody, error) {
	leaf, err := routerWatcherLeaf(threadID)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(routerRoot)
	if err != nil {
		return nil, err
	}
	var nonce [32]byte
	if _, err := io.ReadFull(entropy, nonce[:]); err != nil {
		return nil, fmt.Errorf("watcher incarnation entropy: %w", err)
	}
	record := routerWatcherRecord{Version: 1, Kind: "sirsi-thread-watch-router", ThreadID: threadID,
		AgentID: agentID, RouterRoot: root, PID: os.Getpid(), Nonce: hex.EncodeToString(nonce[:])}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	if _, err := decodeRouterWatcherRecord(data, threadID); err != nil {
		return nil, err
	}
	files, err := openRouterWatcherFiles(home, true)
	if err != nil {
		return nil, err
	}
	// Do not overwrite an old numeric/otherwise unverifiable private record.
	if old, err := files.read(leaf); err == nil {
		previous, err := decodeRouterWatcherRecord(old, threadID)
		if err != nil {
			files.Close()
			return nil, err
		}
		if previous.AgentID != record.AgentID || previous.RouterRoot != record.RouterRoot {
			files.Close()
			return nil, errors.New("watcher scope differs from retained lease; no takeover or signal")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		files.Close()
		return nil, err
	}
	if err := files.write(leaf, append(data, '\n')); err != nil {
		files.Close()
		return nil, err
	}
	return &routerWatcherCustody{files: files, record: record, leaf: leaf}, nil
}

// Does not remove the publication on exit: a late old watcher must never unlink
// its successor's lease. Two bounded inert files per thread may remain; neither
// is a stopped receipt. The next verified-format publication replaces the lease.
func (c *routerWatcherCustody) Close() error { return c.files.Close() }

func (c *routerWatcherCustody) retired() bool {
	retired, _ := c.retirement()
	return retired
}

// retirement distinguishes an exact cooperative request from lost custody.
// Both stop dispatch; only the former confirms a normal watcher exit.
func (c *routerWatcherCustody) retirement() (bool, bool) {
	data, err := c.files.read(c.leaf)
	if err != nil {
		return true, false // loss of custody: self-retire, never signal anything
	}
	current, err := decodeRouterWatcherRecord(data, c.record.ThreadID)
	if err != nil || current != c.record {
		return true, false // successor publication, including same PID/different nonce
	}
	data, err = c.files.read(c.leaf + ".stop")
	if errors.Is(err, os.ErrNotExist) {
		return false, false
	}
	if err != nil {
		return true, false
	}
	stop, err := decodeRouterWatcherRecord(data, c.record.ThreadID)
	if err != nil {
		return true, false
	}
	return stop == c.record, stop == c.record // stale request cannot stop successor
}

func routerWatcherRetirementDecision(revalidate func() error, custody *routerWatcherCustody) (bool, error) {
	if err := revalidate(); err != nil {
		return true, fmt.Errorf("watch-router: custody ancestor changed; watcher retired: %w", err)
	}
	retired, requested := custody.retirement()
	if retired && !requested {
		return true, errors.New("watch-router: custody lost; watcher retired")
	}
	return retired, nil
}

func requestRouterWatcherStopWith(threadID string, home func() (string, error)) (bool, error) {
	leaf, err := routerWatcherLeaf(threadID)
	if err != nil {
		return false, err
	}
	files, err := openRouterWatcherFiles(home, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer files.Close()
	return requestRouterWatcherStopFiles(files, leaf, threadID)
}

func requestRouterWatcherStopFiles(files routerWatcherFiles, leaf, threadID string) (bool, error) {
	data, err := files.read(leaf)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := decodeRouterWatcherRecord(data, threadID); err != nil {
		return false, err
	}
	// Exact whole-record copy. A concurrent successor has a distinct nonce, so
	// even a late write cannot stop that successor. No PID probe or signal.
	if err := files.write(leaf+".stop", data); err != nil {
		return false, err
	}
	return true, nil // request persisted, not confirmed retirement
}

func legacyRouterWatcherDiagnostic(threadID string, lstat func(string) (os.FileInfo, error)) error {
	if _, err := routerWatcherLeaf(threadID); err != nil {
		return err
	}
	// This is deliberately the ONLY legacy-path operation: no read, unlink,
	// chmod, PID adoption, or signal. A legacy watcher may still be running.
	_, err := lstat("/tmp/sirsi-router-watch-" + threadID + ".pid")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect legacy watcher record; retirement unverified: %w", err)
	}
	return errors.New("legacy /tmp watcher record preserved; old bridge may still be running; owner-verified retirement required")
}

// The gate orders callback admission against shutdown. It does not own or kill
// detached consumers. Already admitted callbacks drain to their ordinary return;
// if router pull hangs, retirement remains pending rather than claiming quiet.
type routerWatcherDispatchGate struct {
	mu      sync.Mutex
	stopped bool
	active  int
	wg      sync.WaitGroup
}

func (g *routerWatcherDispatchGate) begin(ctx context.Context, custody *routerWatcherCustody) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped || ctx.Err() != nil || custody.retired() {
		return false
	}
	g.active++
	g.wg.Add(1)
	return true
}

func (g *routerWatcherDispatchGate) end() {
	g.mu.Lock()
	g.active--
	g.mu.Unlock()
	g.wg.Done()
}

func (g *routerWatcherDispatchGate) drain(out io.Writer) {
	g.mu.Lock()
	g.stopped = true
	active := g.active
	g.mu.Unlock()
	if active > 0 {
		fmt.Fprintln(out, "watch-router: callback already in flight; awaiting callback drain without interrupting its consumer; consumer quiet is unproven")
	}
	g.wg.Wait()
}

// routerWatcherCustodyPaths observes every named link to the retained custody
// directory, including replacement of HOME or one of its ancestors. Watching is
// installed before the first custody read to close the subscribe/check gap.
func routerWatcherCustodyPaths(home string) ([]string, error) {
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return nil, errors.New("watch-router: custody HOME must be absolute and clean")
	}
	dir := filepath.Join(home, ".sirsi", "router-watchers")
	var paths []string
	for p := dir; ; p = filepath.Dir(p) {
		paths = append(paths, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	return paths, nil
}

// armRouterWatcherObservation refuses a partial watch: otherwise a detached
// directory or dropped stop could silently leave the watcher live forever.
func armRouterWatcherObservation(paths []string, add func(string) error, retirement func() (bool, error)) (bool, error) {
	for _, path := range paths {
		if err := add(path); err != nil {
			return false, fmt.Errorf("watch-router: cannot observe custody %s: %w", path, err)
		}
	}
	return retirement()
}

func routerWatcherDispatchEvent(root string, event fsnotify.Event) bool {
	if event.Name == filepath.Join(root, "state.json") {
		return true
	}
	for _, name := range []string{"items", "proposals"} {
		dir := filepath.Join(root, name)
		if event.Name == dir || filepath.Dir(event.Name) == dir {
			return true
		}
	}
	return false
}

func routerWatcherObservationError(err error, open bool) error {
	if !open || err == nil {
		return errors.New("watch-router: filesystem observation stream closed; custody lost")
	}
	return fmt.Errorf("watch-router: filesystem observation failed; custody lost: %w", err)
}

// Each subscribed ancestor must still name the exact retained directory. A
// watch on a temporarily substituted inode cannot establish custody.
type routerWatcherAncestorBinding struct {
	path string
	file *os.File
}

func revalidateRouterWatcherAncestors(bindings []routerWatcherAncestorBinding) error {
	for _, binding := range bindings {
		opened, err := binding.file.Stat()
		if err != nil {
			return fmt.Errorf("stat retained custody ancestor %s: %w", binding.path, err)
		}
		named, err := os.Lstat(binding.path)
		if err != nil {
			return fmt.Errorf("lstat custody ancestor %s: %w", binding.path, err)
		}
		if !opened.IsDir() || !named.IsDir() || named.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, named) {
			return fmt.Errorf("watch-router: custody ancestor %s changed after subscription", binding.path)
		}
	}
	return nil
}
