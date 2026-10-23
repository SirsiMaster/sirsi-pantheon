//go:build darwin

package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"golang.org/x/sys/unix"
)

// Only Horus uses this configuration transition. No dependency is process-global.
type horusServiceDependencies struct {
	uid, euid func() int
	home      func() (string, error)
	directory func(string) error
	read      func(context.Context, []byte, string, ...string) ([]byte, error)
	artifact  func(string, bool) ([]byte, string, error)
	intent    func(context.Context, string, int) error
	lock      func(string, func() error) error
	prepare   func(string, []byte, []byte) (horusPublication, error)
	wait      func(context.Context) error
}

type horusPublication interface {
	Publish() error
	Restore() error
	Close() error
}

const horusSigningRequirement = `anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`

func (b nativeServiceBackend) horusDependencies() horusServiceDependencies {
	if b.horus != nil {
		return *b.horus
	}
	return horusServiceDependencies{
		uid: os.Getuid, euid: os.Geteuid, home: func() (string, error) {
			owner, err := user.LookupId(strconv.Itoa(os.Getuid()))
			if err != nil {
				return "", err
			}
			return owner.HomeDir, nil
		},
		directory: func(path string) error {
			fd, err := openHorusParent(filepath.Join(path, ".horus-inspection-only"))
			if err != nil {
				return errors.New("Horus maintenance: working directory is unavailable or noncanonical")
			}
			return unix.Close(fd)
		},
		read: rootRelayRead, lock: withServiceLock, prepare: prepareHorusPublication,
		intent: horusNativeIntent,
		artifact: func(path string, capture bool) ([]byte, string, error) {
			return serviceArtifactPolicy(path, capture, func(fd int) error {
				var st unix.Stat_t
				if err := unix.Fstat(fd, &st); err != nil {
					return err
				}
				return horusArtifactPolicy(path, capture, os.Getuid(), st)
			})
		},
		wait: func(ctx context.Context) error {
			t := time.NewTimer(100 * time.Millisecond)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

func horusArtifactPolicy(path string, plist bool, uid int, st unix.Stat_t) error {
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&0022 != 0 || (st.Uid != 0 && st.Uid != uint32(uid)) {
		return errors.New("Horus maintenance: owned, non-group/other-writable single-link artifact required")
	}
	if !plist && st.Mode&0111 == 0 {
		return errors.New("Horus maintenance: executable program required")
	}
	if path == horusProtectedProgram && st.Uid != 0 {
		return errors.New("Horus maintenance: protected CLI must be root owned")
	}
	return nil
}

func horusOwnerLocation(d horusServiceDependencies) (string, string, error) {
	if d.uid() <= 0 || d.uid() != d.euid() {
		return "", "", errors.New("Horus maintenance: native GUI account owner required; no elevation")
	}
	home, err := d.home()
	if err != nil || !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return "", "", errors.New("Horus maintenance: canonical owner home unavailable")
	}
	return "gui/" + strconv.Itoa(d.uid()) + "/" + HorusServiceLabel, filepath.Join(home, "Library", "LaunchAgents", HorusServiceLabel+".plist"), nil
}

// No model probe or request-body read. Unknown intent is not permission to retire.
func horusNativeIntent(ctx context.Context, home string, uid int) error {
	if _, err := os.Lstat(router.FabricQuarantineMarkerPath(home)); err == nil {
		return errors.New("Horus maintenance: fabric quarantine preserves stand-down")
	} else if !os.IsNotExist(err) {
		return errors.New("Horus maintenance: quarantine state unavailable")
	}
	out, err := rootRelayRead(ctx, nil, "/bin/launchctl", "print-disabled", "gui/"+strconv.Itoa(uid))
	if err != nil {
		return errors.New("Horus maintenance: disabled state unavailable")
	}
	return horusDisabledIntent(out)
}

func horusDisabledIntent(out []byte) error {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "disabled services = {" || strings.TrimSpace(lines[len(lines)-1]) != "}" {
		return errors.New("Horus maintenance: unknown disabled state")
	}
	re := regexp.MustCompile(`^"([^"\\]+)"\s*=>\s*(disabled|enabled|true|false)$`)
	for _, line := range lines[1 : len(lines)-1] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := re.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) != 3 || len(strings.Fields(m[1])) == 0 {
			return errors.New("Horus maintenance: malformed disabled state")
		}
		for _, label := range strings.Fields(m[1]) {
			if label == HorusServiceLabel && (m[2] == "disabled" || m[2] == "true") {
				return errors.New("Horus maintenance: OS-disabled Horus requires separate reviewed restoration")
			}
		}
	}
	return nil
}

