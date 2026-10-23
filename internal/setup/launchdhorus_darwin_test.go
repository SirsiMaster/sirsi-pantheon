//go:build darwin

package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type horusNativeFixture struct {
	t                                                                                                        *testing.T
	uid, euid                                                                                                int
	current                                                                                                  []byte
	bootouts, bootstraps, children, prepares, publishes, restores, waits, prints                             int
	childAt, driftAt                                                                                         int
	intentErr, prepareErr, publishErr, restoreErr, bootoutErr, bootstrapErr                                  error
	childErr, malformedChildren, signatureErr, postDrift, sameInstance, failedProbeUnknown, failedProbeDrift bool
	bootstrapStarts                                                                                          bool
	onBootout                                                                                                func()
	readbackReads                                                                                            int
	before, after                                                                                            []byte
}

const horusLegacyFixtureProgram = "/private/fixture/sirsi-oct12"

func newHorusNativeFixture(t *testing.T) (*horusNativeFixture, nativeServiceBackend) {
	t.Helper()
	f := &horusNativeFixture{t: t, uid: 501, euid: 501, bootstrapStarts: true}
	f.current = []byte(`{"Label":"ai.sirsi.horus.agent-router","WorkingDirectory":"/private/fixture/repo","EnvironmentVariables":{"PATH":"/private/fixture/bin:/usr/bin:/bin","UNRELATED":"gold & black <literal>"},"ProgramArguments":["/bin/zsh","-l","-c","exec \"/private/fixture/sirsi-oct12\" horus supervise"],"KeepAlive":true,"RunAtLoad":true,"StandardErrorPath":"/private/fixture/log","ProcessType":"Background"}`)
	d := horusServiceDependencies{
		directory: func(path string) error {
			if path != "/private/fixture/repo" {
				t.Fatal("working directory drift")
			}
			return nil
		},
		uid: func() int { return f.uid }, euid: func() int { return f.euid }, home: func() (string, error) { return "/Users/fixture", nil }, read: f.read,
		intent: func(ctx context.Context, _ string, _ int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return f.intentErr
		},
		artifact: func(path string, capture bool) ([]byte, string, error) {
			if capture {
				if path != "/Users/fixture/Library/LaunchAgents/"+HorusServiceLabel+".plist" {
					t.Fatal("unselected artifact")
				}
				return bytes.Clone(f.current), horusDigest(f.current), nil
			}
			switch path {
			case horusLegacyFixtureProgram, horusProtectedProgram, "/bin/zsh":
				return nil, horusDigest([]byte(path)), nil
			}
			t.Fatal("unrecognized payload artifact")
			return nil, "", errors.New("unrecognized artifact")
		},
		lock: func(_ string, run func() error) error { return run() },
		prepare: func(_ string, before, after []byte) (horusPublication, error) {
			f.prepares++
			if f.children != 2 || f.bootouts != 0 {
				t.Fatal("staging before double child/instance checks")
			}
			if f.prepareErr != nil {
				return nil, f.prepareErr
			}
			f.before, f.after = bytes.Clone(before), bytes.Clone(after)
			return f, nil
		},
		wait: func(ctx context.Context) error {
			f.waits++
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("unbounded recovery")
			}
			return ctx.Err()
		},
	}
	return f, nativeServiceBackend{horus: &d}
}

func (f *horusNativeFixture) Publish() error {
	f.publishes++
	if f.bootouts != 1 {
		f.t.Fatal("publish before confirmed retirement")
	}
	if f.publishErr != nil {
		return f.publishErr
	}
	f.current = bytes.Clone(f.after)
	return nil
}
func (f *horusNativeFixture) Restore() error {
	f.restores++
	if f.restoreErr != nil {
		return f.restoreErr
	}
	f.current = bytes.Clone(f.before)
	return nil
}
func (f *horusNativeFixture) Close() error { return nil }

