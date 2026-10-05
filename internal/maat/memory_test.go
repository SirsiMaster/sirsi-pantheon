package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingDecisionJournal struct{ decisions []Decision }

func (j *recordingDecisionJournal) Append(d Decision) error {
	j.decisions = append(j.decisions, d)
	return nil
}

func (j *recordingDecisionJournal) Recent(int) ([]Decision, error) { return j.decisions, nil }

func testSignature() FailureSignature {
	return FailureSignature{
		Namespace: "pantheon", Version: "v1", FailureClass: "launchd-disabled",
		Operation: "service-repair", Component: "router", Invariant: "managed-label-enabled",
		ErrorCode: "disabled-override",
	}
}

func testScope() Scope {
	return Scope{Component: "router", Profile: "macos-local", Operation: "service-repair"}
}

func testActions() []RecoveryAction {
	return []RecoveryAction{{ID: "repair-managed-labels", Label: "Repair managed labels", Instruction: "Use the approved router repair flow and then re-run this preflight."}}
}

func TestFailureSignatureDigestIsDeterministicAndComplete(t *testing.T) {
	first, err := testSignature().Digest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := testSignature().Digest()
	if err != nil || first != second {
		t.Fatalf("digest must be deterministic: %q %q %v", first, second, err)
	}
	changed := testSignature()
	changed.ErrorCode = "bootstrap-failed"
	other, err := changed.Digest()
	if err != nil || other == first {
		t.Fatalf("digest must bind every signature field: %q %q %v", first, other, err)
	}
}

func TestStoreEvidenceAppendAndExactScopePreflight(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	evidence := []byte("managed label was disabled\n")
	digest, err := store.PutEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := store.PutEvidence(evidence); err != nil || again != digest {
		t.Fatalf("create-only evidence must be idempotent: %q %v", again, err)
	}
	incident, err := NewIncident(testSignature(), digest, testScope(), "managed-label-guard", "v1", testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatalf("same incident must be idempotent: %v", err)
	}

	receipt, err := store.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Decision != PreflightReject || len(receipt.IncidentKeys) != 1 || len(receipt.RecoveryActions) != 1 {
		t.Fatalf("unexpected reject receipt: %#v", receipt)
	}
	if !validDigest(receipt.ActionManifestSHA256) || !validDigest(receipt.RegistrySnapshotSHA256) || len(receipt.EvaluatedGuards) != 1 || len(receipt.MeasuredChecks) != 1 || receipt.RecoveryReference == "" || receipt.EvaluatedAtUTC.IsZero() {
		t.Fatalf("receipt is not evidence-bound: %#v", receipt)
	}
	nonMatching := testScope()
	nonMatching.Profile = "macos-remote"
	pass, err := store.Preflight(nonMatching)
	if err != nil {
		t.Fatal(err)
	}
	if pass.Decision != PreflightPass || len(pass.IncidentKeys) != 0 {
		t.Fatalf("exact scope must not block a different action: %#v", pass)
	}
}