// Parse the one shipped wrapper as data, never shell it. Quoted forms must be
// exactly Go's %q encoding used by supervisorPlistContent; no shell expansion.
func horusLegacyPayload(command string) (string, error) {
	if !strings.HasPrefix(command, "exec ") || !strings.HasSuffix(command, " horus supervise") {
		return "", errors.New("Horus maintenance: unrecognized legacy command; retain selected configuration")
	}
	word := strings.TrimSuffix(strings.TrimPrefix(command, "exec "), " horus supervise")
	path := word
	if strings.HasPrefix(word, `"`) {
		var err error
		path, err = strconv.Unquote(word)
		if err != nil || strconv.Quote(path) != word {
			return "", errors.New("Horus maintenance: unrecognized legacy quoting")
		}
	}
	// Even quoted shell expansions are not a literal payload identity.
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "$`\\\n\r\t\"';&|<>(){}*?![]") || (path == word && strings.ContainsRune(path, ' ')) {
		return "", errors.New("Horus maintenance: literal canonical legacy payload required")
	}
	return path, nil
}

func horusConfiguration(raw []byte) (map[string]json.RawMessage, string, bool, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil || config == nil {
		return nil, "", false, errors.New("Horus maintenance: invalid plist object")
	}
	var label, program, user, workDir string
	var args []string
	if json.Unmarshal(config["Label"], &label) != nil || label != HorusServiceLabel || json.Unmarshal(config["ProgramArguments"], &args) != nil || len(args) == 0 {
		return nil, "", false, errors.New("Horus maintenance: selected label/arguments mismatch")
	}
	if v, ok := config["Program"]; ok {
		if json.Unmarshal(v, &program) != nil || program != args[0] {
			return nil, "", false, errors.New("Horus maintenance: Program/argv disagreement")
		}
	}
	if v, ok := config["UserName"]; ok {
		if json.Unmarshal(v, &user) != nil || user != "" {
			return nil, "", false, errors.New("Horus maintenance: GUI owner must not be overridden")
		}
	}
	if json.Unmarshal(config["WorkingDirectory"], &workDir) != nil || !filepath.IsAbs(workDir) || filepath.Clean(workDir) != workDir {
		return nil, "", false, errors.New("Horus maintenance: explicit canonical registry working directory required")
	}
	var env map[string]string
	if v, ok := config["EnvironmentVariables"]; ok {
		if json.Unmarshal(v, &env) != nil {
			return nil, "", false, errors.New("Horus maintenance: malformed environment")
		}
		for key := range env {
			upper := strings.ToUpper(key)
			if upper == "TOKEN" || upper == "PASSWORD" || upper == "SECRET" || strings.HasSuffix(upper, "_TOKEN") || strings.HasSuffix(upper, "_PASSWORD") || strings.HasSuffix(upper, "_SECRET") {
				return nil, "", false, errors.New("Horus maintenance: inline credential in selected environment; establish existing credential-free enrollment/selector before conversion; original retained")
			}
		}
	}
	if len(args) == 3 && args[0] == horusProtectedProgram && args[1] == "horus" && args[2] == "supervise" {
		return config, horusProtectedProgram, false, nil
	}
	if len(args) != 4 || args[0] != "/bin/zsh" || args[1] != "-l" || args[2] != "-c" {
		return nil, "", false, errors.New("Horus maintenance: unrecognized selected legacy wrapper")
	}
	payload, err := horusLegacyPayload(args[3])
	return config, payload, true, err
}

func horusDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Dictionary order is not a plist semantic. Keep numeric tokens exact rather
// than rounding large integers through float64 during nested comparison.
func horusSameConfiguration(a, b []byte) bool {
	decode := func(data []byte) (any, error) {
		var value any
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil // complete JSON values are marshaled at both call sites
	}
	x, ex := decode(a)
	y, ey := decode(b)
	return ex == nil && ey == nil && reflect.DeepEqual(x, y)
}

