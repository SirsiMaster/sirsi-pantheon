package maat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func memoryGuardFixture(t *testing.T, id, version string) (*Store, Incident) {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	digest, err := s.PutEvidence([]byte("owned inert recurrence evidence"))
	if err != nil {
		t.Fatal(err)
	}
	i, err := NewIncident(testSignature(), digest, testScope(), id, version, testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(i); err != nil {
		t.Fatal(err)
	}
	return s, i
}

func TestFailureMemoryExecutesClosedTypedGuard(t *testing.T) {
	s, i := memoryGuardFixture(t, ActiveIncidentGuardID, ActiveIncidentGuardVersion)
	calls := 0
	receipt, err := s.preflight(testScope(), func(id, version string) (memoryGuard, bool) {
		guard, ok := resolveMemoryGuard(id, version)
		if !ok {
			t.Fatalf("production registry lost supported guard: %q %q", id, version)
		}
		evaluate := guard.evaluate
		guard.evaluate = func(input memoryGuardInput) (CheckOutcome, error) {
			calls++
			if input.Action != testScope() || input.Incident.Key != i.Key {
				t.Fatalf("wrong typed input: %+v", input)
			}
			return evaluate(input)
		}
		return guard, true
	})
	if err != nil || calls != 1 || receipt.Decision != PreflightReject || len(receipt.MeasuredChecks) != 1 || len(receipt.EvaluatedGuards) != 1 {
		t.Fatalf("actual guard execution missing: calls=%d receipt=%+v err=%v", calls, receipt, err)
	}
	if receipt.MeasuredChecks[0].Outcome != "active-incident-recurrence-rejected" || receipt.MeasuredChecks[0].IncidentKey != i.Key {
		t.Fatalf("receipt did not retain actual typed result: %+v", receipt.MeasuredChecks)
	}
	guardSource, err := os.ReadFile("memoryguard.go")
	if err != nil {
		t.Fatal(err)
	}
	storeSource, err := os.ReadFile("memory.go")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(append(append(guardSource, 0), storeSource...))
	if got := receipt.EvaluatedGuards[0]; got.ID != ActiveIncidentGuardID || got.Version != ActiveIncidentGuardVersion || got.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("guard identity must bind both implementation sources: %+v", got)
	}
	// Exercise the public route too, not just the per-call observation seam.
	public, err := s.Preflight(testScope())
	if err != nil || len(public.MeasuredChecks) != 1 || len(public.EvaluatedGuards) != 1 || public.MeasuredChecks[0] != receipt.MeasuredChecks[0] || public.EvaluatedGuards[0] != receipt.EvaluatedGuards[0] {
		t.Fatalf("public dispatch diverged: %+v %v", public, err)
	}
}

func TestFailureMemoryGuardRejectsInvalidTypedInput(t *testing.T) {
	_, i := memoryGuardFixture(t, ActiveIncidentGuardID, ActiveIncidentGuardVersion)
	for _, change := range []struct {
		name string
		edit func(*memoryGuardInput)
	}{
		{"empty-action", func(in *memoryGuardInput) { in.Action = Scope{} }},
		{"other-scope", func(in *memoryGuardInput) { in.Action.Profile = "other" }},
		{"terminal", func(in *memoryGuardInput) { in.Incident.Status = IncidentRetired }},
		{"unknown-id", func(in *memoryGuardInput) { in.Incident.GuardID = "imported.shell" }},
		{"unknown-version", func(in *memoryGuardInput) { in.Incident.GuardVersion = "v999" }},
		{"invalid-key", func(in *memoryGuardInput) { in.Incident.Key = "not-a-digest" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			input := memoryGuardInput{Action: testScope(), Incident: i}
			change.edit(&input)
			if _, err := evaluateActiveIncident(input); err == nil {
				t.Fatal("invalid typed guard input accepted")
			}
		})
	}
}

