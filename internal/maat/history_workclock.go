package maat

import (
	"context"
	"time"
)

type historyWorkClockKey struct{}

type historyWorkClock struct {
	started    time.Time
	budget     time.Duration
	excluded   time.Duration
	pausedAt   time.Time
	pauseDepth int
}

func (clock *historyWorkClock) expired() bool {
	if clock == nil {
		return false
	}
	elapsed := time.Since(clock.started) - clock.excluded
	if !clock.pausedAt.IsZero() {
		elapsed -= time.Since(clock.pausedAt)
	}
	return elapsed >= clock.budget
}

func (clock *historyWorkClock) pause() {
	if clock == nil {
		return
	}
	if clock.pauseDepth == 0 {
		clock.pausedAt = time.Now()
	}
	clock.pauseDepth++
}

func (clock *historyWorkClock) resume() {
	if clock != nil && clock.pauseDepth > 0 {
		clock.pauseDepth--
	}
	if clock != nil && clock.pauseDepth == 0 && !clock.pausedAt.IsZero() {
		clock.excluded += time.Since(clock.pausedAt)
		clock.pausedAt = time.Time{}
	}
}

func historyWorkExpired(ctx context.Context, fallback time.Time) bool {
	if clock, ok := ctx.Value(historyWorkClockKey{}).(*historyWorkClock); ok {
		return clock.expired()
	}
	return !time.Now().Before(fallback)
}
