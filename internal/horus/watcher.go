// Package horus — watcher.go
//
// Phase 2: Live file watching via fsnotify. When Go source files change,
// the watcher invalidates the cached SymbolGraph and triggers a re-parse.
// This keeps the code graph always current without manual re-scans.
//
// Architecture:
//
//	Watcher monitors a project root for .go file changes.
//	On create/write/rename of .go files → invalidate cache → re-parse.
//	On delete → invalidate only (graph shrinks).
//	Debounced at 500ms to batch rapid saves (IDE auto-format + save).
//
// Usage:
//
//	w := horus.NewWatcher("/path/to/project")
//	w.OnUpdate = func(g *SymbolGraph) { /* use fresh graph */ }
//	w.Start(ctx)
//	defer w.Stop()
package horus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a project directory for Go source changes
// and keeps the SymbolGraph cache current.
type Watcher struct {
	root    string
	cache   *Cache
	parser  *GoParser
	watcher *fsnotify.Watcher
	mu      sync.RWMutex
	graph   *SymbolGraph
	running bool
	stopCh  chan struct{}

	// OnUpdate is called with the fresh SymbolGraph after a re-parse.
	// Called on a background goroutine — must be thread-safe.
	OnUpdate func(g *SymbolGraph)

	// Stats
	Rebuilds int64
	Errors   int64
}

// NewWatcher creates a file watcher for the given project root.
func NewWatcher(root string) *Watcher {
	return &Watcher{
		root:   root,
		cache:  NewCache(),
		parser: NewGoParser(),
		stopCh: make(chan struct{}),
	}
}

// Graph returns the current symbol graph (may be nil if not yet parsed).
func (w *Watcher) Graph() *SymbolGraph {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.graph
}

// Start begins watching for file changes. Non-blocking.
func (w *Watcher) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher

	// Walk the directory tree and add all directories
	err = filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			// Skip irrelevant directories
			if name == ".git" || name == "node_modules" || name == "vendor" ||
				name == ".next" || name == "dist" || name == ".turbo" {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		watcher.Close()
		return err
	}

	// Load initial graph from cache or parse
	if g, ok := w.cache.Get(w.root); ok {
		w.mu.Lock()
		w.graph = g
		w.mu.Unlock()
	} else {
		w.rebuild()
	}

	w.running = true
	go w.loop(ctx)
	return nil
}

// Stop shuts down the watcher.
func (w *Watcher) Stop() {
	if !w.running {
		return
	}
	w.running = false
	close(w.stopCh)
	if w.watcher != nil {
		w.watcher.Close()
	}
}

// IsRunning returns whether the watcher is active.
func (w *Watcher) IsRunning() bool {
	return w.running
}

func (w *Watcher) loop(ctx context.Context) {
	// Debounce timer — batch rapid file changes
	var debounce *time.Timer
	pending := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only care about Go source files
			if !isGoSource(event.Name) {
				continue
			}

			// Relevant operations: create, write, rename, remove
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}

			// Debounce: wait 500ms after last change before rebuilding.
			// This batches IDE auto-format + save into one rebuild.
			pending = true
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(500*time.Millisecond, func() {
				if pending {
					pending = false
					w.rebuild()
				}
			})

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			_ = err
			w.Errors++
		}
	}
}

func (w *Watcher) rebuild() {
	g, err := w.parser.ParseDir(w.root)
	if err != nil {
		w.Errors++
		return
	}

	w.mu.Lock()
	w.graph = g
	w.Rebuilds++
	w.mu.Unlock()

	// Update cache
	_ = w.cache.Put(w.root, g)

	// Notify consumer
	if w.OnUpdate != nil {
		w.OnUpdate(g)
	}
}

func isGoSource(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return true // test files affect the graph too
	}
	return strings.HasSuffix(path, ".go")
}
