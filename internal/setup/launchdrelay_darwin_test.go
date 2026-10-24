//go:build darwin

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func relayInstanceFixture() ServiceInstance {
	return ServiceInstance{Label: rootRelayLabel, Target: rootRelayTarget, Plist: rootRelayPlist,
		Program: rootRelayProgram, PlistSHA256: strings.Repeat("a", 64), ProgramSHA256: strings.Repeat("b", 64),
		PID: 123, Started: "old-start"}
}

func TestRootRelayExactSpoolArgumentProfiles(t *testing.T) {
	for _, args := range [][]string{
		{"/usr/local/libexec/sirsi-pantheon/sirsi", "router", "relay", "serve", "--spool", "/var/sirsipantheon/relay"},
		{"/usr/local/libexec/sirsi-pantheon/sirsi", "router", "relay", "serve", "--spool", "/private/var/sirsipantheon/relay"},
	} {
		t.Run(args[5], func(t *testing.T) {
			f, b := newRelayNativeFixture(t)
			f.config.Program = "/usr/local/libexec/sirsi-pantheon/sirsi"
			f.config.ProgramArguments = append([]string(nil), args...)
			before, err := json.Marshal(f.config)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateRootRelayConfiguration(f.config); err != nil {
				t.Fatalf("observed literal profile refused: %v", err)
			}
			// Existing instance-owned native doubles only; no root read or tool
			// execution. Verify the actual inspection path accepts both profiles.
			p, err := b.Inspect(context.Background(), rootRelayLabel)
			if err != nil || p != relayInstanceFixture() {
				t.Fatalf("profile lost exact artifact/instance bindings: %+v err=%v", p, err)
			}
			after, err := json.Marshal(f.config)
			if err != nil || string(before) != string(after) || f.bootouts+f.bootstraps+f.locks != 0 {
				t.Fatal("read-only profile inspection rewrote config or mutated service")
			}
		})
	}
}

func TestRootRelaySpoolArgumentLookalikesRefused(t *testing.T) {
	for _, spool := range []string{
		"/var/sirsipantheon/relay/",
		"/private/var/sirsipantheon/relay/",
		"/var//sirsipantheon/relay",
		"/private//var/sirsipantheon/relay",
		"/var/./sirsipantheon/relay",
		"/private/var/./sirsipantheon/relay",
		"/var/sirsipantheon/../relay",
		"/private/var/sirsipantheon/relay/../relay",
		"/System/Volumes/Data/private/var/sirsipantheon/relay",
		"/var/sirsipantheon/relay-copy",
		"/private/var/sirsipantheon/relay-copy",
		"/var/sirsipantheon/relay ",
		"var/sirsipantheon/relay",
		"/tmp/relay",
	} {
		t.Run(spool, func(t *testing.T) {
			f, b := newRelayNativeFixture(t)
			f.config.ProgramArguments[5] = spool
			if err := validateRootRelayConfiguration(f.config); err == nil {
				t.Fatal("unobserved spool literal accepted")
			}
			if _, err := b.Inspect(context.Background(), rootRelayLabel); err == nil || f.prints+f.childReads+f.spoolReads+f.bootouts+f.bootstraps+f.locks != 0 {
				t.Fatalf("unsafe profile reached instance/service operations: %v", err)
			}
		})
	}
}

type relayNativeFixture struct {
	t                                                                                             *testing.T
	uid, euid                                                                                     int
	config                                                                                        rootRelayConfiguration
	bootouts, bootstraps, prints, childReads, waits, locks, postReads                             int
	bootstrapFailures                                                                             int
	bootoutFailure, lockFailure                                                                   bool
	bootoutError, failedProbeError                                                                error
	failedProbeReads                                                                              int
	failedProbeDrift                                                                              bool
	childAt, driftAt                                                                              int
	childError, malformedChildren, sameInstance, postDrift, changedArtifacts, unavailableReadback bool
	onBootout                                                                                     func()
	spoolReads, spoolBusyAt                                                                       int
	spoolError                                                                                    bool
}