func (b nativeServiceBackend) horusArtifacts(ctx context.Context) (ServiceInstance, []byte, []byte, error) {
	d := b.horusDependencies()
	target, path, err := horusOwnerLocation(d)
	p := ServiceInstance{Label: HorusServiceLabel, Target: target, Plist: path}
	if err != nil {
		return p, nil, nil, err
	}
	home, err := d.home()
	if err != nil {
		return p, nil, nil, err
	}
	if err = d.intent(ctx, home, d.uid()); err != nil {
		return p, nil, nil, err
	}
	original, hash, err := d.artifact(path, true)
	if err != nil {
		return p, nil, nil, err
	}
	p.PlistSHA256 = hash
	raw, err := d.read(ctx, original, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	if err != nil {
		return p, nil, nil, err
	}
	config, payload, legacy, err := horusConfiguration(raw)
	if err != nil {
		return p, nil, nil, err
	}
	var workDir string
	if json.Unmarshal(config["WorkingDirectory"], &workDir) != nil {
		return p, nil, nil, errors.New("Horus maintenance: missing working directory")
	}
	if err = d.directory(workDir); err != nil {
		return p, nil, nil, err
	}
	p.Program = payload
	_, p.ProgramSHA256, err = d.artifact(payload, false)
	if err != nil {
		return p, nil, nil, err
	}
	if legacy {
		_, p.LauncherSHA256, err = d.artifact("/bin/zsh", false)
		if err != nil {
			return p, nil, nil, err
		}
	}
	_, p.ReplacementSHA256, err = d.artifact(horusProtectedProgram, false)
	if err != nil {
		return p, nil, nil, err
	}
	// Verify provenance without executing either payload or printing tool output.
	for _, program := range []string{payload, horusProtectedProgram} {
		if _, err = d.read(ctx, nil, "/usr/bin/codesign", "--verify", "--strict", "-R", horusSigningRequirement, program); err != nil {
			return p, nil, nil, errors.New("Horus maintenance: installed payload signature verification failed")
		}
	}
	converted := original
	if legacy {
		delete(config, "Program")
		config["ProgramArguments"], _ = json.Marshal([]string{horusProtectedProgram, "horus", "supervise"})
		encoded, err := json.Marshal(config)
		if err != nil {
			return p, nil, nil, errors.New("Horus maintenance: cannot encode selected conversion")
		}
		converted, err = d.read(ctx, encoded, "/usr/bin/plutil", "-convert", "xml1", "-o", "-", "-")
		if err != nil {
			return p, nil, nil, err
		}
		// Verify lossless semantics before publishing any staged file.
		check, err := d.read(ctx, converted, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
		if err != nil {
			return p, nil, nil, err
		}
		var roundTrip map[string]json.RawMessage
		if json.Unmarshal(check, &roundTrip) != nil {
			return p, nil, nil, errors.New("Horus maintenance: conversion readback unavailable")
		}
		canonical, _ := json.Marshal(roundTrip)
		if !horusSameConfiguration(encoded, canonical) {
			return p, nil, nil, errors.New("Horus maintenance: conversion changed unrelated plist semantics")
		}
	}
	p.ConvertedPlistSHA256 = horusDigest(converted)
	return p, original, converted, nil
}

func (b nativeServiceBackend) inspectHorus(ctx context.Context) (ServiceInstance, error) {
	d := b.horusDependencies()
	p, _, _, err := b.horusArtifacts(ctx)
	if err != nil {
		return p, err
	}
	state, err := d.read(ctx, nil, "/bin/launchctl", "print", p.Target)
	if err != nil {
		return p, err
	}
	m := regexp.MustCompile(`(?m)^\s*pid = ([0-9]+)\s*$`).FindSubmatch(state)
	if len(m) != 2 {
		return p, errors.New("Horus maintenance: no selected running instance")
	}
	p.PID, err = strconv.Atoi(string(m[1]))
	if err != nil || p.PID <= 1 {
		return p, errors.New("Horus maintenance: invalid selected PID")
	}
	start, err := d.read(ctx, nil, "/bin/ps", "-p", strconv.Itoa(p.PID), "-o", "lstart=")
	if err != nil {
		return p, err
	}
	p.Started = strings.TrimSpace(string(start))
	comm, err := d.read(ctx, nil, "/bin/ps", "-p", strconv.Itoa(p.PID), "-o", "comm=")
	if err != nil || strings.TrimSpace(string(comm)) != p.Program {
		return p, errors.New("Horus maintenance: loaded executable does not match selected payload")
	}
	final, _, _, err := b.horusArtifacts(ctx)
	if err != nil {
		return p, err
	}
	final.PID, final.Started = p.PID, p.Started
	if final != p {
		return p, errors.New("Horus maintenance: selected artifacts changed during inspection")
	}
	p.PlanSHA256 = horusPlanDigest(p)
	return p, validateServiceInstance(p)
}

func (b nativeServiceBackend) refreshHorus(ctx context.Context, p ServiceInstance) error {
	d := b.horusDependencies()
	target, path, err := horusOwnerLocation(d)
	if err != nil {
		return err
	}
	if err = validateServiceInstance(p); err != nil || p.Target != target || p.Plist != path {
		return errors.New("Horus maintenance: complete selected GUI-owner conversion required")
	}
	return d.lock(path, func() (result error) {
		// Horus Go duties can run without children. The existing owner --drained
		// assertion must quiesce producers and in-progress passes; these repeated
		// observations are cooperative samples, not an atomic drain handshake.
		check := func() error {
			current, err := b.inspectHorus(ctx)
			if err != nil {
				return err
			}
			if current != p {
				return errors.New("Horus maintenance: instance/configuration changed before retirement")
			}
			processes, err := d.read(ctx, nil, "/bin/ps", "-axo", "pid=,ppid=")
			if err != nil {
				return errors.New("Horus maintenance: child probe unavailable")
			}
			if err = rootRelayChildrenClear(processes, p.PID); err != nil {
				return fmt.Errorf("Horus maintenance: unsafe child snapshot: %w", err)
			}
			return nil
		}
		for pass := 0; pass < 2; pass++ {
			if err := check(); err != nil {
				return err
			}
		}
		artifacts, original, converted, err := b.horusArtifacts(ctx)
		if err != nil {
			return err
		}
		artifacts.PID, artifacts.Started = p.PID, p.Started
		artifacts.PlanSHA256 = horusPlanDigest(artifacts)
		if artifacts != p {
			return errors.New("Horus maintenance: conversion changed before staging")
		}
		publication, err := d.prepare(path, original, converted)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, publication.Close()) }() // retained leaves are recovery evidence, never auto-unlinked
		if err := check(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err = d.read(ctx, nil, "/bin/launchctl", "bootout", target); err != nil {
			probe, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			current, probeErr := b.inspectHorus(probe)
			if probeErr != nil {
				return fmt.Errorf("Horus maintenance: retirement OUTCOME UNKNOWN; no further mutation: %w", errors.Join(err, probeErr))
			}
			if current != p {
				return fmt.Errorf("Horus maintenance: changed instance after failed retirement; no further mutation: %w", err)
			}
			return fmt.Errorf("Horus maintenance: original instance remains after failed retirement; no further mutation: %w", err)
		}
		recovery, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err = publication.Publish(); err != nil {
			// No new program has been bootstrapped. Restore only the transaction's
			// retained identities; unknown replacement is explicit recovery debt.
			if restoreErr := publication.Restore(); restoreErr != nil {
				return fmt.Errorf("Horus maintenance: publication failed; original backup retained, recovery required: %w", errors.Join(err, restoreErr))
			}
			return errors.Join(err, b.horusBootstrapReadback(recovery, p, false))
		}
		return b.horusBootstrapReadback(recovery, p, true)
	})
}

func (b nativeServiceBackend) horusBootstrapReadback(ctx context.Context, before ServiceInstance, converted bool) (result error) {
	defer func() {
		if result != nil {
			result = fmt.Errorf("%s: %w", horusRecoveryDescription(before, converted), result)
		}
	}()
	d := b.horusDependencies()
	want := before
	if converted {
		want = horusExpectedAfter(before)
	}
	// Validate the selected bytes and both payloads again before bootstrap.
	actual, _, _, err := b.horusArtifacts(ctx)
	if err != nil {
		return err
	}
	actual.PID, actual.Started = want.PID, want.Started
	actual.PlanSHA256 = horusPlanDigest(actual)
	if actual != want {
		return errors.New("Horus maintenance: retired job artifacts changed; backup retained, recovery required")
	}
	domain := strings.TrimSuffix(before.Target, "/"+HorusServiceLabel)
	_, bootstrapErr := d.read(ctx, nil, "/bin/launchctl", "bootstrap", domain, before.Plist)
	// A failed bootstrap is not proof of absence. Never repeat it or launch the
	// legacy payload speculatively. Read back only, preserving the original error.
	for attempt := 0; attempt < 40; attempt++ {
		current, inspectErr := b.inspectHorus(ctx)
		if inspectErr == nil {
			wanted := want
			wanted.PID, wanted.Started = current.PID, current.Started
			wanted.PlanSHA256 = horusPlanDigest(wanted)
			if current != wanted || (current.PID == before.PID && current.Started == before.Started) {
				return errors.Join(bootstrapErr, errors.New("Horus maintenance: unexpected recovered instance/configuration"))
			}
			confirmed, err := b.inspectHorus(ctx)
			if err != nil || confirmed != current {
				return errors.Join(bootstrapErr, errors.New("Horus maintenance: recovery instance drifted"))
			}
			return bootstrapErr
		}
		if err := d.wait(ctx); err != nil {
			return errors.Join(bootstrapErr, err)
		}
	}
	return errors.Join(bootstrapErr, errors.New("Horus maintenance: bounded restart readback exhausted; backup retained, recovery required"))
}

func horusRecoveryDescription(before ServiceInstance, converted bool) string {
	want := before
	if converted {
		want = horusExpectedAfter(before)
	}
	args, _ := json.Marshal([]string{"/bin/launchctl", "bootstrap", strings.TrimSuffix(before.Target, "/"+HorusServiceLabel), before.Plist})
	backup := filepath.Join(filepath.Dir(before.Plist), horusBackupLeaf(before.PlistSHA256))
	return fmt.Sprintf("Horus maintenance: retained original backup %s; owner recovery requires rechecking selected plist sha256 %s and program %s sha256 %s, intent and job state; exact recovery argv %s (no enable, bootout or legacy replay implied)", backup, want.PlistSHA256, want.Program, want.ProgramSHA256, args)
}
