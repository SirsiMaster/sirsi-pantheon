package ledger

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type FabricStage string

const (
	FabricStageSnapshot              FabricStage = "snapshot"
	FabricStageNodeStatus            FabricStage = "node_status"
	FabricStageOperationalAgents     FabricStage = "operational_agents"
	FabricStageRuntimeClassification FabricStage = "runtime_classification"
	FabricStageHostHealth            FabricStage = "host_health"
	FabricStageProjection            FabricStage = "projection"
)

type fabricStageTraceKey struct{}

type fabricStageTrace struct {
	mu      sync.Mutex
	stage   FabricStage
	started time.Time
	elapsed time.Duration
}

// FabricStageError identifies the stage active when cancellation or a deadline
// was observed. Elapsed is time since that stage began, not its full budget.
type FabricStageError struct {
	Stage   FabricStage
	Elapsed time.Duration
	Cause   error
}

func (e *FabricStageError) Error() string {
	observation := "cancellation"
	if errors.Is(e.Cause, context.DeadlineExceeded) {
		observation = "deadline"
	}
	return fmt.Sprintf("fabric %s observed during stage %s after %s", observation, e.Stage, e.Elapsed.Round(time.Millisecond))
}

func (e *FabricStageError) Unwrap() error { return e.Cause }

func (t *fabricStageTrace) begin(stage FabricStage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stage != "" {
		t.elapsed = time.Since(t.started)
	}
	t.stage = stage
	t.started = time.Now()
}

func (t *fabricStageTrace) snapshot() (FabricStage, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	elapsed := t.elapsed
	if t.stage != "" {
		elapsed = time.Since(t.started)
	}
	return t.stage, elapsed
}

func (t *fabricStageTrace) wrap(err error) error {
	if err == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
		return err
	}
	var attributed *FabricStageError
	if errors.As(err, &attributed) {
		return err
	}
	stage, elapsed := t.snapshot()
	if stage == "" {
		return err
	}
	return &FabricStageError{Stage: stage, Elapsed: elapsed, Cause: err}
}

func (t *fabricStageTrace) finalize(err, contextErr error) error {
	if contextErr == nil {
		return t.wrap(err)
	}
	var attributed *FabricStageError
	if errors.As(err, &attributed) {
		if errors.Is(err, contextErr) {
			return err
		}
		return &FabricStageError{
			Stage:   attributed.Stage,
			Elapsed: attributed.Elapsed,
			Cause:   errors.Join(contextErr, err),
		}
	}
	if err != nil && !errors.Is(err, contextErr) {
		contextErr = errors.Join(contextErr, err)
	}
	return t.wrap(contextErr)
}

// FabricReadStage marks a fixed, low-cardinality acquisition stage. The trace
// is private to the coalesced producer operation and contains no read payload.
func FabricReadStage(ctx context.Context, stage FabricStage, read func() error) error {
	switch stage {
	case FabricStageSnapshot, FabricStageNodeStatus, FabricStageOperationalAgents, FabricStageRuntimeClassification, FabricStageHostHealth, FabricStageProjection:
	default:
		return errors.New("fabric stage invalid")
	}
	trace, _ := ctx.Value(fabricStageTraceKey{}).(*fabricStageTrace)
	if trace == nil {
		trace = &fabricStageTrace{}
	}
	trace.begin(stage)
	err := read()
	return trace.wrap(err)
}
