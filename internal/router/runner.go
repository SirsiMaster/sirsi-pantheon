package router

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"
)

// NotifyFunc is the function signature for agent notification.
type NotifyFunc func(target, docType, docID, repoRoot string) error

// RunnerOptions configures the autorouter dispatch loop.
type RunnerOptions struct {
	RepoRoot       string
	Agent          string // registered agent_id, legacy "codex"/"claude", or "all"
	DryRun         bool
	Once           bool
	Interval       time.Duration
	Out            io.Writer
	Notify         NotifyFunc // legacy dispatch (used when Executor is nil)
	Executor       *Executor  // v3 dispatch: registry-based, writeback-verified
	LedgerPath     string
	FailureBackoff time.Duration
}

// Dispatch represents a single pending notification to deliver.
type Dispatch struct {
	Target  string
	DocID   string
	Type    DocType
	Title   string
	ModTime time.Time
	Size    int
}

// Runner polls the idea-router state and dispatches notifications
// for pending inbox items. It does NOT acknowledge items — the target
// agent must ack after reading.
type Runner struct {
	router       *Router
	opts         RunnerOptions
	dispatched   map[string]string
	failedUntil  map[string]time.Time
	failureCount map[string]int // consecutive failure count per key
	ledger       *DispatchLedger
	lastEmpty    bool // suppress repeated "No pending" messages
}

