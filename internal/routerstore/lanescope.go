package routerstore

// Lane-scoped host tokens (schema v25). A token with a scope may mint sessions for, and act as, ONLY the
// listed agents, through a deny-by-default lane API: any store method not named in laneAPI is refused, so
// a new method is closed to scoped tokens until someone decides what scoping means for it.
//
// Why: a host token is a host-wide credential and the agent id is client-supplied, so a token placed in a
// cloud sandbox could otherwise act as ANY lane (including ra), send as any sender, and read the whole
// ledger (ListAll, Get). A scoped token confines a cloud lane to its own identity: it may send to anyone
// (messaging is the point), read and work only what is addressed to it, and see only items it is a party to.

import (
	"errors"
	"fmt"
	"reflect"
)

// ErrLaneScope: a scoped token tried something outside its lane.
var ErrLaneScope = errors.New("routerstore: lane scope: this token is confined to its listed agents")

// laneRule says which arguments of a method must be inside the token's scope.
type laneRule struct {
	agents []int // positions in the decoded arg list (context.Context included) whose string value must be an in-scope agent
	item   int   // position of an item id the caller must be a party to; -1 = none
	toOnly bool  // with item: the caller must be the RECIPIENT (work on it), not merely the sender
	custom func(scope map[string]bool, s *server, in []reflect.Value) error
}

var noItem = -1

// laneAPI is the complete set of methods a scoped token may call. Everything else is refused.
var laneAPI = map[string]laneRule{
	// identity and registration
	"MintSession":          {agents: []int{1}, item: noItem},
	"MintSessionForThread": {agents: []int{1}, item: noItem},
	"RegisterAgent":        {agents: []int{0}, item: noItem},
	"Heartbeat":            {agents: []int{0}, item: noItem},
	"ThreadBinding":        {item: noItem},
	"ListThreads":          {item: noItem},
	"GetThreadRecord":      {item: noItem}, // actual returned row is scope-checked before serialization
	"ListAgents":           {item: noItem},
	"GetAgent":             {item: noItem},
	"OperationalAgents":    {item: noItem},
	"ClassifyLane":         {item: noItem}, // read-only liveness of any lane: needed to see where a send will land
	"UpsertThreadCAS":      {item: noItem, custom: threadRecordsInScope},
	"UpsertThreads":        {item: noItem, custom: threadRecordsInScope},
	"ResumeThreadCAS":      {item: noItem, custom: threadRecordsInScope},
	"DeleteThreadCAS":      {item: noItem, custom: deleteThreadInScope},
	// messaging: send to anyone, read and work only what is yours
	"Send":         {agents: []int{0}, item: noItem},
	"SendGuarded":  {item: noItem, custom: sendReqInScope},
	"Inbox":        {agents: []int{0}, item: noItem},
	"Get":          {item: 0},
	"Render":       {item: 0},
	"AckItem":      {item: 0, toOnly: true},
	"CloseItem":    {item: 0, toOnly: true},
	"ClaimNext":    {agents: []int{0}, item: noItem},
	"Complete":     {item: 0, toOnly: true},
	"Fail":         {item: 0, toOnly: true},
	"Block":        {item: 0, toOnly: true},
	"StartWork":    {item: 0, toOnly: true},
	"RenewLease":   {item: 0, toOnly: true},
	"VerifyLease":  {item: 0, toOnly: true},
	"SetBlockedBy": {item: 0, toOnly: true},
	"ReassignItem": {agents: []int{1}, item: 0, toOnly: true},
	// waiting and nudging
	"Wait":         {agents: []int{1}, item: noItem},
	"ListenNotify": {agents: []int{1}, item: noItem},
	"NotifyAgent":  {item: noItem}, // a wake nudge for any recipient
	"NotifyPath":   {item: noItem},
	// the lane's own task registry
	"AddTask":              {item: noItem, custom: taskAgentInScope},
	"GetTask":              {agents: []int{0}, item: noItem},
	"ListTasks":            {agents: []int{0}, item: noItem},
	"ClaimTask":            {agents: []int{0}, item: noItem},
	"ClaimNextTask":        {agents: []int{0}, item: noItem},
	"CompleteTaskLease":    {agents: []int{0}, item: noItem},
	"ReleaseTaskLease":     {agents: []int{0}, item: noItem},
	"RenewTaskLease":       {agents: []int{0}, item: noItem},
	"UpdateTask":           {agents: []int{0}, item: noItem},
	"ResetTaskAttempts":    {agents: []int{0}, item: noItem},
	"TaskEligibility":      {agents: []int{0}, item: noItem},
	"TaskLeaseEventsSince": {agents: []int{0}, item: noItem},
	"VerifyCompletion":     {agents: []int{0}, item: noItem},
	"RunnableFor":          {agents: []int{0}, item: noItem},
}

