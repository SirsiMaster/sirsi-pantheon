package main

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestHostRestartRequiresExplicitAuthenticatedConfirmation(t *testing.T) {
	originalGOOS := hostRestartGOOS
	originalRun := hostRestartRun
	t.Cleanup(func() {
		hostRestartGOOS = originalGOOS
		hostRestartRun = originalRun
	})
	hostRestartGOOS = "darwin"
	hostRestartRun = func(string, ...string) error {
		t.Fatal("restart command must not execute without both consent flags")
		return nil
	}

	command := newHostRestartCommand()
	command.SetArgs(nil)
	if err := command.Execute(); err == nil {
		t.Fatal("expected missing authenticated flag to fail closed")
	}

	command = newHostRestartCommand()
	command.SetArgs([]string{"--authenticated"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected missing confirmation flag to fail closed")
	}
}

func TestHostRestartUsesOnlyAppleInteractiveAuthRestart(t *testing.T) {
	originalGOOS := hostRestartGOOS
	originalRun := hostRestartRun
	t.Cleanup(func() {
		hostRestartGOOS = originalGOOS
		hostRestartRun = originalRun
	})
	hostRestartGOOS = "darwin"

	var calls [][]string
	hostRestartRun = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	command := newHostRestartCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--authenticated", "--confirm", "--delay-minutes", "7"})
	if err := command.Execute(); err != nil {
		t.Fatalf("authenticated restart command failed: %v", err)
	}

	want := [][]string{
		{"/usr/bin/fdesetup", "isactive"},
		{"/usr/bin/fdesetup", "supportsauthrestart"},
		{"/usr/bin/sudo", "/usr/bin/fdesetup", "authrestart", "-delayminutes", "7"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected command sequence:\n got: %#v\nwant: %#v", calls, want)
	}
}

func TestHostRestartStopsBeforeRestartWhenPreflightFails(t *testing.T) {
	originalGOOS := hostRestartGOOS
	originalRun := hostRestartRun
	t.Cleanup(func() {
		hostRestartGOOS = originalGOOS
		hostRestartRun = originalRun
	})
	hostRestartGOOS = "darwin"

	calls := 0
	hostRestartRun = func(string, ...string) error {
		calls++
		return errors.New("not active")
	}

	command := newHostRestartCommand()
	command.SetArgs([]string{"--authenticated", "--confirm"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected FileVault preflight failure")
	}
	if calls != 1 {
		t.Fatalf("expected one fail-closed preflight call, got %d", calls)
	}
}

func TestHostRestartRejectsInvalidDelayBeforeExecution(t *testing.T) {
	originalGOOS := hostRestartGOOS
	originalRun := hostRestartRun
	t.Cleanup(func() {
		hostRestartGOOS = originalGOOS
		hostRestartRun = originalRun
	})
	hostRestartGOOS = "darwin"
	hostRestartRun = func(string, ...string) error {
		t.Fatal("invalid delay must fail before execution")
		return nil
	}

	command := newHostRestartCommand()
	command.SetArgs([]string{"--authenticated", "--confirm", "--delay-minutes", "-2"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected invalid delay to fail")
	}
}
