package liveness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func localInstallFixture(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	engine := filepath.Join(home, "sirsi")
	if err := os.WriteFile(engine, []byte("inert engine, never executed"), 0o700); err != nil {
		t.Fatal(err)
	}
	return home, engine
}

func TestInspectLocalWatchReturnsDigestOfExactPlistBytes(t *testing.T) {
	home, engine := localInstallFixture(t)
	path := filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := LocalPlistContent(engine, home)
	if err := os.WriteFile(path, []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	observation, err := InspectLocalWatch(context.Background(), engine, home, localFixtureCommand(engine, home, false, &calls))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(plist))
	if observation.State != "observed" || observation.Concern || observation.ConfigurationSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("inspection did not bind exact plist bytes: %+v", observation)
	}
}

func TestInspectLocalWatchFindsConfiguredButUnloadedScheduleWithoutTreatingUnknownAsUnloaded(t *testing.T) {
	for _, mode := range []string{"unloaded", "unknown-list"} {
		t.Run(mode, func(t *testing.T) {
			home, engine := localInstallFixture(t)
			path := filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(LocalPlistContent(engine, home)), 0o600); err != nil {
				t.Fatal(err)
			}
			base := localFixtureCommand(engine, home, false, &[]string{})
			run := func(ctx context.Context, command string, input []byte, args ...string) ([]byte, error) {
				if command == "/bin/launchctl" && args[0] == "print" {
					return nil, errors.New("fixture label is not loaded")
				}
				if command == "/bin/launchctl" && args[0] == "list" {
					if mode == "unknown-list" {
						return []byte("diagnostic output"), nil
					}
					return []byte("PID\tStatus\tLabel\n-\t0\tcom.example.unrelated\n"), nil
				}
				return base(ctx, command, input, args...)
			}
			observation, err := InspectLocalWatch(context.Background(), engine, home, run)
			if mode == "unloaded" {
				if err != nil || !observation.Concern || observation.Detail != "exact local metadata schedule is configured and enabled but not loaded" {
					t.Fatalf("known unloaded schedule = %+v, %v", observation, err)
				}
				return
			}
			if err == nil || observation.Concern {
				t.Fatalf("unknown launchctl list was converted to a repair finding: %+v, %v", observation, err)
			}
		})
	}
}

func TestLocalWatchListedAcceptsSignedStatusesAndRejectsMalformedRows(t *testing.T) {
	for _, tc := range []struct {
		name      string
		output    string
		want      bool
		wantError bool
	}{
		{
			name:   "negative-signal-status-unrelated-target-absent",
			output: "PID Status Label\n- -9 com.example.old-job\n123 0 com.example.other\n",
			want:   false,
		},
		{
			name:   "negative-15-target-listed",
			output: "PID Status Label\n- -15 " + Label + "\n",
			want:   true,
		},
		{
			name:      "malformed-negative-status",
			output:    "PID Status Label\n- --9 com.example.old-job\n",
			wantError: true,
		},
		{
			name:      "status-overflow",
			output:    "PID Status Label\n- 2147483648 com.example.old-job\n",
			wantError: true,
		},
		{
			name:      "malformed-row-after-target-still-refused",
			output:    "PID Status Label\n- -15 " + Label + "\n- unknown com.example.bad\n",
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := localWatchListed(tc.output, Label)
			if (err != nil) != tc.wantError || got != tc.want {
				t.Fatalf("localWatchListed() = %v, %v; want %v, error=%v", got, err, tc.want, tc.wantError)
			}
		})
	}
}

func localFixtureCommand(engine, home string, loadError bool, calls *[]string) LocalCommand {
	return func(_ context.Context, path string, input []byte, args ...string) ([]byte, error) {
		*calls = append(*calls, path+" "+strings.Join(args, " "))
		if path == "/usr/bin/plutil" {
			if !strings.Contains(string(input), "--local") {
				return nil, errors.New("missing actual local flag")
			}
			return json.Marshal(map[string]any{"Label": Label, "ProgramArguments": []string{engine, "liveness-watch", "run", "--local"}, "WorkingDirectory": home, "RunAtLoad": true, "StartInterval": StartInterval})
		}
		if path != "/bin/launchctl" {
			return nil, errors.New("forbidden native/model/network command")
		}
		switch args[0] {
		case "print-disabled":
			return []byte("disabled services = {\n}"), nil
		case "unload":
			return nil, errors.New("not yet loaded")
		case "load":
			if loadError {
				return nil, errors.New("load refused")
			}
			return nil, nil
		case "print":
			return []byte(fmt.Sprintf("gui/%d/%s = {\n state = not running\n arguments = {\n%s\nliveness-watch\nrun\n--local\n}\nworking directory = %s\n}", os.Getuid(), Label, engine, home)), nil
		}
		return nil, errors.New("unexpected operation")
	}
}

