package maat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// MemorySchema is the first executable version of Ma'at's local failure-memory
// contract. It intentionally stores evidence and recovery guidance, never a
// runnable shell command or a second router projection.
const MemorySchema = "sirsi.maat.failure-memory.v1"

const maxMemoryObjectBytes = 8 << 20

// PreflightDecision is the only preflight result a caller can receive. Unknown is a
// failure-safe result: callers may inspect and repair evidence, but may not
// treat an unverified registry as a pass.
type PreflightDecision string

const (
	PreflightPass         PreflightDecision = "pass"
	PreflightReject       PreflightDecision = "reject"
	PreflightUnverifiable PreflightDecision = "unverifiable"
)

// FailureSignature deliberately excludes volatile process, time, prompt, and
// absolute-path data. Those belong in evidence bytes, not in the stable key.
type FailureSignature struct {
	Namespace    string `json:"namespace"`
	Version      string `json:"version"`
	FailureClass string `json:"failure_class"`
	Operation    string `json:"operation"`
	Component    string `json:"component"`
	Invariant    string `json:"invariant"`
	ErrorCode    string `json:"error_code"`
}

func (s FailureSignature) validate() error {
	for name, value := range map[string]string{
		"namespace": s.Namespace, "version": s.Version, "failure_class": s.FailureClass,
		"operation": s.Operation, "component": s.Component, "invariant": s.Invariant,
		"error_code": s.ErrorCode,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 160 || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("maat memory: invalid signature %s", name)
		}
	}
	return nil
}

// Digest returns the SHA-256 of the exact typed JSON projection. Struct-field
// order is part of this versioned schema, so a decoder cannot invent a map
// ordering or silently ignore a signature dimension.
func (s FailureSignature) Digest() (string, error) {
	if err := s.validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("maat memory: marshal signature: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Scope is intentionally exact. Empty strings are not wildcards: a record with
// indeterminate scope is rejected at write time rather than overblocking or
// silently permitting a different action.
type Scope struct {
	Component string `json:"component"`
	Profile   string `json:"profile"`
	Operation string `json:"operation"`
}

func (s Scope) validate() error {
	for name, value := range map[string]string{"component": s.Component, "profile": s.Profile, "operation": s.Operation} {
		if strings.TrimSpace(value) == "" || len(value) > 160 || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("maat memory: invalid scope %s", name)
		}
	}
	return nil
}

func (s Scope) matches(other Scope) bool {
	return s.Component == other.Component && s.Profile == other.Profile && s.Operation == other.Operation
}

// RecoveryAction is presentation data for every active incident. The caller
// must obtain its own consent and invoke an existing authoritative operation;
// Ma'at does not execute imported text.
type RecoveryAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Instruction string `json:"instruction"`
}

func (a RecoveryAction) validate() error {
	for name, value := range map[string]string{"id": a.ID, "label": a.Label, "instruction": a.Instruction} {
		if strings.TrimSpace(value) == "" || len(value) > 512 || strings.ContainsRune(value, 0) {
			return fmt.Errorf("maat memory: invalid recovery action %s", name)
		}
	}
	return nil
}

type IncidentStatus string

const (
	IncidentActive     IncidentStatus = "active"
	IncidentSuperseded IncidentStatus = "superseded"
	IncidentRetired    IncidentStatus = "retired"
)

// Incident is a create-only, evidence-bound fact. Status transitions are new
// records with a successor key; prior evidence is never rewritten.
type Incident struct {
	Schema          string           `json:"schema"`
	Key             string           `json:"key"`
	Signature       FailureSignature `json:"signature"`
	EvidenceSHA256  string           `json:"evidence_sha256"`
	Scope           Scope            `json:"scope"`
	GuardID         string           `json:"guard_id"`
	GuardVersion    string           `json:"guard_version"`
	Status          IncidentStatus   `json:"status"`
	SuccessorKey    string           `json:"successor_key,omitempty"`
	RecoveryActions []RecoveryAction `json:"recovery_actions"`
}