func (f *horusNativeFixture) read(ctx context.Context, input []byte, exe string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	joined := strings.Join(args, " ")
	switch exe {
	case "/usr/bin/plutil":
		if joined != "-convert json -o - -" && joined != "-convert xml1 -o - -" {
			f.t.Fatal("unknown plutil operation")
		}
		// Data-only codec double: all configuration semantics still traverse
		// the real conversion logic, never an installed payload or native tool.
		return bytes.Clone(input), nil
	case "/usr/bin/codesign":
		const inlineRequirement = `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`
		if len(args) != 5 || args[0] != "--verify" || args[1] != "--strict" || args[2] != "-R" || args[3] != inlineRequirement {
			f.t.Fatal("signature qualification drift")
		}
		if f.signatureErr {
			return nil, errors.New("signature refused")
		}
		return nil, nil
	case "/bin/ps":
		if joined == "-axo pid=,ppid=" {
			f.children++
			if f.childErr {
				return nil, errors.New("process probe unavailable")
			}
			if f.malformedChildren {
				return []byte("unknown"), nil
			}
			if f.childAt > 0 && f.children >= f.childAt {
				return []byte("1 0\n123 1\n456 123\n"), nil
			}
			return []byte("1 0\n123 1\n"), nil
		}
		if len(args) != 4 || args[0] != "-p" || args[2] != "-o" {
			f.t.Fatal("unknown ps selector")
		}
		if args[3] == "lstart=" {
			if args[1] == "123" {
				return []byte("old-start"), nil
			}
			return []byte("new-start"), nil
		}
		if args[3] == "comm=" {
			_, payload, _, err := horusConfiguration(f.current)
			return []byte(payload), err
		}
	case "/bin/launchctl":
		target := "gui/501/" + HorusServiceLabel
		switch joined {
		case "print " + target:
			f.prints++
			if f.bootoutErr != nil && f.bootouts > 0 {
				if _, ok := ctx.Deadline(); !ok {
					f.t.Fatal("failed bootout probe not bounded")
				}
				if f.failedProbeUnknown {
					return nil, errors.New("print unavailable")
				}
				if f.failedProbeDrift {
					return []byte("pid = 125\n"), nil
				}
			}
			if f.bootstraps > 0 {
				f.readbackReads++
				if !f.bootstrapStarts {
					return nil, errors.New("no running instance")
				}
				if f.sameInstance {
					return []byte("pid = 123\n"), nil
				}
				if f.postDrift && f.readbackReads > 1 {
					return []byte("pid = 125\n"), nil
				}
				return []byte("pid = 124\n"), nil
			}
			if f.driftAt > 0 && f.prints >= f.driftAt {
				return []byte("pid = 125\n"), nil
			}
			return []byte("pid = 123\n"), nil
		case "bootout " + target:
			if f.children != 3 || f.prepares != 1 {
				f.t.Fatal("bootout before final recheck")
			}
			f.bootouts++
			if f.onBootout != nil {
				f.onBootout()
			}
			return nil, f.bootoutErr
		case "bootstrap gui/501 /Users/fixture/Library/LaunchAgents/" + HorusServiceLabel + ".plist":
			f.bootstraps++
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 15*time.Second {
				f.t.Fatal("bootstrap recovery not independently bounded")
			}
			return nil, f.bootstrapErr
		}
	}
	f.t.Fatalf("unexpected native operation: %s %s", exe, joined)
	return nil, errors.New("unexpected native operation")
}

