package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

var maatTriageInspectRecords bool
var maatTriageReconcileRecord string

func maatDiagnosticObservation(report *guard.DoctorReport, host string) (maat.DiagnosticObservation, error) {
	if report == nil || strings.TrimSpace(host) == "" {
		return maat.DiagnosticObservation{}, errors.New("local diagnostic provenance unavailable")
	}
	if len(host) > 1024 || len(report.Findings) > 128 {
		return maat.DiagnosticObservation{}, errors.New("local diagnostic exceeds bounded provenance")
	}
	budget := len(report.Duration) + len(report.Status)
	if budget > 1<<20 {
		return maat.DiagnosticObservation{}, errors.New("local diagnostic exceeds bounded provenance")
	}
	for _, f := range report.Findings {
		budget += len(f.Check) + len(f.Message) + len(f.Detail) + len(f.Fix) + len(f.FixKind) + len(f.Resolution)
		if budget > 1<<20 {
			return maat.DiagnosticObservation{}, errors.New("local diagnostic exceeds bounded provenance")
		}
	}
	raw, err := json.Marshal(report)
	if err != nil || len(raw) > 1<<20 {
		return maat.DiagnosticObservation{}, errors.New("local diagnostic provenance malformed")
	}
	source := sha256.Sum256(raw)
	identity := sha256.Sum256([]byte(strings.TrimSpace(host)))
	o := maat.DiagnosticObservation{Schema: maat.DiagnosticObservationSchema, SourceSHA256: hex.EncodeToString(source[:]), HostSHA256: hex.EncodeToString(identity[:]), ObservedAtUTC: report.Timestamp.UTC(), Score: report.Score, Statuses: []maat.DiagnosticStatus{}}
	for _, finding := range report.Findings {
		repair := ""
		if guard.ResolutionFor(finding) == guard.ResolutionRepair {
			switch finding.Check {
			case maatRepairLaunchdDisabledCheck:
				repair = maat.SystemOneRepairLaunchdDisabled
			case maatRepairLivenessWatchCheck:
				repair = maat.SystemOneRepairLivenessWatch
			case maatRepairBinaryDriftCheck:
				repair = maat.SystemOneRepairBinaryDrift
			}
		}
		o.Statuses = append(o.Statuses, maat.DiagnosticStatus{Check: maat.DiagnosticCheckCode(finding.Check), Severity: int(finding.Severity), RepairID: repair})
	}
	sort.Slice(o.Statuses, func(i, j int) bool { return o.Statuses[i].Check < o.Statuses[j].Check })
	return o, o.Validate()
}

func recordMaatDiagnosticWithRequest(report *guard.DoctorReport, host string, verdict maat.MaatVerdict, snapshot string, request *maat.DiagnosticRequestSession) error {
	observation, err := maatDiagnosticObservation(report, host)
	if err != nil {
		return emitMaatDiagnosticRefusal(verdict, "provenance_unknown", "")
	}
	expected, err := observation.Verdict()
	actualBytes, actualErr := json.Marshal(verdict)
	expectedBytes, expectedErr := json.Marshal(expected)
	if err != nil || actualErr != nil || expectedErr != nil || string(actualBytes) != string(expectedBytes) {
		return emitMaatDiagnosticRefusal(verdict, "provenance_changed", "")
	}
	if request != nil {
		return recordRequestedMaatDiagnostic(report, observation, verdict, snapshot, request)
	}
	status := maat.InspectDefaultStore()
	if status.State != maat.DefaultStoreReady {
		return emitMaatDiagnosticRefusal(verdict, "memory_unavailable", "")
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		return emitMaatDiagnosticRefusal(verdict, "memory_unavailable", "")
	}
	defer func() { _ = store.Close() }()
	manifest := maat.DiagnosticRecordManifest{Schema: maat.DiagnosticRecordSchema, UID: uint32(os.Geteuid()), StoreUUID: status.StoreUUID, Observation: observation}
	id, err := manifest.Digest()
	if err != nil {
		return emitMaatDiagnosticRefusal(verdict, "manifest_unverifiable", "")
	}
	receipt, err := store.PreflightDiagnosticRecord(manifest)
	if err != nil || receipt.Decision != maat.PreflightPass {
		return emitMaatDiagnosticRefusal(verdict, "preflight_refused", id)
	}
	retained, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		return emitMaatDiagnosticRefusal(verdict, "receipt_unavailable", id)
	}
	journal, err := newMaatDecisionJournal()
	if err != nil {
		return emitMaatDiagnosticRefusal(verdict, "journal_unavailable", id)
	}
	exact, ok := journal.(maat.DiagnosticRecordJournal)
	if !ok {
		return emitMaatDiagnosticRefusal(verdict, "journal_readback_unavailable", id)
	}
	verify := func() error {
		nowHost, err := maatTriageHostname()
		if err != nil {
			return errors.New("host provenance unknown")
		}
		now, err := maatDiagnosticObservation(report, nowHost)
		if err != nil {
			return err
		}
		before, _ := json.Marshal(observation)
		after, _ := json.Marshal(now)
		if string(before) != string(after) {
			return errors.New("selected diagnostic provenance changed")
		}
		return nil
	}
	result, err := store.RecordDiagnostic(manifest, retained, exact, verify)
	if err != nil {
		if errors.Is(err, maat.ErrDiagnosticNotRecorded) {
			return emitMaatDiagnosticRefusal(verdict, "not_recorded", id)
		}
		if errors.Is(err, maat.ErrDiagnosticNotSubmitted) {
			return emitMaatDiagnosticRefusal(verdict, "not_submitted", id)
		}
		return emitMaatDiagnosticRefusal(verdict, "delivery_unverified", id)
	}
	return emitMaatDiagnosticRecorded(verdict, snapshot, result, id)
}

