package setup

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
)

const LegacyAppEngine = "/usr/local/libexec/sirsi-pantheon/sirsi"
const LegacyAppRequestLimit = 24 * 1024

// Public selectors only. Local OS authorization is not a registry/agent grant.
// This envelope cannot select a command, executable, environment or directory.
type LegacyAppRequest struct {
	Profile         string `json:"profile"`
	Operation       string `json:"operation"`
	EngineSHA256    string `json:"engine_sha256"`
	Legacy          string `json:"legacy"`
	Transaction     string `json:"transaction"`
	PlanSHA256      string `json:"plan_sha256"`
	Artifact        string `json:"artifact"`
	RestoreJSON     string `json:"restore_json"`
	TicketJSON      string `json:"ticket_json"`
	TargetOperation string `json:"target_operation"`
}

func DecodeLegacyAppRequest(encoded string) (LegacyAppRequest, error) {
	var result LegacyAppRequest
	if len(encoded) > base64.StdEncoding.EncodedLen(LegacyAppRequestLimit) {
		return result, errors.New("maintenance app: request exceeds bound")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(raw) > LegacyAppRequestLimit {
		return result, errors.New("maintenance app: bounded base64 request required")
	}
	// Exact keys and duplicate detection; encoding/json's case aliases are not
	// accepted. All values must be strings and every closed field is present.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return result, errors.New("maintenance app: object required")
	}
	allowed := map[string]bool{"profile": true, "operation": true, "engine_sha256": true, "legacy": true, "transaction": true, "plan_sha256": true, "artifact": true, "restore_json": true, "ticket_json": true, "target_operation": true}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return result, err
		}
		name, ok := key.(string)
		if !ok || !allowed[name] || seen[name] {
			return result, errors.New("maintenance app: unknown or duplicate field")
		}
		seen[name] = true
		value, err := decoder.Token()
		if _, ok := value.(string); err != nil || !ok {
			return result, errors.New("maintenance app: string fields required")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return result, err
	}
	if _, err := decoder.Token(); err != io.EOF || len(seen) != len(allowed) {
		return result, errors.New("maintenance app: complete closed object required")
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	_, err = result.Arguments()
	return result, err
}

func appHex(s string, n int) bool {
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == n && hex.EncodeToString(raw) == s
}
func appPath(s string) bool { return len(s) <= 4096 && filepath.IsAbs(s) && filepath.Clean(s) == s }

func (r LegacyAppRequest) Arguments() ([]string, error) {
	bad := errors.New("maintenance app: exact operation/profile/selector binding required")
	if r.Profile != RootRelayMaintenanceProfile || !appHex(r.EngineSHA256, 32) {
		return nil, bad
	}
	args := []string{r.Operation}
	switch r.Operation {
	case "prepare-operation":
		if r.TargetOperation != "recover" && r.TargetOperation != "abort" && r.TargetOperation != "restore-confirm" {
			return nil, bad
		}
		if !appHex(r.Transaction, 16) || !appHex(r.PlanSHA256, 32) || r.Legacy != "" || r.Artifact != "" || r.TicketJSON != "" {
			return nil, bad
		}
		if r.TargetOperation == "restore-confirm" {
			p, err := DecodeMaintenanceRestorePlan([]byte(r.RestoreJSON))
			if err != nil || p.Transaction != r.Transaction || p.MaintenanceSHA256 != r.PlanSHA256 {
				return nil, bad
			}
		} else if r.RestoreJSON != "" {
			return nil, bad
		}
		return args, nil
	case "resolve-operation":
		if r.Legacy != "" || r.Transaction != "" || r.PlanSHA256 != "" || r.Artifact != "" || r.RestoreJSON != "" || r.TargetOperation != "" {
			return nil, bad
		}
		if _, err := DecodeMaintenanceAppTicket([]byte(r.TicketJSON)); err != nil {
			return nil, bad
		}
		return args, nil
	case "confirm", "recover", "abort", "restore-confirm":
		t, err := DecodeMaintenanceAppTicket([]byte(r.TicketJSON))
		if err != nil || t.Operation != r.Operation || r.TargetOperation != "" {
			return nil, bad
		}
	default:
		if r.TicketJSON != "" || r.TargetOperation != "" {
			return nil, bad
		}
	}
	switch r.Operation {
	case "preview":
		if !appPath(r.Legacy) || r.Transaction != "" || r.PlanSHA256 != "" || r.Artifact != "" || r.RestoreJSON != "" {
			return nil, bad
		}
		return append(args, "--legacy-payload", r.Legacy), nil
	case "confirm":
		if !appPath(r.Legacy) || !appHex(r.PlanSHA256, 32) || r.Transaction != "" || r.Artifact != "" || r.RestoreJSON != "" {
			return nil, bad
		}
		return append(args, "--legacy-payload", r.Legacy, "--expected-plan-sha256", r.PlanSHA256, "--confirm-maintenance"), nil
	case "status":
		if r.Legacy != "" || r.Transaction != "" || r.PlanSHA256 != "" || r.Artifact != "" || r.RestoreJSON != "" {
			return nil, bad
		}
		return args, nil // Read-only discovery after a lost transaction reply.
	case "recover", "abort", "restore-preview":
		if !appHex(r.Transaction, 16) || !appHex(r.PlanSHA256, 32) || r.Legacy != "" || r.RestoreJSON != "" {
			return nil, bad
		}
		args = append(args, "--transaction", r.Transaction, "--expected-plan-sha256", r.PlanSHA256)
		if r.Operation == "restore-preview" {
			if !appPath(r.Artifact) {
				return nil, bad
			}
			return append(args, "--artifact", r.Artifact), nil
		}
		if r.Artifact != "" {
			return nil, bad
		}
		return append(args, "--confirm-maintenance"), nil
	case "restore-confirm":
		if r.Legacy != "" || r.Artifact != "" || !appHex(r.Transaction, 16) || !appHex(r.PlanSHA256, 32) {
			return nil, bad
		}
		plan, err := DecodeMaintenanceRestorePlan([]byte(r.RestoreJSON))
		if err != nil || plan.Transaction != r.Transaction || plan.MaintenanceSHA256 != r.PlanSHA256 || !appHex(plan.SHA256, 32) {
			return nil, bad
		}
		return append(args, "--expected-restore-sha256", plan.SHA256, "--confirm-maintenance"), nil
	}
	return nil, bad
}

type LegacyAppReply struct {
	Schema     int    `json:"schema"`
	Operation  string `json:"operation"`
	OK         bool   `json:"ok"`
	BodyBase64 string `json:"body_base64"`
	Error      string `json:"error"`
}