func TestHorusLegacySelectedConversion(t *testing.T) {
	f, b := newHorusNativeFixture(t)
	p, err := b.Inspect(context.Background(), HorusServiceLabel)
	if err != nil {
		t.Fatal(err)
	}
	if p.Program != horusLegacyFixtureProgram || p.LauncherSHA256 == "" || p.ReplacementSHA256 == "" || p.PlanSHA256 == "" || p.PlistSHA256 == p.ConvertedPlistSHA256 {
		t.Fatal("incomplete before/after plan")
	}
	if f.prepares+f.bootouts+f.bootstraps != 0 {
		t.Fatal("plan mutated native state")
	}
	original := bytes.Clone(f.current)
	c := ServiceController{Backend: b, Authorize: func(context.Context, string) error { return nil }}
	next, err := c.Refresh(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	want := horusExpectedAfter(p)
	want.PID, want.Started = 124, "new-start"
	want.PlanSHA256 = horusPlanDigest(want)
	if next != want || f.bootouts != 1 || f.bootstraps != 1 || f.publishes != 1 || f.restores != 0 {
		t.Fatalf("incorrect conversion: %+v", next)
	}
	var oldConfig, newConfig map[string]json.RawMessage
	if json.Unmarshal(original, &oldConfig) != nil || json.Unmarshal(f.current, &newConfig) != nil {
		t.Fatal("bad fixture")
	}
	delete(oldConfig, "ProgramArguments")
	delete(newConfig, "ProgramArguments")
	oldBytes, _ := json.Marshal(oldConfig)
	newBytes, _ := json.Marshal(newConfig)
	if !bytes.Equal(oldBytes, newBytes) || !bytes.Equal(original, f.before) {
		t.Fatal("unrelated configuration/environment or original backup changed")
	}
}

func TestHorusProfileRefusesBeforeMutation(t *testing.T) {
	for _, name := range []string{"root", "effective-root", "wrong-target", "wrong-pin", "wrong-plan", "active-child", "child-appears", "third-pass-child", "child-error", "malformed-children", "instance-drift", "quarantine", "disabled", "intent-unknown", "signature", "backup-occupied"} {
		t.Run(name, func(t *testing.T) {
			f, b := newHorusNativeFixture(t)
			p, err := b.Inspect(context.Background(), HorusServiceLabel)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "root":
				f.uid, f.euid = 0, 0
			case "effective-root":
				f.euid = 0
			case "wrong-target":
				p.Target = "system/" + HorusServiceLabel
				p.PlanSHA256 = horusPlanDigest(p)
			case "wrong-pin":
				p.ProgramSHA256 = strings.Repeat("e", 64)
				p.PlanSHA256 = horusPlanDigest(p)
			case "wrong-plan":
				p.PlanSHA256 = strings.Repeat("f", 64)
			case "active-child":
				f.childAt = 1
			case "child-appears":
				f.childAt = 2
			case "third-pass-child":
				f.childAt = 3
			case "child-error":
				f.childErr = true
			case "malformed-children":
				f.malformedChildren = true
			case "instance-drift":
				f.driftAt = f.prints + 2
			case "quarantine", "disabled", "intent-unknown":
				f.intentErr = errors.New(name)
			case "signature":
				f.signatureErr = true
			case "backup-occupied":
				f.prepareErr = errors.New("occupied backup")
			}
			if err := b.Refresh(context.Background(), p); err == nil {
				t.Fatal("unsafe conversion accepted")
			}
			if f.bootouts+f.bootstraps+f.publishes != 0 {
				t.Fatal("native mutation before refusal")
			}
		})
	}
}

func TestHorusConfigurationRefusalsAndLiteralLegacyIdentity(t *testing.T) {
	for _, command := range []string{"exec /private/fixture/sirsi-oct12 horus supervise", `exec "/private/fixture/sirsi-oct12" horus supervise`, `exec "/private/fixture/with space/sirsi" horus supervise`} {
		if _, err := horusLegacyPayload(command); err != nil {
			t.Fatalf("literal rejected: %v", err)
		}
	}
	for _, command := range []string{"sirsi horus supervise", "exec sirsi horus supervise", "exec /tmp/a;id horus supervise", `exec "/tmp/$(id)" horus supervise`, "exec /tmp/a horus supervise --once", "exec /tmp/../sirsi horus supervise", "exec /tmp/a horus supervise; true"} {
		if _, err := horusLegacyPayload(command); err == nil {
			t.Fatal("nonliteral legacy accepted")
		}
	}
	for _, name := range []string{"foreign-label", "wrong-command", "program-mismatch", "run-as", "inline-token", "relative-workdir"} {
		t.Run(name, func(t *testing.T) {
			f, b := newHorusNativeFixture(t)
			var config map[string]any
			if json.Unmarshal(f.current, &config) != nil {
				t.Fatal("fixture")
			}
			switch name {
			case "foreign-label":
				config["Label"] = "ai.sirsi.other"
			case "wrong-command":
				config["ProgramArguments"] = []string{horusProtectedProgram, "router", "serve"}
			case "program-mismatch":
				config["Program"] = "/tmp/other"
			case "run-as":
				config["UserName"] = "root"
			case "inline-token":
				config["EnvironmentVariables"] = map[string]string{"SIRSI_ROUTER_TOKEN": "SECRET_FIXTURE_DO_NOT_PRINT"}
			case "relative-workdir":
				config["WorkingDirectory"] = "repo"
			}
			f.current, _ = json.Marshal(config)
			_, err := b.Inspect(context.Background(), HorusServiceLabel)
			if err == nil || strings.Contains(fmt.Sprint(err), "SECRET_FIXTURE_DO_NOT_PRINT") || f.prepares+f.bootouts+f.bootstraps != 0 {
				t.Fatal("configuration refusal leaked, mutated, or accepted")
			}
		})
	}
}