// IncidentTransition is the only way an active incident can stop blocking a
// matching preflight.  It is a separate create-only fact rather than a rewrite
// of the incident: the predecessor, copied identity fields, and terminal
// status are all included in its digest.  A root may have exactly one terminal
// transition, so a caller cannot fork or silently reopen its history.
type IncidentTransition struct {
	Schema         string           `json:"schema"`
	Key            string           `json:"key"`
	PredecessorKey string           `json:"predecessor_key"`
	Signature      FailureSignature `json:"signature"`
	EvidenceSHA256 string           `json:"evidence_sha256"`
	Scope          Scope            `json:"scope"`
	GuardID        string           `json:"guard_id"`
	GuardVersion   string           `json:"guard_version"`
	Status         IncidentStatus   `json:"status"`
}

type incidentTransitionDigest struct {
	Schema         string           `json:"schema"`
	PredecessorKey string           `json:"predecessor_key"`
	Signature      FailureSignature `json:"signature"`
	EvidenceSHA256 string           `json:"evidence_sha256"`
	Scope          Scope            `json:"scope"`
	GuardID        string           `json:"guard_id"`
	GuardVersion   string           `json:"guard_version"`
	Status         IncidentStatus   `json:"status"`
}

func newIncidentTransition(predecessor Incident, status IncidentStatus) (IncidentTransition, error) {
	if err := predecessor.validate(); err != nil || predecessor.Status != IncidentActive {
		return IncidentTransition{}, errors.New("maat memory: transition predecessor must be an active valid incident")
	}
	if status != IncidentSuperseded && status != IncidentRetired {
		return IncidentTransition{}, errors.New("maat memory: transition status must be terminal")
	}
	d := incidentTransitionDigest{Schema: MemorySchema, PredecessorKey: predecessor.Key, Signature: predecessor.Signature, EvidenceSHA256: predecessor.EvidenceSHA256, Scope: predecessor.Scope, GuardID: predecessor.GuardID, GuardVersion: predecessor.GuardVersion, Status: status}
	b, err := json.Marshal(d)
	if err != nil {
		return IncidentTransition{}, fmt.Errorf("maat memory: marshal transition digest: %w", err)
	}
	sum := sha256.Sum256(b)
	return IncidentTransition{Schema: d.Schema, Key: hex.EncodeToString(sum[:]), PredecessorKey: d.PredecessorKey, Signature: d.Signature, EvidenceSHA256: d.EvidenceSHA256, Scope: d.Scope, GuardID: d.GuardID, GuardVersion: d.GuardVersion, Status: d.Status}, nil
}

func (t IncidentTransition) validateAgainst(predecessor Incident) error {
	if predecessor.Status != IncidentActive || t.Schema != MemorySchema || !validDigest(t.Key) || !validDigest(t.PredecessorKey) || t.PredecessorKey != predecessor.Key {
		return errors.New("maat memory: invalid transition envelope")
	}
	if t.Status != IncidentSuperseded && t.Status != IncidentRetired {
		return errors.New("maat memory: invalid transition status")
	}
	expected, err := newIncidentTransition(predecessor, t.Status)
	if err != nil || expected != t {
		return errors.New("maat memory: transition does not bind its predecessor")
	}
	return nil
}

func NewIncident(signature FailureSignature, evidenceSHA256 string, scope Scope, guardID, guardVersion string, actions []RecoveryAction) (Incident, error) {
	key, err := signature.Digest()
	if err != nil {
		return Incident{}, err
	}
	if err := scope.validate(); err != nil {
		return Incident{}, err
	}
	if !validDigest(evidenceSHA256) || strings.TrimSpace(guardID) == "" || strings.TrimSpace(guardVersion) == "" || len(actions) == 0 {
		return Incident{}, errors.New("maat memory: incident requires evidence, guard identity, and recovery action")
	}
	for _, action := range actions {
		if err := action.validate(); err != nil {
			return Incident{}, err
		}
	}
	return Incident{Schema: MemorySchema, Key: key, Signature: signature, EvidenceSHA256: evidenceSHA256, Scope: scope, GuardID: guardID, GuardVersion: guardVersion, Status: IncidentActive, RecoveryActions: actions}, nil
}

