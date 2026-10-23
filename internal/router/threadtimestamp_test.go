package router

import (
	"strings"
	"testing"
	"time"
)

func TestThreadTimestampUTCExactWidthAndOrdering(t *testing.T) {
	base := time.Date(2026, 8, 6, 9, 31, 0, 0, time.UTC)
	inputs := []struct {
		name string
		at   time.Time
		want string
	}{
		{"zero-time", time.Time{}, "0001-01-01T00:00:00.000000000Z"},
		{"zero-fraction", base, "2026-08-06T09:31:00.000000000Z"},
		{"one-nanosecond", base.Add(time.Nanosecond), "2026-08-06T09:31:00.000000001Z"},
		{"half-second", base.Add(500 * time.Millisecond), "2026-08-06T09:31:00.500000000Z"},
		{"last-nanosecond", base.Add(time.Second - time.Nanosecond), "2026-08-06T09:31:00.999999999Z"},
		{"next-second", base.Add(time.Second), "2026-08-06T09:31:01.000000000Z"},
		{"positive-offset", base.In(time.FixedZone("east", 5*3600+1800)), "2026-08-06T09:31:00.000000000Z"},
		{"negative-offset", base.Add(500 * time.Millisecond).In(time.FixedZone("west", -7*3600)), "2026-08-06T09:31:00.500000000Z"},
		{"offset-day-boundary", time.Date(2026, 8, 7, 0, 0, 0, 123, time.FixedZone("east", 14*3600)), "2026-08-06T10:00:00.000000123Z"},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			got := threadTimestamp(input.at)
			if got != input.want || len(got) != 30 {
				t.Fatalf("timestamp=%q (%d bytes), want %q (30 bytes)", got, len(got), input.want)
			}
			parsed, err := time.Parse(time.RFC3339Nano, got)
			if err != nil || !parsed.Equal(input.at) {
				t.Fatalf("timestamp changed instant: parsed=%v err=%v input=%v", parsed, err, input.at)
			}
		})
	}
	// Compare every pair, including equal instants expressed in different zones.
	// Lexical order must equal chronological order, not merely one chosen pair.
	for _, a := range inputs {
		for _, b := range inputs {
			want := 0
			if a.at.Before(b.at) {
				want = -1
			} else if a.at.After(b.at) {
				want = 1
			}
			if got := strings.Compare(threadTimestamp(a.at), threadTimestamp(b.at)); got != want {
				t.Errorf("%s vs %s: lexical order=%d, chronological order=%d", a.name, b.name, got, want)
			}
		}
	}
}

func TestThreadRecordsUsesCanonicalTimestamp(t *testing.T) {
	seen := time.Date(2026, 8, 6, 23, 45, 0, 0, time.FixedZone("west", -7*3600))
	records, err := threadRecords(&ThreadRegistry{Threads: map[string]*Thread{
		"fixture": {ThreadID: "fixture", AgentID: "fixture", Host: "fixture-host", Status: ThreadStatusActive, LastSeenAt: seen},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].LastSeenAt != "2026-08-07T06:45:00.000000000Z" {
		t.Fatalf("thread record bypassed UTC fixed-width encoding: %+v", records)
	}
}
