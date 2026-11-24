//go:build windows

package main

import "errors"

type routerWatcherAncestorMonitor struct {
	changed chan struct{}
	errors  chan error
	done    chan struct{}
}

func startRouterWatcherAncestors(_ []string) (*routerWatcherAncestorMonitor, error) {
	return nil, errors.New("watch-router: custody ancestor observation is not qualified on Windows")
}
func (m *routerWatcherAncestorMonitor) revalidate() error {
	return errors.New("watch-router: custody ancestor observation unavailable")
}
func (m *routerWatcherAncestorMonitor) Close() error { return nil }