func (i Incident) validate() error {
	if i.Schema != MemorySchema || !validDigest(i.Key) || !validDigest(i.EvidenceSHA256) || i.GuardID == "" || i.GuardVersion == "" {
		return errors.New("maat memory: invalid incident envelope")
	}
	key, err := i.Signature.Digest()
	if err != nil || key != i.Key {
		return errors.New("maat memory: incident key does not bind signature")
	}
	if err := i.Scope.validate(); err != nil {
		return err
	}
	if len(i.RecoveryActions) == 0 {
		return errors.New("maat memory: incident has no recovery action")
	}
	for _, action := range i.RecoveryActions {
		if err := action.validate(); err != nil {
			return err
		}
	}
	switch i.Status {
	case IncidentActive:
		if i.SuccessorKey != "" {
			return errors.New("maat memory: active incident cannot name successor")
		}
	case IncidentSuperseded, IncidentRetired:
		if !validDigest(i.SuccessorKey) {
			return errors.New("maat memory: closed incident requires successor key")
		}
	default:
		return errors.New("maat memory: invalid incident status")
	}
	return nil
}

type PreflightReceipt struct {
	Schema                 string            `json:"schema"`
	Action                 Scope             `json:"action"`
	ActionManifestSHA256   string            `json:"action_manifest_sha256"`
	RegistrySnapshotSHA256 string            `json:"registry_snapshot_sha256"`
	EvaluatedGuards        []GuardEvaluation `json:"evaluated_guards"`
	MeasuredChecks         []CheckOutcome    `json:"measured_checks"`
	Decision               PreflightDecision `json:"decision"`
	IncidentKeys           []string          `json:"incident_keys"`
	RecoveryActions        []RecoveryAction  `json:"recovery_actions"`
	RecoveryReference      string            `json:"recovery_reference"`
	EvaluatedAtUTC         time.Time         `json:"evaluated_at_utc"`
}

type GuardEvaluation struct{ ID, Version, SHA256 string }
type CheckOutcome struct{ IncidentKey, EvidenceSHA256, Status, Outcome string }

// Store holds retained no-follow descriptors for the root and its two
// append-only namespaces. It never follows evidence or incident leaf links.
type Store struct {
	rootFD       int
	evidenceFD   int
	incidentFD   int
	transitionFD int
	writesFD     int
	lockFD       int
}

// writeIntent is a tiny, create-only write-ahead record.  It is durable before
// the governed leaf is created; a crash or persistence error therefore leaves
// an explicit unresolved record instead of a plausible clean registry.
type writeIntent struct {
	Schema     string `json:"schema"`
	Key        string `json:"key"`
	Namespace  string `json:"namespace"`
	Leaf       string `json:"leaf"`
	SHA256     string `json:"sha256"`
	ByteLength int    `json:"byte_length"`
}

type writeIntentDigest struct {
	Schema     string `json:"schema"`
	Namespace  string `json:"namespace"`
	Leaf       string `json:"leaf"`
	SHA256     string `json:"sha256"`
	ByteLength int    `json:"byte_length"`
}

type writeCommit struct {
	Schema    string `json:"schema"`
	IntentKey string `json:"intent_key"`
}

func newWriteIntent(namespace, leaf string, expected []byte) (writeIntent, error) {
	if namespace != "evidence" && namespace != "incidents" && namespace != "transitions" {
		return writeIntent{}, errors.New("maat memory: invalid write namespace")
	}
	if leaf == "" || filepath.Base(leaf) != leaf || len(expected) == 0 || len(expected) > maxMemoryObjectBytes {
		return writeIntent{}, errors.New("maat memory: invalid write intent")
	}
	sum := sha256.Sum256(expected)
	d := writeIntentDigest{Schema: MemorySchema, Namespace: namespace, Leaf: leaf, SHA256: hex.EncodeToString(sum[:]), ByteLength: len(expected)}
	b, err := json.Marshal(d)
	if err != nil {
		return writeIntent{}, err
	}
	key := sha256.Sum256(b)
	return writeIntent{Schema: d.Schema, Key: hex.EncodeToString(key[:]), Namespace: d.Namespace, Leaf: d.Leaf, SHA256: d.SHA256, ByteLength: d.ByteLength}, nil
}

func (i writeIntent) validate() error {
	if i.Schema != MemorySchema || !validDigest(i.Key) || !validDigest(i.SHA256) || i.ByteLength <= 0 || i.ByteLength > maxMemoryObjectBytes {
		return errors.New("maat memory: invalid write intent envelope")
	}
	if (i.Namespace != "evidence" && i.Namespace != "incidents" && i.Namespace != "transitions") || i.Leaf == "" || filepath.Base(i.Leaf) != i.Leaf {
		return errors.New("maat memory: invalid write intent target")
	}
	d := writeIntentDigest{Schema: i.Schema, Namespace: i.Namespace, Leaf: i.Leaf, SHA256: i.SHA256, ByteLength: i.ByteLength}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != i.Key {
		return errors.New("maat memory: write intent key does not bind target")
	}
	return nil
}