func newRelayNativeFixture(t *testing.T) (*relayNativeFixture, nativeServiceBackend) {
	t.Helper()
	f := &relayNativeFixture{t: t, config: rootRelayConfiguration{Label: rootRelayLabel, UserName: "_sirsipantheon",
		ProgramArguments: []string{rootRelayProgram, "router", "relay", "serve", "--spool", rootRelaySpool}}}
	d := rootRelayDependencies{
		admission: func(ctx context.Context, op func() error) error { return op() },
		uid:       func() int { return f.uid }, euid: func() int { return f.euid }, read: f.read,
		spoolClear: func(ctx context.Context) error {
			f.spoolReads++
			if f.spoolError {
				return errors.New("spool probe unavailable")
			}
			if f.spoolBusyAt != 0 && f.spoolReads >= f.spoolBusyAt {
				return errors.New("spool has inflight work")
			}
			return ctx.Err()
		},
		artifact: func(path string, capture bool) ([]byte, string, error) {
			p := relayInstanceFixture()
			switch path {
			case rootRelayPlist:
				if !capture {
					t.Fatal("plist not captured")
				}
				return []byte("retained-private-plist"), p.PlistSHA256, nil
			case rootRelayProgram:
				if capture {
					t.Fatal("program capture requested")
				}
				if f.changedArtifacts && f.bootstraps > 0 {
					return nil, strings.Repeat("c", 64), nil
				}
				return nil, p.ProgramSHA256, nil
			default:
				t.Fatalf("unexpected artifact path %q", path)
				return nil, "", errors.New("unexpected artifact")
			}
		},
		lock: func(path string, operation func() error) error {
			if path != rootRelayPlist {
				t.Fatalf("unexpected lock %q", path)
			}
			f.locks++
			if f.lockFailure {
				return errors.New("maintenance lock held")
			}
			return operation()
		},
		wait: func(ctx context.Context, delay time.Duration) error {
			f.waits++
			if delay != 250*time.Millisecond {
				t.Fatal("unexpected retry interval")
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("recovery lost its bound")
			}
			return ctx.Err() // no sleeps or native operations in fixtures
		},
	}
	return f, nativeServiceBackend{relay: &d}
}