func TestLocalInstallerIdleLoadedAndRootless(t *testing.T) {
	home, engine := localInstallFixture(t)
	var calls []string
	message, err := InstallLocal(context.Background(), engine, home, localFixtureCommand(engine, home, false, &calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "verified") {
		t.Fatal(message)
	}
	b, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", Label+".plist"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "--local") || !strings.Contains(string(b), home) || strings.Contains(string(b), ".agents") {
		t.Fatalf("nonlocal schedule: %s", b)
	}
	if len(calls) != 6 {
		t.Fatalf("unexpected operation count %v", calls)
	}
}

func TestLocalInstallerRefusesIntentUnknownAndLoadFailure(t *testing.T) {
	for _, mode := range []string{"disabled", "unknown", "load-failure"} {
		t.Run(mode, func(t *testing.T) {
			home, engine := localInstallFixture(t)
			var calls []string
			base := localFixtureCommand(engine, home, mode == "load-failure", &calls)
			run := func(ctx context.Context, path string, input []byte, args ...string) ([]byte, error) {
				if args[0] == "print-disabled" && mode != "load-failure" {
					if mode == "unknown" {
						return nil, errors.New("fixture error")
					}
					return []byte("disabled services = {\n\"" + Label + "\" => disabled\n}"), nil
				}
				return base(ctx, path, input, args...)
			}
			if _, err := InstallLocal(context.Background(), engine, home, run); err == nil {
				t.Fatal("refusal accepted")
			}
			if mode != "load-failure" && len(calls) != 0 {
				t.Fatalf("mutation before intent check: %v", calls)
			}
		})
	}
}

func TestLocalPlistEscapesAndEngineHasNoFallback(t *testing.T) {
	content := LocalPlistContent("/owned/Sirsi & Co/sirsi", "/owned/A < B")
	if strings.Contains(content, "Sirsi & Co") {
		t.Fatal("unescaped XML")
	}
	decoder := xml.NewDecoder(strings.NewReader(content))
	for {
		_, err := decoder.Token()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			break
		}
	}
	if err := validateLocalEngine("sirsi"); err == nil {
		t.Fatal("PATH fallback accepted")
	}
}

func TestNativeLocalProbesNeverGenerate(t *testing.T) {
	home, engine := localInstallFixture(t)
	type invocation struct {
		path  string
		args  []string
		stdin []byte
	}
	var commands []invocation
	run := func(_ context.Context, path string, input []byte, args ...string) ([]byte, error) {
		commands = append(commands, invocation{
			path: path, args: append([]string(nil), args...), stdin: append([]byte(nil), input...),
		})
		if path == "/usr/bin/vm_stat" && len(args) == 0 && len(input) == 0 {
			return []byte("Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free: 123.\n"), nil
		}
		if path == "/bin/launchctl" && reflect.DeepEqual(args, []string{"print-disabled", fmt.Sprintf("gui/%d", os.Getuid())}) && len(input) == 0 {
			return []byte("disabled services = {\n}"), nil
		}
		return nil, errors.New("forbidden command")
	}
	for _, p := range NativeLocalProbes(engine, home, run) {
		if _, err := p.Read(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	want := []invocation{
		{path: "/bin/launchctl", args: []string{"print-disabled", fmt.Sprintf("gui/%d", os.Getuid())}},
		{path: "/bin/launchctl", args: []string{"print-disabled", fmt.Sprintf("gui/%d", os.Getuid())}},
		{path: "/usr/bin/vm_stat"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("unexpected generation/probe operation: %v", commands)
	}
}

func TestLocalInstallerUnknownReadbackCannotVerify(t *testing.T) {
	home, engine := localInstallFixture(t)
	var calls []string
	base := localFixtureCommand(engine, home, false, &calls)
	run := func(ctx context.Context, path string, input []byte, args ...string) ([]byte, error) {
		if path == "/bin/launchctl" && args[0] == "print" {
			return []byte("unrelated success text mentions " + Label), nil
		}
		return base(ctx, path, input, args...)
	}
	if _, err := InstallLocal(context.Background(), engine, home, run); err == nil {
		t.Fatal("unknown job output verified install")
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", Label+".plist")); err != nil {
		t.Fatal("failed readback lost retained configuration", err)
	}
}

func TestLocalDisabledMalformedSuccessRefused(t *testing.T) {
	for _, body := range []string{"Permission denied", "disabled services = {\n arbitrary diagnostic\n}", "disabled services = {\n\"ai.sirsi.example\" => unknown\n}"} {
		_, err := localDisabled(context.Background(), func(context.Context, string, []byte, ...string) ([]byte, error) { return []byte(body), nil })
		if err == nil {
			t.Fatalf("malformed disabled success admitted: %s", body)
		}
	}
}
