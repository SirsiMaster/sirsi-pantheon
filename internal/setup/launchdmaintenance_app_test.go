package setup

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func appRequestFixture() LegacyAppRequest {
	return LegacyAppRequest{Profile: RootRelayMaintenanceProfile, Operation: "status", EngineSHA256: strings.Repeat("a", 64)}
}
func encodeAppFixture(t *testing.T, r LegacyAppRequest) string {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestLegacyAppRequestClosedSelectors(t *testing.T) {
	for _, op := range []string{"preview", "confirm", "status", "recover", "abort", "restore-preview", "restore-confirm"} {
		t.Run(op, func(t *testing.T) {
			r := appRequestFixture()
			r.Operation = op
			switch op {
			case "restore-confirm":
				r.Transaction = strings.Repeat("c", 32)
				r.PlanSHA256 = strings.Repeat("b", 64)
				p := MaintenanceRestorePlan{Transaction: r.Transaction, MaintenanceSHA256: r.PlanSHA256, SHA256: strings.Repeat("d", 64)}
				encoded, err := json.Marshal(p)
				if err != nil {
					t.Fatal(err)
				}
				r.RestoreJSON = string(append(encoded, '\n'))
			case "preview", "confirm":
				r.Legacy = "/owned/a ' \" ; $(never)"
				if op == "confirm" {
					r.PlanSHA256 = strings.Repeat("b", 64)
				}
			case "recover", "abort", "restore-preview":
				r.Transaction = strings.Repeat("c", 32)
				r.PlanSHA256 = strings.Repeat("b", 64)
				if op == "restore-preview" {
					r.Artifact = "/owned/artifact"
				}
			}
			if op == "confirm" || op == "recover" || op == "abort" || op == "restore-confirm" {
				plan := fixtureMaintenancePlan()
				if r.Legacy != "" {
					plan.LegacyPayload = r.Legacy
					plan.SHA256 = maintenanceDigest(plan)
				}
				r.PlanSHA256 = plan.SHA256
				transaction := r.Transaction
				if transaction == "" {
					transaction = strings.Repeat("c", 32)
				}
				ticket := MaintenanceAppTicket{Schema: 1, Generation: 1, ID: strings.Repeat("d", 32), Nonce: strings.Repeat("e", 32), Operation: op, Plan: plan, Transaction: transaction}
				if op == "restore-confirm" {
					restoration := MaintenanceRestorePlan{Transaction: transaction, MaintenanceSHA256: r.PlanSHA256, SHA256: strings.Repeat("f", 64), Revision: 1}
					raw, _ := json.Marshal(restoration)
					r.RestoreJSON = string(append(raw, '\n'))
					ticket.RestoreSHA256 = restoration.SHA256
					ticket.RestoreRevision = 1
				}
				raw, _ := json.Marshal(ticket)
				r.TicketJSON = string(raw)
			}
			got, err := DecodeLegacyAppRequest(encodeAppFixture(t, r))
			if err != nil || got != r {
				t.Fatal(got, err)
			}
			args, err := got.Arguments()
			if err != nil || args[0] != op {
				t.Fatal(args, err)
			}
		})
	}
}
func TestLegacyAppRequestRefusesAliasesNullExtraAndCommand(t *testing.T) {
	raw, _ := json.Marshal(appRequestFixture())
	for _, kind := range []string{"duplicate", "alias", "extra", "null", "missing", "trailing", "operation", "profile", "selector", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			input := string(raw)
			switch kind {
			case "duplicate":
				input = strings.Replace(input, `"operation":"status"`, `"operation":"status","operation":"status"`, 1)
			case "alias":
				input = strings.Replace(input, `"operation":`, `"Operation":`, 1)
			case "extra":
				input = strings.Replace(input, `"operation":"status"`, `"command":"id","operation":"status"`, 1)
			case "null":
				input = strings.Replace(input, `"legacy":""`, `"legacy":null`, 1)
			case "missing":
				input = strings.Replace(input, `"legacy":"",`, "", 1)
			case "trailing":
				input += "{}"
			case "operation":
				input = strings.Replace(input, `"status"`, `"restart"`, 1)
			case "profile":
				input = strings.Replace(input, RootRelayMaintenanceProfile, "arbitrary", 1)
			case "selector":
				input = strings.Replace(input, `"legacy":""`, `"legacy":"/unexpected"`, 1)
			case "oversize":
				input = strings.Repeat(" ", LegacyAppRequestLimit+1)
			}
			if _, err := DecodeLegacyAppRequest(base64.StdEncoding.EncodeToString([]byte(input))); err == nil {
				t.Fatal("unsafe app request accepted", kind)
			}
		})
	}
}