func (f *relayNativeFixture) read(ctx context.Context, input []byte, executable string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	joined := strings.Join(args, " ")
	switch executable {
	case "/usr/bin/plutil":
		if string(input) != "retained-private-plist" || joined != "-convert json -o - -" {
			f.t.Fatal("plist not decoded from retained bytes")
		}
		return json.Marshal(f.config)
	case "/bin/ps":
		if joined == "-axo pid=,ppid=" {
			f.childReads++
			if f.childError {
				return nil, errors.New("enumeration unavailable")
			}
			if f.malformedChildren {
				return []byte("unknown"), nil
			}
			if f.childAt != 0 && f.childReads >= f.childAt {
				return []byte("1 0\n123 1\n456 123\n"), nil
			}
			return []byte("1 0\n123 1\n"), nil
		}
		if joined == "-p 123 -o lstart=" {
			return []byte("old-start"), nil
		}
		if joined == "-p 124 -o lstart=" || joined == "-p 125 -o lstart=" {
			return []byte("new-start"), nil
		}
	case "/bin/launchctl":
		switch joined {
		case "print " + rootRelayTarget:
			f.prints++
			if f.bootoutFailure && f.bootouts > 0 {
				f.failedProbeReads++
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 5*time.Second {
					f.t.Fatal("failed bootout probe lost its independent bound")
				}
				if f.failedProbeError != nil {
					return nil, f.failedProbeError
				}
				if f.failedProbeDrift {
					return []byte("pid = 125\n"), nil
				}
				return []byte("pid = 123\n"), nil
			}
			if f.bootstraps > f.bootstrapFailures {
				f.postReads++
				if f.unavailableReadback {
					return nil, errors.New("not running")
				}
				if f.sameInstance {
					return []byte("pid = 123\n"), nil
				}
				if f.postDrift && f.postReads > 1 {
					return []byte("pid = 125\n"), nil
				}
				return []byte("pid = 124\n"), nil
			}
			if f.driftAt != 0 && f.prints >= f.driftAt {
				return []byte("pid = 125\n"), nil
			}
			return []byte("pid = 123\n"), nil
		case "bootout " + rootRelayTarget:
			if f.locks != 1 || f.childReads != 2 || f.spoolReads != 2 {
				f.t.Fatal("bootout escaped locked double inspection")
			}
			f.bootouts++
			if f.onBootout != nil {
				f.onBootout()
			}
			if f.bootoutFailure {
				if f.bootoutError != nil {
					return nil, f.bootoutError
				}
				return nil, errors.New("bootout failed")
			}
			return nil, nil
		case "bootstrap system " + rootRelayPlist:
			if f.bootouts != 1 {
				f.t.Fatal("bootstrap before exact retirement")
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 15*time.Second {
				f.t.Fatal("unbounded recovery")
			}
			f.bootstraps++
			if f.bootstraps <= f.bootstrapFailures {
				return nil, errors.New("bootstrap failed")
			}
			return nil, nil
		}
	}
	f.t.Fatalf("unexpected native operation %s %s", executable, joined)
	return nil, errors.New("unexpected operation")
}

func TestRootRelayProfileRefusals(t *testing.T) {
	for _, name := range []string{"unprivileged", "effective-only-root", "real-only-root", "missing-user", "root-user", "other-user", "wrong-label", "wrong-program", "argv-program-mismatch", "wrong-command", "missing-spool", "wrong-spool", "wrong-flag", "extra-flag", "active-child", "child-appears", "probe-error", "malformed-probe", "instance-drift", "wrong-target", "changed-start", "changed-plist", "changed-program", "lock-failure"} {
		t.Run(name, func(t *testing.T) {
			f, b := newRelayNativeFixture(t)
			p := relayInstanceFixture()
			switch name {
			case "unprivileged":
				f.uid, f.euid = 501, 501
			case "effective-only-root":
				f.uid = 501
			case "real-only-root":
				f.euid = 501
			case "missing-user":
				f.config.UserName = ""
			case "root-user":
				f.config.UserName = "root"
			case "other-user":
				f.config.UserName = "other"
			case "wrong-label":
				f.config.Label = "ai.sirsi.other"
			case "wrong-program":
				f.config.Program = "/usr/local/libexec/sirsi"
			case "argv-program-mismatch":
				f.config.ProgramArguments[0] = "/tmp/sirsi"
			case "wrong-command":
				f.config.ProgramArguments[3] = "install"
			case "missing-spool":
				f.config.ProgramArguments = f.config.ProgramArguments[:4]
			case "wrong-spool":
				f.config.ProgramArguments[5] = "/tmp/relay"
			case "wrong-flag":
				f.config.ProgramArguments[4] = "--spool=/tmp/relay"
			case "extra-flag":
				f.config.ProgramArguments = append(f.config.ProgramArguments, "--unknown")
			case "active-child":
				f.childAt = 1
			case "child-appears":
				f.childAt = 2
			case "probe-error":
				f.childError = true
			case "malformed-probe":
				f.malformedChildren = true
			case "instance-drift":
				f.driftAt = 2
			case "wrong-target":
				p.Target = "gui/501/" + rootRelayLabel
			case "changed-start":
				p.Started = "different-start"
			case "changed-plist":
				p.PlistSHA256 = strings.Repeat("d", 64)
			case "changed-program":
				p.ProgramSHA256 = strings.Repeat("d", 64)
			case "lock-failure":
				f.lockFailure = true
			}
			if err := b.Refresh(context.Background(), p); err == nil {
				t.Fatal("unsafe relay refresh accepted")
			}
			if f.bootouts != 0 || f.bootstraps != 0 {
				t.Fatal("native mutation before refusal")
			}
		})
	}
}

func TestRootRelayExactNewInstanceAndRecoveryBounds(t *testing.T) {
	for _, name := range []string{"success", "explicit-program", "bootstrap-retry", "bootstrap-exhausted", "same-instance", "post-instance-drift", "post-artifact-drift", "readback-exhausted"} {
		t.Run(name, func(t *testing.T) {
			f, b := newRelayNativeFixture(t)
			switch name {
			case "explicit-program":
				f.config.Program = rootRelayProgram
			case "bootstrap-retry":
				f.bootstrapFailures = 2
			case "bootstrap-exhausted":
				f.bootstrapFailures = 100
			case "same-instance":
				f.sameInstance = true
			case "post-instance-drift":
				f.postDrift = true
			case "post-artifact-drift":
				f.changedArtifacts = true
			case "readback-exhausted":
				f.unavailableReadback = true
			}
			err := b.Refresh(context.Background(), relayInstanceFixture())
			wantSuccess := name == "success" || name == "explicit-program" || name == "bootstrap-retry"
			if (err == nil) != wantSuccess {
				t.Fatalf("result: %v", err)
			}
			if f.bootouts != 1 || f.bootstraps > 20 || f.waits > 40 {
				t.Fatalf("unbounded/wrong side effects: %+v", f)
			}
			if name == "bootstrap-exhausted" && f.bootstraps != 20 {
				t.Fatal("bootstrap retry bound not exercised")
			}
			if name == "readback-exhausted" && f.postReads != 40 {
				t.Fatal("readback retry bound not exercised")
			}
			if name == "bootstrap-retry" && f.bootstraps != 3 {
				t.Fatal("bootstrap did not recover on third attempt")
			}
		})
	}
}

func TestRootRelayCanceledCallerRecoversButControllerDoesNotClaimSuccess(t *testing.T) {
	f, b := newRelayNativeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.onBootout = cancel
	c := ServiceController{Backend: b, Authorize: func(context.Context, string) error { return nil }}
	if _, err := c.Refresh(ctx, relayInstanceFixture()); err == nil || !strings.Contains(err.Error(), "readback failed") {
		t.Fatalf("canceled controller falsely claimed success: %v", err)
	}
	if f.bootouts != 1 || f.bootstraps != 1 || f.postReads != 2 {
		t.Fatal("cancellation abandoned bounded backend recovery")
	}
}

func TestRootRelayFailedBootoutDoesNotBootstrap(t *testing.T) {
	for _, name := range []string{"same-instance", "changed-instance", "probe-unknown"} {
		t.Run(name, func(t *testing.T) {
			f, b := newRelayNativeFixture(t)
			f.bootoutFailure = true
			original := errors.New("fixture bootout timed out after possible retirement")
			f.bootoutError = original
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.onBootout = cancel // probe must not inherit this canceled request
			want := "original selected relay instance remains alive"
			if name == "changed-instance" {
				f.failedProbeDrift = true
				want = "changed concurrently"
			}
			if name == "probe-unknown" {
				f.failedProbeError = errors.New("launchctl print unavailable")
				want = "OUTCOME UNKNOWN"
			}
			err := b.Refresh(ctx, relayInstanceFixture())
			if !errors.Is(err, original) || !strings.Contains(err.Error(), want) {
				t.Fatalf("lost failure or classification: %v", err)
			}
			if f.failedProbeError != nil && !errors.Is(err, f.failedProbeError) {
				t.Fatal("unknown-outcome error lost probe failure")
			}
			if f.bootouts != 1 || f.bootstraps != 0 || f.failedProbeReads != 1 || f.waits != 0 {
				t.Fatal("failed bootout repeated a mutation or skipped bounded read-only probe")
			}
		})
	}
}

func TestRootRelayProcessEnumerationFailsClosed(t *testing.T) {
	for _, listing := range []string{"", "unknown", "123 x", "123 1\n123 1", "1 0", "-1 0\n123 1", "123 -1", "123 1 extra"} {
		if err := rootRelayChildrenClear([]byte(listing), 123); err == nil {
			t.Fatalf("unknown enumeration accepted: %q", listing)
		}
	}
}

func TestRootRelaySpoolRefusesWithoutOSChildren(t *testing.T) {
	for _, name := range []string{"inflight", "second-pass-arrival", "probe-error"} {
		t.Run(name, func(t *testing.T) {
			f, b := newRelayNativeFixture(t)
			f.spoolBusyAt = 1
			if name == "second-pass-arrival" {
				f.spoolBusyAt = 2
			}
			if name == "probe-error" {
				f.spoolBusyAt = 0
				f.spoolError = true
			}
			if err := b.Refresh(context.Background(), relayInstanceFixture()); err == nil {
				t.Fatal("busy/unknown spool accepted with zero OS children")
			}
			if f.bootouts != 0 || f.bootstraps != 0 {
				t.Fatal("relay retired with busy/unknown spool")
			}
			if name == "second-pass-arrival" && f.spoolReads != 2 {
				t.Fatal("second preboot spool sample not exercised")
			}
		})
	}
}

func TestRootRelayArtifactPolicyAndDescriptor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plist bool
		mode  uint16
		uid   uint32
		valid bool
	}{
		{"private-plist", true, unix.S_IFREG | 0600, 0, true},
		{"public-plist", true, unix.S_IFREG | 0644, 0, false},
		{"foreign-plist", true, unix.S_IFREG | 0600, 501, false},
		{"executable", false, unix.S_IFREG | 0755, 0, true},
		{"writable-program", false, unix.S_IFREG | 0775, 0, false},
		{"nonexecutable", false, unix.S_IFREG | 0644, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := rootRelayArtifactPolicy(tc.plist, unix.Stat_t{Mode: tc.mode, Uid: tc.uid}); (err == nil) != tc.valid {
				t.Fatalf("policy: %v", err)
			}
		})
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixture")
	if err := os.WriteFile(path, []byte("artifact"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, _, err = serviceArtifactPolicy(path, true, func(fd int) error {
		calls++
		var st unix.Stat_t
		if err := unix.Fstat(fd, &st); err != nil {
			return err
		}
		if st.Mode&0777 != 0600 {
			return fmt.Errorf("wrong retained descriptor mode")
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("descriptor policy calls=%d err=%v", calls, err)
	}
}

func TestRootRelayOutputBoundCannotBeBypassedByReadFrom(t *testing.T) {
	var out rootRelayOutput
	if _, err := io.Copy(&out, io.LimitReader(strings.NewReader(strings.Repeat("x", 256*1024+1)), 256*1024+1)); err == nil {
		t.Fatal("oversize native output bypassed bound")
	}
	if out.data.Len() > 256*1024 {
		t.Fatal("oversize response retained")
	}
}

func TestRootRelayProducerAdmissionRefusalNeverBootsOut(t *testing.T) {
	f, b := newRelayNativeFixture(t)
	b.relay.admission = func(context.Context, func() error) error { return errors.New("producer still in flight") }
	if e := b.refreshRootRelay(context.Background(), relayInstanceFixture()); e == nil {
		t.Fatal("admission refusal ignored")
	}
	if f.bootouts != 0 || f.bootstraps != 0 {
		t.Fatal("mutated service after admission refusal")
	}
}
