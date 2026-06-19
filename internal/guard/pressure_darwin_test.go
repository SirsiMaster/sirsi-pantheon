//go:build darwin && cgo

package guard

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestFanoutPressureRaceWithCancel races continuous fanoutPressure against
// continuous subscribe+cancel churn. Each cancel triggers the ctx.Done teardown,
// which removes via an in-place left-shift that mutates the backing array; two
// stable subscribers keep ≥2 elements so the shift always overwrites slots a
// concurrent fan-out may be reading. Run under `go test -race`, this reliably trips
// the aliased-slice bug claude-home and codex both flagged (copying only the slice
// header in fanoutPressure) and passes once the contents are copied under the lock
// (A21). No other test reaches this concurrency.
func TestFanoutPressureRaceWithCancel(t *testing.T) {
	// Start from a clean registry so prior darwin tests' subscribers don't leak in.
	pressureSubMu.Lock()
	pressureSubs = nil
	pressureSubMu.Unlock()

	w := NewPressureWatcher()
	// Two stable subscribers so the backing array always holds ≥2 elements.
	for i := 0; i < 2; i++ {
		if err := w.Subscribe(context.Background(), make(chan PressureEvent, 1)); err != nil {
			t.Fatalf("stable subscribe %d: %v", i, err)
		}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Continuous fan-out (the unlocked read of the subscriber slice).
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					fanoutPressure(PressureWarn)
				}
			}
		}()
	}

	// Continuous churn: subscribe then cancel → async in-place removal, concurrent
	// with the fan-out reads above.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				ctx, cancel := context.WithCancel(context.Background())
				_ = w.Subscribe(ctx, make(chan PressureEvent, 1))
				cancel()
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}
