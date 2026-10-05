package routerboard

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunRejectsNonPositivePollIntervalBeforePolling(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			board := New("/bin/false", "", "test-build")
			err := board.Run(context.Background(), interval)
			if err == nil || !strings.Contains(err.Error(), "poll interval must be positive") {
				t.Fatalf("Run(%s) error = %v, want positive-interval validation", interval, err)
			}
			if body, version := board.Snapshot(); len(body) != 0 || version != 0 {
				t.Fatalf("invalid interval started polling: version=%d body=%s", version, body)
			}
		})
	}
}

func TestRunWithCanceledContextDoesNotStartPoll(t *testing.T) {
	board := New("/bin/false", "", "test-build")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := board.Run(ctx, time.Second); err != nil {
		t.Fatalf("Run with canceled context: %v", err)
	}
	if body, version := board.Snapshot(); len(body) != 0 || version != 0 {
		t.Fatalf("canceled Run started polling: version=%d body=%s", version, body)
	}
}
