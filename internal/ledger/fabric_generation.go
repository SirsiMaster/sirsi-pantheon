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
	return FabricGenerationView{ID: d.ID, Read: d.Read, Board: d.Board, Facts: d.Facts, Events: d.Events, Completed: d.Completed}, nil
}

// FabricGenerationProducer serializes acquisition and real transition
// observation. Existing request/poll owners call Read; it starts no goroutine,
// timer, listener or global cache. Failed/cancelled reads never advance history.
type FabricGenerationProducer struct {
	mu              sync.Mutex
	read            func() (FabricRead, error)
	build, instance string
	sequence        uint64
	previous        map[string]string
	events          []FabricTransition
	completed       int
	seeded          bool
}

func NewFabricGenerationProducer(read func() (FabricRead, error), build string) *FabricGenerationProducer {
	return &FabricGenerationProducer{read: read, build: build, previous: map[string]string{}, events: []FabricTransition{}}
}

func (p *FabricGenerationProducer) Read(ctx context.Context) (*FabricGeneration, error) {
	return p.readGeneration(ctx, nil)
}

func (p *FabricGenerationProducer) ReadPage(ctx context.Context, offset int, snapshot string) (*FabricGeneration, error) {
	return p.readGeneration(ctx, func(read FabricRead) error { _, err := FabricTasksFrom(read.Snapshot, offset, snapshot); return err })
}

func (p *FabricGenerationProducer) readGeneration(ctx context.Context, validate func(FabricRead) error) (*FabricGeneration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.read == nil {
		return nil, errors.New("fabric reader unavailable")
	}
	input, err := p.read()
	if err != nil {
		return nil, err
	}
	// Detach all maps/slices/pointers before adapters or history see the input.
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var read FabricRead
	if err = json.Unmarshal(raw, &read); err != nil {
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
	if p.instance == "" {
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		p.instance = hex.EncodeToString(nonce[:]) // trace identity, never authority
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
	d := fabricGenerationData{ID: p.instance + ":" + hexSequence(p.sequence+1), Read: read, Board: board, Facts: facts, Events: events, Completed: completed}
	sealed, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	p.previous, p.events, p.completed, p.seeded = next, events, completed, true
	p.sequence++
	return &FabricGeneration{sealed: sealed}, nil
}

func hexSequence(n uint64) string {
	var bytes [8]byte
	for i := 7; i >= 0; i-- {
		bytes[i] = byte(n)
		n >>= 8
	}
	return hex.EncodeToString(bytes[:])
}