func TestHorusBootoutAmbiguityIsReadOnly(t *testing.T) {
	for _, name := range []string{"original", "changed", "unknown"} {
		t.Run(name, func(t *testing.T) {
			f, b := newHorusNativeFixture(t)
			p, err := b.Inspect(context.Background(), HorusServiceLabel)
			if err != nil {
				t.Fatal(err)
			}
			f.bootoutErr = errors.New("bootout timed out")
			f.failedProbeDrift = name == "changed"
			f.failedProbeUnknown = name == "unknown"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.onBootout = cancel
			err = b.Refresh(ctx, p)
			if !errors.Is(err, f.bootoutErr) || f.bootouts != 1 || f.bootstraps+f.publishes+f.restores != 0 {
				t.Fatal("ambiguous bootout triggered mutation or lost original error")
			}
			if name == "unknown" && !strings.Contains(err.Error(), "OUTCOME UNKNOWN") {
				t.Fatal("unknown retirement hidden")
			}
		})
	}
}

func TestHorusPublicationAndRecoveryFailures(t *testing.T) {
	for _, name := range []string{"publication-rollback", "rollback-refused", "bootstrap-unknown", "bootstrap-error-but-running", "readback-drift", "old-instance", "caller-canceled"} {
		t.Run(name, func(t *testing.T) {
			f, b := newHorusNativeFixture(t)
			p, err := b.Inspect(context.Background(), HorusServiceLabel)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "publication-rollback":
				f.publishErr = errors.New("publication failed")
			case "rollback-refused":
				f.publishErr = errors.New("publication failed")
				f.restoreErr = errors.New("replacement changed")
			case "bootstrap-unknown":
				f.bootstrapErr = errors.New("bootstrap failed")
				f.bootstrapStarts = false
			case "bootstrap-error-but-running":
				f.bootstrapErr = errors.New("bootstrap returned error")
			case "readback-drift":
				f.postDrift = true
			case "old-instance":
				f.sameInstance = true
			case "caller-canceled":
				f.onBootout = cancel
			}
			c := ServiceController{Backend: b, Authorize: func(context.Context, string) error { return nil }}
			_, err = c.Refresh(ctx, p)
			if err == nil || f.bootouts != 1 || f.bootstraps > 1 || f.waits > 40 {
				t.Fatal("failure hidden or recovery unbounded")
			}
			if f.bootstrapErr != nil && !errors.Is(err, f.bootstrapErr) {
				t.Fatal("original bootstrap error lost")
			}
			if name == "publication-rollback" && (f.restores != 1 || f.bootstraps != 1 || !bytes.Equal(f.current, f.before)) {
				t.Fatal("exact original recovery not exercised")
			}
			if name == "rollback-refused" && f.bootstraps != 0 {
				t.Fatal("unknown rollback configuration bootstrapped")
			}
			if name == "bootstrap-unknown" && f.waits != 40 {
				t.Fatal("finite readback bound not exercised")
			}
			if name == "caller-canceled" && (f.bootstraps != 1 || f.readbackReads != 2) {
				t.Fatal("cancellation abandoned independent recovery")
			}
		})
	}
}

