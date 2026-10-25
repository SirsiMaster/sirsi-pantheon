package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

func TestLivenessLocalActualCommandFreshHome(t *testing.T) {
	for _, mode := range []string{"observed", "unavailable", "journal-failure", "positionals"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(home)
			journal := &maat.FileDecisionJournal{Path: filepath.Join(home, ".sirsi", "maat", "decisions.jsonl")}
			called := 0
			command := newLivenessRunCmd(localWatchDependencies{
				home: func() (string, error) { return home, nil }, engine: func() (string, error) { return filepath.Join(home, "inert-sirsi"), nil }, host: func() (string, error) { return "fixture-local-host", nil },
				journal: func() (maat.DecisionJournal, error) {
					if mode == "journal-failure" {
						return nil, errors.New("owned journal refusal")
					}
					return journal, nil
				},
				probes: func(_, _ string) []liveness.LocalProbe {
					var probes []liveness.LocalProbe
					for _, check := range []string{"local-watch", "launchd-disabled", "menubar", "memory-metadata"} {
						probes = append(probes, liveness.LocalProbe{Check: check, Read: func(context.Context) (liveness.LocalObservation, error) {
							called++
							if mode == "unavailable" {
								return liveness.LocalObservation{}, errors.New("fixture unavailable")
							}
							return liveness.LocalObservation{Check: check, State: "observed", Detail: "owned metadata"}, nil
						}})
					}
					return probes
				},
			})
			args := []string{"--local"}
			if mode == "positionals" {
				args = append(args, "ignored")
			}
			command.SetArgs(args)
			command.SilenceUsage = true
			var out bytes.Buffer
			command.SetOut(&out)
			command.SetErr(&out)
			err := command.Execute()
			if mode == "journal-failure" || mode == "positionals" {
				if err == nil || called != 0 {
					t.Fatalf("refusal reached probes: %d %v", called, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result liveness.LocalWatchResult
			if err = json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if called != 4 || result.LocalRecord != "recorded" || result.RaDelivery != "not_attempted" {
				t.Fatalf("entry did not execute exact local path: %+v calls=%d", result, called)
			}
			if mode == "unavailable" && result.Verdict.Gate == maat.GatePass {
				t.Fatal("unknown metadata accepted as healthy")
			}
			rows, err := journal.Recent(10)
			if err != nil {
				t.Fatal(err)
			}
			view := casebook.Build(rows)
			if len(view.Cases) != 1 || view.Cases[0].Evidence != result.DecisionEvidence {
				t.Fatalf("missing real Casebook record: %+v", view)
			}
			for _, name := range []string{filepath.Join(home, ".agents"), filepath.Join(home, ".sirsi", "router.db"), filepath.Join(home, ".sirsi", "pantheon-repo")} {
				if _, err = os.Stat(name); !os.IsNotExist(err) {
					t.Fatalf("unexpected router path %s: %v", name, err)
				}
			}
		})
	}
}

func TestLivenessWatchStatusRequiresExactLocalLoadedObservation(t *testing.T) {
	const home, engine = "/owned/home", "/Applications/Pantheon.app/Contents/MacOS/sirsi"
	inspect := func(_ context.Context, gotEngine, gotHome string, command liveness.LocalCommand) (liveness.LocalObservation, error) {
		if gotEngine != engine || gotHome != home || command != nil {
			t.Fatalf("status inspector identity engine=%q home=%q command=%v", gotEngine, gotHome, command != nil)
		}
		return liveness.LocalObservation{Check: "local-watch", State: "observed", Detail: "exact local schedule is loaded", ConfigurationSHA256: strings.Repeat("a", 64)}, nil
	}
	status, err := inspectLivenessWatchStatus(context.Background(), func() (string, error) { return home, nil }, func() (string, error) { return engine, nil }, inspect)
	if err != nil || !strings.Contains(status, "verified exact local schedule loaded") {
		t.Fatalf("verified status = %q, %v", status, err)
	}
	status, err = inspectLivenessWatchStatus(context.Background(), func() (string, error) { return home, nil }, func() (string, error) { return engine, nil }, func(context.Context, string, string, liveness.LocalCommand) (liveness.LocalObservation, error) {
		return liveness.LocalObservation{Check: "local-watch", State: "observed", Concern: true, Detail: "watch absent"}, nil
	})
	if err == nil || !strings.Contains(status, "not verified: watch absent") {
		t.Fatalf("absent watch status = %q, %v", status, err)
	}
}

func TestStandaloneLivenessWatchInstallUsesPackagedEngineSelector(t *testing.T) {
	const home = "/owned/operator-home"
	for _, tc := range []struct {
		name        string
		engine      string
		verify      func(string) error
		wantInstall bool
	}{
		{
			name:        "development-or-loose-engine-refused-before-publication",
			engine:      "/tmp/sirsi-test-binary",
			verify:      setup.VerifyPackagedMaatEngine,
			wantInstall: false,
		},
		{
			name:   "canonical-packaged-engine-accepted-through-inert-verifier",
			engine: "/Applications/Pantheon.app/Contents/MacOS/sirsi",
			verify: func(path string) error {
				if path != "/Applications/Pantheon.app/Contents/MacOS/sirsi" {
					t.Fatalf("packaged verifier received %q", path)
				}
				return nil
			},
			wantInstall: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verified, installed := false, false
			var steps []string
			command := newLivenessInstallCmd(livenessInstallDependencies{
				home:   func() (string, error) { return home, nil },
				engine: func() (string, error) { return tc.engine, nil },
				verify: func(path string) error {
					steps = append(steps, "verify")
					verified = true
					return tc.verify(path)
				},
				install: func(_ context.Context, engine, gotHome string) (string, error) {
					steps = append(steps, "install")
					installed = true
					if engine != tc.engine || gotHome != home {
						t.Fatalf("installer received engine=%q home=%q", engine, gotHome)
					}
					return "inert publication", nil
				},
			})
			command.SetArgs(nil)
			command.SilenceUsage = true
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			err := command.Execute()
			if !verified || installed != tc.wantInstall {
				t.Fatalf("selector calls verified=%v installed=%v, want installed=%v; err=%v", verified, installed, tc.wantInstall, err)
			}
			wantSteps := []string{"verify"}
			if tc.wantInstall {
				wantSteps = append(wantSteps, "install")
			}
			if !slices.Equal(steps, wantSteps) {
				t.Fatalf("publication order = %v, want %v", steps, wantSteps)
			}
			if tc.wantInstall && (err != nil || !strings.Contains(output.String(), "inert publication")) {
				t.Fatalf("canonical inert install = %q, %v", output.String(), err)
			}
			if !tc.wantInstall && err == nil {
				t.Fatal("development/loose executable was accepted")
			}
		})
	}
}
