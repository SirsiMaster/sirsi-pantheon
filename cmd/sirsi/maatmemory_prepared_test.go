package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

// The real writer supplies the retained bytes/intent. Remove only its commit
// in this private fixture to represent process loss before commit publication.
func TestMaatPreparedReceiptInspectorAndConfirmedCommandWithoutCasebookRow(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	s, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	r, err := s.Preflight(maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".sirsi", "maat", "failure-memory")
	paths, err := filepath.Glob(filepath.Join(root, "writes", "*.prepared.json"))
	if err != nil {
		t.Fatal(err)
	}
	commit := ""
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var intent struct {
			Key       string `json:"key"`
			Leaf      string `json:"leaf"`
			Namespace string `json:"namespace"`
		}
		if err := json.Unmarshal(raw, &intent); err != nil {
			t.Fatal(err)
		}
		if intent.Namespace == "evidence" && intent.Leaf == h.Reference().ReceiptSHA256 {
			commit = filepath.Join(root, "writes", intent.Key+".committed.json")
		}
	}
	if commit == "" {
		t.Fatal("retention intent missing")
	}
	if err := os.Remove(commit); err != nil {
		t.Fatal(err)
	}
	oldJSON, oldGlobal := maatJSON, JsonOutput
	oldJournal, oldInstall := newMaatDecisionJournal, maatRepairInstallLiveness
	t.Cleanup(func() {
		maatJSON, JsonOutput = oldJSON, oldGlobal
		newMaatDecisionJournal, maatRepairInstallLiveness = oldJournal, oldInstall
	})
	maatJSON, JsonOutput = true, false
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) {
		t.Fatal("inspector/recovery opened a Casebook journal")
		return nil, nil
	}
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) {
		t.Fatal("receipt recovery ran installer")
		return "", nil
	}
	inspectFlags := maatRepairLivenessWatchCmd.Flags()
	for _, name := range []string{"inspect-failure-memory", "initialize-failure-memory", "reobserve-incomplete"} {
		f := inspectFlags.Lookup(name)
		value, changed := f.Value.String(), f.Changed
		t.Cleanup(func() {
			if err := f.Value.Set(value); err != nil {
				t.Error(err)
			}
			f.Changed = changed
		})
	}
	if err := maatRepairLivenessWatchCmd.ParseFlags([]string{"--inspect-failure-memory", "--initialize-failure-memory=false", "--reobserve-incomplete=false"}); err != nil {
		t.Fatal(err)
	}
	inspect := func() {
		if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, inspectFlags.Args()); err != nil {
			t.Fatal(err)
		}
	}
	var inspection maatLivenessMemoryInspection
	output := captureStdout(t, inspect)
	if err := json.Unmarshal([]byte(output), &inspection); err != nil {
		t.Fatal(err)
	}
	if inspection.PreparedPreflight == nil || inspection.ReceiptRecoveryError != "" {
		t.Fatalf("inspector did not expose recoverable receipt: %s", output)
	}
	prepared := inspection.PreparedPreflight
	if prepared.Target.Reference.ReceiptSHA256 != h.Reference().ReceiptSHA256 {
		t.Fatal("inspector selected unrelated receipt")
	}
	// Exercise actual Cobra flag wiring; restore bound values and Changed bits.
	flags := maatFailureMemoryReceiptCmd.Flags()
	names := []string{"sha256", "store", "json", "recover", "confirm", "prepared-target"}
	for _, name := range names {
		f := flags.Lookup(name)
		value, changed := f.Value.String(), f.Changed
		t.Cleanup(func() {
			if err := f.Value.Set(value); err != nil {
				t.Error(err)
			}
			f.Changed = changed
		})
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(prepared.TargetJSON))
	args := []string{"--sha256", prepared.Target.Reference.ReceiptSHA256, "--store=", "--json", "--recover", "--prepared-target", encoded, "--confirm=false"}
	if err := maatFailureMemoryReceiptCmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, flags.Args()); err == nil {
		t.Fatal("unconfirmed target recovered")
	}
	if _, err := os.Stat(commit); !os.IsNotExist(err) {
		t.Fatal("unconfirmed recovery changed intent")
	}
	if err := maatFailureMemoryReceiptCmd.ParseFlags([]string{"--confirm"}); err != nil {
		t.Fatal(err)
	}
	output = captureStdout(t, func() {
		if err := maatFailureMemoryReceiptCmd.RunE(maatFailureMemoryReceiptCmd, flags.Args()); err != nil {
			t.Fatal(err)
		}
	})
	var recovered maat.PreflightReceiptRead
	if err := json.Unmarshal([]byte(output), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.ActionAuthorized || recovered.Freshness != "historical" || !recovered.Receipt.EvaluatedAtUTC.Equal(r.EvaluatedAtUTC) {
		t.Fatal("recovery promoted history")
	}
	output = captureStdout(t, inspect)
	// Decode into a new value: absent fields do not clear pointers on reuse.
	var clean maatLivenessMemoryInspection
	if err := json.Unmarshal([]byte(output), &clean); err != nil {
		t.Fatal(err)
	}
	if clean.PreparedPreflight != nil || clean.IncompleteAttempt || clean.ProjectionDebt || clean.ReceiptRecoveryError != "" {
		t.Fatalf("recovery did not restore clean inspection: %s", output)
	}
	s, err = maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Preflight(r.Action); err != nil {
		t.Fatalf("fresh assessment unavailable: %v", err)
	}
}
