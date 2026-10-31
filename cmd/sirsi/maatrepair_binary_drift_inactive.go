package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
)

func recordMaatBinaryDriftInactiveDispositionWith(
	ctx context.Context,
	journal *maat.FileDecisionJournal,
	evidence string,
	hostname func() (string, error),
	now func() time.Time,
	scan func(context.Context) (*selfupdate.DriftReport, error),
) (maatBinaryDriftInactiveDispositionReceipt, error) {
	if ctx == nil || journal == nil || hostname == nil || now == nil || scan == nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, errors.New("Ma'at binary-drift disposition: incomplete recheck providers")
	}
	host, err := hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		if err == nil {
			err = errors.New("empty host identity")
		}
		return maatBinaryDriftInactiveDispositionReceipt{}, fmt.Errorf("Ma'at binary-drift disposition: resolve current host: %w", err)
	}
	host = strings.TrimSpace(host)
	_, err := validateMaatBinaryDriftInactiveCase(journal, evidence, host)
	if err != nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, err
	}
	from := now().UTC()
	if err := ctx.Err(); err != nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, fmt.Errorf("Ma'at binary-drift disposition: fresh recheck canceled: %w", err)
	}
	drift, err := scan(ctx)
	if err != nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, fmt.Errorf("Ma'at binary-drift disposition: fresh recheck failed: %w", err)
	}
	through := now().UTC()
	if err := ctx.Err(); err != nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, fmt.Errorf("Ma'at binary-drift disposition: fresh recheck canceled: %w", err)
	}
	if through.Before(from) {
		return maatBinaryDriftInactiveDispositionReceipt{}, errors.New("Ma'at binary-drift disposition: observation clock moved backwards")
	}
	if drift == nil || !drift.Healthy {
		return maatBinaryDriftInactiveDispositionReceipt{}, errors.New("Ma'at binary-drift disposition: binary drift remains active or unavailable")
	}
	finding := pantheonBinaryDriftFinding(drift)
	if finding.Check != maatRepairBinaryDriftCheck || finding.Severity != guard.SeverityOK || finding.Resolution != guard.ResolutionInfo {
		return maatBinaryDriftInactiveDispositionReceipt{}, errors.New("Ma'at binary-drift disposition: fresh observation is not a canonical inactive finding")
	}
	raw, err := json.Marshal(drift)
	if err != nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, fmt.Errorf("Ma'at binary-drift disposition: hash fresh observation: %w", err)
	}
	digest := sha256.Sum256(raw)
	decision, err := maat.BuildBinaryDriftInactiveDisposition("sirsi maat repair binary-drift", maat.BinaryDriftInactiveDisposition{
		OriginEvidence: evidence, Host: host, ObservedFromUTC: from, ObservedThroughUTC: through,
		ObservationSHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, fmt.Errorf("Ma'at binary-drift disposition: build typed owner decision: %w", err)
	}
	stored, _, err := journal.AppendBinaryDriftInactiveDispositionOnce(decision)
	if err != nil {
		return maatBinaryDriftInactiveDispositionReceipt{}, fmt.Errorf("Ma'at binary-drift disposition: record owner decision: %w", err)
	}
	observation := stored.BinaryDriftInactiveDisposition
	return maatBinaryDriftInactiveDispositionReceipt{
		Schema: "sirsi.maat.binary-drift-inactive-disposition-receipt.v1", CaseEvidence: evidence,
		DispositionEvidence: stored.Evidence, Host: observation.Host,
		ObservedFromUTC: observation.ObservedFromUTC, ObservedThroughUTC: observation.ObservedThroughUTC,
		FindingState: "no longer active", RepairPerformed: false,
	}, nil
}

func validateMaatBinaryDriftInactiveCase(journal *maat.FileDecisionJournal, evidence, host string) (maat.Decision, error) {
	if strings.TrimSpace(evidence) == "" {
		return maat.Decision{}, errors.New("Ma'at binary-drift disposition: exact source case evidence is required")
	}
	source, count, err := journal.FindUniqueByEvidence(evidence)
	if err != nil {
		return maat.Decision{}, fmt.Errorf("Ma'at binary-drift disposition: read exact source case: %w", err)
	}
	if count != 1 || source.Kind != "system one screen" || source.SystemOne == nil {
		return maat.Decision{}, errors.New("Ma'at binary-drift disposition: exact unique System One source case was not found")
	}
	verdict := *source.SystemOne
	if err := maat.ValidateMaatVerdict(verdict); err != nil {
		return maat.Decision{}, fmt.Errorf("Ma'at binary-drift disposition: source case is invalid: %w", err)
	}
	if err := maat.ValidateBinaryDriftInactiveSource(source, evidence, host); err != nil {
		return maat.Decision{}, fmt.Errorf("Ma'at binary-drift disposition: %w", err)
	}
	for _, kind := range []string{"diagnostic repair attempt", "diagnostic verified repair", "diagnostic owner review"} {
		found, err := journal.HasRelatedEvidence(kind, evidence)
		if err != nil {
			return maat.Decision{}, fmt.Errorf("Ma'at binary-drift disposition: inspect prior owner or updater decisions: %w", err)
		}
		if found {
			return maat.Decision{}, errors.New("Ma'at binary-drift disposition: source case already has an owner or updater decision")
		}
	}
	if found, err := journal.HasRelatedEvidence("diagnostic owner acceptance", evidence); err != nil {
		return maat.Decision{}, fmt.Errorf("Ma'at binary-drift disposition: inspect prior owner acceptance: %w", err)
	} else if found {
		_, typedCount, err := journal.FindBinaryDriftInactiveDisposition(evidence)
		if err != nil || typedCount != 1 {
			return maat.Decision{}, errors.New("Ma'at binary-drift disposition: source case already has another owner acceptance")
		}
	}
	return source, nil
}
