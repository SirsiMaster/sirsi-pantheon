package ledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Only metadata already needed by the legacy reader is captured. Registry
// commands/environment/credentials are never included, even in this private frame.
type FabricLaneMetadata struct {
	Type, WakeMechanism string
}

type FabricThreadMetadata struct {
	ID, Agent, Workstream string
	IdleSeconds           *float64
	Stale                 bool
}

type FabricMetadata struct {
	Lanes   map[string]FabricLaneMetadata
	Threads []FabricThreadMetadata
	Errors  []string // closed observation error codes, not private error text
}

type FabricTransition struct {
	At, Agent, TaskID, Subject, From, To string
}

type fabricGenerationData struct {
	ID        string
	Read      FabricRead
	Board     FabricBoard
	Facts     FabricFacts
	Events    []FabricTransition
	Completed int
}

// FabricGeneration is an immutable, in-process observation, not a store, public
// raw-snapshot endpoint or permission. Its sealed bytes are never exposed.
type FabricGeneration struct{ sealed []byte }

type FabricGenerationReader func(context.Context) (*FabricGeneration, error)

type FabricGenerationSource interface {
	Read(context.Context) (*FabricGeneration, error)
	ReadPage(context.Context, int, string) (*FabricGeneration, error)
}

var ErrFabricPageRejected = errors.New("fabric page selection rejected")

// View returns a fresh owned copy. Accidental JSON serialization of this private
// adapter input emits no raw task/registry data; public handlers select DTOs.
type FabricGenerationView struct {
	ID        string             `json:"-"`
	Read      FabricRead         `json:"-"`
	Board     FabricBoard        `json:"-"`
	Facts     FabricFacts        `json:"-"`
	Events    []FabricTransition `json:"-"`
	Completed int                `json:"-"`
}

func (g *FabricGeneration) View() (FabricGenerationView, error) {
	if g == nil {
		return FabricGenerationView{}, errors.New("fabric generation unavailable")
	}
	var d fabricGenerationData
	if err := json.Unmarshal(g.sealed, &d); err != nil {
		return FabricGenerationView{}, err
	}
	return FabricGenerationView(d), nil
}

// FabricGenerationProducer owns one active acquisition and one history commit.
// Only overlapping unfiltered readers share it. Completed observations are never
// cached for later requests, and abandoned work retains the slot until it exits.
const fabricAcquisitionCeiling = 15 * time.Second

type fabricWaiter struct{ ctx context.Context }
type fabricOperation struct {
	ctx                   context.Context
	cancel                context.CancelFunc
	done                  chan struct{}
	waiters               map[*fabricWaiter]struct{}
	trace                 *fabricStageTrace
	unfiltered, abandoned bool
	generation            *FabricGeneration
	err                   error
}
type fabricCandidate struct {
	generation *FabricGeneration
	previous   map[string]string
	events     []FabricTransition
	instance   string
	completed  int
}
type FabricGenerationProducer struct {
	mu              sync.Mutex
	parent          context.Context
	stop            context.CancelFunc
	active          *fabricOperation
	read            func(context.Context) (FabricRead, error)
	build, instance string
	sequence        uint64
	previous        map[string]string
	events          []FabricTransition
	completed       int
	seeded          bool
}

// Legacy readers keep their original callback. Their interior work remains
// context-less: cancellation discards the result and never frees its slot early.
func NewFabricGenerationProducer(read func() (FabricRead, error), build string) *FabricGenerationProducer {
	var contextual func(context.Context) (FabricRead, error)
	if read != nil {
		contextual = func(context.Context) (FabricRead, error) { return read() }
	}
	return NewFabricGenerationProducerContext(context.Background(), contextual, build)
}

func NewFabricGenerationProducerContext(parent context.Context, read func(context.Context) (FabricRead, error), build string) *FabricGenerationProducer {
	ctx, stop := context.WithCancel(parent)
	return &FabricGenerationProducer{parent: ctx, stop: stop, read: read, build: build, previous: map[string]string{}, events: []FabricTransition{}}
}

