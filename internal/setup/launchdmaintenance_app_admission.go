package setup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
)

type MaintenanceAppTicket struct {
	Schema          int             `json:"schema"`
	Generation      uint64          `json:"generation"`
	ID              string          `json:"operation_id"`
	Nonce           string          `json:"nonce"`
	Operation       string          `json:"operation"`
	Plan            MaintenancePlan `json:"plan"`
	Transaction     string          `json:"transaction"`
	RestoreSHA256   string          `json:"restore_sha256"`
	RestoreRevision uint64          `json:"restore_revision"`
}

func (t MaintenanceAppTicket) validate() error {
	if t.Schema != 1 || t.Generation == 0 || !appHex(t.ID, 16) || !appHex(t.Nonce, 16) || !appHex(t.Transaction, 16) || t.Plan.validate() != nil {
		return errors.New("maintenance app: invalid controller ticket")
	}
	switch t.Operation {
	case "confirm", "recover", "abort":
		if t.RestoreSHA256 != "" || t.RestoreRevision != 0 {
			return errors.New("maintenance app: unrelated restoration binding")
		}
	case "restore-confirm":
		if !appHex(t.RestoreSHA256, 32) || t.RestoreRevision == 0 {
			return errors.New("maintenance app: exact restoration binding required")
		}
	default:
		return errors.New("maintenance app: unknown ticket operation")
	}
	return nil
}
func DecodeMaintenanceAppTicket(raw []byte) (MaintenanceAppTicket, error) {
	var t MaintenanceAppTicket
	if len(raw) > 8192 || json.Unmarshal(raw, &t) != nil {
		return t, errors.New("maintenance app: bounded ticket required")
	}
	canonical, _ := json.Marshal(t)
	if string(raw) != string(canonical) || t.validate() != nil {
		return t, errors.New("maintenance app: exact closed controller ticket required")
	}
	return t, nil
}

type MaintenanceAppAdmission struct {
	Schema    int                  `json:"schema"`
	HighWater uint64               `json:"high_water"`
	Ticket    MaintenanceAppTicket `json:"ticket"`
	State     string               `json:"state"`
	Result    MaintenanceStatus    `json:"result"`
	Error     string               `json:"error"`
}
type MaintenanceAppView struct {
	Status     MaintenanceStatus       `json:"status"`
	Admission  MaintenanceAppAdmission `json:"admission"`
	TicketJSON string                  `json:"ticket_json"`
}
type MaintenanceAppPreview struct {
	Plan       MaintenancePlan      `json:"plan"`
	Ticket     MaintenanceAppTicket `json:"ticket"`
	TicketJSON string               `json:"ticket_json"`
}
type MaintenanceAppResolution struct {
	Ticket            MaintenanceAppTicket `json:"ticket"`
	Disposition       string               `json:"disposition"`
	Status            MaintenanceStatus    `json:"status"`
	SafeForNewPreview bool                 `json:"safe_for_new_preview"`
	PastOutcome       string               `json:"past_outcome"`
}
type MaintenanceAppMutationResult struct {
	TicketJSON        string            `json:"ticket_json"`
	Status            MaintenanceStatus `json:"status"`
	Terminal          bool              `json:"terminal"`
	SafeForNewPreview bool              `json:"safe_for_new_preview"`
}

func decodeMaintenanceAppState(raw []byte, result *MaintenanceAppAdmission) error {
	if len(raw) == 0 || len(raw) > 32*1024 || json.Unmarshal(raw, result) != nil {
		return errors.New("maintenance app: corrupt bounded admission state")
	}
	canonical, _ := json.Marshal(result)
	if string(raw) != string(append(canonical, '\n')) || result.Schema != 1 || result.HighWater < result.Ticket.Generation || result.Ticket.validate() != nil {
		return errors.New("maintenance app: exact admission state required; no reset")
	}
	switch result.State {
	case "reserved", "running", "terminal":
		if result.HighWater != result.Ticket.Generation {
			return errors.New("maintenance app: admission generation differs")
		}
	case "fenced":
		if result.HighWater <= result.Ticket.Generation {
			return errors.New("maintenance app: fence high-water did not advance")
		}
	default:
		return errors.New("maintenance app: unknown admission state")
	}
	return nil
}
func validAppStateTransition(old, next MaintenanceAppAdmission) error {
	raw, _ := json.Marshal(next)
	var checked MaintenanceAppAdmission
	if err := decodeMaintenanceAppState(append(raw, '\n'), &checked); err != nil {
		return err
	}
	if next.HighWater < old.HighWater {
		return errors.New("maintenance app: monotonic generation cannot reset/overflow")
	}
	if next.State == "reserved" {
		if old.HighWater == math.MaxUint64 || old.State == "running" || next.HighWater != old.HighWater+1 || next.Ticket.Generation != next.HighWater || next.Ticket.ID == old.Ticket.ID {
			return errors.New("maintenance app: invalid reservation supersession")
		}
		return nil
	}
	if next.Ticket != old.Ticket {
		return errors.New("maintenance app: immutable ticket changed")
	}
	switch next.State {
	case "running":
		if old.State == "reserved" && next.HighWater == old.HighWater && next.Result == old.Result {
			return nil
		}
	case "terminal":
		if old.State == "running" && next.HighWater == old.HighWater {
			return nil
		}
	case "fenced":
		if (old.State == "reserved" || old.State == "running") && next.HighWater == old.HighWater+1 {
			return nil
		}
	}
	return errors.New("maintenance app: invalid admission transition")
}
func appRandom() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func appDebt(d maintenanceOperations, s MaintenanceStatus) (bool, error) {
	if s.ID == "" {
		return false, nil
	}
	if s.Phase != "completed" && s.Phase != "aborted" {
		return true, nil
	}
	if d.pending != nil {
		return d.pending(s)
	}
	return false, nil
}
func appStoreAvailable(d maintenanceOperations) error {
	if d.appLoad == nil || d.appPersist == nil || d.appSync == nil {
		return errors.New("maintenance app: durable admission unavailable; no operation was admitted")
	}
	return nil
}

