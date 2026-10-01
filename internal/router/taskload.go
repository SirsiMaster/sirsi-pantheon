package router

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

// TaskLoad is the ledger-task half of a lane's dispatch trigger. A wake loop that
// counts only inbox items starts nobody for work that lives on the ledger — which
// is where the router now places work (2026-10-01: 22 requests moved to tasks and
// the quiet lanes behind them never woke).
type TaskLoad struct {
	// Dispatchable tasks are claimable AND the lane's own to do: they count toward
	// starting a worker. Owner-assigned, other-party, blocked and leased tasks do not.
	Dispatchable int
	Leased       int
	Actionable   int
}

// LaneTaskLoad reads agent's task load from the durable store through the shared
// RunnableFor predicate (never a retyped subset). It is zero when the host is not
// cut over to the store (legacy file mode has no durable ledger), so every
// store-less path keeps its previous behaviour.
func LaneTaskLoad(routerRoot, agent string) (TaskLoad, error) {
	if !routercfg.StoreWake() {
		return TaskLoad{}, nil
	}
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		return TaskLoad{}, fmt.Errorf("router: task load: %w", err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Store().RunnableFor(agent)
	if err != nil {
		return TaskLoad{}, fmt.Errorf("router: task load for %s: %w", agent, err)
	}
	return TaskLoad{Dispatchable: st.DispatchableLedgerTasks, Leased: st.LeasedLedgerTasks, Actionable: st.ActionableLedgerTasks}, nil
}

// dispatchDepth is the number a wake loop compares against zero to decide whether
// to start a worker, and against the previous value to score progress.
func dispatchDepth(items int, tl TaskLoad) int { return items + tl.Dispatchable }

// taskMark folds the task counts into the progress fingerprint. A consumer that
// claims a task (dispatchable→leased) or finishes one (actionable drops) has
// taken a durable action even though no inbox item changed, so it must not be
// judged stalled and killed by the 30-minute stall gate.
func taskMark(tl TaskLoad) string {
	return fmt.Sprintf("\ntasks:%d/%d/%d", tl.Dispatchable, tl.Leased, tl.Actionable)
}
