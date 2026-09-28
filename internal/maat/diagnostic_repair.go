package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// DiagnosticRepair is the factual outcome of one bounded, already-authorized
// repair. It does not carry a command or grant mutation authority: the caller
// owns the repair contract and Ma'at records only the observation before it,
// the exact bounded operation, and the observation after it.
type DiagnosticRepair struct {
	Check     string
	Operation string
	Before    string
	After     string
	// Determination is deliberately closed. A successful subprocess is not a
	// resolved repair unless the caller's post-repair observation says so.
	Determination string
	Detail        string
}

// RecordDiagnosticRepair appends a bounded, evidence-linked repair outcome to
// the one Ma'at journal. An incomplete repair remains an open case; a resolved
// repair requires a real post-repair observation from its caller.
func RecordDiagnosticRepair(j DecisionJournal, requester string, repair DiagnosticRepair) (Decision, error) {
	if j == nil {
		return Decision{}, fmt.Errorf("maat diagnostic repair: nil journal")
	}
	requester = strings.TrimSpace(requester)
	repair.Check = strings.TrimSpace(repair.Check)
	repair.Operation = strings.TrimSpace(repair.Operation)
	repair.Before = strings.TrimSpace(repair.Before)
	repair.After = strings.TrimSpace(repair.After)
	repair.Determination = strings.TrimSpace(repair.Determination)
	repair.Detail = strings.TrimSpace(repair.Detail)
	if requester == "" || repair.Check == "" || repair.Operation == "" || repair.Before == "" || repair.After == "" {
		return Decision{}, fmt.Errorf("maat diagnostic repair: requester, check, operation, before, and after are required")
	}
	if repair.Determination != "resolved" && repair.Determination != "failed" {
		return Decision{}, fmt.Errorf("maat diagnostic repair: determination must be resolved or failed")
	}
	if len(repair.Check) > 256 || len(repair.Operation) > 512 || len(repair.Before) > 16*1024 || len(repair.After) > 16*1024 || len(repair.Detail) > 16*1024 {
		return Decision{}, fmt.Errorf("maat diagnostic repair: repair fields exceed bounded record size")
	}

	sum := sha256.Sum256([]byte(strings.Join([]string{
		repair.Check, repair.Operation, repair.Before, repair.After, repair.Determination, repair.Detail,
	}, "\x00")))
	why := fmt.Sprintf("operation: %s; before: %s; after: %s", repair.Operation, repair.Before, repair.After)
	if repair.Detail != "" {
		why += "; detail: " + repair.Detail
	}
	decision := Decision{
		Kind:          "diagnostic verified repair",
		Requester:     requester,
		Resource:      repair.Check,
		Assessed:      "bounded diagnostic repair with post-repair verification",
		Determination: repair.Determination,
		Why:           why,
		Evidence:      "diagnostic-repair:sha256=" + hex.EncodeToString(sum[:]),
	}
	if err := j.Append(decision); err != nil {
		return Decision{}, fmt.Errorf("maat diagnostic repair: append repair outcome: %w", err)
	}
	return decision, nil
}
