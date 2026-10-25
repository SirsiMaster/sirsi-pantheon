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
	"time"

	"golang.org/x/sys/unix"
)

var ErrHistoricalReceiptUnavailable = errors.New("maat memory: historical receipt bytes unavailable")

const PreflightReceiptReadSchema = "sirsi.maat.failure-memory-receipt-read.v1"

// ReceiptStoreBinding distinguishes initialized default custody from a legacy
// explicitly selected private root. Neither is an execution grant.
type ReceiptStoreBinding struct {
	State     string `json:"state"`
	StoreUUID string `json:"store_uuid,omitempty"`
}

type PreflightReceiptReference struct {
	ReceiptSHA256           string              `json:"receipt_sha256"`
	ActionSHA256            string              `json:"action_sha256"`
	RegistrySHA256          string              `json:"registry_sha256"`
	RegistrySnapshotVersion int                 `json:"registry_snapshot_version"`
	Action                  Scope               `json:"action"`
	StoreBinding            ReceiptStoreBinding `json:"store_binding"`
}

// StoredPreflightReceipt cannot be constructed from a caller's receipt or JSON.
// It carries the exact bytes verified after the governed write/readback. A copy
// of the receipt is decoded on access so slices cannot mutate the handle.
// An in-process issuance seal prevents retention from becoming a raw receipt
// importer. It is deliberately absent from the canonical historical JSON.
type preflightReceiptOrigin struct {
	device    uint64
	inode     uint64
	storeUUID string
	canonical string
}

type StoredPreflightReceipt struct {
	canonical string
	reference PreflightReceiptReference
}

func (h StoredPreflightReceipt) Receipt() (PreflightReceipt, error) {
	r, err := decodePreflightReceipt([]byte(h.canonical))
	if err != nil {
		return PreflightReceipt{}, err
	}
	digest, err := r.Digest()
	if err != nil || digest != h.reference.ReceiptSHA256 {
		return PreflightReceipt{}, errors.New("maat memory: invalid stored receipt handle")
	}
	return r, nil
}

func (h StoredPreflightReceipt) Reference() PreflightReceiptReference { return h.reference }

type PreflightReceiptRead struct {
	Schema           string              `json:"schema"`
	ReceiptSHA256    string              `json:"receipt_sha256"`
	Receipt          PreflightReceipt    `json:"receipt"`
	StoreBinding     ReceiptStoreBinding `json:"store_binding"`
	ReadAtUTC        time.Time           `json:"read_at_utc"`
	Freshness        string              `json:"freshness"`
	RegistryRelation string              `json:"registry_relation"`
	ActionAuthorized bool                `json:"action_authorized"`
}

func (s *Store) receiptBinding() ReceiptStoreBinding {
	if validUUID(s.storeUUID) {
		return ReceiptStoreBinding{State: "initialized", StoreUUID: s.storeUUID}
	}
	return ReceiptStoreBinding{State: "legacy-explicit"}
}

// OpenExistingStore never creates namespaces, a lock, binding or genesis.
func OpenExistingStore(root string) (*Store, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("maat memory: existing store root must be absolute")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return openStoreFDExisting(fd)
}

// RetainPreflightReceipt accepts only a complete, locally recomputable v2
// assessment. WAL bookkeeping is excluded from semantic identity, never from
// validation. Persistence precedes the projection or any attempt admission.
func (s *Store) RetainPreflightReceipt(r PreflightReceipt) (StoredPreflightReceipt, error) {
	return s.retainPreflightReceiptWithWrite(r, s.createOrVerify)
}