func emitMaatDiagnosticRecorded(verdict maat.MaatVerdict, snapshot string, result maat.DiagnosticRecordResult, id string) error {
	if JsonOutput || maatJSON {
		return emitJSON(struct {
			maat.MaatVerdict
			SnapshotEvidence string `json:"snapshot_evidence"`
			DecisionEvidence string `json:"decision_evidence"`
			RecordState      string `json:"record_state"`
			RecordID         string `json:"record_id"`
		}{verdict, snapshot, result.DecisionEvidence, "recorded_verified", id})
	}
	return renderMaatTriage(verdict, snapshot, "recorded_verified", systemOneSummary(verdict), result.DecisionEvidence)
}

// No top-level MaatVerdict on refusal: existing native record consumers must
// not decode a failed/unknown append as "Ma'at recorded this observation".
func emitMaatDiagnosticRefusal(verdict maat.MaatVerdict, state, id string) error {
	if JsonOutput || maatJSON {
		if err := emitJSON(struct {
			Schema      string           `json:"schema"`
			RecordState string           `json:"record_state"`
			RecordID    string           `json:"record_id,omitempty"`
			Observation maat.MaatVerdict `json:"observation"`
			Recovery    string           `json:"recovery"`
		}{maat.DiagnosticRecordSchema, state, id, verdict, "Observation remains available. Inspect local memory and diagnostic record debt; reconciliation reads exact journal evidence without resending."}); err != nil {
			return err
		}
	}
	return errors.New("Ma'at diagnostic record is not verified (" + state + "); local observation remains available; inspect only, no automatic resend")
}

func inspectOrReconcileMaatDiagnostic() error {
	if maatTriageInspectRecords && (maatTriageConfirm || maatTriageReconcileRecord != "") {
		return errors.New("diagnostic record inspection is read-only; select one operation")
	}
	if maatTriageReconcileRecord != "" && !maatTriageConfirm {
		return errors.New("exact data-only diagnostic reconciliation requires confirmation")
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		return errors.New("diagnostic record custody unavailable; no store was initialized")
	}
	defer func() { _ = store.Close() }()
	if maatTriageInspectRecords {
		pending, err := store.PendingDiagnosticRecords()
		if err != nil {
			return errors.New("diagnostic record history unverifiable; no resend")
		}
		return emitJSON(struct {
			Schema  string                        `json:"schema"`
			Pending []maat.DiagnosticRecordIntent `json:"pending"`
		}{maat.DiagnosticRecordSchema, pending})
	}
	journal, err := newMaatDecisionJournal()
	if err != nil {
		return errors.New("diagnostic record readback unavailable")
	}
	exact, ok := journal.(maat.DiagnosticRecordJournal)
	if !ok {
		return errors.New("strict diagnostic record readback unavailable")
	}
	result, err := store.ReconcileDiagnosticRecord(maatTriageReconcileRecord, exact)
	if err != nil {
		return errors.New("diagnostic delivery remains unknown; no record was resent")
	}
	return emitJSON(result)
}