func TestFailureMemoryUnsupportedGuardIsScopedUnverifiable(t *testing.T) {
	for _, tc := range []struct{ name, id, version string }{
		{"unknown-id", "imported.command", "v1"},
		{"unknown-version", ActiveIncidentGuardID, "v999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, i := memoryGuardFixture(t, tc.id, tc.version)
			r, err := s.Preflight(testScope())
			if err == nil || r.Decision != PreflightUnverifiable || len(r.MeasuredChecks) != 0 || len(r.EvaluatedGuards) != 0 || len(r.IncidentKeys) != 1 || r.IncidentKeys[0] != i.Key || !validDigest(r.RegistrySnapshotSHA256) {
				t.Fatalf("unsupported matching guard falsely measured: %+v %v", r, err)
			}
			other := testScope()
			other.Profile = "unrelated-observation"
			r, err = s.Preflight(other)
			if err != nil || r.Decision != PreflightPass || len(r.MeasuredChecks) != 0 || len(r.EvaluatedGuards) != 0 || len(r.IncidentKeys) != 0 {
				t.Fatalf("unsupported out-of-scope guard blocked observation: %+v %v", r, err)
			}
			if _, err := s.Transition(i.Key, IncidentRetired); err != nil {
				t.Fatal(err)
			}
			r, err = s.Preflight(testScope())
			if err != nil || r.Decision != PreflightPass || len(r.MeasuredChecks) != 0 || len(r.EvaluatedGuards) != 0 {
				t.Fatalf("terminal unsupported guard was dispatched: %+v %v", r, err)
			}
		})
	}
}

func TestFailureMemoryMixedGuardsPreserveActualChecks(t *testing.T) {
	s, i := memoryGuardFixture(t, ActiveIncidentGuardID, ActiveIncidentGuardVersion)
	other := i
	other.Signature.ErrorCode = "other-failure"
	other.Key, _ = other.Signature.Digest()
	other.GuardID = "unsupported"
	if err := s.Append(other); err != nil {
		t.Fatal(err)
	}
	r, err := s.Preflight(testScope())
	if err == nil || r.Decision != PreflightUnverifiable || len(r.IncidentKeys) != 2 || len(r.MeasuredChecks) != 1 || len(r.EvaluatedGuards) != 1 || r.MeasuredChecks[0].IncidentKey != i.Key {
		t.Fatalf("mixed registry lost checks or promoted partial evaluation: %+v %v", r, err)
	}
}

func TestFailureMemoryFIFOReadRefusesWithoutPeer(t *testing.T) {
	root := t.TempDir()
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readExact(fd, "fifo"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted as a regular catalog leaf")
		}
	case <-time.After(time.Second):
		// Release a regressed blocking open and join it before failing/closing fd.
		peer, err := unix.Openat(fd, "fifo", unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatalf("blocked FIFO reader; could not join peer: %v", err)
		}
		_ = unix.Close(peer)
		<-done
		t.Fatal("catalog FIFO open blocked waiting for a peer")
	}
}

func TestFailureMemoryReadBoundAndEvidenceLimit(t *testing.T) {
	s, _ := memoryGuardFixture(t, ActiveIncidentGuardID, ActiveIncidentGuardVersion)
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "oversize"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxMemoryObjectBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if _, err := readExact(fd, "oversize"); err == nil {
		t.Fatal("oversize stored leaf accepted")
	}
	if _, err := s.PutEvidence(make([]byte, maxMemoryObjectBytes+1)); err == nil {
		t.Fatal("oversize evidence accepted")
	}
}

func TestFailureMemoryPreparedDebtSurvivesReopen(t *testing.T) {
	root := t.TempDir()
	s, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	data := []byte("exact recovery tuple")
	intent, err := newWriteIntent("evidence", mustDigestLeaf(t, data), data)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := createOrVerifyRaw(s.writesFD, intent.Key+".prepared.json", encoded); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if r, err := s.Preflight(testScope()); err == nil || r.Decision != PreflightUnverifiable {
		t.Fatalf("reopened debt was lost: %+v %v", r, err)
	}
	if _, err := s.PutEvidence([]byte("unrelated")); err == nil {
		t.Fatal("reopened debt allowed unrelated write")
	}
	if _, err := s.PutEvidence(data); err != nil {
		t.Fatal(err)
	}
	if r, err := s.Preflight(testScope()); err != nil || r.Decision != PreflightPass {
		t.Fatalf("exact retry did not resolve debt: %+v %v", r, err)
	}
}