func OpenStore(root string) (*Store, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("maat memory: store root must be absolute")
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("maat memory: open store root: %w", err)
	}
	closeRoot := true
	defer func() {
		if closeRoot {
			_ = unix.Close(rootFD)
		}
	}()
	if err := requireDirectory(rootFD); err != nil {
		return nil, err
	}
	evidenceFD, err := openOrCreateDirectory(rootFD, "evidence")
	if err != nil {
		return nil, err
	}
	closeEvidence := true
	defer func() {
		if closeEvidence {
			_ = unix.Close(evidenceFD)
		}
	}()
	incidentFD, err := openOrCreateDirectory(rootFD, "incidents")
	if err != nil {
		return nil, err
	}
	closeIncident := true
	defer func() {
		if closeIncident {
			_ = unix.Close(incidentFD)
		}
	}()
	transitionFD, err := openOrCreateDirectory(rootFD, "transitions")
	if err != nil {
		return nil, err
	}
	closeTransition := true
	defer func() {
		if closeTransition {
			_ = unix.Close(transitionFD)
		}
	}()
	writesFD, err := openOrCreateDirectory(rootFD, "writes")
	if err != nil {
		return nil, err
	}
	closeWrites := true
	defer func() {
		if closeWrites {
			_ = unix.Close(writesFD)
		}
	}()
	lockFD, err := unix.Openat(rootFD, ".lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("maat memory: open registry lock: %w", err)
	}
	closeLock := true
	defer func() {
		if closeLock {
			_ = unix.Close(lockFD)
		}
	}()
	if err := requireRegular(lockFD); err != nil {
		return nil, err
	}
	if err := unix.Fsync(rootFD); err != nil {
		return nil, fmt.Errorf("maat memory: fsync lock directory: %w", err)
	}
	closeRoot, closeEvidence, closeIncident, closeTransition, closeWrites, closeLock = false, false, false, false, false, false
	return &Store{rootFD: rootFD, evidenceFD: evidenceFD, incidentFD: incidentFD, transitionFD: transitionFD, writesFD: writesFD, lockFD: lockFD}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	var first error
	for _, fd := range []*int{&s.lockFD, &s.writesFD, &s.transitionFD, &s.incidentFD, &s.evidenceFD, &s.rootFD} {
		if *fd >= 0 {
			if err := unix.Close(*fd); err != nil && first == nil {
				first = err
			}
			*fd = -1
		}
	}
	return first
}

