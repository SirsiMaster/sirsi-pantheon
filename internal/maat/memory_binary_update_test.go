package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	internalversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"golang.org/x/sys/unix"
)

func binaryUpdateFixture(t *testing.T) (*Store, string, BinaryUpdateManifest) {
	t.Helper()
	root := privateMemoryStoreRoot(t)
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	self, updater, target := filepath.Join(root, "sirsi-self"), filepath.Join(root, "brew"), filepath.Join(root, "target")
	for _, path := range []string{self, updater, target} {
		if err := os.WriteFile(path, []byte("binary:"+filepath.Base(path)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	identity := func(path string) BinaryUpdateTarget {
		var st unix.Stat_t
		if err := unix.Stat(path, &st); err != nil {
			t.Fatal(err)
		}
		sum, err := testFileDigest(path)
		if err != nil {
			t.Fatal(err)
		}
		return BinaryUpdateTarget{Path: path, Method: "homebrew", Version: "1.0.0", SHA256: sum, OwnerUID: st.Uid, Device: uint64(st.Dev), Inode: st.Ino, Mode: uint32(st.Mode)}
	}
	selfID, updaterID, targetID := identity(self), identity(updater), identity(target)
	runningInfo := internalversion.Current("sirsi")
	runningInfo.Path, runningInfo.Version = self, "1.0.0"
	runningInfo.RouterSchemaMax = routerstore.MaxSupportedSchemaVersion()
	m := BinaryUpdateManifest{
		Schema: BinaryUpdateManifestSchema, CaseEvidence: "maat-system-one:sha256=" + strings.Repeat("a", 64), PreviewBinding: strings.Repeat("b", 64),
		Updater: "brew upgrade sirsi-pantheon", UpdaterPath: updater, UpdaterSHA256: updaterID.SHA256,
		UpdaterOwnerUID: updaterID.OwnerUID, UpdaterDevice: updaterID.Device, UpdaterInode: updaterID.Inode, UpdaterMode: updaterID.Mode,
		SelfPath: self, SelfVersion: "1.0.0", SelfSHA256: selfID.SHA256, RunningInfo: runningInfo, SelfOwnerUID: selfID.OwnerUID, SelfDevice: selfID.Device, SelfInode: selfID.Inode, SelfMode: selfID.Mode,
		Targets: []BinaryUpdateTarget{targetID}, AuthorizedUID: uint32(os.Geteuid()), ObservedSchemaCompatible: true, ObservedSchemaVersion: 1,
		StoreUUID: store.storeUUID, ObservedAtUTC: time.Now().UTC(),
	}
	return store, root, m
}

func testFileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func binaryUpdateClaimEvidence(m BinaryUpdateManifest) string {
	sum := sha256.Sum256([]byte(m.CaseEvidence))
	return "diagnostic-repair-attempt:sha256=" + hex.EncodeToString(sum[:])
}

func TestBinaryUpdateManifestRejectsUnorderedOrUnownedTargets(t *testing.T) {
	_, _, m := binaryUpdateFixture(t)
	m.Targets = append(m.Targets, m.Targets[0])
	if err := m.Validate(); err == nil {
		t.Fatal("duplicate target path accepted")
	}
	_, _, m = binaryUpdateFixture(t)
	m.Targets[0].OwnerUID++
	if err := m.Validate(); err == nil {
		t.Fatal("foreign target owner accepted")
	}
}

func TestBinaryUpdateManifestDigestBindsRunningSchemaCeiling(t *testing.T) {
	_, _, m := binaryUpdateFixture(t)
	before, err := m.Digest()
	if err != nil {
		t.Fatal(err)
	}
	m.RunningInfo.RouterSchemaMax--
	after, err := m.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("running router-schema ceiling was not bound by the action manifest")
	}
}

func TestBinaryUpdateHistoricalManifestRemainsReadableButCannotBeAdmitted(t *testing.T) {
	store, _, m := binaryUpdateFixture(t)
	oldNow := binaryUpdateNow
	now := time.Now().UTC()
	binaryUpdateNow = func() time.Time { return now }
	t.Cleanup(func() { binaryUpdateNow = oldNow })
	m.ObservedAtUTC = now.Add(-3 * time.Minute)
	if err := m.Validate(); err != nil {
		t.Fatalf("historical manifest failed structural validation: %v", err)
	}
	if _, err := m.Digest(); err != nil {
		t.Fatalf("historical manifest digest failed: %v", err)
	}
	if _, err := store.PreflightBinaryUpdate(m); err == nil {
		t.Fatal("stale historical manifest admitted a new updater action")
	}
}

func TestBinaryUpdateAgedAttemptOutcomeAndProjectionRemainReadableAfterReopen(t *testing.T) {
	store, root, m := binaryUpdateFixture(t)
	oldNow := binaryUpdateNow
	now := m.ObservedAtUTC.Add(time.Second)
	binaryUpdateNow = func() time.Time { return now }
	t.Cleanup(func() { binaryUpdateNow = oldNow })
	m.ObservedAtUTC = now
	receipt, err := store.PreflightBinaryUpdate(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetainPreflightReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginBinaryUpdateAttempt(m, receipt, func() (string, error) { return binaryUpdateClaimEvidence(m), nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordBinaryUpdateInvocation(attempt); err != nil {
		t.Fatal(err)
	}
	outcome, err := store.RecordBinaryUpdateOutcome(attempt, true, false, strings.Repeat("f", 64), "fresh postcheck retained a failed result")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := BuildDiagnosticRepairDecision("fixture", DiagnosticRepair{Check: "binary-drift", Operation: "brew upgrade sirsi-pantheon", OriginEvidence: m.CaseEvidence, Before: m.PreviewBinding, After: "still active", Determination: "failed", Detail: outcome.Detail})
	if err != nil {
		t.Fatal(err)
	}
	decision.Time = outcome.RecordedAtUTC.UTC().Format(time.RFC3339Nano)
	debt, err := store.BeginBinaryUpdateProjection(attempt, outcome, decision)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PersistBinaryUpdateProjectionOutcome(debt); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: filepath.Join(root, "aged-casebook.jsonl")}
	if err := journal.AppendIdempotent(debt.Decision); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkBinaryUpdateProjectionReadBack(debt); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Minute)
	reopened, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.storeUUID = m.StoreUUID
	defer reopened.Close()
	history, err := reopened.BinaryUpdateReconciliations(m.CaseEvidence)
	if err != nil || len(history) != 1 || history[0].Outcome == nil || history[0].Projection == nil || !history[0].ClaimOwned || !history[0].InvocationRecorded || !history[0].ProjectionReadBack {
		t.Fatalf("aged reopened history=%+v err=%v", history, err)
	}
	if debts, err := reopened.PendingBinaryUpdateProjectionDebts(); err != nil || len(debts) != 0 {
		t.Fatalf("aged projection debts=%+v err=%v", debts, err)
	}
	if pending, err := reopened.PendingBinaryUpdateAttempt(); !errors.Is(err, ErrNoIncompleteBinaryUpdateAttempt) {
		t.Fatalf("aged terminal attempt=%+v err=%v", pending, err)
	}
}

func TestBinaryUpdateReceiptAndAttemptFenceRegistryDrift(t *testing.T) {
	store, _, m := binaryUpdateFixture(t)
	receipt, err := store.PreflightBinaryUpdate(m)
	if err != nil || receipt.Decision != PreflightPass {
		t.Fatalf("preflight=%+v err=%v", receipt, err)
	}
	if _, err := store.RetainPreflightReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	evidence, err := store.PutEvidence([]byte("new recurrence after reviewed receipt"))
	if err != nil {
		t.Fatal(err)
	}
	sig := FailureSignature{Namespace: "pantheon", Version: "v1", FailureClass: "binary-drift", Operation: "update", Component: "sirsi", Invariant: "binary-targets-converge", ErrorCode: "drift-detected"}
	key, err := sig.Digest()
	if err != nil {
		t.Fatal(err)
	}
	incident := Incident{Schema: MemorySchema, Key: key, Signature: sig, EvidenceSHA256: evidence, Scope: binaryUpdateScope(), GuardID: ActiveIncidentGuardID, GuardVersion: ActiveIncidentGuardVersion, Status: IncidentActive,
		RecoveryActions: []RecoveryAction{{ID: "review-binary-update", Label: "Review binary update", Instruction: "Refresh the exact System One screen and review a new bounded plan."}}}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	claims := 0
	if _, err := store.BeginBinaryUpdateAttempt(m, receipt, func() (string, error) { claims++; return "claim", nil }); err == nil || claims != 0 {
		t.Fatalf("registry drift admission err=%v claim calls=%d", err, claims)
	}
}

func TestBinaryUpdateInvocationBarrierRechecksRegistryUnderStoreLock(t *testing.T) {
	store, _, m := binaryUpdateFixture(t)
	r, err := store.PreflightBinaryUpdate(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetainPreflightReceipt(r); err != nil {
		t.Fatal(err)
	}
	a, err := store.BeginBinaryUpdateAttempt(m, r, func() (string, error) { return binaryUpdateClaimEvidence(m), nil })
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := store.PutEvidence([]byte("recurrence between attempt and invocation barrier"))
	if err != nil {
		t.Fatal(err)
	}
	sig := FailureSignature{Namespace: "pantheon", Version: "v1", FailureClass: "binary-drift", Operation: "update", Component: "sirsi", Invariant: "binary-targets-converge", ErrorCode: "drift-detected"}
	key, err := sig.Digest()
	if err != nil {
		t.Fatal(err)
	}
	incident := Incident{Schema: MemorySchema, Key: key, Signature: sig, EvidenceSHA256: evidence, Scope: binaryUpdateScope(), GuardID: ActiveIncidentGuardID, GuardVersion: ActiveIncidentGuardVersion, Status: IncidentActive,
		RecoveryActions: []RecoveryAction{{ID: "review-binary-update", Label: "Review binary update", Instruction: "Refresh the exact System One screen and review a new bounded plan."}}}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordBinaryUpdateInvocation(a); err == nil {
		t.Fatal("invocation barrier admitted a registry that changed after the initial preflight")
	}
	history, err := store.BinaryUpdateReconciliations(m.CaseEvidence)
	if err != nil || len(history) != 1 || history[0].InvocationRecorded || history[0].Outcome != nil {
		t.Fatalf("changed-registry attempt history=%+v err=%v", history, err)
	}
}

func TestBinaryUpdateConcurrentPreviewVariantsShareOneLaunchClaim(t *testing.T) {
	storeA, root, mA := binaryUpdateFixture(t)
	storeB, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	storeB.storeUUID = storeA.storeUUID
	defer storeB.Close()
	rA, err := storeA.PreflightBinaryUpdate(mA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.RetainPreflightReceipt(rA); err != nil {
		t.Fatal(err)
	}
	mB := mA
	mB.PreviewBinding = strings.Repeat("d", 64)
	mB.ObservedAtUTC = time.Now().UTC()
	rB, err := storeB.PreflightBinaryUpdate(mB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeB.RetainPreflightReceipt(rB); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	claims := 0
	start := make(chan struct{})
	errs := make(chan error, 2)
	claim := func(label string) func() (string, error) {
		return func() (string, error) { mu.Lock(); claims++; mu.Unlock(); return label, nil }
	}
	go func() {
		<-start
		_, e := storeA.BeginBinaryUpdateAttempt(mA, rA, claim(binaryUpdateClaimEvidence(mA)))
		errs <- e
	}()
	go func() {
		<-start
		_, e := storeB.BeginBinaryUpdateAttempt(mB, rB, claim(binaryUpdateClaimEvidence(mB)))
		errs <- e
	}()
	close(start)
	e1, e2 := <-errs, <-errs
	mu.Lock()
	gotClaims := claims
	mu.Unlock()
	if gotClaims != 1 || (e1 == nil) == (e2 == nil) {
		t.Fatalf("claim count=%d admission errors=(%v,%v), want one claim and one admission", gotClaims, e1, e2)
	}
}

func TestBinaryUpdateUnknownLaunchRemainsUnresolvedAndCannotRetry(t *testing.T) {
	store, _, m := binaryUpdateFixture(t)
	r, err := store.PreflightBinaryUpdate(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetainPreflightReceipt(r); err != nil {
		t.Fatal(err)
	}
	a, err := store.BeginBinaryUpdateAttempt(m, r, func() (string, error) { return binaryUpdateClaimEvidence(m), nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordBinaryUpdateInvocation(a); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingBinaryUpdateAttempt()
	if err != nil || pending.ID != a.ID || pending.Manifest.RunningInfo.RouterSchemaMax != m.RunningInfo.RouterSchemaMax || pending.Manifest.RunningInfo.Commit != m.RunningInfo.Commit {
		t.Fatalf("unknown launch pending=%+v err=%v", pending, err)
	}
	if _, err := store.BeginBinaryUpdateAttempt(m, r, func() (string, error) { t.Fatal("retry claim must not run"); return "", nil }); err == nil {
		t.Fatal("unresolved updater attempt authorized replay")
	}
}

func TestBinaryUpdatePostcheckFailureIsTerminalFailure(t *testing.T) {
	store, _, m := binaryUpdateFixture(t)
	r, err := store.PreflightBinaryUpdate(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetainPreflightReceipt(r); err != nil {
		t.Fatal(err)
	}
	a, err := store.BeginBinaryUpdateAttempt(m, r, func() (string, error) { return binaryUpdateClaimEvidence(m), nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordBinaryUpdateInvocation(a); err != nil {
		t.Fatal(err)
	}
	o, err := store.RecordBinaryUpdateOutcome(a, true, false, strings.Repeat("c", 64), "fresh target observation remained drifted")
	if err != nil || o.Determination != "failed" {
		t.Fatalf("postcheck outcome=%+v err=%v", o, err)
	}
	if _, err := store.PendingBinaryUpdateAttempt(); !errors.Is(err, ErrNoIncompleteBinaryUpdateAttempt) {
		t.Fatalf("terminal attempt remains falsely unresolved: %v", err)
	}
}

func TestBinaryUpdateClaimRefusalRetainsOnlyUnownedLocalFact(t *testing.T) {
	store, _, m := binaryUpdateFixture(t)
	r, err := store.PreflightBinaryUpdate(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetainPreflightReceipt(r); err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginBinaryUpdateAttempt(m, r, func() (string, error) { return "", errors.New("duplicate exact case claim") })
	if err == nil || attempt.ID == "" {
		t.Fatalf("claim refusal did not retain a typed attempt: attempt=%+v err=%v", attempt, err)
	}
	history, err := store.BinaryUpdateReconciliations(m.CaseEvidence)
	if err != nil || len(history) != 1 || history[0].Outcome == nil || history[0].InvocationRecorded || history[0].ClaimOwned {
		t.Fatalf("claim refusal history=%+v err=%v", history, err)
	}
	if history[0].Outcome.FactCode != "launch_claim_refused" {
		t.Fatalf("claim refusal fact=%+v", history[0].Outcome)
	}
	decision, err := BuildDiagnosticRepairDecision("fixture", DiagnosticRepair{Check: "binary-drift", Operation: "brew upgrade sirsi-pantheon", OriginEvidence: m.CaseEvidence, Before: m.PreviewBinding, After: "updater was not invoked", Determination: "failed", Detail: "known claim refusal"})
	if err != nil {
		t.Fatal(err)
	}
	decision.Time = history[0].Outcome.RecordedAtUTC.UTC().Format(time.RFC3339Nano)
	if _, err := store.BeginBinaryUpdateProjection(attempt, *history[0].Outcome, decision); err == nil {
		t.Fatal("unowned local refusal was allowed to create a global origin-terminal projection")
	}
	debts, err := store.PendingBinaryUpdateProjectionDebts()
	if err != nil || len(debts) != 0 {
		t.Fatalf("completed projection still appears pending: debts=%+v err=%v", debts, err)
	}
	if pending, err := store.PendingBinaryUpdateAttempt(); !errors.Is(err, ErrNoIncompleteBinaryUpdateAttempt) {
		t.Fatalf("known no-invocation outcome remained pending: attempt=%+v err=%v", pending, err)
	}
}

func TestBinaryUpdateProjectionReceiptLeafIsNotParsedAsProjectionDebt(t *testing.T) {
	store, _, m := binaryUpdateFixture(t)
	r, err := store.PreflightBinaryUpdate(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetainPreflightReceipt(r); err != nil {
		t.Fatal(err)
	}
	a, err := store.BeginBinaryUpdateAttempt(m, r, func() (string, error) { return binaryUpdateClaimEvidence(m), nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordBinaryUpdateInvocation(a); err != nil {
		t.Fatal(err)
	}
	o, err := store.RecordBinaryUpdateOutcome(a, true, false, strings.Repeat("d", 64), "postcheck remains active")
	if err != nil {
		t.Fatal(err)
	}
	d, err := BuildDiagnosticRepairDecision("fixture", DiagnosticRepair{Check: "binary-drift", Operation: "brew upgrade", OriginEvidence: m.CaseEvidence, Before: m.PreviewBinding, After: "still active", Determination: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	d.Time = o.RecordedAtUTC.UTC().Format(time.RFC3339Nano)
	debt, err := store.BeginBinaryUpdateProjection(a, o, d)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PersistBinaryUpdateProjectionOutcome(debt); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "casebook.jsonl")}
	if err := journal.AppendIdempotent(debt.Decision); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkBinaryUpdateProjectionReadBack(debt); err != nil {
		t.Fatal(err)
	}
	debts, err := store.PendingBinaryUpdateProjectionDebts()
	if err != nil || len(debts) != 0 {
		t.Fatalf("completed projection receipt was misread as debt: debts=%+v err=%v", debts, err)
	}
}