// NewRunner creates a Runner with the given options.
func NewRunner(r *Router, opts RunnerOptions) *Runner {
	if opts.Interval == 0 {
		opts.Interval = 10 * time.Second
	}
	if opts.Agent == "" {
		opts.Agent = "all"
	}
	if opts.Notify == nil {
		opts.Notify = NotifyAgent
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	rr := &Runner{
		router:       r,
		opts:         opts,
		dispatched:   make(map[string]string),
		failedUntil:  make(map[string]time.Time),
		failureCount: make(map[string]int),
	}
	if opts.LedgerPath != "" {
		ledger, err := LoadDispatchLedger(opts.LedgerPath)
		if err != nil {
			fmt.Fprintf(opts.Out, "Warning: dispatch ledger disabled: %v\n", err)
		} else {
			rr.ledger = ledger
		}
	}
	return rr
}

// Run executes the dispatch loop until context is canceled or --once completes.
func (rr *Runner) Run(ctx context.Context) error {
	for {
		if err := rr.Tick(ctx); err != nil {
			return err
		}
		if rr.opts.Once {
			return nil
		}
		timer := time.NewTimer(rr.opts.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// Tick performs a single scan of pending dispatches and notifies as needed.
func (rr *Runner) Tick(ctx context.Context) error {
	dispatches, err := rr.PendingDispatches()
	if err != nil {
		return err
	}
	if len(dispatches) == 0 {
		if !rr.lastEmpty {
			fmt.Fprintln(rr.opts.Out, "No pending dispatches.")
			rr.lastEmpty = true
		}
		return nil
	}
	rr.lastEmpty = false
	for _, d := range dispatches {
		key := d.Target + ":" + d.DocID
		fingerprint := d.Fingerprint()
		if rr.dispatched[key] == fingerprint {
			continue
		}
		if until := rr.failedUntil[key+":"+fingerprint]; !until.IsZero() && time.Now().Before(until) {
			continue
		}
		if rr.ledger != nil && rr.ledger.WasDispatched(key, fingerprint) {
			rr.dispatched[key] = fingerprint
			continue
		}
		if rr.opts.DryRun {
			fmt.Fprintf(rr.opts.Out, "[dry-run] Would notify %s for %s %s — %s\n", d.Target, d.Type, d.DocID, d.Title)
			rr.dispatched[key] = fingerprint
			continue
		}
		fmt.Fprintf(rr.opts.Out, "Dispatching to %s for %s %s — %s\n", d.Target, d.Type, d.DocID, d.Title)
		var dispatchErr error
		if rr.opts.Executor != nil {
			// v3 path: registry-based dispatch with writeback verification
			item := rr.opts.Executor.workQueue.AddItem(d.DocID, d.Target, "", d.Title)
			_ = rr.opts.Executor.workQueue.Save()
			dispatchErr = rr.opts.Executor.Dispatch(ctx, item)
		} else {
			// legacy path: NotifyFunc
			dispatchErr = rr.opts.Notify(d.Target, string(d.Type), d.DocID, rr.opts.RepoRoot)
		}
		if err := dispatchErr; err != nil {
			failKey := key + ":" + fingerprint
			rr.failureCount[failKey]++
			count := rr.failureCount[failKey]
			backoff := rr.opts.FailureBackoff
			if backoff > 0 {
				// Exponential backoff: double each consecutive failure, cap at 5 min
				scaled := time.Duration(count) * backoff
				if scaled > 5*time.Minute {
					scaled = 5 * time.Minute
				}
				rr.failedUntil[failKey] = time.Now().Add(scaled)
				if count <= 2 {
					fmt.Fprintf(rr.opts.Out, "  Warning: %s dispatch failed (attempt %d), backing off %s: %v\n", d.Target, count, scaled, err)
				} else if count%5 == 0 {
					fmt.Fprintf(rr.opts.Out, "  Warning: %s dispatch still failing (attempt %d), next retry in %s\n", d.Target, count, scaled)
				}
			} else {
				if count <= 2 {
					fmt.Fprintf(rr.opts.Out, "  Warning: notification failed: %v\n", err)
				}
			}
			continue
		}
		// Reset failure counter on success
		delete(rr.failureCount, key+":"+fingerprint)
		rr.dispatched[key] = fingerprint
		if rr.ledger != nil {
			if err := rr.ledger.MarkDispatched(key, fingerprint); err != nil {
				fmt.Fprintf(rr.opts.Out, "  Warning: dispatch ledger update failed: %v\n", err)
			}
		}
	}
	return nil
}

// PendingDispatches reads the router state and returns items that need dispatch.
func (rr *Runner) PendingDispatches() ([]Dispatch, error) {
	state, err := rr.router.ReadState()
	if err != nil {
		return nil, err
	}
	state.NormalizePending()
	var out []Dispatch
	seen := make(map[string]bool)
	add := func(target string, ids []string) {
		if rr.opts.Agent != "all" && rr.opts.Agent != target {
			return
		}
		for _, id := range ids {
			key := target + ":" + id
			if seen[key] {
				continue
			}
			if rr.opts.Executor != nil {
				if item := rr.opts.Executor.workQueue.Find(key); item != nil && workItemSuppressesDispatch(item.Status) {
					continue
				}
			}
			seen[key] = true
			doc, err := rr.router.Get(id)
			if err != nil {
				fmt.Fprintf(rr.opts.Out, "Skipping %s for %s: %v\n", id, target, err)
				continue
			}
			out = append(out, Dispatch{
				Target:  target,
				DocID:   id,
				Type:    doc.Type,
				Title:   doc.Title,
				ModTime: doc.ModTime,
				Size:    len(doc.Content),
			})
		}
	}
	// v3 path: read dynamic Pending map (keyed by agent_id)
	if rr.opts.Executor != nil && state.Pending != nil {
		for agentID, ids := range state.Pending {
			add(agentID, ids)
		}
	}
	// Also read legacy fields when not using v3 migration.
	if rr.opts.Executor == nil || len(state.Pending["codex-pantheon"]) == 0 {
		add("codex", state.PendingForCodex)
	}
	if rr.opts.Executor == nil || len(state.Pending["claude-pantheon"]) == 0 {
		add("claude", state.PendingForClaude)
	}
	return out, nil
}

// Fingerprint changes when the underlying router document changes.
func (d Dispatch) Fingerprint() string {
	return string(d.Type) + ":" + strconv.FormatInt(d.ModTime.UnixNano(), 10) + ":" + strconv.Itoa(d.Size)
}

func workItemSuppressesDispatch(status WorkStatus) bool {
	switch status {
	case StatusDispatched, StatusStarted, StatusWorking, StatusCompleted:
		return true
	default:
		return false
	}
}
