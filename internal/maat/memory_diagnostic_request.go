package maat

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const DiagnosticRequestSchema = "sirsi.maat.diagnostic-request.v1"
const diagnosticRequestSuffix = ".diagnostic-request.json"
const diagnosticRequestStartSuffix = ".diagnostic-request-start.json"
const diagnosticRequestBindingSuffix = ".diagnostic-request-binding.json"
const diagnosticRequestFenceSuffix = ".diagnostic-request-fence.json"

// A correlation key is not permission. Each invocation still requires the
// ordinary local actor/store, full preflight and retained exact receipt.
type DiagnosticRequest struct {
	Schema    string `json:"schema"`
	ID        string `json:"id"`
	UID       uint32 `json:"uid"`
	StoreUUID string `json:"store_uuid"`
}
type diagnosticRequestBinding struct {
	Request  DiagnosticRequest `json:"request"`
	RecordID string            `json:"record_id"`
}
type DiagnosticRequestRead struct {
	Request DiagnosticRequest       `json:"request"`
	State   string                  `json:"state"`
	Intent  *DiagnosticRecordIntent `json:"intent,omitempty"`
	Result  *DiagnosticRecordResult `json:"result,omitempty"`
}

func ValidDiagnosticRequestID(id string) bool {
	return validUUID(id) && strings.ToLower(id) == id && strings.Count(id, "-") == 4
}