// Local-app-only API. Agent commands keep their original authorizer and wire.
func (c ServiceController) RunLegacyAppMaintenance(ctx context.Context, r LegacyAppRequest) (any, error) {
	if _, err := r.Arguments(); err != nil {
		return nil, err
	}
	d, err := c.maintenance(ctx)
	if err != nil {
		return nil, err
	}
	if err := appStoreAvailable(d); err != nil {
		return nil, err
	}
	if r.Operation == "restore-preview" {
		return c.PreviewLegacyRestore(ctx, r.Transaction, r.PlanSHA256, r.Artifact)
	}
	var output any
	err = d.lock(func() error {
		state, err := d.appLoad()
		if err != nil {
			return err
		}
		if r.Operation == "status" {
			s, err := d.load()
			raw, _ := json.Marshal(state.Ticket)
			output = MaintenanceAppView{s, state, string(raw)}
			return err
		}
		if r.Operation == "preview" || r.Operation == "prepare-operation" {
			if state.State == "running" {
				return errors.New("maintenance app: resolve the exact outstanding operation before another admission")
			}
			current, err := d.load()
			if err != nil {
				return err
			}
			debt, err := appDebt(d, current)
			if err != nil {
				return err
			}
			if r.Operation == "preview" && debt {
				return errors.New("maintenance app: retained lifecycle debt requires exact recovery, not a new confirmation")
			}
			if state.HighWater == math.MaxUint64 {
				return errors.New("maintenance app: generation exhausted; retained state cannot reset")
			}
			var plan MaintenancePlan
			op := r.TargetOperation
			if r.Operation == "preview" {
				plan, err = d.preview(ctx, r.Legacy)
				op = "confirm"
			} else {
				plan = current.Plan
				if current.ID != r.Transaction || plan.SHA256 != r.PlanSHA256 {
					return errors.New("maintenance app: recovery selection changed")
				}
			}
			if err != nil || plan.validate() != nil {
				return errors.Join(errors.New("maintenance app: preview invalid"), err)
			}
			id, err := appRandom()
			if err != nil {
				return err
			}
			nonce, err := appRandom()
			if err != nil {
				return err
			}
			t := MaintenanceAppTicket{Schema: 1, Generation: state.HighWater + 1, ID: id, Nonce: nonce, Operation: op, Plan: plan, Transaction: r.Transaction}
			if op == "confirm" {
				t.Transaction = nonce
			}
			if op == "restore-confirm" {
				p, err := DecodeMaintenanceRestorePlan([]byte(r.RestoreJSON))
				if err != nil || p.validate(current) != nil {
					return errors.New("maintenance app: captured restoration no longer matches")
				}
				t.RestoreSHA256 = p.SHA256
				t.RestoreRevision = p.Revision
			}
			if err := t.validate(); err != nil {
				return err
			}
			raw, err := json.Marshal(t)
			if err != nil || len(raw) > 8192 {
				return errors.New("maintenance app: ticket exceeds client/controller bound; no reservation created")
			}
			next := MaintenanceAppAdmission{Schema: 1, HighWater: t.Generation, Ticket: t, State: "reserved"}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := d.appPersist(state, next); err != nil {
				return err
			}
			output = MaintenanceAppPreview{plan, t, string(raw)}
			return nil
		}
		t, err := DecodeMaintenanceAppTicket([]byte(r.TicketJSON))
		if err != nil {
			return err
		}
		if r.Operation == "resolve-operation" {
			resolution := MaintenanceAppResolution{Ticket: t, PastOutcome: "unknown"}
			current, err := d.load()
			if err != nil {
				return err
			}
			debt, err := appDebt(d, current)
			if err != nil {
				return err
			}
			if state.Ticket != t {
				if t.Generation >= state.HighWater {
					return errors.New("maintenance app: operation binding differs")
				}
				if err := d.appSync(state); err != nil {
					return err
				}
				resolution.Disposition = "future-fenced-past-detail-not-retained"
				resolution.SafeForNewPreview = !debt && state.State != "running"
				output = resolution
				return nil
			}
			resolution.Status = current
			if current.ID != "" && (current.ID != t.Transaction || current.Plan != t.Plan) {
				resolution.Status = MaintenanceStatus{}
			}
			if state.State == "terminal" {
				if err := d.appSync(state); err != nil {
					return err
				}
				resolution.Disposition = "matching-terminal"
				resolution.Status = state.Result
				resolution.PastOutcome = "recorded-result"
				resolution.SafeForNewPreview = !debt
				output = resolution
				return nil
			}
			if state.State != "fenced" {
				if state.HighWater == math.MaxUint64 {
					return errors.New("maintenance app: generation exhausted")
				}
				next := state
				next.HighWater++
				next.State = "fenced"
				next.Result = resolution.Status
				if state.State == "reserved" {
					next.Error = "never-admitted"
				} else {
					next.Error = "admitted-past-outcome-unknown"
				}
				if err := d.appPersist(state, next); err != nil {
					return err
				}
				state = next
			}
			if err := d.appSync(state); err != nil {
				return err
			}
			resolution.Disposition = "future-execution-fenced"
			if state.Error == "never-admitted" {
				resolution.PastOutcome = "never-admitted"
			} // Only a never-running durable reservation.
			if debt {
				resolution.Disposition = "fenced-interrupted-needs-recovery"
			}
			resolution.SafeForNewPreview = !debt
			output = resolution
			return nil
		}
		if state.Ticket != t || state.HighWater != t.Generation || state.State == "fenced" {
			return errors.New("maintenance app: superseded or fenced operation; no late execution")
		}
		if state.State == "terminal" {
			if err := d.appSync(state); err != nil {
				return err
			}
			s, e := d.load()
			if e != nil {
				return e
			}
			debt, e := appDebt(d, s)
			if e != nil {
				return e
			}
			output = MaintenanceAppMutationResult{r.TicketJSON, state.Result, true, !debt}
			if state.Error != "" {
				return errors.New(state.Error)
			}
			return nil
		}
		if state.State != "reserved" {
			return errors.New("maintenance app: operation already admitted; resolve its result rather than replay")
		}
		if t.Operation != r.Operation || r.PlanSHA256 != t.Plan.SHA256 || r.Operation != "confirm" && r.Transaction != t.Transaction || r.Operation == "confirm" && r.Legacy != t.Plan.LegacyPayload {
			return errors.New("maintenance app: operation selectors differ from the ticket")
		}
		if r.Operation == "restore-confirm" {
			p, err := DecodeMaintenanceRestorePlan([]byte(r.RestoreJSON))
			if err != nil || p.SHA256 != t.RestoreSHA256 || p.Revision != t.RestoreRevision {
				return errors.New("maintenance app: restoration ticket differs")
			}
		}
		running := state
		running.State = "running"
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := d.appPersist(state, running); err != nil {
			return err
		}
		var result MaintenanceStatus
		workErr := ctx.Err()
		if workErr == nil {
			switch r.Operation {
			case "confirm":
				result, workErr = confirmLegacyMaintenanceLocked(ctx, d, t.Plan, t.Transaction)
			case "recover":
				result, workErr = recoverLegacyMaintenanceLocked(ctx, d, t.Transaction, t.Plan.SHA256)
			case "abort":
				result, workErr = abortLegacyMaintenanceLocked(ctx, d, t.Transaction, t.Plan.SHA256)
			case "restore-confirm":
				p, _ := DecodeMaintenanceRestorePlan([]byte(r.RestoreJSON))
				result, workErr = confirmLegacyRestoreLocked(ctx, d, p)
			default:
				workErr = errors.New("maintenance app: unsupported mutation")
			}
		}
		done := running
		done.State = "terminal"
		done.Result = result
		if result.ID != "" && (result.ID != t.Transaction || result.Plan != t.Plan) {
			done.Result = MaintenanceStatus{}
		}
		if workErr != nil {
			done.Error = string([]rune(workErr.Error())[:min(1200, len([]rune(workErr.Error())))])
		}
		persistErr := d.appPersist(running, done)
		observed, inspectErr := d.load()
		debt := true
		if inspectErr == nil {
			debt, inspectErr = appDebt(d, observed)
		}
		output = MaintenanceAppMutationResult{r.TicketJSON, done.Result, persistErr == nil, !debt && inspectErr == nil}
		return errors.Join(workErr, persistErr, inspectErr)
	})
	return output, err
}