func TestFailureMemoryConcurrentIndependentStores(t *testing.T) {
	root := t.TempDir()
	const writers = 4
	stores := make([]*Store, writers)
	for n := range stores {
		s, err := OpenStore(root)
		if err != nil {
			t.Fatal(err)
		}
		stores[n] = s
		t.Cleanup(func() { _ = s.Close() })
	}
	start, done := make(chan struct{}), make(chan error, writers)
	for n, s := range stores {
		go func(n int, s *Store) {
			<-start
			digest, err := s.PutEvidence([]byte("shared idempotent evidence"))
			if err == nil {
				sig := testSignature()
				sig.ErrorCode = fmt.Sprintf("fixture-%d", n)
				var i Incident
				i, err = NewIncident(sig, digest, testScope(), ActiveIncidentGuardID, ActiveIncidentGuardVersion, testActions())
				if err == nil {
					err = s.Append(i)
				}
			}
			done <- err
		}(n, s)
	}
	close(start)
	for range writers {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
	r, err := stores[0].Preflight(testScope())
	if err != nil || r.Decision != PreflightReject || len(r.MeasuredChecks) != writers || len(r.IncidentKeys) != writers || len(r.EvaluatedGuards) != 1 {
		t.Fatalf("concurrent writes lost or duplicated evidence: %+v %v", r, err)
	}
	// Separate open-file descriptions must serialize competing terminal events.
	key := r.IncidentKeys[0]
	terminal := make(chan error, 2)
	for n, status := range []IncidentStatus{IncidentRetired, IncidentSuperseded} {
		go func() { _, err := stores[n].Transition(key, status); terminal <- err }()
	}
	succeeded := 0
	for range 2 {
		if <-terminal == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("terminal history forked: %d successful transitions", succeeded)
	}
	r, err = stores[0].Preflight(testScope())
	if err != nil || r.Decision != PreflightReject || len(r.MeasuredChecks) != writers-1 {
		t.Fatalf("concurrent terminal event corrupted remaining scope: %+v %v", r, err)
	}
}

func TestFailureMemoryFailedLeafWriteLeavesReopenDebt(t *testing.T) {
	root := t.TempDir()
	s, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	data := []byte("expected immutable leaf")
	leaf := mustDigestLeaf(t, data)
	path := filepath.Join(root, "evidence", leaf)
	conflict := []byte("preserve this conflicting fixture")
	if err := os.WriteFile(path, conflict, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutEvidence(data); err == nil {
		t.Fatal("conflicting leaf write unexpectedly committed")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := s.Preflight(testScope()); err == nil || r.Decision != PreflightUnverifiable {
		t.Fatalf("failed governed write lost durable debt: %+v %v", r, err)
	}
	if _, err := s.PutEvidence([]byte("unrelated write")); err == nil {
		t.Fatal("unrelated write bypassed failed leaf debt")
	}
	preserved := filepath.Join(root, "preserved-conflicting-fixture")
	if err := os.Rename(path, preserved); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutEvidence(data); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(preserved)
	if err != nil || !bytes.Equal(actual, conflict) {
		t.Fatal("recovery lost conflicting fixture bytes")
	}
	if r, err := s.Preflight(testScope()); err != nil || r.Decision != PreflightPass {
		t.Fatalf("exact failed-write retry did not resolve debt: %+v %v", r, err)
	}
}

type failingMemoryJournal struct{ appends, reads int }

var errMemoryJournal = errors.New("owned fixture append failure")

func (j *failingMemoryJournal) Append(Decision) error { j.appends++; return errMemoryJournal }
func (j *failingMemoryJournal) Recent(int) ([]Decision, error) {
	j.reads++
	return nil, errors.New("projection must not read policy from journal")
}

func TestFailureMemoryProjectionAppendFailurePreservesAuthority(t *testing.T) {
	s, _ := memoryGuardFixture(t, ActiveIncidentGuardID, ActiveIncidentGuardVersion)
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	j := &failingMemoryJournal{}
	if err := ProjectFailureMemoryPreflight(j, r); !errors.Is(err, errMemoryJournal) || j.appends != 1 || j.reads != 0 {
		t.Fatalf("append failure hidden or reverse policy read: %+v %v", j, err)
	}
	after, err := json.Marshal(r)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("projection mutated original receipt")
	}
	if retry, err := s.Preflight(testScope()); err != nil || retry.Decision != PreflightReject {
		t.Fatalf("journal failure changed registry authority: %+v %v", retry, err)
	}
}
