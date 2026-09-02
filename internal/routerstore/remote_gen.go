// Code generated from store_iface.go by the ADR-062 rs-09 stub generator; DO NOT EDIT by hand.
// Regenerate when the Store interface changes — `var _ Store = (*RemoteStore)(nil)` in remote.go
// fails to compile until this file matches the interface again.

package routerstore

import "time"

func (rs *RemoteStore) AckWakeEvent(eventID, token, ackRef string) error {
	return rs.call("AckWakeEvent", []any{eventID, token, ackRef})
}
func (rs *RemoteStore) AddRequirement(title, source, sourceRef, owner string) (Requirement, error) {
	var o0 Requirement
	err := rs.call("AddRequirement", []any{title, source, sourceRef, owner}, &o0)
	return o0, err
}
func (rs *RemoteStore) AddTask(t Task) error { return rs.call("AddTask", []any{t}) }
func (rs *RemoteStore) AllocateIdentifier(namespace, title, owner string) (Identifier, error) {
	var o0 Identifier
	err := rs.call("AllocateIdentifier", []any{namespace, title, owner}, &o0)
	return o0, err
}
func (rs *RemoteStore) Backfill(items []Item) (BackfillReport, error) {
	var o0 BackfillReport
	err := rs.call("Backfill", []any{items}, &o0)
	return o0, err
}
func (rs *RemoteStore) BindItemSession(id, session string) error {
	return rs.call("BindItemSession", []any{id, session})
}
func (rs *RemoteStore) BindTaskSession(agent, taskID, session string) error {
	return rs.call("BindTaskSession", []any{agent, taskID, session})
}
func (rs *RemoteStore) Block(id, token, reason string) error {
	return rs.call("Block", []any{id, token, reason})
}
func (rs *RemoteStore) Breakers() ([]Breaker, error) {
	var o0 []Breaker
	err := rs.call("Breakers", nil, &o0)
	return o0, err
}
func (rs *RemoteStore) ClaimIdentifierNumber(namespace string, number int, title, slug, owner string) (Identifier, error) {
	var o0 Identifier
	err := rs.call("ClaimIdentifierNumber", []any{namespace, number, title, slug, owner}, &o0)
	return o0, err
}
func (rs *RemoteStore) ClaimNext(agent string, ttl time.Duration) (*Lease, error) {
	var o0 *Lease
	err := rs.call("ClaimNext", []any{agent, ttl}, &o0)
	return o0, err
}
func (rs *RemoteStore) ClaimNextTask(agent, worker, threadID string, ttl time.Duration) (*TaskLease, error) {
	var o0 *TaskLease
	err := rs.call("ClaimNextTask", []any{agent, worker, threadID, ttl}, &o0)
	return o0, err
}
func (rs *RemoteStore) ClaimTask(agent, taskID, worker, threadID string, ttl time.Duration) (*TaskLease, error) {
	var o0 *TaskLease
	err := rs.call("ClaimTask", []any{agent, taskID, worker, threadID, ttl}, &o0)
	return o0, err
}
func (rs *RemoteStore) ClaimWakeEvent(ttl time.Duration) (*WakeEvent, error) {
	var o0 *WakeEvent
	err := rs.call("ClaimWakeEvent", []any{ttl}, &o0)
	return o0, err
}
func (rs *RemoteStore) ClaimWakeEventFor(agent string, ttl time.Duration) (*WakeEvent, error) {
	var o0 *WakeEvent
	err := rs.call("ClaimWakeEventFor", []any{agent, ttl}, &o0)
	return o0, err
}
func (rs *RemoteStore) ClassifyLane(agent string, wakeRoutable bool, recentWithin time.Duration) (LaneOperationalState, error) {
	var o0 LaneOperationalState
	err := rs.call("ClassifyLane", []any{agent, wakeRoutable, recentWithin}, &o0)
	return o0, err
}
func (rs *RemoteStore) CloseItem(id, result string) error {
	return rs.call("CloseItem", []any{id, result})
}
func (rs *RemoteStore) Complete(id, token, result string) error {
	return rs.call("Complete", []any{id, token, result})
}
func (rs *RemoteStore) CompleteTaskLease(agent, taskID, token, resultRef string) error {
	return rs.call("CompleteTaskLease", []any{agent, taskID, token, resultRef})
}
func (rs *RemoteStore) Counters() (DispatchCounters, error) {
	var o0 DispatchCounters
	err := rs.call("Counters", nil, &o0)
	return o0, err
}
func (rs *RemoteStore) DeleteThreadCAS(threadID, status, lastSeenAt string) (bool, error) {
	var o0 bool
	err := rs.call("DeleteThreadCAS", []any{threadID, status, lastSeenAt}, &o0)
	return o0, err
}
func (rs *RemoteStore) ExportItem(dir, id string) (string, error) {
	var o0 string
	err := rs.call("ExportItem", []any{dir, id}, &o0)
	return o0, err
}
func (rs *RemoteStore) ExportMarkdown(dir string) (int, error) {
	var o0 int
	err := rs.call("ExportMarkdown", []any{dir}, &o0)
	return o0, err
}
func (rs *RemoteStore) Fail(id, token, reason, failureClass string) error {
	return rs.call("Fail", []any{id, token, reason, failureClass})
}
func (rs *RemoteStore) FailWakeEvent(eventID, token, failure string) error {
	return rs.call("FailWakeEvent", []any{eventID, token, failure})
}
func (rs *RemoteStore) ForceOwner(id, action, reason string) error {
	return rs.call("ForceOwner", []any{id, action, reason})
}
func (rs *RemoteStore) GC(keep time.Duration) (int64, error) {
	var o0 int64
	err := rs.call("GC", []any{keep}, &o0)
	return o0, err
}
func (rs *RemoteStore) Get(id string) (Item, error) {
	var o0 Item
	err := rs.call("Get", []any{id}, &o0)
	return o0, err
}
func (rs *RemoteStore) GetAgent(id string) (Agent, error) {
	var o0 Agent
	err := rs.call("GetAgent", []any{id}, &o0)
	return o0, err
}
func (rs *RemoteStore) GetSession(id string) (Session, error) {
	var o0 Session
	err := rs.call("GetSession", []any{id}, &o0)
	return o0, err
}
func (rs *RemoteStore) GetState(key string) (string, bool, error) {
	var o0 string
	var o1 bool
	err := rs.call("GetState", []any{key}, &o0, &o1)
	return o0, o1, err
}
func (rs *RemoteStore) GetTask(agent, taskID string) (Task, error) {
	var o0 Task
	err := rs.call("GetTask", []any{agent, taskID}, &o0)
	return o0, err
}
func (rs *RemoteStore) Heartbeat(id string) error { return rs.call("Heartbeat", []any{id}) }
func (rs *RemoteStore) ImportThreadsIfEmpty(records []ThreadRecord) error {
	return rs.call("ImportThreadsIfEmpty", []any{records})
}
func (rs *RemoteStore) Inbox(agent string) ([]Item, error) {
	var o0 []Item
	err := rs.call("Inbox", []any{agent}, &o0)
	return o0, err
}
func (rs *RemoteStore) ItemSession(id string) (string, error) {
	var o0 string
	err := rs.call("ItemSession", []any{id}, &o0)
	return o0, err
}
func (rs *RemoteStore) ListAgents() ([]Agent, error) {
	var o0 []Agent
	err := rs.call("ListAgents", nil, &o0)
	return o0, err
}
func (rs *RemoteStore) ListAll() ([]Item, error) {
	var o0 []Item
	err := rs.call("ListAll", nil, &o0)
	return o0, err
}
func (rs *RemoteStore) ListIdentifiers(namespace string) ([]Identifier, error) {
	var o0 []Identifier
	err := rs.call("ListIdentifiers", []any{namespace}, &o0)
	return o0, err
}
func (rs *RemoteStore) ListRequirements(owner string) ([]Requirement, error) {
	var o0 []Requirement
	err := rs.call("ListRequirements", []any{owner}, &o0)
	return o0, err
}
func (rs *RemoteStore) ListTasks(agent string) ([]Task, error) {
	var o0 []Task
	err := rs.call("ListTasks", []any{agent}, &o0)
	return o0, err
}
func (rs *RemoteStore) ListThreads() ([]ThreadRecord, error) {
	var o0 []ThreadRecord
	err := rs.call("ListThreads", nil, &o0)
	return o0, err
}
func (rs *RemoteStore) ListWakeEvents(agent string) ([]WakeEvent, error) {
	var o0 []WakeEvent
	err := rs.call("ListWakeEvents", []any{agent}, &o0)
	return o0, err
}
func (rs *RemoteStore) MarkRequirementAudit(agent, evidenceRef string) error {
	return rs.call("MarkRequirementAudit", []any{agent, evidenceRef})
}
func (rs *RemoteStore) MintSession(host, agent, runtimeHash string) (Session, error) {
	var o0 Session
	err := rs.call("MintSession", []any{host, agent, runtimeHash}, &o0)
	return o0, err
}
func (rs *RemoteStore) OperationalAgents() ([]string, error) {
	var o0 []string
	err := rs.call("OperationalAgents", nil, &o0)
	return o0, err
}
func (rs *RemoteStore) PublishIdentifier(namespace string, number int, slug string) error {
	return rs.call("PublishIdentifier", []any{namespace, number, slug})
}
func (rs *RemoteStore) Put(it Item) error { return rs.call("Put", []any{it}) }
func (rs *RemoteStore) ReclaimExpiredTaskLeases(dryRun bool) ([]ExpiredLeaseReclaim, error) {
	var o0 []ExpiredLeaseReclaim
	err := rs.call("ReclaimExpiredTaskLeases", []any{dryRun}, &o0)
	return o0, err
}
func (rs *RemoteStore) ReconcileOperationalState(agent string, wakeRoutable bool) (ReconcileReport, error) {
	var o0 ReconcileReport
	err := rs.call("ReconcileOperationalState", []any{agent, wakeRoutable}, &o0)
	return o0, err
}
func (rs *RemoteStore) RecordEvidence(reqID string, ev Evidence) error {
	return rs.call("RecordEvidence", []any{reqID, ev})
}
func (rs *RemoteStore) RegisterAgent(id string, pid int) error {
	return rs.call("RegisterAgent", []any{id, pid})
}
func (rs *RemoteStore) ReleaseTaskLease(agent, taskID, token, reason string) error {
	return rs.call("ReleaseTaskLease", []any{agent, taskID, token, reason})
}
func (rs *RemoteStore) Render(id string) (string, error) {
	var o0 string
	err := rs.call("Render", []any{id}, &o0)
	return o0, err
}
func (rs *RemoteStore) RenewLease(id, token string, ttl time.Duration) error {
	return rs.call("RenewLease", []any{id, token, ttl})
}
func (rs *RemoteStore) RenewTaskLease(agent, taskID, token string, ttl time.Duration) error {
	return rs.call("RenewTaskLease", []any{agent, taskID, token, ttl})
}
func (rs *RemoteStore) ResetBreaker(domain string) error {
	return rs.call("ResetBreaker", []any{domain})
}
func (rs *RemoteStore) ResetTaskAttempts(agent, taskID string) error {
	return rs.call("ResetTaskAttempts", []any{agent, taskID})
}
func (rs *RemoteStore) ResumeThreadCAS(record ThreadRecord, suspendedAt string) error {
	return rs.call("ResumeThreadCAS", []any{record, suspendedAt})
}
func (rs *RemoteStore) RevokeSession(id string) error { return rs.call("RevokeSession", []any{id}) }
func (rs *RemoteStore) RunnableFor(agent string) (RunnableState, error) {
	var o0 RunnableState
	err := rs.call("RunnableFor", []any{agent}, &o0)
	return o0, err
}
func (rs *RemoteStore) Satisfy(reqID string) error { return rs.call("Satisfy", []any{reqID}) }
func (rs *RemoteStore) Send(from, to, title, msgType, instructions string) (string, error) {
	var o0 string
	err := rs.call("Send", []any{from, to, title, msgType, instructions}, &o0)
	return o0, err
}
func (rs *RemoteStore) SendGuarded(r SendReq) (string, bool, error) {
	var o0 string
	var o1 bool
	err := rs.call("SendGuarded", []any{r}, &o0, &o1)
	return o0, o1, err
}
func (rs *RemoteStore) SetBlockedBy(id, blockedBy string) error {
	return rs.call("SetBlockedBy", []any{id, blockedBy})
}
func (rs *RemoteStore) SetState(key, value string) error {
	return rs.call("SetState", []any{key, value})
}
func (rs *RemoteStore) SetWake(id, status, attemptedAt, adapter, wakeErr string) error {
	return rs.call("SetWake", []any{id, status, attemptedAt, adapter, wakeErr})
}
func (rs *RemoteStore) StartWork(id, token string) error {
	return rs.call("StartWork", []any{id, token})
}
func (rs *RemoteStore) TaskSession(agent, taskID string) (string, error) {
	var o0 string
	err := rs.call("TaskSession", []any{agent, taskID}, &o0)
	return o0, err
}
func (rs *RemoteStore) TouchSession(id string) error { return rs.call("TouchSession", []any{id}) }
func (rs *RemoteStore) UnmetRequirements(owner string) ([]Requirement, error) {
	var o0 []Requirement
	err := rs.call("UnmetRequirements", []any{owner}, &o0)
	return o0, err
}
func (rs *RemoteStore) UpdateTask(agent, taskID string, u TaskUpdate) (Task, error) {
	var o0 Task
	err := rs.call("UpdateTask", []any{agent, taskID, u}, &o0)
	return o0, err
}
func (rs *RemoteStore) UpsertThreadCAS(r ThreadRecord) (bool, error) {
	var o0 bool
	err := rs.call("UpsertThreadCAS", []any{r}, &o0)
	return o0, err
}
func (rs *RemoteStore) UpsertThreads(records []ThreadRecord) error {
	return rs.call("UpsertThreads", []any{records})
}
func (rs *RemoteStore) VerifyCompletion(agent string) (CompletionReport, error) {
	var o0 CompletionReport
	err := rs.call("VerifyCompletion", []any{agent}, &o0)
	return o0, err
}
func (rs *RemoteStore) Waive(reqID, reason, ownerDecisionRef string) error {
	return rs.call("Waive", []any{reqID, reason, ownerDecisionRef})
}
func (rs *RemoteStore) WithdrawIdentifier(namespace string, number int) error {
	return rs.call("WithdrawIdentifier", []any{namespace, number})
}