// Close cancels acquisition on shutdown. It does not claim a remote spool/SQL
// operation has been retracted or joined; the active slot stays until actual exit.
func (p *FabricGenerationProducer) Close() { p.stop() }

func liveFabricWaiters(op *fabricOperation) int {
	n := 0
	for w := range op.waiters {
		if w.ctx.Err() == nil {
			n++
		}
	}
	return n
}

func (p *FabricGenerationProducer) Read(ctx context.Context) (*FabricGeneration, error) {
	return p.readGeneration(ctx, nil)
}

func (p *FabricGenerationProducer) ReadPage(ctx context.Context, offset int, snapshot string) (*FabricGeneration, error) {
	return p.readGeneration(ctx, func(read FabricRead) error { _, err := FabricTasksFrom(read.Snapshot, offset, snapshot); return err })
}

// ReadTaskSelection validates before committing a new transition baseline, just
// like unfiltered ReadPage. Rejected filtered cursors cannot swallow activity.
func (p *FabricGenerationProducer) ReadTaskSelection(ctx context.Context, offset int, snapshot, filter string) (*FabricGeneration, error) {
	return p.readGeneration(ctx, func(read FabricRead) error {
		_, err := FabricTasksFilteredFrom(read.Snapshot, offset, snapshot, filter)
		return err
	})
}

func (p *FabricGenerationProducer) readGeneration(ctx context.Context, validate func(FabricRead) error) (*FabricGeneration, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.mu.Lock()
		if err := p.parent.Err(); err != nil {
			p.mu.Unlock()
			return nil, err
		}
		op := p.active
		if op != nil && (liveFabricWaiters(op) == 0 || op.ctx.Err() != nil) {
			op.abandoned = true
			op.cancel()
		}
		if op != nil && (validate != nil || !op.unfiltered || op.abandoned) {
			done := op.done
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-p.parent.Done():
				return nil, p.parent.Err()
			case <-done:
				continue
			}
		}
		w := &fabricWaiter{ctx: ctx}
		if op == nil {
			acquisition, cancel := context.WithTimeout(p.parent, fabricAcquisitionCeiling)
			trace := &fabricStageTrace{}
			acquisition = context.WithValue(acquisition, fabricStageTraceKey{}, trace)
			op = &fabricOperation{ctx: acquisition, cancel: cancel, done: make(chan struct{}), waiters: map[*fabricWaiter]struct{}{}, trace: trace, unfiltered: validate == nil}
			p.active = op
			op.waiters[w] = struct{}{}
			go p.runFabricOperation(op, validate)
		} else {
			op.waiters[w] = struct{}{}
		}
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.leaveFabricWaiter(op, w)
			return nil, op.trace.wrap(ctx.Err())
		case <-op.ctx.Done():
			// Prefer the sealed result when completion released this context.
			select {
			case <-op.done:
				if err := ctx.Err(); err != nil {
					return nil, op.trace.wrap(err)
				}
				return op.generation, op.err
			default:
			}
			p.leaveFabricWaiter(op, w)
			return nil, op.trace.wrap(op.ctx.Err())
		case <-op.done:
			if err := ctx.Err(); err != nil {
				return nil, op.trace.wrap(err)
			}
			return op.generation, op.err
		}
	}
}

func (p *FabricGenerationProducer) leaveFabricWaiter(op *fabricOperation, w *fabricWaiter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active != op {
		return
	}
	delete(op.waiters, w)
	if liveFabricWaiters(op) == 0 || op.ctx.Err() != nil {
		op.abandoned = true
		op.cancel()
	}
}

func (p *FabricGenerationProducer) runFabricOperation(op *fabricOperation, validate func(FabricRead) error) {
	candidate, err := p.buildFabricCandidate(op.ctx, validate)
	p.mu.Lock()
	defer p.mu.Unlock()
	// Cancellation and successful commit are linearized at this lock boundary.
	contextErr := op.ctx.Err()
	if p.active != op || op.abandoned || liveFabricWaiters(op) == 0 {
		if contextErr == nil {
			contextErr = context.Canceled
		}
	}
	err = op.trace.finalize(err, contextErr)
	if err == nil {
		p.previous, p.events, p.completed, p.seeded = candidate.previous, candidate.events, candidate.completed, true
		p.instance = candidate.instance
		p.sequence++
		op.generation = candidate.generation
	}
	op.err = err
	p.active = nil
	close(op.done)
	op.cancel()
}