// laneScope returns nil when the call is inside the token's scope, ErrLaneScope otherwise.
func (s *server) laneScope(scope map[string]bool, sess Session, name string, in []reflect.Value) error {
	rule, ok := laneAPI[name]
	if !ok {
		return fmt.Errorf("%w: %s is not part of the lane API", ErrLaneScope, name)
	}
	if sess.Agent != "" && !scope[sess.Agent] {
		return fmt.Errorf("%w: session agent %q is outside the token's scope", ErrLaneScope, sess.Agent)
	}
	for _, i := range rule.agents {
		if i >= len(in) {
			return fmt.Errorf("%w: %s: missing agent argument", ErrLaneScope, name)
		}
		if a := in[i].String(); !scope[a] {
			return fmt.Errorf("%w: %s as %q", ErrLaneScope, name, a)
		}
	}
	if rule.item >= 0 {
		if rule.item >= len(in) {
			return fmt.Errorf("%w: %s: missing item argument", ErrLaneScope, name)
		}
		// One error for "no such item" and "not yours", so a scoped token cannot probe which ids exist.
		it, err := s.store.Get(in[rule.item].String())
		if err != nil {
			return fmt.Errorf("%w: item not visible to this token", ErrLaneScope)
		}
		if !scope[it.To] && (rule.toOnly || !scope[it.From]) {
			return fmt.Errorf("%w: item not visible to this token", ErrLaneScope)
		}
	}
	if rule.custom != nil {
		return rule.custom(scope, s, in)
	}
	return nil
}