func (s *Store) PutEvidence(data []byte) (string, error) {
	if s == nil || s.evidenceFD < 0 || len(data) == 0 || len(data) > maxMemoryObjectBytes {
		return "", errors.New("maat memory: invalid evidence bytes")
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if err := s.lock(unix.LOCK_EX); err != nil {
		return "", err
	}
	defer s.unlock()
	if err := s.createOrVerify("evidence", s.evidenceFD, digest, data); err != nil {
		return "", fmt.Errorf("maat memory: store evidence: %w", err)
	}
	return digest, nil
}

func (s *Store) Append(incident Incident) error {
	if s == nil || s.incidentFD < 0 {
		return errors.New("maat memory: store is closed")
	}
	if err := incident.validate(); err != nil {
		return err
	}
	// Introduction records are the only records accepted by this primitive.
	// A terminal status needs a predecessor-bound event written by the dedicated
	// transition path; accepting one here would let an arbitrary caller hide an
	// active incident merely by supplying a digest-shaped successor string.
	if incident.Status != IncidentActive {
		return errors.New("maat memory: status transitions require a predecessor-bound transition record")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.verifyEvidence(incident.EvidenceSHA256); err != nil {
		return fmt.Errorf("maat memory: incident evidence unavailable: %w", err)
	}
	data, err := json.Marshal(incident)
	if err != nil {
		return fmt.Errorf("maat memory: marshal incident: %w", err)
	}
	if err := s.createOrVerify("incidents", s.incidentFD, incident.Key+".json", data); err != nil {
		return fmt.Errorf("maat memory: append incident: %w", err)
	}
	return nil
}

// Transition records a terminal outcome for an existing incident.  It is
// deliberately narrow: terminal events cannot be supplied directly, cannot
// fork, and cannot be applied to a substituted or unverifiable predecessor.
func (s *Store) Transition(predecessorKey string, status IncidentStatus) (IncidentTransition, error) {
	if s == nil || s.incidentFD < 0 || s.transitionFD < 0 {
		return IncidentTransition{}, errors.New("maat memory: store is closed")
	}
	if !validDigest(predecessorKey) {
		return IncidentTransition{}, errors.New("maat memory: invalid predecessor key")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return IncidentTransition{}, err
	}
	defer s.unlock()
	data, err := readExact(s.incidentFD, predecessorKey+".json")
	if err != nil {
		return IncidentTransition{}, fmt.Errorf("maat memory: read transition predecessor: %w", err)
	}
	var predecessor Incident
	if err := json.Unmarshal(data, &predecessor); err != nil || predecessor.validate() != nil || predecessor.Key != predecessorKey || predecessor.Status != IncidentActive {
		return IncidentTransition{}, errors.New("maat memory: transition predecessor is invalid")
	}
	if err := s.verifyEvidence(predecessor.EvidenceSHA256); err != nil {
		return IncidentTransition{}, fmt.Errorf("maat memory: transition evidence unavailable: %w", err)
	}
	names, err := readDirectoryNames(s.transitionFD)
	if err != nil {
		return IncidentTransition{}, err
	}
	for _, name := range names {
		if !strings.HasSuffix(name, ".json") || !validDigest(strings.TrimSuffix(name, ".json")) {
			return IncidentTransition{}, errors.New("maat memory: malformed transition namespace")
		}
		priorBytes, err := readExact(s.transitionFD, name)
		if err != nil {
			return IncidentTransition{}, err
		}
		var prior IncidentTransition
		if err := json.Unmarshal(priorBytes, &prior); err != nil || prior.Key+".json" != name {
			return IncidentTransition{}, errors.New("maat memory: invalid transition record")
		}
		if prior.PredecessorKey == predecessorKey {
			if err := prior.validateAgainst(predecessor); err != nil {
				return IncidentTransition{}, err
			}
			return IncidentTransition{}, errors.New("maat memory: incident already has a terminal transition")
		}
	}
	transition, err := newIncidentTransition(predecessor, status)
	if err != nil {
		return IncidentTransition{}, err
	}
	encoded, err := json.Marshal(transition)
	if err != nil {
		return IncidentTransition{}, fmt.Errorf("maat memory: marshal transition: %w", err)
	}
	if err := s.createOrVerify("transitions", s.transitionFD, transition.Key+".json", encoded); err != nil {
		return IncidentTransition{}, fmt.Errorf("maat memory: append transition: %w", err)
	}
	return transition, nil
}

func (s *Store) Preflight(action Scope) (PreflightReceipt, error) {
	if s == nil || s.incidentFD < 0 {
		return PreflightReceipt{}, errors.New("maat memory: store is closed")
	}
	if err := action.validate(); err != nil {
		return PreflightReceipt{}, err
	}
	if err := s.lock(unix.LOCK_SH); err != nil {
		return PreflightReceipt{}, err
	}
	defer s.unlock()
	journalLines, err := s.verifyWriteJournal()
	if err != nil {
		return PreflightReceipt{Schema: MemorySchema, Action: action, Decision: PreflightUnverifiable, EvaluatedAtUTC: time.Now().UTC()}, err
	}
	names, err := readDirectoryNames(s.incidentFD)
	if err != nil {
		return PreflightReceipt{}, err
	}
	actionBytes, _ := json.Marshal(action)
	actionDigest := sha256.Sum256(actionBytes)
	receipt := PreflightReceipt{Schema: MemorySchema, Action: action, ActionManifestSHA256: hex.EncodeToString(actionDigest[:]), Decision: PreflightPass, EvaluatedAtUTC: time.Now().UTC()}
	snapshot := sha256.New()
	_, _ = snapshot.Write([]byte(MemorySchema + "\n"))
	for _, line := range journalLines {
		_, _ = snapshot.Write([]byte(line + "\n"))
	}
	incidents := make(map[string]Incident, len(names))
	for _, name := range names {
		if !strings.HasSuffix(name, ".json") || !validDigest(strings.TrimSuffix(name, ".json")) {
			receipt.Decision = PreflightUnverifiable
			return receipt, errors.New("maat memory: malformed incident namespace")
		}
		data, err := readExact(s.incidentFD, name)
		if err != nil {
			receipt.Decision = PreflightUnverifiable
			return receipt, err
		}
		var incident Incident
		if err := json.Unmarshal(data, &incident); err != nil || incident.validate() != nil || name != incident.Key+".json" {
			receipt.Decision = PreflightUnverifiable
			return receipt, errors.New("maat memory: invalid incident record")
		}
		if err := s.verifyEvidence(incident.EvidenceSHA256); err != nil {
			receipt.Decision = PreflightUnverifiable
			return receipt, fmt.Errorf("maat memory: incident evidence unavailable: %w", err)
		}
		fileDigest := sha256.Sum256(data)
		_, _ = snapshot.Write([]byte("incidents/" + name + "\x00" + hex.EncodeToString(fileDigest[:]) + "\n"))
		incidents[incident.Key] = incident
	}
	transitionNames, err := readDirectoryNames(s.transitionFD)
	if err != nil {
		receipt.Decision = PreflightUnverifiable
		return receipt, err
	}
	terminal := make(map[string]IncidentTransition, len(transitionNames))
	for _, name := range transitionNames {
		if !strings.HasSuffix(name, ".json") || !validDigest(strings.TrimSuffix(name, ".json")) {
			receipt.Decision = PreflightUnverifiable
			return receipt, errors.New("maat memory: malformed transition namespace")
		}
		data, err := readExact(s.transitionFD, name)
		if err != nil {
			receipt.Decision = PreflightUnverifiable
			return receipt, err
		}
		var transition IncidentTransition
		if err := json.Unmarshal(data, &transition); err != nil {
			receipt.Decision = PreflightUnverifiable
			return receipt, errors.New("maat memory: invalid transition record")
		}
		predecessor, ok := incidents[transition.PredecessorKey]
		if !ok || transition.Key+".json" != name || transition.validateAgainst(predecessor) != nil {
			receipt.Decision = PreflightUnverifiable
			return receipt, errors.New("maat memory: invalid transition record")
		}
		if _, duplicate := terminal[transition.PredecessorKey]; duplicate {
			receipt.Decision = PreflightUnverifiable
			return receipt, errors.New("maat memory: incident has multiple terminal transitions")
		}
		terminal[transition.PredecessorKey] = transition
		fileDigest := sha256.Sum256(data)
		_, _ = snapshot.Write([]byte("transitions/" + name + "\x00" + hex.EncodeToString(fileDigest[:]) + "\n"))
	}
	seenGuards := map[string]bool{}
	for _, key := range sortedIncidentKeys(incidents) {
		incident := incidents[key]
		guardKey := incident.GuardID + "\x00" + incident.GuardVersion
		if !seenGuards[guardKey] {
			guardDigest := sha256.Sum256([]byte(guardKey))
			receipt.EvaluatedGuards = append(receipt.EvaluatedGuards, GuardEvaluation{ID: incident.GuardID, Version: incident.GuardVersion, SHA256: hex.EncodeToString(guardDigest[:])})
			seenGuards[guardKey] = true
		}
		status := incident.Status
		if transition, ok := terminal[incident.Key]; ok {
			status = transition.Status
		}
		outcome := "out-of-scope"
		if status == IncidentActive && incident.Scope.matches(action) {
			outcome = "active-incident-matched"
			receipt.Decision = PreflightReject
			receipt.IncidentKeys = append(receipt.IncidentKeys, incident.Key)
			receipt.RecoveryActions = append(receipt.RecoveryActions, incident.RecoveryActions...)
		}
		if status != IncidentActive {
			outcome = "terminal-transition"
		}
		receipt.MeasuredChecks = append(receipt.MeasuredChecks, CheckOutcome{IncidentKey: incident.Key, EvidenceSHA256: incident.EvidenceSHA256, Status: string(status), Outcome: outcome})
	}
	sort.Strings(receipt.IncidentKeys)
	sort.Slice(receipt.EvaluatedGuards, func(i, j int) bool {
		return receipt.EvaluatedGuards[i].ID+receipt.EvaluatedGuards[i].Version < receipt.EvaluatedGuards[j].ID+receipt.EvaluatedGuards[j].Version
	})
	snapshotDigest := snapshot.Sum(nil)
	receipt.RegistrySnapshotSHA256 = hex.EncodeToString(snapshotDigest)
	if len(receipt.RecoveryActions) > 0 {
		actions, _ := json.Marshal(receipt.RecoveryActions)
		recoveryDigest := sha256.Sum256(actions)
		receipt.RecoveryReference = "maat-recovery:" + hex.EncodeToString(recoveryDigest[:])
	}
	return receipt, nil
}

func sortedIncidentKeys(incidents map[string]Incident) []string {
	keys := make([]string, 0, len(incidents))
	for key := range incidents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *Store) verifyEvidence(expectedDigest string) error {
	data, err := readExact(s.evidenceFD, expectedDigest)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expectedDigest {
		return errors.New("maat memory: evidence bytes do not match digest")
	}
	return nil
}

func openOrCreateDirectory(parentFD int, name string) (int, error) {
	if err := unix.Mkdirat(parentFD, name, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return -1, fmt.Errorf("maat memory: create %s: %w", name, err)
	}
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("maat memory: open %s: %w", name, err)
	}
	if err := requireDirectory(fd); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	if err := unix.Fsync(parentFD); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("maat memory: fsync parent: %w", err)
	}
	return fd, nil
}