// The sole active operation reads the last committed baseline. It stages every
// change locally; only runFabricOperation publishes after validation/live demand.
func (p *FabricGenerationProducer) buildFabricCandidate(ctx context.Context, validate func(FabricRead) error) (*fabricCandidate, error) {
	var input FabricRead
	err := FabricReadStage(ctx, FabricStageSnapshot, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.read == nil {
			return errors.New("fabric reader unavailable")
		}
		var err error
		input, err = p.read(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	// Detach all maps/slices/pointers before adapters or history see the input.
	var read FabricRead
	err = FabricReadStage(ctx, FabricStageProjection, func() error {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &read)
	})
	if err != nil {
		return nil, err
	}
	// Preserve the old registry/ledger union once, before every projection.
	// Registry-only services have no fabricated tasks or worker classification.
	seen := map[string]bool{}
	for _, agent := range read.Snapshot.Agents {
		if !fabricID(agent.AgentID) || seen[agent.AgentID] {
			return nil, errors.New("fabric lane identity invalid or duplicated")
		}
		seen[agent.AgentID] = true
	}
	registered := map[string]bool{}
	for id := range read.Metadata.Lanes {
		registered[id] = true
	}
	if read.Node != nil {
		for _, id := range read.Node.RegisteredAgents {
			registered[id] = true
		}
	}
	for id := range registered {
		if !fabricID(id) {
			return nil, errors.New("fabric registry lane identity invalid")
		}
		if !seen[id] {
			read.Snapshot.Agents = append(read.Snapshot.Agents, Agent{AgentID: id, Items: []Item{}, Tasks: []routerstore.Task{}})
			seen[id] = true
		}
	}
	read.Snapshot.Agents = read.Snapshot.OrderedAgents()
	if validate != nil {
		if err = validate(read); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFabricPageRejected, err)
		}
	}
	at, err := time.Parse(time.RFC3339Nano, read.Snapshot.GeneratedAt)
	if err != nil {
		return nil, errors.New("fabric observation time unavailable")
	}
	instance := p.instance
	if instance == "" {
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		instance = hex.EncodeToString(nonce[:]) // trace identity, never authority
	}
	next := map[string]string{}
	events := append([]FabricTransition{}, p.events...)
	completed := p.completed
	for _, lane := range read.Snapshot.OrderedAgents() {
		for _, task := range lane.Tasks {
			key := lane.AgentID + "\x00" + task.TaskID
			next[key] = task.Status
			if before, known := p.previous[key]; known && before != task.Status {
				events = append([]FabricTransition{{At: at.UTC().Format(time.RFC3339Nano), Agent: lane.AgentID, TaskID: task.TaskID, Subject: task.Subject, From: before, To: task.Status}}, events...)
				if task.Status == "done" {
					completed++
				}
			}
		}
	}
	if len(events) > 200 {
		events = events[:200]
	}
	read.Evidence.Activity = []FabricActivity{}
	read.Evidence.ActivitySeeded = p.seeded
	for _, event := range events {
		read.Evidence.Activity = append(read.Evidence.Activity, FabricActivity{At: event.At, Agent: event.Agent, TaskID: event.TaskID, From: event.From, To: event.To})
	}
	facts := fabricFacts(read.Snapshot)
	board := fabricFromFacts(read.Snapshot, read.Node, p.build, read.Evidence, facts)
	d := fabricGenerationData{ID: instance + ":" + hexSequence(p.sequence+1), Read: read, Board: board, Facts: facts, Events: events, Completed: completed}
	sealed, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &fabricCandidate{generation: &FabricGeneration{sealed: sealed}, previous: next, events: events, completed: completed, instance: instance}, nil
}

func hexSequence(n uint64) string {
	var bytes [8]byte
	for i := 7; i >= 0; i-- {
		bytes[i] = byte(n)
		n >>= 8
	}
	return hex.EncodeToString(bytes[:])
}
