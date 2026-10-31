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
	if err := maat.ValidateBinaryDriftInactiveCase(journal, evidence, host); err != nil {
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
	if !inactiveDriftReportComplete(drift) {
		return maatBinaryDriftInactiveDispositionReceipt{}, errors.New("Ma'at binary-drift disposition: fresh observation contains an unavailable or unstamped sibling")
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

// inactiveDriftReportComplete closes the narrower disposition boundary: the
// general drift report may be healthy while an individual sibling probe failed.
func inactiveDriftReportComplete(drift *selfupdate.DriftReport) bool {
	if drift == nil {
		return false
	}
	for _, sibling := range drift.Siblings {
		if strings.TrimSpace(sibling.Err) != "" || strings.TrimSpace(sibling.Version) == "" || strings.TrimSpace(sibling.Version) == "dev" {
			return false
		}
	}
	return true
}