func requireDirectory(fd int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("maat memory: stat directory: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("maat memory: expected directory")
	}
	return nil
}

func requireRegular(fd int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("maat memory: stat regular object: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return errors.New("maat memory: expected singly linked regular object")
	}
	return nil
}

func (s *Store) lock(mode int) error {
	if s == nil || s.lockFD < 0 {
		return errors.New("maat memory: store is closed")
	}
	if err := unix.Flock(s.lockFD, mode); err != nil {
		return fmt.Errorf("maat memory: lock registry: %w", err)
	}
	return nil
}

func (s *Store) unlock() {
	if s != nil && s.lockFD >= 0 {
		_ = unix.Flock(s.lockFD, unix.LOCK_UN)
	}
}

// createOrVerifyRaw is only used for the write journal's own create-only
// records. Every governed evidence, incident, and transition leaf goes through
// Store.createOrVerify so an intent survives a crash before the leaf exists.
func createOrVerifyRaw(parentFD int, name string, expected []byte) error {
	fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if errors.Is(err, unix.EEXIST) {
		actual, readErr := readExact(parentFD, name)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(actual, expected) {
			return errors.New("create-only object already exists with different bytes")
		}
		return nil
	}
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = unix.Close(fd)
		}
	}()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return errors.New("created object identity is invalid")
	}
	for written := 0; written < len(expected); {
		n, writeErr := unix.Write(fd, expected[written:])
		if writeErr != nil {
			return writeErr
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		written += n
	}
	if err := unix.Fsync(fd); err != nil {
		return err
	}
	if err := unix.Close(fd); err != nil {
		return err
	}
	closed = true
	if err := unix.Fsync(parentFD); err != nil {
		return err
	}
	actual, err := readExact(parentFD, name)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, expected) {
		return errors.New("created object readback differs")
	}
	return nil
}