func (s *Store) diagnosticRequest(id string) (DiagnosticRequest, error) {
	if !ValidDiagnosticRequestID(id) || !validUUID(s.storeUUID) {
		return DiagnosticRequest{}, errors.New("maat diagnostic: canonical request UUID and initialized custody required")
	}
	return DiagnosticRequest{DiagnosticRequestSchema, id, uint32(os.Geteuid()), s.storeUUID}, nil
}
func (s *Store) writeDiagnosticRequest(leaf string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.createOrVerify("attempts", s.attemptsFD, leaf, raw)
}
func (s *Store) readDiagnosticRequestLocked(id string) (DiagnosticRequest, error) {
	expected, err := s.diagnosticRequest(id)
	if err != nil {
		return expected, err
	}
	raw, err := readExact(s.attemptsFD, id+diagnosticRequestSuffix)
	if err != nil {
		return expected, err
	}
	var value DiagnosticRequest
	if decodeDiagnosticCanonical(raw, &value) != nil || value != expected {
		return value, errors.New("maat diagnostic: request actor/store/identity differs")
	}
	return value, nil
}
func (s *Store) requestBindingLocked(request DiagnosticRequest) (diagnosticRequestBinding, error) {
	var value diagnosticRequestBinding
	raw, err := readExact(s.attemptsFD, request.ID+diagnosticRequestBindingSuffix)
	if err != nil {
		return value, err
	}
	if decodeDiagnosticCanonical(raw, &value) != nil || value.Request != request || !validDigest(value.RecordID) {
		return value, errors.New("maat diagnostic: invalid request/record binding")
	}
	startRaw, err := readExact(s.attemptsFD, request.ID+diagnosticRequestStartSuffix)
	if err != nil {
		return value, err
	}
	var start DiagnosticRequest
	if decodeDiagnosticCanonical(startRaw, &start) != nil || start != request {
		return value, errors.New("maat diagnostic: binding lacks exact consumed request")
	}
	return value, nil
}
func (s *Store) requestFencedLocked(request DiagnosticRequest) (bool, error) {
	raw, err := readExact(s.attemptsFD, request.ID+diagnosticRequestFenceSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var value DiagnosticRequest
	if decodeDiagnosticCanonical(raw, &value) != nil || value != request {
		return false, errors.New("maat diagnostic: invalid durable request fence")
	}
	return true, nil
}

func (s *Store) PrepareDiagnosticRequest(id string) (DiagnosticRequestRead, error) {
	if err := s.lock(unix.LOCK_EX); err != nil {
		return DiagnosticRequestRead{}, err
	}
	defer s.unlock()
	request, err := s.diagnosticRequest(id)
	if err != nil {
		return DiagnosticRequestRead{}, err
	}
	// Idempotent metadata preparation can recover only its exact governed tuple.
	if err = s.writeDiagnosticRequest(id+diagnosticRequestSuffix, request); err != nil {
		return DiagnosticRequestRead{}, err
	}
	return s.readDiagnosticRequestOutcomeLocked(id)
}

// This typed handle owns the existing memory lock until Close. Observation,
// preflight and synchronous journal append cannot outlive an exclusive resolve
// of this request. It is not an arbitrary action/executor API.
type DiagnosticRequestSession struct {
	mu      sync.Mutex
	store   *Store
	request DiagnosticRequest
	used    bool
}

func (r *DiagnosticRequestSession) Request() DiagnosticRequest { return r.request }
func (s *Store) BeginDiagnosticRequest(id string) (*DiagnosticRequestSession, error) {
	if s == nil || s.rootFD < 0 || !validUUID(s.storeUUID) {
		return nil, errors.New("maat diagnostic: initialized retained store required")
	}
	// An independent open-file description of this SAME retained store keeps
	// another method on the caller's handle from converting/releasing our
	// flock. No alternate path, namespace, database or authority is created.
	fd, err := unix.Openat(s.rootFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	owned, err := openStoreFDExisting(fd)
	if err != nil {
		return nil, err
	}
	owned.storeUUID = s.storeUUID // inherited only from the independently bound retained root
	session, err := owned.beginDiagnosticRequestOwned(id)
	if err != nil {
		_ = owned.Close()
		return nil, err
	}
	return session, nil
}
func (s *Store) beginDiagnosticRequestOwned(id string) (*DiagnosticRequestSession, error) {
	if err := s.lock(unix.LOCK_EX); err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			s.unlock()
		}
	}()
	if _, err := s.verifyWriteJournal(); err != nil {
		return nil, err
	}
	request, err := s.readDiagnosticRequestLocked(id)
	if err != nil {
		return nil, err
	}
	fenced, err := s.requestFencedLocked(request)
	if err != nil {
		return nil, err
	}
	if fenced {
		return nil, errors.New("maat diagnostic: request is durably fenced; inspect its outcome")
	}
	if _, err = readExact(s.attemptsFD, id+diagnosticRequestStartSuffix); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("maat diagnostic: request already consumed or start unverifiable; lookup/resolve only")
	}
	if err = s.writeDiagnosticRequest(id+diagnosticRequestStartSuffix, request); err != nil {
		return nil, err
	}
	failed = false
	return &DiagnosticRequestSession{store: s, request: request}, nil
}
func (r *DiagnosticRequestSession) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.store == nil {
		return nil
	}
	r.store.unlock()
	err := r.store.Close()
	r.store = nil
	return err
}
func (r *DiagnosticRequestSession) Record(observation DiagnosticObservation, j DiagnosticRecordJournal, verify func() error) (DiagnosticRecordResult, error) {
	if r == nil {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: missing request session")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.store == nil || r.used || j == nil || verify == nil {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: consumed/closed request session")
	}
	r.used = true
	s := r.store
	raw, err := json.Marshal(observation)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	if err = json.Unmarshal(raw, &observation); err != nil {
		return DiagnosticRecordResult{}, err
	}
	if err := observation.ValidateFresh(); err != nil {
		return DiagnosticRecordResult{}, err
	}
	m := DiagnosticRecordManifest{DiagnosticRecordSchema, r.request.UID, r.request.StoreUUID, observation}
	id, err := m.Digest()
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	receipt, err := s.issuePreflightBoundLocked(diagnosticScope(), id, resolveMemoryGuard)
	if err != nil || receipt.Decision != PreflightPass {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: request full preflight refused")
	}
	handle, err := s.retainPreflightReceiptLocked(receipt, s.createOrVerify)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	if err = s.writeDiagnosticRequest(r.request.ID+diagnosticRequestBindingSuffix, diagnosticRequestBinding{r.request, id}); err != nil {
		return DiagnosticRecordResult{}, err
	}
	return s.recordDiagnosticLocked(m, handle, j, verify, id)
}

