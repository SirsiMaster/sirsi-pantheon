package router

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DaemonOptions configures the resident autorouter runtime.
type DaemonOptions struct {
	RepoRoot    string
	Agent       string
	DryRun      bool
	Interval    time.Duration
	Debounce    time.Duration
	Out         io.Writer
	Notify      NotifyFunc // legacy dispatch
	Executor    *Executor  // v3 dispatch (takes precedence over Notify)
	UseFSNotify bool
	LedgerPath  string
}

// Daemon watches the idea-router and dispatches pending inbox items.
type Daemon struct {
	router *Router
	opts   DaemonOptions
	runner *Runner
}

// NewDaemon creates a daemon around the existing Runner.
func NewDaemon(r *Router, opts DaemonOptions) *Daemon {
	if opts.Interval == 0 {
		opts.Interval = time.Second
	}
	if opts.Debounce == 0 {
		opts.Debounce = 150 * time.Millisecond
	}
	if opts.Agent == "" {
		opts.Agent = "all"
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.LedgerPath == "" && opts.RepoRoot != "" && !opts.DryRun {
		opts.LedgerPath = filepath.Join(opts.RepoRoot, ".agents", "idea-router", "dispatch-ledger.json")
	}
	// UseFSNotify defaults to false (polling-only). Callers that want
	// filesystem watching must set it explicitly.
	return &Daemon{
		router: r,
		opts:   opts,
		runner: NewRunner(r, RunnerOptions{
			RepoRoot:       opts.RepoRoot,
			Agent:          opts.Agent,
			DryRun:         opts.DryRun,
			Interval:       opts.Interval,
			Out:            opts.Out,
			Notify:         opts.Notify,
			Executor:       opts.Executor,
			LedgerPath:     opts.LedgerPath,
			FailureBackoff: 30 * time.Second,
		}),
	}
}

// Run starts the daemon loop until ctx is canceled.
func (d *Daemon) Run(ctx context.Context) error {
	fmt.Fprintln(d.opts.Out, "Autorouter daemon started.")
	if err := d.runner.Tick(ctx); err != nil {
		return err
	}
	if !d.opts.UseFSNotify {
		return d.runPolling(ctx)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(d.opts.Out, "fsnotify unavailable, falling back to polling: %v\n", err)
		return d.runPolling(ctx)
	}
	defer watcher.Close()

	watched := 0
	for _, p := range d.watchPaths() {
		if err := watcher.Add(p); err != nil {
			fmt.Fprintf(d.opts.Out, "Warning: cannot watch %s: %v\n", p, err)
			continue
		}
		watched++
	}
	if watched == 0 {
		fmt.Fprintln(d.opts.Out, "No router paths could be watched; falling back to polling.")
		return d.runPolling(ctx)
	}

	tick := time.NewTicker(d.opts.Interval)
	defer tick.Stop()

	var debounce *time.Timer
	var debounceC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(d.opts.Out, "Autorouter daemon stopped.")
			return nil
		case ev := <-watcher.Events:
			if isRouterWriteEvent(ev) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.NewTimer(d.opts.Debounce)
				debounceC = debounce.C
			}
		case err := <-watcher.Errors:
			fmt.Fprintf(d.opts.Out, "Watch warning: %v\n", err)
		case <-debounceC:
			debounceC = nil
			if err := d.runner.Tick(ctx); err != nil {
				return err
			}
		case <-tick.C:
			if err := d.runner.Tick(ctx); err != nil {
				return err
			}
		}
	}
}

func (d *Daemon) runPolling(ctx context.Context) error {
	tick := time.NewTicker(d.opts.Interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(d.opts.Out, "Autorouter daemon stopped.")
			return nil
		case <-tick.C:
			if err := d.runner.Tick(ctx); err != nil {
				return err
			}
		}
	}
}

func (d *Daemon) watchPaths() []string {
	root := filepath.Join(d.opts.RepoRoot, ".agents", "idea-router")
	paths := []string{
		filepath.Join(root, "state.json"),
		filepath.Join(root, "proposals"),
		filepath.Join(root, "reviews"),
		filepath.Join(root, "decisions"),
	}
	out := paths[:0]
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func isRouterWriteEvent(ev fsnotify.Event) bool {
	return ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) != 0
}
