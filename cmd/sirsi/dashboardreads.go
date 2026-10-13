package main

import (
	"errors"
	"sync"
)

// shareDashboardRead collapses simultaneous browser/embedding reads into one
// canonical collection. It does not cache completed results or conceal errors.
// A client timeout must not start another expensive collection while the first
// is still queued at the relay.
func shareDashboardRead[T any](collect func() (T, error)) func() (T, error) {
	type flight struct {
		done  chan struct{}
		value T
		err   error
	}
	var mu sync.Mutex
	var running *flight
	return func() (T, error) {
		mu.Lock()
		if f := running; f != nil {
			mu.Unlock()
			<-f.done
			return f.value, f.err
		}
		f := &flight{done: make(chan struct{}), err: errors.New("dashboard collection aborted")}
		running = f
		mu.Unlock()
		defer func() {
			mu.Lock()
			running = nil
			close(f.done)
			mu.Unlock()
		}()
		f.value, f.err = collect()
		return f.value, f.err
	}
}
