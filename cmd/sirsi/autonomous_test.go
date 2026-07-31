package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/autoheal"
)

func withAutonomousHarness(t *testing.T, outcomes []autoheal.Outcome) (*bytes.Buffer, *int) {
	t.Helper()
	t.Setenv("SIRSI_BRAIN_CONFIG", filepath.Join(t.TempDir(), "brain.yaml"))
	oldHeal := autonomousHealFn
	oldApprovedHeal := autonomousApprovedHealFn
	oldJSON := autonomousJSON
	calls := 0
	heal := func(_, _ string) ([]autoheal.Outcome, error) {
		calls++
		return outcomes, nil
	}
	autonomousHealFn = heal
	autonomousApprovedHealFn = heal
	autonomousJSON = false
	t.Cleanup(func() {
		autonomousHealFn = oldHeal
		autonomousApprovedHealFn = oldApprovedHeal
		autonomousJSON = oldJSON
	})
	var out bytes.Buffer
	return &out, &calls
}

func TestAutonomousOnRunsOneFixPassNow(t *testing.T) {
	out, calls := withAutonomousHarness(t, []autoheal.Outcome{{
		Check: "Swap Usage", Fix: "sirsi relieve --memory", Applied: true, Reason: "applied",
	}})
	cmd := autonomousCmd
	cmd.SetOut(out)
	if err := runAutonomous(cmd, []string{"on"}); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("autoheal calls = %d, want one immediate fix pass", *calls)
	}
	got := out.String()
	if !strings.Contains(got, "Autonomous mode → ON") || !strings.Contains(got, "1 applied") {
		t.Fatalf("output did not prove the pass:\n%s", got)
	}
}

func TestAutonomousOnReportsModeWhenFixPassFails(t *testing.T) {
	out, _ := withAutonomousHarness(t, nil)
	wantErr := errors.New("repair lever failed")
	autonomousHealFn = func(_, _ string) ([]autoheal.Outcome, error) {
		return nil, wantErr
	}
	cmd := autonomousCmd
	cmd.SetOut(out)

	err := runAutonomous(cmd, []string{"on"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runAutonomous error = %v, want %v", err, wantErr)
	}
	got := out.String()
	if !strings.Contains(got, "Autonomous mode → ON") {
		t.Fatalf("persisted ON state must be rendered even when the pass fails:\n%s", got)
	}
	if !strings.Contains(got, "Fix pass failed: repair lever failed") {
		t.Fatalf("fix-pass failure must be visible in the same output:\n%s", got)
	}
}

func TestAutonomousJSONReportsPersistedModeWhenFixPassFails(t *testing.T) {
	out, _ := withAutonomousHarness(t, nil)
	wantErr := errors.New("repair lever failed")
	autonomousHealFn = func(_, _ string) ([]autoheal.Outcome, error) {
		return nil, wantErr
	}
	autonomousJSON = true
	cmd := autonomousCmd
	cmd.SetOut(out)

	err := runAutonomous(cmd, []string{"on"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runAutonomous error = %v, want %v", err, wantErr)
	}
	var got struct {
		Autonomous bool `json:"autonomous"`
		FixPass    struct {
			Error string `json:"error"`
		} `json:"fix_pass"`
	}
	if decodeErr := json.Unmarshal(out.Bytes(), &got); decodeErr != nil {
		t.Fatalf("decode JSON: %v\n%s", decodeErr, out.String())
	}
	if !got.Autonomous || got.FixPass.Error != wantErr.Error() {
		t.Fatalf("JSON must disclose persisted ON plus pass failure: %+v", got)
	}
}

func TestAutonomousRunExecutesOneFixPassEvenWhenSwitchIsOff(t *testing.T) {
	out, calls := withAutonomousHarness(t, []autoheal.Outcome{{
		Check: "RAM Pressure", Fix: "sirsi relieve --memory", Applied: true, Reason: "applied",
	}})
	cmd := autonomousCmd
	cmd.SetOut(out)
	if err := runAutonomous(cmd, []string{"run"}); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("autoheal calls = %d, want one explicit run attempt", *calls)
	}
	got := out.String()
	if !strings.Contains(got, "Autonomous mode is OFF") {
		t.Fatalf("run should not silently flip the persistent switch:\n%s", got)
	}
	if !strings.Contains(got, "Fix pass ran") || !strings.Contains(got, "1 applied") {
		t.Fatalf("explicit run should fix, not merely report:\n%s", got)
	}
}

func TestAutonomousJSONIncludesFixPassEvidence(t *testing.T) {
	out, _ := withAutonomousHarness(t, []autoheal.Outcome{{
		Check: "Disk Space", Fix: "sirsi clean --include-caution", Applied: false, Reason: "gated-proposal",
	}})
	autonomousJSON = true
	cmd := autonomousCmd
	cmd.SetOut(out)
	if err := runAutonomous(cmd, []string{"on"}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Autonomous bool `json:"autonomous"`
		FixPass    struct {
			Applied  int                `json:"applied"`
			Proposed int                `json:"proposed"`
			Outcomes []autoheal.Outcome `json:"outcomes"`
		} `json:"fix_pass"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if !got.Autonomous || got.FixPass.Applied != 0 || got.FixPass.Proposed != 1 || len(got.FixPass.Outcomes) != 1 {
		t.Fatalf("json missing fix-pass evidence: %+v", got)
	}
}
