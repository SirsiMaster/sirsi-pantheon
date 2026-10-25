//go:build darwin

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

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
				r.Operation = "confirm"
				r.Legacy = "/owned/old"
				r.PlanSHA256 = strings.Repeat("1", 64)
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
