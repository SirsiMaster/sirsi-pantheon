//go:build !darwin && !windows

package main

import "errors"

// A path-based watch cannot prove it subscribed to the retained descriptor.
// Refuse cooperative watcher startup until this platform has a bound monitor.
type routerWatcherAncestorMonitor struct {
	changed chan struct{}
	errors  chan error
	done    chan struct{}
}

func startRouterWatcherAncestors(_ []string) (*routerWatcherAncestorMonitor, error) {
	return nil, errors.New("watch-router: descriptor-bound custody observation is not qualified on this platform")
}
func startRouterWatcherAncestorsWith(_ []string, _, _ func(string) error) (*routerWatcherAncestorMonitor, error) {
	return startRouterWatcherAncestors(nil)
}
func (m *routerWatcherAncestorMonitor) revalidate() error {
	return errors.New("watch-router: custody ancestor observation unavailable")
}
func (m *routerWatcherAncestorMonitor) Close() error { return nil }