func TestStoreRefusesSymlinkEvidenceAndConflictingIncident(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	evidence := []byte("evidence bytes")
	sum := sha256.Sum256(evidence)
	digest := hex.EncodeToString(sum[:])
	target := filepath.Join(root, "outside")
	if err := os.WriteFile(target, evidence, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "evidence", digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutEvidence(evidence); err == nil {
		t.Fatal("symlink evidence must never be accepted")
	}
	if err := os.Remove(filepath.Join(root, "evidence", digest)); err != nil {
		t.Fatal(err)
	}
	stored, err := store.PutEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	incident, err := NewIncident(testSignature(), stored, testScope(), "managed-label-guard", "v1", testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	conflict := incident
	conflict.GuardVersion = "v2"
	if err := store.Append(conflict); err == nil {
		t.Fatal("different bytes under the same incident key must fail")
	}
}

func TestPreflightRejectsMalformedIncidentNamespace(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := os.WriteFile(filepath.Join(root, "incidents", "not-a-digest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.Preflight(testScope())
	if err == nil || receipt.Decision != PreflightUnverifiable {
		t.Fatalf("malformed namespace must be unverifiable: %#v %v", receipt, err)
	}
}

func TestPreflightRefusesTamperedEvidence(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	digest, err := store.PutEvidence([]byte("original evidence"))
	if err != nil {
		t.Fatal(err)
	}
	incident, err := NewIncident(testSignature(), digest, testScope(), "managed-label-guard", "v1", testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "evidence", digest), []byte("tampered evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.Preflight(testScope())
	if err == nil || receipt.Decision != PreflightUnverifiable {
		t.Fatalf("tampered evidence must prevent a pass or reject claim: %#v %v", receipt, err)
	}
}

func TestStoreRefusesSymlinkedLock(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "outside-lock")
	if err := os.WriteFile(target, []byte("not a lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, ".lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(root); err == nil {
		t.Fatal("store must refuse a symlinked registry lock")
	}
}

func TestStoreRefusesCallerSuppliedClosedIncident(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	digest, err := store.PutEvidence([]byte("evidence"))
	if err != nil {
		t.Fatal(err)
	}
	incident, err := NewIncident(testSignature(), digest, testScope(), "guard", "v1", testActions())
	if err != nil {
		t.Fatal(err)
	}
	incident.Status = IncidentRetired
	incident.SuccessorKey = digest
	if err := store.Append(incident); err == nil {
		t.Fatal("caller-supplied closed incident must be refused")
	}
}

func TestTerminalTransitionIsPredecessorBoundAndPreventsBranches(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	digest, err := store.PutEvidence([]byte("evidence"))
	if err != nil {
		t.Fatal(err)
	}
	incident, err := NewIncident(testSignature(), digest, testScope(), "guard", "v1", testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	transition, err := store.Transition(incident.Key, IncidentRetired)
	if err != nil {
		t.Fatal(err)
	}
	if transition.PredecessorKey != incident.Key || transition.Status != IncidentRetired {
		t.Fatalf("unexpected terminal transition: %#v", transition)
	}
	if _, err := store.Transition(incident.Key, IncidentSuperseded); err == nil {
		t.Fatal("a second terminal transition must be rejected")
	}
	receipt, err := store.Preflight(testScope())
	if err != nil || receipt.Decision != PreflightPass || len(receipt.MeasuredChecks) != 1 || receipt.MeasuredChecks[0].Status != string(IncidentRetired) {
		t.Fatalf("terminal transition must close the exact predecessor: %#v %v", receipt, err)
	}
}

func TestPreflightRejectsForgedTransitionContinuity(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	digest, err := store.PutEvidence([]byte("evidence"))
	if err != nil {
		t.Fatal(err)
	}
	incident, err := NewIncident(testSignature(), digest, testScope(), "guard", "v1", testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	forged, err := newIncidentTransition(incident, IncidentRetired)
	if err != nil {
		t.Fatal(err)
	}
	forged.GuardVersion = "forged"
	bytes, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "transitions", forged.Key+".json"), bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.Preflight(testScope())
	if err == nil || receipt.Decision != PreflightUnverifiable {
		t.Fatalf("forged transition must fail closed: %#v %v", receipt, err)
	}
}

func TestProjectFailureMemoryPreflightUsesExistingDecisionJournal(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	digest, err := store.PutEvidence([]byte("evidence"))
	if err != nil {
		t.Fatal(err)
	}
	incident, err := NewIncident(testSignature(), digest, testScope(), "guard", "v1", testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	journal := &recordingDecisionJournal{}
	if err := ProjectFailureMemoryPreflight(journal, receipt); err != nil {
		t.Fatal(err)
	}
	evidence, err := receipt.EvidenceReference()
	if err != nil {
		t.Fatal(err)
	}
	if len(journal.decisions) != 1 || journal.decisions[0].Kind != "failure memory preflight" || journal.decisions[0].Determination != string(PreflightReject) || journal.decisions[0].Evidence != evidence || !strings.Contains(evidence, "receipt-sha256=") || !strings.Contains(evidence, "action-sha256="+receipt.ActionManifestSHA256) || !strings.Contains(evidence, "registry-sha256="+receipt.RegistrySnapshotSHA256) {
		t.Fatalf("preflight must be a one-way factual projection: %#v", journal.decisions)
	}
}

func TestPreparedWriteFailsClosedButExactRetryResolvesIt(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data := []byte("durable evidence")
	intent, err := newWriteIntent("evidence", mustDigestLeaf(t, data), data)
	if err != nil {
		t.Fatal(err)
	}
	intentBytes, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := createOrVerifyRaw(store.writesFD, intent.Key+".prepared.json", intentBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Preflight(testScope()); err == nil {
		t.Fatal("an unresolved prepared write must make the registry unverifiable")
	}
	if _, err := store.PutEvidence(data); err != nil {
		t.Fatalf("only the exact prepared write may be resumed: %v", err)
	}
	if _, err := store.verifyWriteJournal(); err != nil {
		t.Fatalf("completed retry must close its write-ahead record: %v", err)
	}
	other := []byte("other evidence")
	otherIntent, err := newWriteIntent("evidence", mustDigestLeaf(t, other), other)
	if err != nil {
		t.Fatal(err)
	}
	otherBytes, err := json.Marshal(otherIntent)
	if err != nil {
		t.Fatal(err)
	}
	if err := createOrVerifyRaw(store.writesFD, otherIntent.Key+".prepared.json", otherBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutEvidence([]byte("third evidence")); err == nil {
		t.Fatal("a different unresolved write must block later mutations")
	}
}

func mustDigestLeaf(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
