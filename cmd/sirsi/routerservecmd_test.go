package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// A cold Cloud Run sandbox can miss the first Postgres connect; the open must be
// retried a bounded number of times and then fail with the last error.
func TestOpenWithRetry(t *testing.T) {
	calls := 0
	fail := errors.New("dial timeout")
	var log bytes.Buffer
	s, err := openWithRetry(&log, 3, 0, func() (routerstore.Store, error) {
		calls++
		if calls < 3 {
			return nil, fail
		}
		return routerstore.OpenPath(":memory:")
	})
	if err != nil || s == nil || calls != 3 {
		t.Fatalf("want success on attempt 3: err=%v calls=%d", err, calls)
	}
	_ = s.Close()
	if strings.Count(log.String(), "retrying") != 2 {
		t.Fatalf("want 2 retry lines, got:\n%s", log.String())
	}

	calls = 0
	if _, err := openWithRetry(&log, 2, 0, func() (routerstore.Store, error) { calls++; return nil, fail }); !errors.Is(err, fail) || calls != 2 {
		t.Fatalf("want last error after 2 attempts: err=%v calls=%d", err, calls)
	}
}

func TestMigrateMarkerAllowed(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	ok := "migrate-store " + now.Add(-5*time.Minute).Format(time.RFC3339) + "\n"
	if err := migrateMarkerAllowed(ok, now); err != nil {
		t.Fatalf("own fresh marker must be accepted: %v", err)
	}
	for name, c := range map[string]string{
		"operator quarantine": "quarantine set by owner 2026-08-06\n",
		"stale":               "migrate-store " + now.Add(-2*time.Hour).Format(time.RFC3339),
		"future":              "migrate-store " + now.Add(time.Minute).Format(time.RFC3339),
		"garbage timestamp":   "migrate-store yesterday",
		"empty":               "",
	} {
		if err := migrateMarkerAllowed(c, now); err == nil {
			t.Fatalf("%s marker must be refused", name)
		}
	}
}
