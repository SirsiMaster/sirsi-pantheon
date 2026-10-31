package ledger

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

const fabricClassificationWorkers = 4

type fabricLaneClassifier func(context.Context, string, bool, time.Duration) (routerstore.LaneOperationalState, error)

type fabricLaneClassification struct {
	state routerstore.LaneOperationalState
	err   error
}

// classifyFabricAgents runs only the existing passive lane reads concurrently.
// Results and errors are consumed in input order, and no partial map escapes.
func classifyFabricAgents(ctx context.Context, agents []Agent, unavailable map[string]bool, classify fabricLaneClassifier) (map[string]routerstore.LaneOperationalState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return map[string]routerstore.LaneOperationalState{}, nil
	}
	workers := min(fabricClassificationWorkers, len(agents))
	jobs := make(chan int)
	results := make([]fabricLaneClassification, len(agents))
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					continue
				}
				agent := agents[index]
				blocked, declared := unavailable[agent.AgentID]
				results[index].state, results[index].err = classify(ctx, agent.AgentID, declared && !blocked, 5*time.Minute)
			}
		}()
	}
	for index := range agents {
		if ctx.Err() != nil {
			break
		}
		select {
		case jobs <- index:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		for _, result := range results {
			if result.err != nil {
				return nil, errors.Join(err, fmt.Errorf("fabric runtime unavailable: %w", result.err))
			}
		}
		return nil, err
	}
	out := make(map[string]routerstore.LaneOperationalState, len(agents))
	for index, agent := range agents {
		result := results[index]
		if result.err != nil {
			return nil, fmt.Errorf("fabric runtime unavailable: %w", result.err)
		}
		if result.state.Agent != agent.AgentID {
			return nil, fmt.Errorf("fabric runtime unavailable: classifier result missing agent %s", agent.AgentID)
		}
		out[agent.AgentID] = result.state
	}
	return out, nil
}
