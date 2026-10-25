package main

import (
	"encoding/json"
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

var maatTriagePrepareRequest, maatTriageRecordRequest, maatTriageReadRequest, maatTriageResolveRequest string
var maatTriageInspectRequests bool

func maatDiagnosticRequestMetadataSelected() bool {
	return maatTriagePrepareRequest != "" || maatTriageReadRequest != "" || maatTriageResolveRequest != "" || maatTriageInspectRequests
}
func validateMaatDiagnosticRequestFlags() error {
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
func inspectMaatDiagnosticRequest() error {
	store, err := maat.OpenDefaultStore()
	if err != nil {
		return errors.New("diagnostic request custody unavailable; inspect local memory, no store was initialized")
	}
	defer func() { _ = store.Close() }()
	if maatTriageInspectRequests {
		values, err := store.DiagnosticRequests()
		if err != nil {
			return err
		}
		records, err := store.DiagnosticRecordHistory()
		if err != nil {
			return err
		}
		return emitJSON(struct {
			Schema   string                       `json:"schema"`
			Requests []maat.DiagnosticRequestRead `json:"requests"`
			Records  []maat.DiagnosticRecordRead  `json:"records"`
		}{maat.DiagnosticRequestSchema, values, records})
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