// Per-call write seam exercises actual prepared/leaf interruption boundaries;
// production always uses the ordinary governed writer above.
func (s *Store) retainPreflightReceiptWithWrite(r PreflightReceipt, write func(string, int, string, []byte) error) (StoredPreflightReceipt, error) {
	if r.RegistrySnapshotVersion != 2 || r.validate() != nil {
		return StoredPreflightReceipt{}, errors.New("maat memory: retention requires a complete current v2 receipt")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return StoredPreflightReceipt{}, err
	}
	defer s.unlock()
	var identity unix.Stat_t
	if err := unix.Fstat(s.rootFD, &identity); err != nil {
		return StoredPreflightReceipt{}, err
	}
	issued, _ := json.Marshal(r)
	if r.localOrigin == nil || r.localOrigin.device != uint64(identity.Dev) || r.localOrigin.inode != uint64(identity.Ino) || r.localOrigin.storeUUID != s.storeUUID || r.localOrigin.canonical != string(issued) {
		return StoredPreflightReceipt{}, errors.New("maat memory: receipt was not issued by this local store assessment")
	}
	if _, err := decodePreflightReceipt(issued); err != nil {
		return StoredPreflightReceipt{}, err
	}
	sum := sha256.Sum256(issued)
	digest := hex.EncodeToString(sum[:])
	// Finish only this exact issued tuple BEFORE clean-WAL reassessment. A
	// changed registry or old observation can close historical persistence debt
	// but cannot obtain a newly eligible handle.
	resumed, err := s.resumePreparedPreflightLocked(digest, issued, true)
	if err != nil {
		return StoredPreflightReceipt{}, err
	}
	if time.Since(r.EvaluatedAtUTC) > 2*time.Minute || r.EvaluatedAtUTC.After(time.Now().UTC().Add(10*time.Second)) {
		if resumed {
			return StoredPreflightReceipt{}, errors.New("maat memory: exact receipt persistence recovered; fresh assessment is required before projection or admission")
		}
		return StoredPreflightReceipt{}, errors.New("maat memory: fresh assessment is required before projection or admission")
	}
	current, evalErr := s.preflightBoundLocked(r.Action, r.ActionManifestSHA256, resolveMemoryGuard)
	// An unsupported closed guard can yield a complete unverifiable receipt;
	// filesystem/WAL failures have no valid semantic snapshot and are refused.
	if evalErr != nil && (current.Decision != PreflightUnverifiable || !validDigest(current.RegistrySnapshotSHA256)) {
		return StoredPreflightReceipt{}, evalErr
	}
	current.EvaluatedAtUTC = r.EvaluatedAtUTC
	expected, err := json.Marshal(r)
	if err != nil {
		return StoredPreflightReceipt{}, err
	}
	actual, err := json.Marshal(current)
	if err != nil || !bytes.Equal(actual, expected) {
		return StoredPreflightReceipt{}, errors.New("maat memory: receipt no longer matches the complete local assessment")
	}
	if _, err := decodePreflightReceipt(expected); err != nil {
		return StoredPreflightReceipt{}, err
	}
	if err := write("evidence", s.evidenceFD, digest, expected); err != nil {
		return StoredPreflightReceipt{}, err
	}
	if _, err := s.readPreflightReceiptLocked(digest); err != nil {
		return StoredPreflightReceipt{}, err
	}
	return StoredPreflightReceipt{canonical: string(expected), reference: PreflightReceiptReference{
		ReceiptSHA256: digest, ActionSHA256: r.ActionManifestSHA256, RegistrySHA256: r.RegistrySnapshotSHA256,
		RegistrySnapshotVersion: r.RegistrySnapshotVersion, Action: r.Action, StoreBinding: s.receiptBinding(),
	}}, nil
}

func (s *Store) ReadPreflightReceipt(digest string) (PreflightReceiptRead, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return PreflightReceiptRead{}, err
	}
	defer s.unlock()
	return s.preflightReceiptReadLocked(digest)
}
func (s *Store) preflightReceiptReadLocked(digest string) (PreflightReceiptRead, error) {
	r, err := s.readPreflightReceiptLocked(digest)
	if err != nil {
		return PreflightReceiptRead{}, err
	}
	relation := "unverifiable"
	if r.RegistrySnapshotVersion == 2 {
		current, currentErr := s.preflightBoundLocked(r.Action, r.ActionManifestSHA256, resolveMemoryGuard)
		if validDigest(current.RegistrySnapshotSHA256) && (currentErr == nil || current.Decision == PreflightUnverifiable) {
			relation = "changed"
			if current.RegistrySnapshotSHA256 == r.RegistrySnapshotSHA256 {
				relation = "same"
			}
		}
	}
	return PreflightReceiptRead{Schema: PreflightReceiptReadSchema, ReceiptSHA256: digest, Receipt: r,
		StoreBinding: s.receiptBinding(), ReadAtUTC: time.Now().UTC(), Freshness: "historical", RegistryRelation: relation}, nil
}

func (s *Store) readPreflightReceiptLocked(digest string) (PreflightReceipt, error) {
	if !validDigest(digest) {
		return PreflightReceipt{}, errors.New("maat memory: invalid receipt digest")
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		return PreflightReceipt{}, err
	}
	data, err := readExact(s.evidenceFD, digest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PreflightReceipt{}, fmt.Errorf("%w: %w", ErrHistoricalReceiptUnavailable, err)
		}
		return PreflightReceipt{}, fmt.Errorf("maat memory: historical receipt read refused: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return PreflightReceipt{}, errors.New("maat memory: receipt digest mismatch")
	}
	if err := s.verifyCommittedBytes("evidence", digest, data); err != nil {
		return PreflightReceipt{}, err
	}
	return decodePreflightReceipt(data)
}