func TestHorusDisabledIntentParser(t *testing.T) {
	for _, tc := range []struct {
		body string
		bad  bool
	}{
		{"\n\tdisabled services = {\n\t\"" + HorusServiceLabel + "\" => enabled\n\t}\n", false},
		{"disabled services = {\n}", false},
		{"disabled services = {\n\"other\" => disabled\n}", false},
		{"disabled services = {\n\"" + HorusServiceLabel + "\" => true\n}", true},
		{"disabled services = {\n\"other " + HorusServiceLabel + "\" => disabled\n}", true},
		{"", true}, {"disabled services = {\nunknown\n}", true},
	} {
		if err := horusDisabledIntent([]byte(tc.body)); (err != nil) != tc.bad {
			t.Fatalf("intent classification: %v", err)
		}
	}
}

func TestHorusNestedConfigurationComparison(t *testing.T) {
	a := []byte(`{"EnvironmentVariables":{"Z":"last","A":"first"},"Settings":{"Z":{"b":2,"a":1},"A":[3,2,1]}}`)
	b := []byte(`{"Settings":{"A":[3,2,1],"Z":{"a":1,"b":2}},"EnvironmentVariables":{"A":"first","Z":"last"}}`)
	if !horusSameConfiguration(a, b) {
		t.Fatal("nested dictionary ordering rejected")
	}
	if horusSameConfiguration([]byte(`{"n":9007199254740992}`), []byte(`{"n":9007199254740993}`)) {
		t.Fatal("numeric evidence rounded")
	}
	if horusSameConfiguration(a, []byte(`{"EnvironmentVariables":{"A":"changed","Z":"last"}}`)) {
		t.Fatal("changed nested configuration accepted")
	}
}

