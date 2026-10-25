package liveness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
)

func localFixtureProbes() []LocalProbe {
	var probes []LocalProbe
	for _, check := range localChecks {
		probes = append(probes, LocalProbe{Check: check, Read: func(context.Context) (LocalObservation, error) {
			return LocalObservation{Check: check, State: "observed", Detail: "owned fixture metadata"}, nil
		}})
	}
	return probes
}

func TestLocalWatchFreshHomeCasebook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	j, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunLocal(context.Background(), j, "fixture-local-host", localFixtureProbes())
	if err != nil || result.LocalRecord != "recorded" || result.RaDelivery != "not_attempted" || result.Verdict.Gate != maat.GatePass {
		t.Fatalf("%+v %v", result, err)
	}
	rows, err := j.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	view := casebook.Build(rows)
	if len(view.Cases) != 1 || view.Cases[0].Evidence != result.DecisionEvidence {
		t.Fatalf("Casebook did not retain exact observation: %+v", view)
	}
	for _, leaf := range []string{"router.db", "router-service.env", "pantheon-repo"} {
		if _, err = os.Stat(filepath.Join(home, ".sirsi", leaf)); !os.IsNotExist(err) {
			t.Fatalf("unexpected router artifact %s: %v", leaf, err)
		}
	}
}

func TestLocalWatchUnknownIsNotHealthy(t *testing.T) {
	for _, mode := range []string{"probe-error", "explicit-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			probes := localFixtureProbes()
			probes[0].Read = func(context.Context) (LocalObservation, error) {
				if mode == "probe-error" {
					return LocalObservation{}, errors.New("PRIVATE should not be retained")
				}
				return LocalObservation{Check: "local-watch", State: "unavailable", Detail: "fixture observation unavailable"}, nil
			}
			j := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
			r, err := RunLocal(context.Background(), j, "fixture", probes)
			if err != nil {
				t.Fatal(err)
			}
			if r.Verdict.Gate == maat.GatePass || r.Verdict.Floor.Passed || r.LocalRecord != "recorded" {
				t.Fatalf("unknown became healthy: %+v", r)
			}
		})
	}
}

func TestLocalWatchAbsentOptionalAndConcern(t *testing.T) {
	for _, concern := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional-absent", true: "watch-absent"}[concern], func(t *testing.T) {
			p := localFixtureProbes()
			p[2].Read = func(context.Context) (LocalObservation, error) {
				return LocalObservation{Check: "menubar", State: "not-applicable", Detail: "optional menubar not configured"}, nil
			}
			if concern {
				p[0].Read = func(context.Context) (LocalObservation, error) {
					return LocalObservation{Check: "local-watch", State: "observed", Concern: true, Detail: "local metadata watch is not installed"}, nil
				}
			}
			r, err := RunLocal(context.Background(), &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}, "fixture", p)
			if err != nil {
				t.Fatal(err)
			}
			if concern && (r.Verdict.Gate == maat.GatePass || r.Verdict.Findings[0].RepairID != maat.SystemOneRepairLivenessWatch) {
				t.Fatalf("missing closed repair: %+v", r)
			}
			if !concern && r.Verdict.Gate != maat.GatePass {
				t.Fatalf("optional missing became compulsory: %+v", r)
			}
		})
	}
}

func TestLocalWatchCancellationDoesNotAppend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := localFixtureProbes()
	p[0].Read = func(context.Context) (LocalObservation, error) {
		cancel()
		return LocalObservation{Check: "local-watch", State: "observed", Detail: "late metadata"}, nil
	}
	j := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	r, err := RunLocal(ctx, j, "fixture", p)
	if err == nil || r.LocalRecord != "failed" {
		t.Fatalf("canceled observation accepted: %+v %v", r, err)
	}
	if _, err = os.Stat(j.Path); !os.IsNotExist(err) {
		t.Fatalf("canceled pass appended: %v", err)
	}
}

type refusingLocalJournal struct{}

func (refusingLocalJournal) Append(maat.Decision) error          { return errors.New("fixture journal refused") }
func (refusingLocalJournal) Recent(int) ([]maat.Decision, error) { return nil, nil }

func TestLocalWatchJournalFailureAndClosedInputs(t *testing.T) {
	r, err := RunLocal(context.Background(), refusingLocalJournal{}, "fixture", localFixtureProbes())
	if err == nil || r.LocalRecord != "failed" || r.DecisionEvidence != "" {
		t.Fatalf("journal error hidden: %+v %v", r, err)
	}
	p := localFixtureProbes()
	p[0].Check = "arbitrary-plugin"
	if _, err = RunLocal(context.Background(), refusingLocalJournal{}, "fixture", p); err == nil {
		t.Fatal("unknown probe accepted")
	}
	p = localFixtureProbes()
	p[0].Read = func(context.Context) (LocalObservation, error) {
		return LocalObservation{Check: "local-watch", State: "invented", Detail: "fixture"}, nil
	}
	if _, err = RunLocal(context.Background(), refusingLocalJournal{}, "fixture", p); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestLocalWatchSuppliesFiniteProbeContexts(t *testing.T) {
	probes := localFixtureProbes()
	for i := range probes {
		check := probes[i].Check
		probes[i].Read = func(ctx context.Context) (LocalObservation, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 2*time.Second || time.Until(deadline) <= 0 {
				t.Fatal("probe has no finite budget")
			}
			return LocalObservation{Check: check, State: "observed", Detail: "bounded fixture read"}, nil
		}
	}
	if _, err := RunLocal(context.Background(), &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}, "fixture", probes); err != nil {
		t.Fatal(err)
	}
}