// Canonical comparison rejects duplicate/case-aliased keys, unknown fields,
// omitted fields, alternate encoding and trailing JSON without rewriting old
// null slices or the historical PascalCase nested keys.
func decodePreflightReceipt(data []byte) (PreflightReceipt, error) {
	if len(data) == 0 || len(data) > maxMemoryObjectBytes {
		return PreflightReceipt{}, errors.New("maat memory: invalid receipt size")
	}
	var r PreflightReceipt
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return PreflightReceipt{}, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return PreflightReceipt{}, errors.New("maat memory: trailing receipt bytes")
	}
	canonical, err := json.Marshal(r)
	if err != nil || !bytes.Equal(canonical, data) || r.validate() != nil || (r.RegistrySnapshotVersion != 0 && r.RegistrySnapshotVersion != 2) {
		return PreflightReceipt{}, errors.New("maat memory: invalid or noncanonical historical receipt")
	}
	if r.Decision == PreflightPass && (len(r.IncidentKeys) != 0 || len(r.MeasuredChecks) != 0 || len(r.EvaluatedGuards) != 0 || len(r.RecoveryActions) != 0 || r.RecoveryReference != "") {
		return PreflightReceipt{}, errors.New("maat memory: contradictory passing receipt")
	}
	if r.Decision == PreflightReject && (len(r.IncidentKeys) == 0 || len(r.MeasuredChecks) == 0 || len(r.EvaluatedGuards) == 0) {
		return PreflightReceipt{}, errors.New("maat memory: incomplete rejecting receipt")
	}
	for _, g := range r.EvaluatedGuards {
		if g.ID != ActiveIncidentGuardID || g.Version != ActiveIncidentGuardVersion {
			return PreflightReceipt{}, errors.New("maat memory: unsupported evaluated guard")
		}
	}
	for _, c := range r.MeasuredChecks {
		if c.Status != "active" || c.Outcome != "active-incident-recurrence-rejected" {
			return PreflightReceipt{}, errors.New("maat memory: unsupported check outcome")
		}
	}
	previous := ""
	for _, key := range r.IncidentKeys {
		if key <= previous {
			return PreflightReceipt{}, errors.New("maat memory: unordered or duplicate incident keys")
		}
		previous = key
	}
	if len(r.RecoveryActions) > 0 {
		b, _ := json.Marshal(r.RecoveryActions)
		sum := sha256.Sum256(b)
		if r.RecoveryReference != "maat-recovery:"+hex.EncodeToString(sum[:]) {
			return PreflightReceipt{}, errors.New("maat memory: recovery reference mismatch")
		}
	} else if r.RecoveryReference != "" {
		return PreflightReceipt{}, errors.New("maat memory: recovery reference without actions")
	}
	knownIncidents := make(map[string]bool, len(r.IncidentKeys))
	for _, key := range r.IncidentKeys {
		knownIncidents[key] = true
	}
	seenChecks := map[string]bool{}
	for _, check := range r.MeasuredChecks {
		if !knownIncidents[check.IncidentKey] || seenChecks[check.IncidentKey] {
			return PreflightReceipt{}, errors.New("maat memory: duplicate or unbound measured check")
		}
		seenChecks[check.IncidentKey] = true
	}
	if r.Decision == PreflightReject && len(r.MeasuredChecks) != len(r.IncidentKeys) {
		return PreflightReceipt{}, errors.New("maat memory: rejecting receipt omits measured incidents")
	}
	previousGuard := ""
	for _, guard := range r.EvaluatedGuards {
		key := guard.ID + "\x00" + guard.Version
		if key <= previousGuard {
			return PreflightReceipt{}, errors.New("maat memory: duplicate or unordered evaluated guard")
		}
		previousGuard = key
	}
	for _, key := range r.IncidentKeys {
		if !validDigest(key) {
			return PreflightReceipt{}, errors.New("maat memory: invalid receipt incident key")
		}
	}
	for _, a := range r.RecoveryActions {
		if a.validate() != nil {
			return PreflightReceipt{}, errors.New("maat memory: invalid receipt recovery action")
		}
	}
	return r, nil
}

// Requires the exact canonical WAL pair for these bytes, not merely an
// unrelated valid journal. Callers hold the shared or exclusive store lock.
func (s *Store) verifyCommittedBytes(namespace, leaf string, data []byte) error {
	intent, err := newWriteIntent(namespace, leaf, data)
	if err != nil {
		return err
	}
	prepared, _ := json.Marshal(intent)
	committed, _ := json.Marshal(writeCommit{Schema: MemorySchema, IntentKey: intent.Key})
	for _, record := range []struct {
		name     string
		expected []byte
	}{{intent.Key + ".prepared.json", prepared}, {intent.Key + ".committed.json", committed}} {
		actual, err := readExact(s.writesFD, record.name)
		if err != nil || !bytes.Equal(actual, record.expected) {
			return errors.New("maat memory: exact committed object custody is unavailable")
		}
	}
	return nil
}

