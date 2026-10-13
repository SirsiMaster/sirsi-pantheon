package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDashboardReadSharesInflightWithoutCaching(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	want := errors.New("relay unavailable")
	read := shareDashboardRead(func() (int32, error) {
		n := calls.Add(1)
		if n == 1 {
			close(started)
			<-release
			return n, want
		}
		return n, nil
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := read()
		if n != 1 || err != want {
			t.Errorf("first: %d %v", n, err)
		}
	}()
	<-started
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := read()
			if n != 1 || err != want {
				t.Errorf("shared: %d %v", n, err)
			}
		}()
	}
	// The collector remains blocked while the concurrent callers join it.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate collections: %d", calls.Load())
	}
	if n, err := read(); n != 2 || err != nil {
		t.Fatalf("failed read was cached: %d %v", n, err)
	}
}
