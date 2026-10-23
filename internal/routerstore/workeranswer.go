package routerstore

// Headless workers are subordinate to lead lanes (owner 2026-10-18): only a lead lane answers router items,
// unless the lead lane has given its worker the answering role (registry worker_answers, which the wake loop
// stamps on its worker thread as answer_role). Enforced here, in the service, so no consumer can route around
// it; `router respond` runs WorkerMayAnswer before it sends, so a refused answer never reaches the requester.
// Acknowledging stays open: an ack says "received", not "answered".

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// ErrWorkerAnswers: a headless worker tried to answer without its lane's answering role.
var ErrWorkerAnswers = errors.New("routerstore: a headless worker may not answer router items unless its lead lane granted it the answering role (registry worker_answers)")

// answerMethods are the verbs that answer an item; the value is the position of the item id in the decoded args.
var answerMethods = map[string]int{"CloseItem": 0, "Complete": 0, "ReconcileMailboxNotice": -1}

// WorkerMayAnswer reports whether the thread may answer: true for any non-worker thread (leads are judged by the
// Rule of Ra, not here) and for a worker thread whose record carries answer_role.
func WorkerMayAnswer(threads []ThreadRecord, threadID string) bool {
	for _, t := range threads {
		if t.ThreadID != threadID {
			continue
		}
		var p struct {
			Surface    string `json:"surface"`
			AnswerRole bool   `json:"answer_role"`
		}
		_ = json.Unmarshal(t.Payload, &p)
		return p.Surface != "worker" || p.AnswerRole
	}
	return true // an unregistered caller is the Rule of Ra's to refuse
}

// keepAnswerRole carries a stored answer_role onto a thread write that has no answer_role KEY at all.
// Thread writes are whole-payload rewrites of every same-host record, and a process still running a binary
// that predates the field re-encodes each record without it: on 2026-10-18 a Horus supervisor started Oct 14
// and an M5 dashboard started Oct 15 erased every worker's grant within minutes of the loops stamping it.
// Current clients always send the key (true or false), so a lead lane can still revoke; only a writer that
// cannot know the field is overridden. A lookup failure keeps the write as sent (the hold then fails closed).
func (s *server) keepAnswerRole(name string, in []reflect.Value) {
	var recs []ThreadRecord
	switch name {
	case "UpsertThreads":
		recs = in[0].Interface().([]ThreadRecord)
	case "UpsertThreadCAS", "ResumeThreadCAS":
		recs = []ThreadRecord{in[0].Interface().(ThreadRecord)}
	default:
		return
	}
	var stored map[string]json.RawMessage
	changed := false
	for i := range recs {
		var p map[string]json.RawMessage
		if json.Unmarshal(recs[i].Payload, &p) != nil {
			continue
		}
		if _, has := p["answer_role"]; has {
			continue
		}
		if stored == nil {
			stored = map[string]json.RawMessage{}
			threads, err := s.store.ListThreads()
			if err != nil {
				return
			}
			for _, t := range threads {
				var q struct {
					AnswerRole json.RawMessage `json:"answer_role"`
				}
				if json.Unmarshal(t.Payload, &q) == nil && len(q.AnswerRole) > 0 {
					stored[t.ThreadID] = q.AnswerRole
				}
			}
		}
		v, ok := stored[recs[i].ThreadID]
		if !ok {
			continue
		}
		p["answer_role"] = v
		if b, err := json.Marshal(p); err == nil {
			recs[i].Payload, changed = b, true
		}
	}
	if !changed {
		return
	}
	if name == "UpsertThreads" {
		in[0] = reflect.ValueOf(recs)
	} else {
		in[0] = reflect.ValueOf(recs[0])
	}
}

// workerAnswerHold is the service-side check. A failed thread lookup refuses (fail closed).
func (s *server) workerAnswerHold(sess Session, name string, in []reflect.Value) error {
	if _, ok := answerMethods[name]; !ok || sess.ThreadID == "" {
		return nil
	}
	threads, err := s.store.ListThreads()
	if err != nil {
		return fmt.Errorf("%w (thread lookup failed: %v)", ErrWorkerAnswers, err)
	}
	if !WorkerMayAnswer(threads, sess.ThreadID) {
		return fmt.Errorf("%w (worker thread %s of %s)", ErrWorkerAnswers, sess.ThreadID, sess.Agent)
	}
	return nil
}