func (r PreflightReceiptReference) EvidenceReference() string {
	return "maat-failure-memory:receipt-sha256=" + r.ReceiptSHA256 + ";action-sha256=" + r.ActionSHA256 + ";registry-sha256=" + r.RegistrySHA256
}
func (r PreflightReceiptReference) validate() error {
	if !validDigest(r.ReceiptSHA256) || !validDigest(r.ActionSHA256) || !validDigest(r.RegistrySHA256) || r.RegistrySnapshotVersion != 2 || r.Action.validate() != nil {
		return errors.New("maat memory: invalid typed receipt reference")
	}
	if (r.StoreBinding.State == "initialized" && validUUID(r.StoreBinding.StoreUUID)) || (r.StoreBinding.State == "legacy-explicit" && r.StoreBinding.StoreUUID == "") {
		return nil
	}
	return errors.New("maat memory: invalid receipt store binding")
}

// resumePreparedPreflightLocked permits one exact prepared key, never an
// arbitrary WAL exception. Only normal issuance-bound retry may supply a leaf
// that was not yet written. Reopened recovery must read complete retained bytes.
func (s *Store) resumePreparedPreflightLocked(digest string, data []byte, allowMissingTarget bool) (bool, error) {
	intent, err := newWriteIntent("evidence", digest, data)
	if err != nil {
		return false, err
	}
	expected, _ := json.Marshal(intent)
	prepared, err := readExact(s.writesFD, intent.Key+".prepared.json")
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !bytes.Equal(prepared, expected) {
		return false, errors.New("maat memory: prepared receipt intent differs from exact target and bytes")
	}
	commit, err := readExact(s.writesFD, intent.Key+".committed.json")
	if err == nil {
		expectedCommit, _ := json.Marshal(writeCommit{Schema: MemorySchema, IntentKey: intent.Key})
		if !bytes.Equal(commit, expectedCommit) {
			return false, errors.New("maat memory: invalid receipt commit")
		}
		return false, nil
	}
	if !errors.Is(err, unix.ENOENT) {
		return false, err
	}
	if _, err := s.verifyWriteJournalAllow(intent.Key); err != nil {
		return false, err
	}
	target, err := readExact(s.evidenceFD, digest)
	if errors.Is(err, unix.ENOENT) && allowMissingTarget {
		target = data
	} else if err != nil {
		return false, err
	}
	if !bytes.Equal(target, data) {
		return false, errors.New("maat memory: interrupted receipt target is missing, truncated or substituted")
	}
	if err := s.createOrVerify("evidence", s.evidenceFD, digest, data); err != nil {
		return false, err
	}
	if _, err := s.readPreflightReceiptLocked(digest); err != nil {
		return false, err
	}
	return true, nil
}

// RecoverPreflightReceipt completes a fully retained exact prepared object,
// or verifies its already completed retry. It does not import bytes, reconstruct
// a missing leaf, change its time, project a decision or create local issuance.
func (s *Store) RecoverPreflightReceipt(digest string) (PreflightReceiptRead, error) {
	if !validDigest(digest) {
		return PreflightReceiptRead{}, errors.New("maat memory: invalid historical recovery digest")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return PreflightReceiptRead{}, err
	}
	defer s.unlock()
	data, err := readExact(s.evidenceFD, digest)
	if err != nil {
		return PreflightReceiptRead{}, fmt.Errorf("maat memory: complete retained receipt bytes required for recovery: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return PreflightReceiptRead{}, errors.New("maat memory: retained recovery target digest mismatch")
	}
	receipt, err := decodePreflightReceipt(data)
	if err != nil || receipt.RegistrySnapshotVersion != 2 {
		return PreflightReceiptRead{}, errors.New("maat memory: recovery requires the exact canonical v2 receipt")
	}
	intent, err := newWriteIntent("evidence", digest, data)
	if err != nil {
		return PreflightReceiptRead{}, err
	}
	prepared, err := readExact(s.writesFD, intent.Key+".prepared.json")
	if err != nil {
		return PreflightReceiptRead{}, err
	}
	expected, _ := json.Marshal(intent)
	if !bytes.Equal(prepared, expected) {
		return PreflightReceiptRead{}, errors.New("maat memory: recovery has no exact retained receipt intent")
	}
	if _, err := s.resumePreparedPreflightLocked(digest, data, false); err != nil {
		return PreflightReceiptRead{}, err
	}
	return s.preflightReceiptReadLocked(digest)
}
