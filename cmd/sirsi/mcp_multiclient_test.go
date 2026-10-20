package main

import (
	"sync"
	"testing"
)

type blockingMCPServer struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (s blockingMCPServer) Run() error {
	s.started <- struct{}{}
	<-s.release
	return nil
}

func TestRunMCPServerAllowsIndependentClients(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runMCPServer(blockingMCPServer{started: started, release: release}); err != nil {
				t.Errorf("runMCPServer() error: %v", err)
			}
		}()
	}

	// Both transports must enter Run before either is allowed to complete.
	// A process-global singleton would make this second receive impossible.
	<-started
	<-started
	close(release)
	wg.Wait()
}
