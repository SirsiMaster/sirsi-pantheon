package routerstore

import (
	"encoding/json"
	"reflect"
	"strings"
)

// keepAttendedSession stops a client older than #1364 from expiring an attended
// claude/codex session only because its lease went quiet. #1364 made those
// registrations last until closed, but the sweep runs in each Mac's CLI, so every
// Mac kept retiring live sessions every 2 hours until it installed a release. The
// service answers such a write as applied and leaves the row as it was: the
// client's own sweep then continues normally. Any other reap is unchanged.
func (s *server) keepAttendedSession(name string, in []reflect.Value) (bool, ThreadRecord) {
	if (name != "UpsertThreadCAS" && name != "UpsertThreadCASScoped") || len(in) == 0 {
		return false, ThreadRecord{}
	}
	r, ok := in[0].Interface().(ThreadRecord)
	return ok && leaseReapOfAttended(r), r
}

func leaseReapOfAttended(r ThreadRecord) bool {
	if r.Status != "reaped" || r.CreateOnly {
		return false
	}
	var p struct {
		Surface   string `json:"surface"`
		SessionID string `json:"session_id"`
		LastError string `json:"last_error"`
	}
	if json.Unmarshal(r.Payload, &p) != nil {
		return false
	}
	return (p.Surface == "claude" || p.Surface == "codex") && p.SessionID != "" &&
		strings.HasPrefix(p.LastError, "reaped: session ") && strings.Contains(p.LastError, "lease expired")
}
