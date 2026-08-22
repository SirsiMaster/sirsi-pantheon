package main

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestResidentScanHydratesManifestOnly(t *testing.T) {
	oldRefresh := refreshFromLatestFn
	t.Cleanup(func() {
		refreshFromLatestFn = oldRefresh
	})

	var refreshes atomic.Int32
	refreshFromLatestFn = func() { refreshes.Add(1) }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startPeriodicScan(ctx)

	if refreshes.Load() != 1 {
		t.Fatalf("startup must load exactly one persisted result, got %d", refreshes.Load())
	}
}
