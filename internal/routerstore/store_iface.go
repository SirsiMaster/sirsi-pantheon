// ADR-062 Migration step 1 (rs-01). Keep in sync with SQLiteStore.

package routerstore

import (
	"context"
	"time"
)

// Store is the router ledger contract every consumer programs against.
// SQLiteStore is the in-process backend (Anubis); ADR-062 adds a Postgres
// backend and an HTTP client behind the same interface. No production code
// may depend on the concrete type.
type Store interface {
	AckWakeEvent(eventID, token, ackRef string) error
	AddRequirement(title, source, sourceRef, owner string) (Requirement, error)
	AddTask(t Task) error
	AllocateIdentifier(namespace, title, owner string) (Identifier, error)
	Backfill(items []Item) (BackfillReport, error)
	BindItemSession(id, session string) error
	BindTaskSession(agent, taskID, session string) error
	Block(id, token, reason string) error
	Breakers() ([]Breaker, error)
	ClaimIdentifierNumber(namespace string, number int, title, slug, owner string) (Identifier, error)
	ClaimNext(agent string, ttl time.Duration) (*Lease, error)
	ClaimNextTask(agent, worker, threadID string, ttl time.Duration) (*TaskLease, error)
	ClaimTask(agent, taskID, worker, threadID string, ttl time.Duration) (*TaskLease, error)
	ClaimWakeEvent(ttl time.Duration) (*WakeEvent, error)
	ClaimWakeEventFor(agent string, ttl time.Duration) (*WakeEvent, error)
	ClassifyLane(agent string, wakeRoutable bool, recentWithin time.Duration) (LaneOperationalState, error)
	Close() error
	CloseItem(id, result string) error
	Complete(id, token, result string) error
	CompleteTaskLease(agent, taskID, token, resultRef string) error
	Counters() (DispatchCounters, error)
	DeleteThreadCAS(threadID, status, lastSeenAt string) (bool, error)
	ExportItem(dir, id string) (string, error)
	ExportMarkdown(dir string) (int, error)
	Fail(id, token, reason, failureClass string) error
	FailWakeEvent(eventID, token, failure string) error
	ForceOwner(id, action, reason string) error
	GC(keep time.Duration) (int64, error)
	Get(id string) (Item, error)
	GetAgent(id string) (Agent, error)
	GetSession(id string) (Session, error)
	GetState(key string) (string, bool, error)
	GetTask(agent, taskID string) (Task, error)
	Heartbeat(id string) error
	ImportThreadsIfEmpty(records []ThreadRecord) error
	Inbox(agent string) ([]Item, error)
	ItemSession(id string) (string, error)
	ListAgents() ([]Agent, error)
	ListAll() ([]Item, error)
	ListHostTokens() ([]HostToken, error)
	ListIdentifiers(namespace string) ([]Identifier, error)
	ListRequirements(owner string) ([]Requirement, error)
	ListTasks(agent string) ([]Task, error)
	ListThreads() ([]ThreadRecord, error)
	ListWakeEvents(agent string) ([]WakeEvent, error)
	ListenNotify(ctx context.Context, agent string) (<-chan struct{}, error)
	LookupHostToken(plaintext string) (HostToken, error)
	MarkRequirementAudit(agent, evidenceRef string) error
	MintHostToken(host, label string) (string, HostToken, error)
	MintSession(host, agent, runtimeHash string) (Session, error)
	NotifyAgent(agent string)
	NotifyPath(agent string) string
	OperationalAgents() ([]string, error)
	PublishIdentifier(namespace string, number int, slug string) error
	Put(it Item) error
	ReclaimExpiredTaskLeases(dryRun bool) ([]ExpiredLeaseReclaim, error)
	ReconcileOperationalState(agent string, wakeRoutable bool) (ReconcileReport, error)
	RecordEvidence(reqID string, ev Evidence) error
	RegisterAgent(id string, pid int) error
	ReleaseTaskLease(agent, taskID, token, reason string) error
	Render(id string) (string, error)
	RenewLease(id, token string, ttl time.Duration) error
	RenewTaskLease(agent, taskID, token string, ttl time.Duration) error
	ResetBreaker(domain string) error
	ResetTaskAttempts(agent, taskID string) error
	ResumeThreadCAS(record ThreadRecord, suspendedAt string) error
	RevokeHostToken(id string) error
	RevokeSession(id string) error
	RunnableFor(agent string) (RunnableState, error)
	Satisfy(reqID string) error
	Send(from, to, title, msgType, instructions string) (string, error)
	SendGuarded(r SendReq) (string, bool, error)
	SetBlockedBy(id, blockedBy string) error
	SetState(key, value string) error
	SetWake(id, status, attemptedAt, adapter, wakeErr string) error
	StartWork(id, token string) error
	TaskSession(agent, taskID string) (string, error)
	TouchSession(id string) error
	UnmetRequirements(owner string) ([]Requirement, error)
	UpdateTask(agent, taskID string, u TaskUpdate) (Task, error)
	UpsertThreadCAS(r ThreadRecord) (bool, error)
	UpsertThreads(records []ThreadRecord) error
	VerifyCompletion(agent string) (CompletionReport, error)
	Wait(ctx context.Context, agent string, timeout time.Duration) (bool, error)
	Waive(reqID, reason, ownerDecisionRef string) error
	WithdrawIdentifier(namespace string, number int) error
}

var _ Store = (*SQLiteStore)(nil)