func threadRecordsInScope(scope map[string]bool, s *server, in []reflect.Value) error {
	for _, v := range in {
		switch r := v.Interface().(type) {
		case ThreadRecord:
			if err := threadRecordInScope(scope, s, r); err != nil {
				return err
			}
		case []ThreadRecord:
			for _, t := range r {
				if err := threadRecordInScope(scope, s, t); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// threadRecordInScope refuses a write whose new agent OR current row owner is outside the token's scope.
// Checking only the incoming agent (as the original PR #1079 did) let a scoped token overwrite an existing
// blank-host or same-host row it did not own, by naming a thread id and claiming it for an in-scope agent;
// the pre-existing host check in threadAuthority only protects a row already bound to a DIFFERENT host
// (SSA PR #1079 review, P1).
func threadRecordInScope(scope map[string]bool, s *server, r ThreadRecord) error {
	if !scope[r.Agent] {
		return fmt.Errorf("%w: thread record for %q", ErrLaneScope, r.Agent)
	}
	existing, err := s.store.ThreadBinding(r.ThreadID)
	if err == nil && !scope[existing.Agent] {
		return fmt.Errorf("%w: thread %q is owned by %q", ErrLaneScope, r.ThreadID, existing.Agent)
	}
	if err != nil && !errors.Is(err, ErrThreadUnknown) {
		return fmt.Errorf("%w: thread lookup failed: %v", ErrLaneScope, err)
	}
	return nil
}

// scopedThreadWriter is implemented by the one store that owns real thread rows. Lane scope is enforced
// server-side only, so RemoteStore (the wire client) never needs it.
type scopedThreadWriter interface {
	UpsertThreadsScoped(records []ThreadRecord, scope []string) error
	UpsertThreadCASScoped(r ThreadRecord, scope []string) (bool, error)
	ResumeThreadCASScoped(record ThreadRecord, suspendedAt string, scope []string) error
	DeleteThreadCASScoped(threadID, status, lastSeenAt, host string, scope []string) (bool, error)
}

// scopedThreadWrite runs a scoped token's UpsertThreads/UpsertThreadCAS/ResumeThreadCAS through the
// atomic store methods above instead of the plain ones, so the existing-owner scope check happens as
// part of the write itself. A recognized scoped mutation never falls back to an
// unscoped write: missing backend capability or malformed arguments fail closed.
func (s *server) scopedThreadWrite(scope map[string]bool, name string, in []reflect.Value) (result []any, handled bool, err error) {
	switch name {
	case "UpsertThreads", "UpsertThreadCAS", "ResumeThreadCAS", "DeleteThreadCAS":
	default:
		return nil, false, nil
	}
	w, ok := s.store.(scopedThreadWriter)
	if !ok {
		return nil, true, fmt.Errorf("%w: atomic scoped thread writes unavailable", ErrLaneScope)
	}
	agents := make([]string, 0, len(scope))
	for a := range scope {
		agents = append(agents, a)
	}
	switch name {
	case "UpsertThreads":
		if len(in) < 1 {
			return nil, true, ErrLaneScope
		}
		records, ok := in[0].Interface().([]ThreadRecord)
		if !ok {
			return nil, true, ErrLaneScope
		}
		for _, r := range records {
			if !scope[r.Agent] {
				return nil, true, fmt.Errorf("%w: thread record for %q", ErrLaneScope, r.Agent)
			}
		}
		return []any{}, true, w.UpsertThreadsScoped(records, agents)
	case "UpsertThreadCAS":
		if len(in) < 1 {
			return nil, true, ErrLaneScope
		}
		r, ok := in[0].Interface().(ThreadRecord)
		if !ok {
			return nil, true, ErrLaneScope
		}
		if !scope[r.Agent] {
			return nil, true, fmt.Errorf("%w: thread record for %q", ErrLaneScope, r.Agent)
		}
		accepted, werr := w.UpsertThreadCASScoped(r, agents)
		return []any{accepted}, true, werr
	case "ResumeThreadCAS":
		if len(in) < 2 {
			return nil, true, ErrLaneScope
		}
		r, ok := in[0].Interface().(ThreadRecord)
		if !ok {
			return nil, true, ErrLaneScope
		}
		suspendedAt, ok := in[1].Interface().(string)
		if !ok {
			return nil, true, ErrLaneScope
		}
		if !scope[r.Agent] {
			return nil, true, fmt.Errorf("%w: thread record for %q", ErrLaneScope, r.Agent)
		}
		return []any{}, true, w.ResumeThreadCASScoped(r, suspendedAt, agents)
	case "DeleteThreadCAS":
		if len(in) != 4 {
			return nil, true, ErrLaneScope
		}
		args := make([]string, 4)
		for i := range in {
			value, ok := in[i].Interface().(string)
			if !ok {
				return nil, true, ErrLaneScope
			}
			args[i] = value
		}
		deleted, err := w.DeleteThreadCASScoped(args[0], args[1], args[2], args[3], agents)
		return []any{deleted}, true, err
	default:
		return nil, false, nil
	}
}

func deleteThreadInScope(scope map[string]bool, s *server, in []reflect.Value) error {
	if len(in) == 0 {
		return fmt.Errorf("%w: missing thread id", ErrLaneScope)
	}
	b, err := s.store.ThreadBinding(in[0].String())
	if err != nil || !scope[b.Agent] {
		return fmt.Errorf("%w: thread not visible to this token", ErrLaneScope)
	}
	return nil
}

func sendReqInScope(scope map[string]bool, _ *server, in []reflect.Value) error {
	if len(in) == 0 {
		return fmt.Errorf("%w: missing send request", ErrLaneScope)
	}
	r, ok := in[0].Interface().(SendReq)
	if !ok || !scope[r.From] {
		return fmt.Errorf("%w: send as %q", ErrLaneScope, r.From)
	}
	return nil
}

func taskAgentInScope(scope map[string]bool, _ *server, in []reflect.Value) error {
	if len(in) == 0 {
		return fmt.Errorf("%w: missing task", ErrLaneScope)
	}
	t, ok := in[0].Interface().(Task)
	if !ok || !scope[t.Agent] {
		return fmt.Errorf("%w: task for %q", ErrLaneScope, t.Agent)
	}
	return nil
}

// scopeSet turns a token's agent list into a lookup set; nil/empty means unscoped (returns nil).
func scopeSet(agents []string) map[string]bool {
	if len(agents) == 0 {
		return nil
	}
	m := make(map[string]bool, len(agents))
	for _, a := range agents {
		m[a] = true
	}
	return m
}