func (s *Store) createOrVerify(namespace string, parentFD int, name string, expected []byte) error {
	if s == nil || s.writesFD < 0 {
		return errors.New("maat memory: store is closed")
	}
	intent, err := newWriteIntent(namespace, name, expected)
	if err != nil {
		return err
	}
	if _, err := s.verifyWriteJournalAllow(intent.Key); err != nil {
		return err
	}
	intentBytes, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("maat memory: marshal write intent: %w", err)
	}
	if err := createOrVerifyRaw(s.writesFD, intent.Key+".prepared.json", intentBytes); err != nil {
		return fmt.Errorf("maat memory: persist write intent: %w", err)
	}
	if err := createOrVerifyRaw(parentFD, name, expected); err != nil {
		// The durable prepared record is the recovery debt.  It binds the only
		// permitted target and expected bytes, and prevents this registry from
		// being accepted until a retry proves completion or an explicit resolver
		// handles the residue.
		return fmt.Errorf("maat memory: governed write remains prepared: %w", err)
	}
	commitBytes, err := json.Marshal(writeCommit{Schema: MemorySchema, IntentKey: intent.Key})
	if err != nil {
		return fmt.Errorf("maat memory: marshal write commit: %w", err)
	}
	if err := createOrVerifyRaw(s.writesFD, intent.Key+".committed.json", commitBytes); err != nil {
		return fmt.Errorf("maat memory: governed write commit remains prepared: %w", err)
	}
	return nil
}