// Exact read-only lookup includes completed records; a missing request is an
// error, never a claim that an invocation did not run or may be repeated.
func (s *Store) ReadDiagnosticRequest(id string) (DiagnosticRequestRead, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return DiagnosticRequestRead{}, err
	}
	defer s.unlock()
	return s.readDiagnosticRequestOutcomeLocked(id)
}
func (s *Store) readDiagnosticRequestOutcomeLocked(id string) (DiagnosticRequestRead, error) {
	request, err := s.readDiagnosticRequestLocked(id)
	if err != nil {
		return DiagnosticRequestRead{}, err
	}
	pendingResult, _, err := s.diagnosticPendingResultLocked()
	if err != nil {
		return DiagnosticRequestRead{}, err
	}
	read := DiagnosticRequestRead{Request: request, State: "prepared"}
	fenced, err := s.requestFencedLocked(request)
	if err != nil {
		return read, err
	}
	if fenced {
		read.State = "fenced_not_recorded"
	}
	if _, err = readExact(s.attemptsFD, id+diagnosticRequestStartSuffix); err == nil {
		if err = s.validateDiagnosticRequestLeafLocked(id + diagnosticRequestStartSuffix); err != nil {
			return read, err
		}
		if !fenced {
			read.State = "interrupted"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return read, err
	}
	binding, err := s.requestBindingLocked(request)
	if errors.Is(err, os.ErrNotExist) {
		return read, nil
	}
	if err != nil {
		return read, err
	}
	intent, err := s.readDiagnosticIntentLocked(binding.RecordID)
	if errors.Is(err, os.ErrNotExist) {
		return read, nil
	}
	if err != nil {
		return read, err
	}
	read.Intent = &intent
	if pendingResult == intent.ID {
		read.State = "pending"
		return read, nil
	}
	result, err := s.readDiagnosticResultLocked(intent.ID)
	if errors.Is(err, os.ErrNotExist) {
		read.State = "pending"
		return read, nil
	}
	if err != nil {
		return read, err
	}
	read.Result = &result
	read.State = result.State
	return read, nil
}

// Explicit data-only resolution serializes against the complete original
// invocation. Fence commits BEFORE journal lookup, so delayed starts cannot
// turn a verified absence into a later append. No observation/append/retry.
func (s *Store) ResolveDiagnosticRequest(id string, j DiagnosticRecordJournal) (DiagnosticRequestRead, error) {
	if j == nil {
		return DiagnosticRequestRead{}, errors.New("maat diagnostic: exact journal required")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return DiagnosticRequestRead{}, err
	}
	defer s.unlock()
	if _, err := s.readDiagnosticRequestLocked(id); errors.Is(err, os.ErrNotExist) {
		// An unknown preparation reply may precede any leaf. Establish and
		// fence that SAME key under this lock; late preparation/starts cannot
		// escape it. This is an explicit durable fence, not an absence reset.
		request, e := s.diagnosticRequest(id)
		if e != nil {
			return DiagnosticRequestRead{}, e
		}
		if e = s.writeDiagnosticRequest(id+diagnosticRequestSuffix, request); e != nil {
			return DiagnosticRequestRead{}, e
		}
	} else if err != nil {
		return DiagnosticRequestRead{}, err
	}
	read, err := s.readDiagnosticRequestOutcomeLocked(id)
	if err != nil {
		return read, err
	}
	// Finish only an existing exact result WAL before adding the fence. This
	// is data-only and does not inspect absence or admit a delayed invocation.
	if pendingID, _, e := s.diagnosticPendingResultLocked(); e != nil {
		return read, e
	} else if pendingID != "" {
		if read.Intent == nil || read.Intent.ID != pendingID {
			return read, errors.New("maat diagnostic: unrelated result debt")
		}
		if e = s.resolveDiagnosticRequestIntentLocked(*read.Intent, j); e != nil {
			return read, e
		}
		read, e = s.readDiagnosticRequestOutcomeLocked(id)
		if e != nil {
			return read, e
		}
	}
	if err = s.writeDiagnosticRequest(id+diagnosticRequestFenceSuffix, read.Request); err != nil {
		return read, err
	}
	if read.Intent != nil && read.Result == nil {
		if err = s.resolveDiagnosticRequestIntentLocked(*read.Intent, j); err != nil {
			return read, err
		}
	}
	return s.readDiagnosticRequestOutcomeLocked(id)
}

func (s *Store) resolveDiagnosticRequestIntentLocked(intent DiagnosticRecordIntent, j DiagnosticRecordJournal) error {
	verdict, err := intent.Manifest.Observation.Verdict()
	if err != nil {
		return err
	}
	vd, err := diagnosticSHA(verdict)
	if err != nil {
		return err
	}
	_, found, err := j.FindByEvidence("system one screen", "maat-system-one:sha256="+vd)
	if err != nil {
		return err
	}
	if found {
		_, err = s.reconcileDiagnosticRecordLocked(intent, j)
		return err
	}
	// A durable fence plus the held writer lock must precede absence-based
	// readback. Absence alone can never make a request safe to repeat.
	if err = s.verifyDiagnosticRequestFenceLocked(intent.ID); err != nil {
		return err
	}
	ih, err := diagnosticSHA(intent)
	if err != nil {
		return err
	}
	return s.writeDiagnosticRequest(intent.ID+diagnosticResultSuffix, DiagnosticRecordResult{Schema: DiagnosticRecordSchema, ID: intent.ID, IntentSHA256: ih, State: "not_recorded"})
}

type DiagnosticRecordRead struct {
	Intent DiagnosticRecordIntent  `json:"intent"`
	State  string                  `json:"state"`
	Result *DiagnosticRecordResult `json:"result,omitempty"`
}

// Historical uncorrelated records stay visible but never acquire an invented
// request identity. A person may inspect them; an app cannot infer which lost
// pre-correlation invocation produced a particular row.
func (s *Store) DiagnosticRecordHistory() ([]DiagnosticRecordRead, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	pendingResult, _, err := s.diagnosticPendingResultLocked()
	if err != nil {
		return nil, err
	}
	intents, err := s.diagnosticIntentsLocked()
	if err != nil {
		return nil, err
	}
	if len(intents) > 128 {
		return nil, errors.New("maat diagnostic: record history exceeds bounded page")
	}
	names := make([]string, 0, len(intents))
	for id := range intents {
		names = append(names, id)
	}
	sort.Strings(names)
	reads := []DiagnosticRecordRead{}
	for _, id := range names {
		read := DiagnosticRecordRead{Intent: intents[id], State: "pending"}
		if pendingResult == id {
			reads = append(reads, read)
			continue
		}
		result, err := s.readDiagnosticResultLocked(id)
		if err == nil {
			read.Result = &result
			read.State = result.State
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		reads = append(reads, read)
	}
	return reads, nil
}

// Discovery includes pending AND completed requests, so an app restart does
// not lose the durable correlation. A bounded overflow refuses, never omits.
func (s *Store) DiagnosticRequests() ([]DiagnosticRequestRead, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return nil, err
	}
	reads := []DiagnosticRequestRead{}
	for _, name := range names {
		if !strings.HasSuffix(name, diagnosticRequestSuffix) {
			continue
		}
		if len(reads) >= 128 {
			return nil, errors.New("maat diagnostic: request inspection exceeds bounded page")
		}
		read, err := s.readDiagnosticRequestOutcomeLocked(strings.TrimSuffix(name, diagnosticRequestSuffix))
		if err != nil {
			return nil, err
		}
		reads = append(reads, read)
	}
	return reads, nil
}

func isDiagnosticRequestLeaf(name string) bool {
	for _, suffix := range []string{diagnosticRequestSuffix, diagnosticRequestStartSuffix, diagnosticRequestBindingSuffix, diagnosticRequestFenceSuffix} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
func (s *Store) validateDiagnosticRequestLeafLocked(name string) error {
	for _, suffix := range []string{diagnosticRequestSuffix, diagnosticRequestStartSuffix, diagnosticRequestBindingSuffix, diagnosticRequestFenceSuffix} {
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		id := strings.TrimSuffix(name, suffix)
		request, err := s.readDiagnosticRequestLocked(id)
		if err != nil {
			return err
		}
		if suffix == diagnosticRequestBindingSuffix {
			_, err = s.requestBindingLocked(request)
			return err
		}
		raw, err := readExact(s.attemptsFD, name)
		if err != nil {
			return err
		}
		var value DiagnosticRequest
		if decodeDiagnosticCanonical(raw, &value) != nil || value != request {
			return errors.New("maat diagnostic: invalid request phase")
		}
		return nil
	}
	return errors.New("maat diagnostic: unknown request leaf")
}
func (s *Store) verifyDiagnosticRequestFenceLocked(recordID string) error {
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return err
	}
	for _, name := range names {
		if !strings.HasSuffix(name, diagnosticRequestBindingSuffix) {
			continue
		}
		request, err := s.readDiagnosticRequestLocked(strings.TrimSuffix(name, diagnosticRequestBindingSuffix))
		if err != nil {
			return err
		}
		binding, err := s.requestBindingLocked(request)
		if err != nil {
			return err
		}
		if binding.RecordID == recordID {
			fenced, err := s.requestFencedLocked(request)
			if err != nil {
				return err
			}
			if fenced {
				return nil
			}
		}
	}
	return errors.New("maat diagnostic: no exact durable request fence for nonrecorded result")
}
