package routerstore

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func observationInt(n int) *int { return &n }
func observationFixture(now time.Time) RelayOutboxObservation {
	return RelayOutboxObservation{Version: 1, RootDevice: 9, RootInode: 10, ProducerUID: 501,
		StartedAt: now.Add(-time.Millisecond), ObservedAt: now, Enumerated: true, Complete: true, Queued: observationInt(2),
		Lanes: []RelayLaneObservation{{Agent: "codex-pantheon", State: "observed", Queued: observationInt(2)}, {Agent: "quiet", State: "observed", Queued: observationInt(0)}, {Agent: "absent", State: "absent"}}}
}

func TestRelayObservationCoverageAndIdentity(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*RelayOutboxObservation)
		bad    bool
	}{
		{"complete", func(s *RelayOutboxObservation) {}, false},
		{"partial-keeps-eligible-lane", func(s *RelayOutboxObservation) {
			s.Lanes[1] = RelayLaneObservation{Agent: "quiet", State: "unavailable", ErrorCode: "unavailable"}
			s.Complete = false
			s.Queued = nil
		}, false},
		{"incomplete-enumeration", func(s *RelayOutboxObservation) {
			s.Enumerated = false
			s.ErrorCode = "limit"
			s.Complete = false
			s.Queued = nil
		}, false},
		{"partial-must-not-sum", func(s *RelayOutboxObservation) {
			s.Lanes[1] = RelayLaneObservation{Agent: "quiet", State: "unavailable", ErrorCode: "unavailable"}
			s.Complete = false
		}, true},
		{"zero-is-not-missing", func(s *RelayOutboxObservation) { s.Lanes[1].Queued = nil }, true},
		{"unavailable-is-not-zero", func(s *RelayOutboxObservation) {
			s.Lanes[1].State = "unavailable"
			s.Lanes[1].ErrorCode = "unavailable"
		}, true},
		{"wrong-total", func(s *RelayOutboxObservation) { s.Queued = observationInt(1) }, true},
		{"wrong-owner", func(s *RelayOutboxObservation) { s.ProducerUID++ }, true},
		{"wrong-root", func(s *RelayOutboxObservation) { s.RootInode++ }, true},
		{"duplicate-lane", func(s *RelayOutboxObservation) { s.Lanes[1].Agent = s.Lanes[0].Agent }, true},
		{"negative-count", func(s *RelayOutboxObservation) { s.Lanes[0].Queued = observationInt(-1) }, true},
		{"future", func(s *RelayOutboxObservation) { s.StartedAt = now.Add(3 * time.Second); s.ObservedAt = s.StartedAt }, true},
		{"stale-inside-new-board", func(s *RelayOutboxObservation) { s.StartedAt = now.Add(-16 * time.Second); s.ObservedAt = s.StartedAt }, true},
		{"long-scan", func(s *RelayOutboxObservation) { s.StartedAt = now.Add(-2 * time.Second) }, true},
		{"private-error-not-permitted", func(s *RelayOutboxObservation) { s.ErrorCode = "permission denied /private/body.json" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := observationFixture(now)
			tc.change(&s)
			err := s.validate(now, 9, 10, 501)
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v bad=%v", err, tc.bad)
			}
		})
	}
}

func TestRelayObservationClosedJSON(t *testing.T) {
	now := time.Now().UTC()
	raw, err := json.Marshal(observationFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	valid := string(raw)
	for _, tc := range []struct {
		name, raw string
		bad       bool
	}{
		{"valid", valid, false},
		{"duplicate-root", strings.Replace(valid, `"version":1`, `"version":1,"version":1`, 1), true},
		{"case-alias-root", strings.Replace(valid, `"version":1`, `"version":1,"Version":1`, 1), true},
		{"case-alias-lane", strings.Replace(valid, `"agent":"codex-pantheon"`, `"agent":"codex-pantheon","Agent":"other"`, 1), true},
		{"duplicate-lane-key", strings.Replace(valid, `"state":"observed"`, `"state":"observed","state":"observed"`, 1), true},
		{"unknown-key", strings.Replace(valid, `"version":1`, `"version":1,"payload":"forbidden"`, 1), true},
		{"null-coverage", strings.Replace(valid, `"enumerated":true`, `"enumerated":null`, 1), true},
		{"missing-coverage", strings.Replace(valid, `"enumerated":true,`, "", 1), true},
		{"trailing", valid + ` {}`, true},
		{"oversize", strings.Repeat(" ", relayObservationBytes+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeRelayObservation([]byte(tc.raw))
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v bad=%v", err, tc.bad)
			}
		})
	}
}
