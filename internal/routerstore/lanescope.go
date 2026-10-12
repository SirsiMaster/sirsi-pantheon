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

func threadRecordsInScope(scope map[string]bool, _ *server, in []reflect.Value) error {
	for _, v := range in {
		switch r := v.Interface().(type) {
		case ThreadRecord:
			if !scope[r.Agent] {
				return fmt.Errorf("%w: thread record for %q", ErrLaneScope, r.Agent)
			}
		case []ThreadRecord:
			for _, t := range r {
				if !scope[t.Agent] {
					return fmt.Errorf("%w: thread record for %q", ErrLaneScope, t.Agent)
				}
			}
		}
	}
	return nil
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
