package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

var maatTriagePrepareRequest, maatTriageRecordRequest, maatTriageReadRequest, maatTriageResolveRequest string
var maatTriageInspectRequests bool
var maatTriageRequestLimit int
var maatTriageRequestCursor string

func maatDiagnosticRequestMetadataSelected() bool {
	return maatTriagePrepareRequest != "" || maatTriageReadRequest != "" || maatTriageResolveRequest != "" || maatTriageInspectRequests
}
func validateMaatDiagnosticRequestFlags() error {
	for _, name := range []string{"prepare-record-request", "record-request", "read-record-request", "resolve-record-request"} {
		flag := maatTriageCmd.Flags().Lookup(name)
		if flag.Changed && flag.Value.String() == "" {
			return errors.New("explicit diagnostic request identity must not be empty")
		}
	}
	if !maatTriageInspectRequests && (maatTriageCmd.Flags().Changed("request-limit") || maatTriageCmd.Flags().Changed("request-cursor")) {
		return errors.New("request continuation options require --inspect-record-requests")
	}
	if maatTriageInspectRequests && (maatTriageRequestLimit < 1 || maatTriageRequestLimit > 128) {
		return errors.New("diagnostic request page limit must be 1..128")
	}
	count := 0
	for _, id := range []string{maatTriagePrepareRequest, maatTriageRecordRequest, maatTriageReadRequest, maatTriageResolveRequest} {
		if id == "" {
			continue
		}
		count++
		// Store repeats closed UUID validation before any mutation.
		if !maat.ValidDiagnosticRequestID(id) {
			return errors.New("Ma'at diagnostic request requires a canonical UUID")
		}
	}
	if maatTriageInspectRequests {
		count++
	}
	if count > 1 || (count > 0 && (maatTriageInspectRecords || maatTriageReconcileRecord != "")) {
		return errors.New("select one diagnostic request/record operation")
	}
	if (maatTriageRecordRequest != "" || maatTriageResolveRequest != "") && !maatTriageConfirm {
		return errors.New("diagnostic request submission/resolution requires explicit confirmation")
	}
	if maatTriageConfirm && (maatTriagePrepareRequest != "" || maatTriageReadRequest != "" || maatTriageInspectRequests) {
		return errors.New("preparation/inspection is separate from record confirmation")
	}
	return nil
}
func inspectMaatDiagnosticRequest(ctx context.Context) error {
	store, err := maat.OpenDefaultStore()
	if err != nil {
		return errors.New("diagnostic request custody unavailable; inspect local memory, no store was initialized")
	}
	defer func() { _ = store.Close() }()
	if maatTriageInspectRequests {
		page, err := store.QueryDiagnosticDiscovery(maat.DiagnosticDiscoveryQuery{Limit: maatTriageRequestLimit, Cursor: maatTriageRequestCursor, Context: ctx})
		if errors.Is(err, maat.ErrHistorySnapshotChanged) {
			if outputErr := emitJSON(struct {
				Schema          string `json:"schema"`
				ErrorCode       string `json:"error_code"`
				RestartRequired bool   `json:"restart_required"`
			}{maat.DiagnosticRequestSchema, "snapshot_changed", true}); outputErr != nil {
				return errors.Join(err, outputErr)
			}
		}
		if err != nil {
			return err
		}
		return emitJSON(page)
	}
	var read maat.DiagnosticRequestRead
	switch {
	case maatTriagePrepareRequest != "":
		read, err = store.PrepareDiagnosticRequest(maatTriagePrepareRequest)
	case maatTriageReadRequest != "":
		read, err = store.ReadDiagnosticRequest(maatTriageReadRequest)
	case maatTriageResolveRequest != "":
		journal, e := newMaatDecisionJournal()
		if e != nil {
			return e
		}
		exact, ok := journal.(maat.DiagnosticRecordJournal)
		if !ok {
			return errors.New("diagnostic request exact journal readback unavailable")
		}
		read, err = store.ResolveDiagnosticRequest(maatTriageResolveRequest, exact)
	}
	if err != nil {
		return err
	}
	return emitJSON(read)
}
func recordRequestedMaatDiagnostic(report *guard.DoctorReport, observation maat.DiagnosticObservation, verdict maat.MaatVerdict, snapshot string, request *maat.DiagnosticRequestSession) error {
	identity := request.Request()
	m := maat.DiagnosticRecordManifest{Schema: maat.DiagnosticRecordSchema, UID: identity.UID, StoreUUID: identity.StoreUUID, Observation: observation}
	id, err := m.Digest()
	if err != nil {
		return emitMaatDiagnosticRefusal(verdict, "manifest_unverifiable", "")
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
		host, e := maatTriageHostname()
		if e != nil {
			return e
		}
		now, e := maatDiagnosticObservation(report, host)
		if e != nil {
			return e
		}
		before, _ := json.Marshal(observation)
		after, _ := json.Marshal(now)
		if string(before) != string(after) {
			return errors.New("diagnostic request provenance changed")
		}
		return nil
	}
	result, err := request.Record(observation, exact, verify)
	if err != nil {
		state := "delivery_unverified"
		if errors.Is(err, maat.ErrDiagnosticNotRecorded) {
			state = "not_recorded"
		}
		if errors.Is(err, maat.ErrDiagnosticNotSubmitted) {
			state = "not_submitted"
		}
		return emitMaatDiagnosticRefusal(verdict, state, id)
	}
	return emitMaatDiagnosticRecorded(verdict, snapshot, result, id)
}
