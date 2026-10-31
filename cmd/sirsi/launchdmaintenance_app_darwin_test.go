//go:build darwin

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

func (f *maintenanceCommandFixture) RunLegacyAppMaintenance(ctx context.Context, r setup.LegacyAppRequest) (any, error) {
	switch r.Operation {
	case "status":
		return f.LegacyMaintenanceStatus(ctx)
	case "confirm":
		return f.ConfirmLegacyMaintenance(ctx, setup.MaintenancePlan{SHA256: r.PlanSHA256})
	}
	return nil, errors.New("inert unsupported operation")
}
func appEntryTicketFixture(t *testing.T) setup.MaintenanceAppTicket {
	t.Helper()
	p := setup.MaintenancePlan{Profile: setup.RootRelayMaintenanceProfile,
		Original:      setup.ServiceInstance{Label: "ai.sirsi.router.relay", Target: "system/ai.sirsi.router.relay", Plist: "/Library/LaunchDaemons/ai.sirsi.router.relay.plist", PlistSHA256: strings.Repeat("1", 64), Program: setup.LegacyAppEngine, ProgramSHA256: strings.Repeat("2", 64), PID: 123, Started: "fixture"},
		LegacyPayload: "/owned/old", LegacySHA256: strings.Repeat("3", 64), LoadedCDHash: strings.Repeat("4", 40), ReplacementCDHash: strings.Repeat("5", 40), KernelStart: "fixture-kernel", LegacySource: "owned-fixture", RootInode: 1,
		WriterScope: "cooperative-publication-only; uncovered-writers-and-old-remote-outcomes-unknown"}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	p.SHA256 = hex.EncodeToString(hash[:])
	return setup.MaintenanceAppTicket{Schema: 1, Generation: 1, ID: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 32), Operation: "confirm", Plan: p, Transaction: strings.Repeat("c", 32)}
}

func TestLegacyAppEntryDenialAndStructuredDebt(t *testing.T) {
	for _, kind := range []string{"denied", "status", "confirm_failure", "extra_field"} {
		t.Run(kind, func(t *testing.T) {
			f := &maintenanceCommandFixture{fail: kind == "confirm_failure"}
			calls := 0
			cmd := newLegacyAppMaintenanceCmd(func(context.Context, setup.LegacyAppRequest) (legacyMaintenanceController, error) {
				calls++
				if kind == "denied" {
					return nil, errors.New("OS identity refused")
				}
				return f, nil
			})
			r := setup.LegacyAppRequest{Profile: setup.RootRelayMaintenanceProfile, Operation: "status", EngineSHA256: strings.Repeat("a", 64)}
			if kind == "confirm_failure" {
				ticket := appEntryTicketFixture(t)
				r.Operation = "confirm"
				r.Legacy = "/owned/old"
				r.PlanSHA256 = ticket.Plan.SHA256
				raw, _ := json.Marshal(ticket)
				r.TicketJSON = string(raw)
			}
			raw, _ := json.Marshal(r)
			if kind == "extra_field" {
				raw = bytes.Replace(raw, []byte(`"operation":`), []byte(`"extra":true,"operation":`), 1)
			}
			cmd.SetArgs([]string{base64.StdEncoding.EncodeToString(raw)})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			err := cmd.Execute()
			if kind == "extra_field" {
				if err == nil || calls != 0 || len(f.calls) != 0 {
					t.Fatal("malformed request reached factory")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var reply setup.LegacyAppReply
			if err := json.Unmarshal(out.Bytes(), &reply); err != nil {
				t.Fatal(err)
			}
			if reply.OK != (kind == "status") {
				t.Fatal(reply)
			}
			body, err := base64.StdEncoding.DecodeString(reply.BodyBase64)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "denied" && len(f.calls) != 0 {
				t.Fatal("denied caller reached controller")
			}
			if kind == "confirm_failure" && (!bytes.Contains(body, []byte("bootstrap_requested")) || reply.Error == "") {
				t.Fatal("nonzero operation debt discarded", reply)
			}
		})
	}
}
