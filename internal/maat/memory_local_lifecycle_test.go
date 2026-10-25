package maat

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type localLifecycleFixture struct {
	store              *Store
	journal            *FileDecisionJournal
	manifest           LivenessWatchManifest
	attempt            LivenessWatchAttempt
	result             LivenessDirectResult
	intake             FailureIntakeReceipt
	evidence           string
	installs, observes int
}

func beginLocalLifecycleFixture(t *testing.T) *localLifecycleFixture {
	t.Helper()
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	uuid, err := InitializeDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	j, err := NewDefaultDecisionJournal()
	if err != nil {
		t.Fatal(err)
	}
	m := fixtureLivenessManifest(t, uuid)
	r, err := s.PreflightLivenessWatch(m)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProjectFailureMemoryPreflight(j, stored); err != nil {
		t.Fatal(err)
	}
	a, err := s.BeginLivenessWatchAttempt(m, r)
	if err != nil {
		t.Fatal(err)
	}
	return &localLifecycleFixture{store: s, journal: j, manifest: m, attempt: a}
}
func (f *localLifecycleFixture) run(t *testing.T, state string, installErr, observeErr error) {
	t.Helper()
	result, err := f.store.RunLivenessWatchDirectAttempt(f.attempt, func(LivenessWatchManifest) (string, error) {
		f.installs++
		return "inert fixture installation", installErr
	}, func(m LivenessWatchManifest) (LivenessWatchObservation, error) {
		f.observes++
		return canonicalCompletionObservation(m, state), observeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	f.result = result
}
func (f *localLifecycleFixture) projectOutcome(t *testing.T) {
	t.Helper()
	decision, err := LivenessCompletionDecision(f.result)
	if err != nil {
		t.Fatal(err)
	}
	debt, err := f.store.BeginLivenessWatchProjection(f.attempt, f.result.Outcome, decision)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.PersistLivenessWatchProjectionOutcome(debt); err != nil {
		t.Fatal(err)
	}
	if err := f.journal.AppendIdempotent(debt.Decision); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MarkLivenessWatchProjectionReadBack(debt); err != nil {
		t.Fatal(err)
	}
}
func (f *localLifecycleFixture) remember(t *testing.T) {
	t.Helper()
	r, err := f.store.RememberLivenessFailure(f.attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.intake = r
}
func (f *localLifecycleFixture) recordPrevention(t *testing.T) {
	t.Helper()
	r, err := f.store.Preflight(localWatchScope())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProjectFailureMemoryPreflight(f.journal, stored); err != nil {
		t.Fatal(err)
	}
	f.evidence, err = r.EvidenceReference()
	if err != nil {
		t.Fatal(err)
	}
}
func failedLocalLifecycleFixture(t *testing.T) *localLifecycleFixture {
	t.Helper()
	f := beginLocalLifecycleFixture(t)
	f.run(t, "known-unloaded", nil, nil)
	f.remember(t)
	f.projectOutcome(t)
	f.recordPrevention(t)
	return f
}
func (f *localLifecycleFixture) preview(t *testing.T, effect string) LocalPreventionDispositionPreview {
	t.Helper()
	view, err := f.store.ReadLivenessFailure(f.intake.IncidentKey)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.store.PreviewLocalPreventionDisposition(view.Incident.Key, view.Head.Key, view.Head.BodySHA256, f.evidence, effect, "Fixture: accept this known local issue without claiming repair.")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLocalManagedWatchRememberPreventOwnerDispositionAndRecurrence(t *testing.T) {
	f := failedLocalLifecycleFixture(t)
	if f.installs != 1 || f.observes != 1 || f.intake.Disposition != "active-linked" {
		t.Fatalf("direct fixture did not retain one known failure: %+v", f.intake)
	}
	if _, err := f.store.PendingLivenessWatchAttempt(); !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		t.Fatalf("known returned failure remained unknown after complete intake/projection: %v", err)
	}
	rejected, err := f.store.Preflight(localWatchScope())
	if err != nil || rejected.Decision != PreflightReject || len(rejected.MeasuredChecks) != 1 || rejected.MeasuredChecks[0].IncidentKey != f.intake.IncidentKey {
		t.Fatalf("failure did not prevent recurrence: %+v %v", rejected, err)
	}
	if other, err := f.store.Preflight(Scope{Component: "other", Profile: "independent", Operation: "different"}); err != nil || other.Decision != PreflightPass {
		t.Fatalf("exact local prevention blocked unrelated scope: %+v %v", other, err)
	}
	introduction, err := readExact(f.store.incidentFD, f.intake.IncidentKey+".json")
	if err != nil {
		t.Fatal(err)
	}
	accept := f.preview(t, "accept-only")
	if !accept.CanCommit {
		t.Fatal(accept.Constraints)
	}
	accepted, err := f.store.CommitLocalPreventionDisposition(accept.PlanSHA256, accept.PlanSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.EventKey != "" || accepted.ActionAuthorized || accepted.Effect != "accept-only" {
		t.Fatalf("acceptance alone changed prevention: %+v", accepted)
	}
	if current, err := f.store.Preflight(localWatchScope()); err != nil || current.Decision != PreflightReject {
		t.Fatalf("conclusion alone lifted prevention: %+v %v", current, err)
	}
	lift := f.preview(t, "retire-local-prevention")
	if !lift.CanCommit {
		t.Fatal(lift.Constraints)
	}
	receipt, err := f.store.CommitLocalPreventionDisposition(lift.PlanSHA256, lift.PlanSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ActionAuthorized || receipt.Effect != "retire-local-prevention" || !strings.Contains(receipt.Health, "unhealthy") || f.installs != 1 {
		t.Fatalf("disposition claimed repair or ran an installer: %+v", receipt)
	}
	view, err := f.store.ReadLocalPreventionDisposition(f.intake.IncidentKey)
	if err != nil || view.Head.Status != IncidentRetired || view.Prevention != "accepted-not-repaired" || len(view.Dispositions) != 2 || view.Outcome.Determination != "incomplete" {
		t.Fatalf("history/health lost: %+v %v", view, err)
	}
	current, err := f.store.PreflightLivenessWatch(f.manifest)
	if err != nil || current.Decision != PreflightPass {
		t.Fatalf("lift did not permit a new review: %+v %v", current, err)
	}
	// A new action still needs a newly retained assessment and a new durable
	// attempt. No installer is called by review, acceptance or retirement.
	if _, err := f.store.BeginLivenessWatchAttempt(f.manifest, current); err == nil {
		t.Fatal("new review bypassed retained-receipt admission")
	}
	if _, err := f.store.RetainPreflightReceipt(current); err != nil {
		t.Fatal(err)
	}
	next, err := f.store.BeginLivenessWatchAttempt(f.manifest, current)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == f.attempt.ID {
		t.Fatal("old attempt replayed")
	}
	result, err := f.store.RunLivenessWatchDirectAttempt(next, func(LivenessWatchManifest) (string, error) { f.installs++; return "second inert invocation", nil }, func(m LivenessWatchManifest) (LivenessWatchObservation, error) {
		return canonicalCompletionObservation(m, "known-unloaded"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.attempt, f.result = next, result
	f.remember(t)
	f.projectOutcome(t)
	if f.intake.IncidentKey != view.Incident.Key {
		t.Fatal("recurrence rewrote the stable signature key")
	}
	after, err := readExact(f.store.incidentFD, f.intake.IncidentKey+".json")
	if err != nil || !bytes.Equal(after, introduction) {
		t.Fatal("recurrence overwrote its original introduction/proof")
	}
	recurred, err := f.store.ReadLivenessFailure(f.intake.IncidentKey)
	if err != nil || recurred.Head.Status != IncidentActive || len(recurred.Events) != 2 || recurred.Events[1].PredecessorEventKey != receipt.EventKey {
		t.Fatalf("recurrence did not bind the exact retired predecessor: %+v %v", recurred, err)
	}
	if _, err := f.store.CommitLocalPreventionDisposition(lift.PlanSHA256, lift.PlanSHA256); err != nil {
		t.Fatal(err)
	}
	final, err := f.store.Preflight(localWatchScope())
	if err != nil || final.Decision != PreflightReject || f.installs != 2 {
		t.Fatalf("prior acceptance closed/replayed a new occurrence: %+v %v installs=%d", final, err, f.installs)
	}
}

func TestLocalCompletionEligibilityNeverPromotesUnknownOrReobserve(t *testing.T) {
	for _, mode := range []string{"exact", "known-absent", "known-unloaded", "unverifiable", "returned-error", "observation-error", "not-invoked"} {
		t.Run(mode, func(t *testing.T) {
			f := beginLocalLifecycleFixture(t)
			state := mode
			var installErr, observeErr error
			if mode == "returned-error" {
				state = "known-unloaded"
				installErr = errors.New("inert installer failure")
			}
			if mode == "observation-error" {
				state = "known-absent"
				observeErr = errors.New("inert canceled or unavailable probe")
			}
			if mode == "not-invoked" {
				result, err := f.store.CloseLivenessWatchBeforeInvocation(f.attempt, func(m LivenessWatchManifest) (LivenessWatchObservation, error) {
					f.observes++
					return canonicalCompletionObservation(m, "known-unloaded"), nil
				})
				if err != nil {
					t.Fatal(err)
				}
				f.result = result
			} else {
				f.run(t, state, installErr, observeErr)
			}
			f.remember(t)
			f.projectOutcome(t)
			eligible := mode == "known-absent" || mode == "known-unloaded"
			if (f.intake.Disposition == "active-linked") != eligible {
				t.Fatalf("wrong eligibility for %s: %+v", mode, f.intake)
			}
			names, err := readDirectoryNames(f.store.incidentFD)
			if err != nil {
				t.Fatal(err)
			}
			if (len(names) == 1) != eligible {
				t.Fatalf("unknown/ineligible input minted incident: %s %v", mode, names)
			}
			unknown := mode == "unverifiable" || mode == "returned-error" || mode == "observation-error"
			pending, pendingErr := f.store.PendingLivenessWatchAttempt()
			if unknown {
				if pendingErr != nil || pending.ID != f.attempt.ID {
					t.Fatalf("unknown action was waived: %v %+v", pendingErr, pending)
				}
			} else if !errors.Is(pendingErr, ErrNoIncompleteLivenessWatchAttempt) {
				t.Fatalf("known completion did not settle after exact readback: %v", pendingErr)
			}
			calls := f.installs
			if err := f.store.ReconcileLivenessWatchData(); err != nil {
				t.Fatal(err)
			}
			if f.installs != calls {
				t.Fatal("data-only reconciliation replayed installer")
			}
		})
	}
}

func TestDirectInvocationClaimIsOnceOnlyAndNeverTurnsMissingWitnessIntoFailure(t *testing.T) {
	f := beginLocalLifecycleFixture(t)
	f.run(t, "known-unloaded", nil, nil)
	if _, err := f.store.RunLivenessWatchDirectAttempt(f.attempt, func(LivenessWatchManifest) (string, error) { f.installs++; return "must never run", nil }, func(LivenessWatchManifest) (LivenessWatchObservation, error) {
		f.observes++
		return LivenessWatchObservation{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if f.installs != 1 || f.observes != 1 {
		t.Fatal("same attempt re-invoked callbacks")
	}
	// A different initialized store cannot adopt this attempt or raw witness.
	foreign, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	if _, err := foreign.RememberLivenessFailure(f.attempt.ID); err == nil {
		t.Fatal("foreign store accepted caller attempt id")
	}
}

func TestLocalOwnerDispositionRefusesStaleHeadCopiedReviewAndPreservesOtherBlockers(t *testing.T) {
	f := failedLocalLifecycleFixture(t)
	view, err := f.store.ReadLivenessFailure(f.intake.IncidentKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"wrong-head", "wrong-body", "wrong-case", "unknown-effect"} {
		head, body, evidence, effect := view.Head.Key, view.Head.BodySHA256, f.evidence, "retire-local-prevention"
		switch mode {
		case "wrong-head":
			head = strings.Repeat("f", 64)
		case "wrong-body":
			body = strings.Repeat("f", 64)
		case "wrong-case":
			evidence = "caller:approved"
		case "unknown-effect":
			effect = "approved"
		}
		if _, err := f.store.PreviewLocalPreventionDisposition(view.Incident.Key, head, body, evidence, effect, "known issue"); err == nil {
			t.Fatalf("%s was accepted", mode)
		}
	}
	plan := f.preview(t, "retire-local-prevention")
	if _, err := f.store.CommitLocalPreventionDisposition(plan.PlanSHA256, strings.Repeat("f", 64)); err == nil {
		t.Fatal("unbound consent accepted")
	}
	// Another exact-scope blocker survives lifting this one local family.
	proof, err := f.store.PutEvidence([]byte("another inert active prevention"))
	if err != nil {
		t.Fatal(err)
	}
	sig := testSignature()
	sig.ErrorCode = "separate-active-blocker"
	other, err := NewIncident(sig, proof, localWatchScope(), ActiveIncidentGuardID, ActiveIncidentGuardVersion, testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Append(other); err != nil {
		t.Fatal(err)
	}
	f.recordPrevention(t)
	plan = f.preview(t, "retire-local-prevention")
	if _, err := f.store.CommitLocalPreventionDisposition(plan.PlanSHA256, plan.PlanSHA256); err != nil {
		t.Fatal(err)
	}
	r, err := f.store.Preflight(localWatchScope())
	if err != nil || r.Decision != PreflightReject || len(r.IncidentKeys) != 1 || r.IncidentKeys[0] != other.Key || f.installs != 1 {
		t.Fatalf("lifting one guard waived others or replayed: %+v %v", r, err)
	}
}

// Each prefix is an owned inert fixture of the existing prepared/committed
// protocol, not an alternate runtime mutation path.
func localLifecyclePrefix(t *testing.T, s *Store, namespace string, fd int, leaf string, value any, stage string) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := newWriteIntent(namespace, leaf, data)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := createOrVerifyRaw(s.writesFD, intent.Key+".prepared.json", prepared); err != nil {
		t.Fatal(err)
	}
	if stage == "prepared" {
		return
	}
	if err := createOrVerifyRaw(fd, leaf, data); err != nil {
		t.Fatal(err)
	}
	if stage == "target" {
		return
	}
	commit, err := json.Marshal(writeCommit{Schema: MemorySchema, IntentKey: intent.Key})
	if err != nil {
		t.Fatal(err)
	}
	if err := createOrVerifyRaw(s.writesFD, intent.Key+".committed.json", commit); err != nil {
		t.Fatal(err)
	}
}

func TestLocalFailureIntakeRecoversExactDurablePrefixesWithoutCallbackReplay(t *testing.T) {
	for _, phase := range []string{"proof-prepared", "proof-target", "introduction-prepared", "introduction-target", "intake-prepared", "intake-target", "projection-missing"} {
		t.Run(phase, func(t *testing.T) {
			f := beginLocalLifecycleFixture(t)
			f.run(t, "known-unloaded", nil, nil)
			c := f.result.Completion
			completionSHA, _ := digestJSON(c)
			proof := localFailureProof{Schema: "sirsi.maat.local-managed-watch-failure-proof.v1", StoreUUID: f.store.storeUUID, AttemptID: c.AttemptID, AttemptSHA256: c.AttemptSHA256, CompletionSHA256: completionSHA, OutcomeSHA256: c.OutcomeSHA256, PreflightReceiptSHA256: c.PreflightReceiptSHA256, Producer: c.Producer, Observation: c.Observation, RecordedAtUTC: c.RecordedAtUTC}
			proofBytes, _ := json.Marshal(proof)
			proofSHA, err := digestCompactBytes(proofBytes)
			if err != nil {
				t.Fatal(err)
			}
			stage := "committed"
			if phase == "proof-prepared" {
				stage = "prepared"
			}
			if phase == "proof-target" {
				stage = "target"
			}
			localLifecyclePrefix(t, f.store, "evidence", f.store.evidenceFD, proofSHA, proof, stage)
			if !strings.HasPrefix(phase, "proof-") {
				i, err := NewIncident(localFailureSignature(c.Observation), proofSHA, localWatchScope(), ActiveIncidentGuardID, ActiveIncidentGuardVersion, localFailureActions())
				if err != nil {
					t.Fatal(err)
				}
				stage = "committed"
				if phase == "introduction-prepared" {
					stage = "prepared"
				}
				if phase == "introduction-target" {
					stage = "target"
				}
				localLifecyclePrefix(t, f.store, "incidents", f.store.incidentFD, i.Key+".json", i, stage)
				if strings.HasPrefix(phase, "intake-") || phase == "projection-missing" {
					body, _ := json.Marshal(i)
					headSHA, err := digestCompactBytes(body)
					if err != nil {
						t.Fatal(err)
					}
					intake := FailureIntakeReceipt{Schema: localIntakeSchema, StoreUUID: f.store.storeUUID, AttemptID: c.AttemptID, CompletionSHA256: completionSHA, OutcomeSHA256: c.OutcomeSHA256, EvidenceSHA256: proofSHA, Disposition: "active-linked", IncidentKey: i.Key, OccurrenceKey: headSHA, RecordedAtUTC: c.RecordedAtUTC}
					stage = "committed"
					if phase == "intake-prepared" {
						stage = "prepared"
					}
					if phase == "intake-target" {
						stage = "target"
					}
					localLifecyclePrefix(t, f.store, "attempts", f.store.attemptsFD, c.AttemptID+".intake-v1.json", intake, stage)
				}
			}
			if r, err := f.store.Preflight(localWatchScope()); err == nil || r.Decision != PreflightUnverifiable {
				t.Fatal("interrupted required intake/projection was treated clear")
			}
			if err := f.store.ReconcileLivenessWatchData(); err != nil {
				t.Fatal(err)
			}
			f.remember(t)
			f.projectOutcome(t)
			if f.installs != 1 || f.observes != 1 {
				t.Fatal("reconciliation executed callbacks")
			}
			if r, err := f.store.Preflight(localWatchScope()); err != nil || r.Decision != PreflightReject || len(r.IncidentKeys) != 1 {
				t.Fatalf("recovered intake lost exact prevention: %+v %v", r, err)
			}
		})
	}
}

func TestOwnerDispositionReconcilesEachRetainedPhaseAndFinalJournalFailure(t *testing.T) {
	for _, phase := range []string{"intent-target", "intent-committed", "proof-prepared", "proof-target", "owner-review", "owner-acceptance", "event-prepared", "event-target", "event-committed", "projection", "receipt-prepared", "receipt-target", "journal-unavailable"} {
		t.Run(phase, func(t *testing.T) {
			f := failedLocalLifecycleFixture(t)
			preview := f.preview(t, "retire-local-prevention")
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			intent := localDispositionIntent{Schema: "sirsi.maat.local-prevention-disposition-intent.v1", PlanSHA256: preview.PlanSHA256, Plan: preview.Plan, Host: host, ConfirmedAtUTC: time.Now().UTC()}
			stage := "committed"
			if phase == "intent-target" {
				stage = "target"
			}
			localLifecyclePrefix(t, f.store, "attempts", f.store.attemptsFD, preview.PlanSHA256+".disposition-v1.json", intent, stage)
			proof, event, projection, receipt, err := f.store.localDispositionRecords(intent)
			if err != nil {
				t.Fatal(err)
			}
			proofBytes, _ := json.Marshal(proof)
			proofSHA, err := digestCompactBytes(proofBytes)
			if err != nil {
				t.Fatal(err)
			}
			if phase != "intent-target" && phase != "intent-committed" {
				stage = "committed"
				if phase == "proof-prepared" {
					stage = "prepared"
				}
				if phase == "proof-target" {
					stage = "target"
				}
				localLifecyclePrefix(t, f.store, "evidence", f.store.evidenceFD, proofSHA, proof, stage)
			}
			beforeReview := phase == "intent-target" || phase == "intent-committed" || phase == "proof-prepared" || phase == "proof-target"
			if !beforeReview {
				if err := f.journal.AppendIdempotent(proof.OwnerReview); err != nil {
					t.Fatal(err)
				}
			}
			if !beforeReview && phase != "owner-review" {
				if err := retainExactLocalOwnerAcceptance(f.journal, proof.OwnerReview, proof.OwnerAcceptance); err != nil {
					t.Fatal(err)
				}
			}
			eventPhase := phase == "event-prepared" || phase == "event-target" || phase == "event-committed" || phase == "projection" || strings.HasPrefix(phase, "receipt-") || phase == "journal-unavailable"
			if eventPhase {
				stage = "committed"
				if phase == "event-prepared" {
					stage = "prepared"
				}
				if phase == "event-target" {
					stage = "target"
				}
				localLifecyclePrefix(t, f.store, "transitions", f.store.transitionFD, event.EventKey+".json", *event, stage)
			}
			if phase == "projection" || strings.HasPrefix(phase, "receipt-") {
				if err := f.journal.AppendIdempotent(projection); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(phase, "receipt-") {
				stage = "prepared"
				if phase == "receipt-target" {
					stage = "target"
				}
				localLifecyclePrefix(t, f.store, "attempts", f.store.attemptsFD, preview.PlanSHA256+".disposition-complete-v1.json", receipt, stage)
			}
			if phase == "journal-unavailable" {
				lock := f.journal.Path + ".lock"
				retained := lock + ".retained-fixture"
				if err := os.Rename(lock, retained); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(lock, 0o700); err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.ReconcileLocalPreventionDisposition(preview.PlanSHA256); err == nil {
					t.Fatal("journal failure claimed full disposition success")
				}
				if r, err := f.store.Preflight(localWatchScope()); err == nil || r.Decision != PreflightUnverifiable {
					t.Fatal("final projection debt did not block new action")
				}
				if err := os.Remove(lock); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(retained, lock); err != nil {
					t.Fatal(err)
				}
			}
			actual, err := f.store.ReconcileLocalPreventionDisposition(preview.PlanSHA256)
			if err != nil {
				t.Fatalf("phase %s stranded: %v", phase, err)
			}
			if digestEqual(actual, receipt) != nil || f.installs != 1 || f.observes != 1 {
				t.Fatal("reconciliation changed tuple/time or ran installer/observer")
			}
			if again, err := f.store.ReconcileLocalPreventionDisposition(preview.PlanSHA256); err != nil || digestEqual(again, receipt) != nil {
				t.Fatalf("data-only completion not idempotent: %+v %v", again, err)
			}
			if r, err := f.store.Preflight(localWatchScope()); err != nil || r.Decision != PreflightPass {
				t.Fatalf("complete historical disposition still blocked new review: %+v %v", r, err)
			}
		})
	}
}

func TestOrdinaryOwnerAcceptanceCannotRetireLocalPreventionAndForksRefuse(t *testing.T) {
	f := failedLocalLifecycleFixture(t)
	review, err := RecordDiagnosticOwnerReview(f.journal, "arbitrary caller label", DiagnosticOwnerReview{Check: "local-managed-watch-prevention", Message: "ordinary conclusion", Detail: "no dedicated effect", OriginEvidence: f.evidence})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcceptDiagnosticOwnerReview(f.journal, "owner", DiagnosticOwnerAcceptance{Evidence: review.Evidence, Conclusion: "accept without repair"}); err != nil {
		t.Fatal(err)
	}
	if r, err := f.store.Preflight(localWatchScope()); err != nil || r.Decision != PreflightReject {
		t.Fatal("ordinary Casebook acceptance changed memory")
	}
	if _, err := f.store.Transition(f.intake.IncidentKey, IncidentRetired); err == nil {
		t.Fatal("generic terminal writer bypassed local effect consent")
	}
	plan := f.preview(t, "retire-local-prevention")
	if _, err := f.store.CommitLocalPreventionDisposition(plan.PlanSHA256, plan.PlanSHA256); err != nil {
		t.Fatal(err)
	}
	view, err := f.store.ReadLivenessFailure(f.intake.IncidentKey)
	if err != nil {
		t.Fatal(err)
	}
	fork := view.Events[0]
	fork.RecordedAtUTC = fork.RecordedAtUTC.Add(time.Second)
	fork.EventKey, err = digestJSON(fork.identity())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.appendLocalEventLocked(fork); err != nil {
		t.Fatal(err)
	}
	if r, err := f.store.Preflight(localWatchScope()); err == nil || r.Decision != PreflightUnverifiable {
		t.Fatal("forked chain was accepted")
	}
}

func TestLocalPreventionMetadataPreservesDecisionAndCaseWireCompatibility(t *testing.T) {
	if !reflect.TypeOf(Decision{}).Comparable() {
		t.Fatal("additive read metadata broke the existing Decision value contract")
	}
	f := failedLocalLifecycleFixture(t)
	row, found, err := f.journal.FindByEvidence("failure memory preflight", f.evidence)
	if err != nil || !found || row.FailureMemoryIncidentKeys == nil || len(*row.FailureMemoryIncidentKeys) != 1 || (*row.FailureMemoryIncidentKeys)[0] != f.intake.IncidentKey {
		t.Fatalf("actual journal omitted controlled incident links: %+v %v", row, err)
	}
	path, err := DefaultStorePath()
	if err != nil || !filepath.IsAbs(path) {
		t.Fatal(err)
	}
}

func TestLocalClaimWithoutCompletionRefusesConcurrentReplayAndInventedIntake(t *testing.T) {
	f := beginLocalLifecycleFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	joined := make(chan error, 1)
	var once sync.Once
	var calls atomic.Int32
	go func() {
		_, err := f.store.RunLivenessWatchDirectAttempt(f.attempt, func(LivenessWatchManifest) (string, error) {
			calls.Add(1)
			close(started)
			select {
			case <-release:
			case <-time.After(5 * time.Second):
				return "", errors.New("fixture bounded installer release expired")
			}
			return "inert completed installer", nil
		}, func(m LivenessWatchManifest) (LivenessWatchObservation, error) {
			return canonicalCompletionObservation(m, "known-unloaded"), nil
		})
		joined <- err
	}()
	// Cleanup releases and joins even when any assertion refuses early.
	didJoin := false
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		if !didJoin {
			select {
			case <-joined:
			case <-time.After(6 * time.Second):
				t.Error("fixture installer did not join")
			}
		}
	})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("installer claim never reached callback")
	}
	neverInstall := func(LivenessWatchManifest) (string, error) { calls.Add(1); return "unexpected replay", nil }
	neverObserve := func(LivenessWatchManifest) (LivenessWatchObservation, error) {
		t.Error("unknown attempt invoked a fresh observer as past outcome")
		return LivenessWatchObservation{}, nil
	}
	if _, err := f.store.RunLivenessWatchDirectAttempt(f.attempt, neverInstall, neverObserve); err == nil {
		t.Fatal("concurrent claimed invocation was replayed")
	}
	if _, err := f.store.CloseLivenessWatchBeforeInvocation(f.attempt, neverObserve); err == nil {
		t.Fatal("claimed invocation was relabeled not-invoked")
	}
	if _, err := f.store.RememberLivenessFailure(f.attempt.ID); err == nil {
		t.Fatal("missing witness became an active failure")
	}
	if pending, err := f.store.PendingLivenessWatchAttempt(); err != nil || pending.ID != f.attempt.ID {
		t.Fatalf("unknown invocation was waived: %+v %v", pending, err)
	}
	names, err := readDirectoryNames(f.store.incidentFD)
	if err != nil || len(names) != 0 || calls.Load() != 1 {
		t.Fatalf("unverified invocation minted incident or replayed: %v %v calls=%d", names, err, calls.Load())
	}
	once.Do(func() { close(release) })
	select {
	case err := <-joined:
		didJoin = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("completed invocation did not join")
	}
}

func TestOwnerConclusionCannotWaiveUnknownActionOrUnrelatedPreparedWAL(t *testing.T) {
	for _, debt := range []string{"unknown-action", "prepared-WAL"} {
		t.Run(debt, func(t *testing.T) {
			f := failedLocalLifecycleFixture(t)
			lift := f.preview(t, "retire-local-prevention")
			if debt == "prepared-WAL" {
				// A distinct inert unresolved transaction is never treated as
				// this owner's consent, proof or historical completion.
				leaf := struct {
					Schema string `json:"schema"`
				}{"fixture.unrelated.pending.v1"}
				data, err := json.Marshal(leaf)
				if err != nil {
					t.Fatal(err)
				}
				key, err := digestCompactBytes(data)
				if err != nil {
					t.Fatal(err)
				}
				localLifecyclePrefix(t, f.store, "evidence", f.store.evidenceFD, key, leaf, "prepared")
			} else {
				// Owned adversarial historical started row with no invocation
				// witness. It is not a new installer admission or actual effect.
				pending := f.attempt
				id, err := newStoreUUID()
				if err != nil {
					t.Fatal(err)
				}
				pending.ID, pending.StartedAtUTC = id, time.Now().UTC()
				if err := f.store.writeLocalAttemptLocked(id+".started.json", pending); err != nil {
					t.Fatal(err)
				}
				// Recording a conclusion is distinct from forgiving unknown
				// action history. It must not turn an incomplete action green.
				accept := f.preview(t, "accept-only")
				if !accept.CanCommit {
					t.Fatal("known head conclusion was confused with lifting prevention")
				}
				if _, err := f.store.CommitLocalPreventionDisposition(accept.PlanSHA256, accept.PlanSHA256); err != nil {
					t.Fatal(err)
				}
				blocked := f.preview(t, "retire-local-prevention")
				if blocked.CanCommit || len(blocked.Constraints) == 0 {
					t.Fatal("unknown action was waived by owner conclusion")
				}
			}
			if _, err := f.store.CommitLocalPreventionDisposition(lift.PlanSHA256, lift.PlanSHA256); err == nil {
				t.Fatal("previous clean preview lifted prevention over newly retained unknown debt")
			}
			r, err := f.store.Preflight(localWatchScope())
			if r.Decision != PreflightUnverifiable {
				t.Fatalf("unknown debt became pass: %+v %v", r, err)
			}
			if f.installs != 1 {
				t.Fatal("owner disposition replayed installer")
			}
		})
	}
}