func (s *Store) verifyWriteJournal() ([]string, error) {
	return s.verifyWriteJournalAllow("")
}

// verifyWriteJournalAllow is used only by a retry of the exact prepared
// intent.  It never permits a different pending write to be bypassed.
func (s *Store) verifyWriteJournalAllow(allowedPreparedKey string) ([]string, error) {
	if s == nil || s.writesFD < 0 {
		return nil, errors.New("maat memory: store is closed")
	}
	names, err := readDirectoryNames(s.writesFD)
	if err != nil {
		return nil, err
	}
	prepared := make(map[string]writeIntent)
	committed := make(map[string]bool)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		var key string
		switch {
		case strings.HasSuffix(name, ".prepared.json"):
			key = strings.TrimSuffix(name, ".prepared.json")
			if !validDigest(key) {
				return nil, errors.New("maat memory: malformed prepared write record")
			}
			data, err := readExact(s.writesFD, name)
			if err != nil {
				return nil, err
			}
			var intent writeIntent
			if err := json.Unmarshal(data, &intent); err != nil || intent.Key != key || intent.validate() != nil {
				return nil, errors.New("maat memory: invalid prepared write record")
			}
			prepared[key] = intent
			digest := sha256.Sum256(data)
			lines = append(lines, "writes/"+name+"\x00"+hex.EncodeToString(digest[:]))
		case strings.HasSuffix(name, ".committed.json"):
			key = strings.TrimSuffix(name, ".committed.json")
			if !validDigest(key) {
				return nil, errors.New("maat memory: malformed committed write record")
			}
			data, err := readExact(s.writesFD, name)
			if err != nil {
				return nil, err
			}
			var commit writeCommit
			if err := json.Unmarshal(data, &commit); err != nil || commit.Schema != MemorySchema || commit.IntentKey != key {
				return nil, errors.New("maat memory: invalid committed write record")
			}
			committed[key] = true
			digest := sha256.Sum256(data)
			lines = append(lines, "writes/"+name+"\x00"+hex.EncodeToString(digest[:]))
		default:
			return nil, errors.New("maat memory: unrecognized write journal record")
		}
	}
	for key, intent := range prepared {
		if !committed[key] {
			if key == allowedPreparedKey {
				continue
			}
			return nil, fmt.Errorf("maat memory: unresolved prepared write %s", key)
		}
		fd, err := s.namespaceFD(intent.Namespace)
		if err != nil {
			return nil, err
		}
		data, err := readExact(fd, intent.Leaf)
		if err != nil {
			return nil, fmt.Errorf("maat memory: committed target unreadable: %w", err)
		}
		sum := sha256.Sum256(data)
		if len(data) != intent.ByteLength || hex.EncodeToString(sum[:]) != intent.SHA256 {
			return nil, errors.New("maat memory: committed target differs from intent")
		}
	}
	for key := range committed {
		if _, ok := prepared[key]; !ok {
			return nil, errors.New("maat memory: write commit has no intent")
		}
	}
	sort.Strings(lines)
	return lines, nil
}

func (s *Store) namespaceFD(namespace string) (int, error) {
	switch namespace {
	case "evidence":
		return s.evidenceFD, nil
	case "incidents":
		return s.incidentFD, nil
	case "transitions":
		return s.transitionFD, nil
	default:
		return -1, errors.New("maat memory: unknown write namespace")
	}
}

func readExact(parentFD int, name string) ([]byte, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Size < 0 || stat.Size > maxMemoryObjectBytes {
		return nil, errors.New("maat memory: invalid stored object identity")
	}
	buf := make([]byte, stat.Size)
	for read := 0; read < len(buf); {
		n, readErr := unix.Read(fd, buf[read:])
		if readErr != nil {
			return nil, readErr
		}
		if n == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		read += n
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return nil, err
	}
	if after.Dev != stat.Dev || after.Ino != stat.Ino || after.Size != stat.Size || after.Mode != stat.Mode || after.Nlink != stat.Nlink {
		return nil, errors.New("maat memory: stored object changed while read")
	}
	return buf, nil
}

func readDirectoryNames(fd int) ([]string, error) {
	dup, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(dup), "maat-memory-directory")
	if file == nil {
		_ = unix.Close(dup)
		return nil, errors.New("maat memory: duplicate directory descriptor unavailable")
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == "." || entry.Name() == ".." {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
