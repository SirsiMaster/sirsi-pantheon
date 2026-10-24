package routerstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const relayObservationName = ".relay-outbox-health-v1.json"
const relayObservationBytes = 131072
const relayObservationMaxAge = 15 * time.Second

// RelayOutboxObservation contains names/counts only, never request IDs or
// contents. Complete describes enumeration AND every lane, not a partial sum.
// These are cooperative interval observations, not queue transaction proofs.
type RelayOutboxObservation struct {
	Version     int                    `json:"version"`
	RootDevice  uint64                 `json:"root_device"`
	RootInode   uint64                 `json:"root_inode"`
	ProducerUID uint32                 `json:"producer_uid"`
	StartedAt   time.Time              `json:"started_at"`
	ObservedAt  time.Time              `json:"observed_at"`
	Enumerated  bool                   `json:"enumerated"`
	Complete    bool                   `json:"complete"`
	Queued      *int                   `json:"queued,omitempty"`
	ErrorCode   string                 `json:"error_code,omitempty"`
	Lanes       []RelayLaneObservation `json:"lanes"`
}

type RelayLaneObservation struct {
	Agent     string `json:"agent"`
	State     string `json:"state"`            // observed | unavailable | absent
	Queued    *int   `json:"queued,omitempty"` // explicit zero != missing
	ErrorCode string `json:"error_code,omitempty"`
}

func relayObservationError(code string) error { return errors.New("relay observation: " + code) }

func validRelayObservationCode(code string) bool {
	switch code {
	case "", "unavailable", "invalid_layout", "changed", "limit":
		return true
	}
	return false
}

// Validate binds a document to an independently opened/validated root policy.
// Never derive expectedUID from this document or from group membership.
func (s RelayOutboxObservation) validate(now time.Time, dev, ino uint64, expectedUID uint32) error {
	if s.Version != 1 || s.RootDevice != dev || s.RootInode != ino || s.ProducerUID != expectedUID {
		return relayObservationError("identity")
	}
	if s.StartedAt.IsZero() || s.ObservedAt.Before(s.StartedAt) || s.ObservedAt.Sub(s.StartedAt) > time.Second ||
		s.ObservedAt.After(now.Add(2*time.Second)) || s.StartedAt.After(now.Add(2*time.Second)) || now.Sub(s.StartedAt) > relayObservationMaxAge {
		return relayObservationError("stale_or_future")
	}
	if len(s.Lanes) > 256 || !validRelayObservationCode(s.ErrorCode) || (s.Enumerated && s.ErrorCode != "") || (!s.Enumerated && s.ErrorCode == "") {
		return relayObservationError("coverage")
	}
	seen, total, complete := map[string]bool{}, 0, s.Enumerated
	for _, lane := range s.Lanes {
		if !validSpoolAgentName(lane.Agent) || seen[lane.Agent] || !validRelayObservationCode(lane.ErrorCode) {
			return relayObservationError("lane")
		}
		seen[lane.Agent] = true
		switch lane.State {
		case "observed":
			if lane.Queued == nil || *lane.Queued < 0 || *lane.Queued > 16384 || lane.ErrorCode != "" {
				return relayObservationError("count")
			}
			total += *lane.Queued
		case "absent":
			if lane.Queued != nil || lane.ErrorCode != "" {
				return relayObservationError("absent")
			}
		case "unavailable":
			if lane.Queued != nil || lane.ErrorCode == "" {
				return relayObservationError("unavailable")
			}
			complete = false
		default:
			return relayObservationError("state")
		}
	}
	if total > 16384 || s.Complete != complete || (complete && (s.Queued == nil || *s.Queued != total)) || (!complete && s.Queued != nil) {
		return relayObservationError("aggregate")
	}
	return nil
}

// Token checking rejects duplicate keys at every depth; exact-key checking
// additionally refuses encoding/json's case-insensitive aliases.
func decodeRelayObservation(raw []byte) (RelayOutboxObservation, error) {
	var s RelayOutboxObservation
	if len(raw) > relayObservationBytes {
		return s, relayObservationError("size")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 8 {
			return relayObservationError("depth")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return relayObservationError("duplicate")
				}
				seen[key] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		case json.Delim('['):
			for d.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := walk(0); err != nil {
		return s, relayObservationError("json")
	}
	if _, err := d.Token(); err != io.EOF {
		return s, relayObservationError("trailing")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return s, relayObservationError("object")
	}
	allowed := map[string]bool{"version": true, "root_device": true, "root_inode": true, "producer_uid": true, "started_at": true, "observed_at": true, "enumerated": true, "complete": true, "queued": true, "error_code": true, "lanes": true}
	for _, key := range []string{"version", "root_device", "root_inode", "producer_uid", "started_at", "observed_at", "enumerated", "complete", "lanes"} {
		if _, ok := fields[key]; !ok {
			return s, relayObservationError("field")
		}
	}
	for key, value := range fields {
		if !allowed[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return s, relayObservationError("field")
		}
	}
	var lanes []map[string]json.RawMessage
	if json.Unmarshal(fields["lanes"], &lanes) != nil || lanes == nil {
		return s, relayObservationError("lanes")
	}
	for _, lane := range lanes {
		if lane["agent"] == nil || lane["state"] == nil {
			return s, relayObservationError("field")
		}
		for key, value := range lane {
			if (key != "agent" && key != "state" && key != "queued" && key != "error_code") || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return s, relayObservationError("field")
			}
		}
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, relayObservationError("json")
	}
	return s, nil
}
