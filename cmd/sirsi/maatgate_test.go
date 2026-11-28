package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/trustboundary"
)

func TestMaatGateRequestModeBoundaries(t *testing.T) {
	// Validate the selection in memory. None of these cases runs a gate,
	// queries a repo, installs a hook, or launches a subprocess.
	tests := []struct {
		name string
		mode gateRequestModes
		want string
	}{
		{name: "default-survey"},
		{name: "report-only", mode: gateRequestModes{report: true}},
		{name: "enforcement", mode: gateRequestModes{prePush: true}},
		{name: "explicit-range", mode: gateRequestModes{base: "base", head: "head", rangeSet: true}},
		{name: "lint-survey", mode: gateRequestModes{all: true, lintOnly: true}},
		{name: "lint-only-explicit-range-refused", mode: gateRequestModes{base: "base", head: "head", rangeSet: true, lintOnly: true}, want: "informational compatibility"},
		{name: "informational-conflict", mode: gateRequestModes{report: true, prePush: true}, want: "informational"},
		{name: "informational-repo-conflict", mode: gateRequestModes{report: true, root: "/fixture"}, want: "informational"},
		{name: "pushed-ref-range-conflict", mode: gateRequestModes{prePush: true, base: "base", head: "head"}, want: "full check"},
		{name: "pushed-ref-lint-conflict", mode: gateRequestModes{prePush: true, lintOnly: true}, want: "full check"},
		{name: "pushed-ref-survey-conflict", mode: gateRequestModes{prePush: true, all: true}, want: "full check"},
		{name: "empty-explicit-range", mode: gateRequestModes{rangeSet: true}, want: "no survey fallback"},
		{name: "partial-range", mode: gateRequestModes{rangeSet: true, head: "head"}, want: "no survey fallback"},
		{name: "blank-range-endpoint", mode: gateRequestModes{rangeSet: true, base: " ", head: "head"}, want: "no survey fallback"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mode.validate()
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("mode validation error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestLintOnlyCompatibilityCannotBeSelectedAsRangeEnforcement(t *testing.T) {
	compatibility := gateRequestModes{all: true, lintOnly: true}
	if err := compatibility.validate(); err != nil {
		t.Fatalf("no-range compatibility mode was rejected: %v", err)
	}
	if compatibility.enforcement() {
		t.Fatal("no-range lint-only must remain informational, not enforcement")
	}
	ranged := gateRequestModes{base: "base", head: "head", rangeSet: true, lintOnly: true}
	if err := ranged.validate(); err == nil || !strings.Contains(err.Error(), "informational compatibility") {
		t.Fatalf("explicit-range lint-only should fail validation, got %v", err)
	}
}

func TestMaatGateDoesNotExposeHookInstall(t *testing.T) {
	if maatGateCmd.Flags().Lookup("install") != nil {
		t.Fatal("gate must not expose an install mode that claims to configure hooks")
	}
}

func TestMaatGateExitContractDistinguishesEnforcementFromSurvey(t *testing.T) {
	tests := []struct {
		name        string
		res         trustboundary.Result
		enforcement bool
		want        bool
	}{
		{name: "complete enforcement pass", res: trustboundary.Result{OK: true, Complete: true}, enforcement: true, want: true},
		{name: "enforcement failure", res: trustboundary.Result{OK: false, Complete: false}, enforcement: true},
		{name: "incomplete enforcement pass", res: trustboundary.Result{OK: true, Complete: false, Skipped: []string{"traceability"}}, enforcement: true},
		{name: "inconsistent skipped enforcement", res: trustboundary.Result{OK: true, Complete: true, Skipped: []string{"secrets"}}, enforcement: true},
		{name: "incomplete informational survey", res: trustboundary.Result{OK: true, Complete: false, Skipped: []string{"traceability"}}, want: true},
		{name: "lint-only compatibility remains informational", res: trustboundary.Result{Scope: "lint-only", OK: true, Complete: true}, want: true},
		{name: "lint-only result cannot satisfy enforcement", res: trustboundary.Result{Scope: "lint-only", OK: true, Complete: true}, enforcement: true},
		{name: "survey still rejects executed failure", res: trustboundary.Result{OK: false, Complete: false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := gateResultAccepted(tc.res, tc.enforcement); got != tc.want {
				t.Fatalf("gateResultAccepted(%+v, enforcement=%v) = %v, want %v", tc.res, tc.enforcement, got, tc.want)
			}
		})
	}
}