// This is an admitted-test data codec fixture, not a service action. Only the
// exact bounded native plutil runs; codesign/launchd/payloads remain doubled.
func TestHorusNativePlutilLosslessNestedConfiguration(t *testing.T) {
	f, b := newHorusNativeFixture(t)
	f.current = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>ai.sirsi.horus.agent-router</string>
<key>WorkingDirectory</key><string>/private/fixture/repo</string>
<key>EnvironmentVariables</key><dict><key>Z_LAST</key><string>gold &amp; black &lt;literal&gt;</string><key>A_FIRST</key><string>first</string></dict>
<key>SoftResourceLimits</key><dict><key>NumberOfFiles</key><integer>1024</integer><key>Core</key><integer>0</integer></dict>
<key>ProgramArguments</key><array><string>/bin/zsh</string><string>-l</string><string>-c</string><string>exec "/private/fixture/sirsi-oct12" horus supervise</string></array>
<key>KeepAlive</key><true/><key>RunAtLoad</key><true/>
</dict></plist>`)
	var firstJSON, lastJSON []byte
	codecCalls := 0
	read := b.horus.read
	b.horus.read = func(ctx context.Context, input []byte, exe string, args ...string) ([]byte, error) {
		if exe != "/usr/bin/plutil" {
			return read(ctx, input, exe, args...)
		}
		codecCalls++
		out, err := rootRelayRead(ctx, input, exe, args...)
		if err == nil && len(args) > 1 && args[1] == "json" {
			if firstJSON == nil {
				firstJSON = bytes.Clone(out)
			}
			lastJSON = bytes.Clone(out)
		}
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, original, converted, err := b.horusArtifacts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if codecCalls != 3 || !bytes.Equal(original, f.current) || !bytes.HasPrefix(converted, []byte("<?xml")) || p.ConvertedPlistSHA256 != horusDigest(converted) {
		t.Fatal("native conversion did not retain exact before/after evidence")
	}
	var oldConfig, newConfig map[string]json.RawMessage
	if json.Unmarshal(firstJSON, &oldConfig) != nil || json.Unmarshal(lastJSON, &newConfig) != nil {
		t.Fatal("native codec readback invalid")
	}
	delete(oldConfig, "ProgramArguments")
	delete(newConfig, "ProgramArguments")
	oldBytes, _ := json.Marshal(oldConfig)
	newBytes, _ := json.Marshal(newConfig)
	if !horusSameConfiguration(oldBytes, newBytes) {
		t.Fatal("native conversion changed unrelated nested semantics")
	}
	if f.prepares+f.bootouts+f.bootstraps != 0 {
		t.Fatal("data-only codec qualification mutated service state")
	}
}

func TestHorusAbsentJobRecoveryRetainsExactOwnerRoute(t *testing.T) {
	f, b := newHorusNativeFixture(t)
	p, err := b.Inspect(context.Background(), HorusServiceLabel)
	if err != nil {
		t.Fatal(err)
	}
	f.bootstrapErr = errors.New("fixture bootstrap failed")
	f.bootstrapStarts = false
	err = b.Refresh(context.Background(), p)
	backup := filepath.Join(filepath.Dir(p.Plist), horusBackupLeaf(p.PlistSHA256))
	argv, _ := json.Marshal([]string{"/bin/launchctl", "bootstrap", "gui/501", p.Plist})
	for _, evidence := range []string{backup, p.ConvertedPlistSHA256, p.ReplacementSHA256, string(argv)} {
		if err == nil || !strings.Contains(err.Error(), evidence) {
			t.Fatal("absent-job recovery lost exact backup/config/program/argv identity")
		}
	}
	if f.bootouts != 1 || f.bootstraps != 1 || f.restores != 0 || f.waits != 40 {
		t.Fatal("absent-job route repeated retirement/bootstrap or replayed legacy configuration")
	}
}

func TestHorusDirectProtectedConfigurationRefresh(t *testing.T) {
	f, b := newHorusNativeFixture(t)
	var config map[string]json.RawMessage
	if err := json.Unmarshal(f.current, &config); err != nil {
		t.Fatal(err)
	}
	config["ProgramArguments"], _ = json.Marshal([]string{horusProtectedProgram, "horus", "supervise"})
	f.current, _ = json.Marshal(config)
	p, err := b.Inspect(context.Background(), HorusServiceLabel)
	if err != nil {
		t.Fatal(err)
	}
	if p.Program != horusProtectedProgram || p.LauncherSHA256 != "" || p.ConvertedPlistSHA256 != p.PlistSHA256 {
		t.Fatal("direct profile falsely treated as legacy")
	}
	if err := b.Refresh(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if f.bootouts != 1 || f.bootstraps != 1 || !bytes.Equal(f.before, f.after) {
		t.Fatal("direct profile changed configuration or widened actions")
	}
}

func TestHorusArtifactQualification(t *testing.T) {
	for _, name := range []string{"protected", "legacy-owner", "public-plist", "private-plist", "foreign", "writable", "nonexecutable", "hardlink", "symlink", "unprotected-owner"} {
		t.Run(name, func(t *testing.T) {
			path, plist := horusProtectedProgram, false
			st := unix.Stat_t{Mode: unix.S_IFREG | 0755, Uid: 0, Nlink: 1}
			good := false
			switch name {
			case "protected":
				good = true
			case "legacy-owner":
				path = horusLegacyFixtureProgram
				st.Uid = 501
				good = true
			case "public-plist":
				path = "/Users/fixture/Library/LaunchAgents/" + HorusServiceLabel + ".plist"
				plist = true
				st.Mode = unix.S_IFREG | 0644
				st.Uid = 501
				good = true
			case "private-plist":
				path = "/Users/fixture/Library/LaunchAgents/" + HorusServiceLabel + ".plist"
				plist = true
				st.Mode = unix.S_IFREG | 0600
				st.Uid = 501
				good = true
			case "foreign":
				path = horusLegacyFixtureProgram
				st.Uid = 502
			case "writable":
				st.Mode = unix.S_IFREG | 0775
			case "nonexecutable":
				st.Mode = unix.S_IFREG | 0644
			case "hardlink":
				st.Nlink = 2
			case "symlink":
				st.Mode = unix.S_IFLNK | 0755
			case "unprotected-owner":
				st.Uid = 501
			}
			if err := horusArtifactPolicy(path, plist, 501, st); (err == nil) != good {
				t.Fatalf("artifact qualification: %v", err)
			}
		})
	}
}
