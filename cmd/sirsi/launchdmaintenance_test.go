package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

type maintenanceCommandFixture struct {
	calls []string
	fail  bool
}

func (f *maintenanceCommandFixture) PreviewLegacyMaintenance(_ context.Context, path string) (setup.MaintenancePlan, error) {
	f.calls = append(f.calls, "preview:"+path)
	return setup.MaintenancePlan{SHA256: strings.Repeat("1", 64)}, nil
}
func (f *maintenanceCommandFixture) ConfirmLegacyMaintenance(_ context.Context, p setup.MaintenancePlan) (setup.MaintenanceStatus, error) {
	f.calls = append(f.calls, "confirm:"+p.SHA256)
	if f.fail {
		return setup.MaintenanceStatus{ID: "retained", Phase: "bootstrap_requested"}, errors.New("owned injected bootstrap failure")
	}
	return setup.MaintenanceStatus{ID: "retained", Phase: "completed"}, nil
}
func (f *maintenanceCommandFixture) LegacyMaintenanceStatus(context.Context) (setup.MaintenanceStatus, error) {
	f.calls = append(f.calls, "status")
	return setup.MaintenanceStatus{ID: "retained", Phase: "bootstrap_requested"}, nil
}
func (f *maintenanceCommandFixture) RecoverLegacyMaintenance(_ context.Context, id, digest string) (setup.MaintenanceStatus, error) {
	f.calls = append(f.calls, "recover:"+id+":"+digest)
	return setup.MaintenanceStatus{ID: id, Phase: "completed"}, nil
}

func TestLegacyMaintenanceCommandEntry(t *testing.T) {
	digest := strings.Repeat("1", 64)
	for _, entry := range []struct {
		name string
		args []string
		want string
		fail bool
	}{
		{"preview", []string{"preview", "--legacy-payload", "/owned/old"}, "preview:/owned/old", false},
		{"confirm", []string{"confirm", "--legacy-payload", "/owned/old", "--expected-plan-sha256", digest, "--confirm-maintenance"}, "confirm:" + digest, false},
		{"status", []string{"status"}, "status", false},
		{"recover", []string{"recover", "--transaction", "retained", "--expected-plan-sha256", digest, "--confirm-maintenance"}, "recover:retained:" + digest, false},
		{"failure_debt", []string{"confirm", "--legacy-payload", "/owned/old", "--expected-plan-sha256", digest, "--confirm-maintenance"}, "confirm:" + digest, true},
	} {
		t.Run(entry.name, func(t *testing.T) {
			f := &maintenanceCommandFixture{fail: entry.fail}
			cmd := newLegacyMaintenanceCmd(f)
			cmd.SilenceUsage = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(entry.args)
			err := cmd.Execute()
			if (err != nil) != entry.fail {
				t.Fatal(err)
			}
			if len(f.calls) == 0 || f.calls[len(f.calls)-1] != entry.want {
				t.Fatal(f.calls)
			}
			if !strings.Contains(out.String(), `"`+map[bool]string{false: "sha256", true: "id"}[entry.name != "preview"]+`"`) {
				t.Fatal(out.String())
			}
			if entry.fail && !strings.Contains(out.String(), "bootstrap_requested") {
				t.Fatal("lost retained recovery result")
			}
		})
	}
}

func TestLegacyMaintenanceCommandRefusesImplicitOrArbitraryAuthority(t *testing.T) {
	for _, args := range [][]string{{"confirm"}, {"confirm", "--drained"}, {"recover"}, {"status", "arbitrary-label"}, {"preview", "--legacy-payload", "/owned/old", "extra"}, {"confirm", "--legacy-payload", "/owned/old", "--expected-plan-sha256", "changed", "--confirm-maintenance"}} {
		f := &maintenanceCommandFixture{}
		cmd := newLegacyMaintenanceCmd(f)
		cmd.SilenceUsage = true
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
		for _, call := range f.calls {
			if strings.HasPrefix(call, "confirm:") || strings.HasPrefix(call, "recover:") {
				t.Fatal("mutation before exact confirmation", f.calls)
			}
		}
	}
}
