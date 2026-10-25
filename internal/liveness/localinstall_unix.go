//go:build !windows

package liveness

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

type limitedLocalOutput struct{ data bytes.Buffer }

func (b *limitedLocalOutput) Write(p []byte) (int, error) {
	if b.data.Len()+len(p) > 64*1024 {
		return 0, fmt.Errorf("local metadata output over bound")
	}
	return b.data.Write(p)
}

func localCommand(ctx context.Context, path string, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.WaitDelay = time.Second
	var output limitedLocalOutput
	cmd.Stdout = &output
	// Diagnostic bodies are neither retained nor included in journal evidence.
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return output.data.Bytes(), err
}

// CurrentLocalEngine is the executable actually running, never a PATH fallback.
func CurrentLocalEngine() (string, error) {
	engine, err := os.Executable()
	if err != nil {
		return "", err
	}
	engine, err = filepath.EvalSymlinks(engine)
	if err != nil {
		return "", err
	}
	if err = validateLocalEngine(engine); err != nil {
		return "", err
	}
	return engine, nil
}

func validateLocalEngine(engine string) error {
	if !filepath.IsAbs(engine) {
		return errors.New("local watch: absolute current engine required")
	}
	info, err := os.Lstat(engine)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return errors.New("local watch: current engine must be a non-writable regular executable")
	}
	return nil
}

// LocalPlistContent retains the existing run-and-exit cadence/label. HOME is
// selected only because this exact --local runtime never resolves a checkout.
func LocalPlistContent(engine, home string) string {
	return strings.Replace(PlistContent(html.EscapeString(engine), html.EscapeString(home)), "<string>run</string>", "<string>run</string>\n\t\t<string>--local</string>", 1)
}

func readLocalRegular(path string, limit int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("local watch: invalid metadata file")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(b)) > limit {
		err = errors.New("local watch: metadata over bound")
	}
	return b, err
}

func localDisabled(ctx context.Context, run LocalCommand) (map[string]bool, error) {
	b, err := run(ctx, "/bin/launchctl", nil, "print-disabled", fmt.Sprintf("gui/%d", os.Getuid()))
	if err != nil {
		return nil, err
	}
	// Do not normalize arbitrary successful diagnostics into an empty/healthy set.
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "disabled services = {") || !strings.HasSuffix(s, "}") {
		return nil, errors.New("invalid disabled metadata")
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "disabled services = {"), "}"))
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		label, value, ok := strings.Cut(line, "=>")
		label = strings.TrimSpace(label)
		value = strings.TrimSpace(value)
		if !ok || len(label) < 3 || label[0] != '"' || label[len(label)-1] != '"' || (value != "enabled" && value != "disabled") || seen[label] {
			return nil, errors.New("malformed disabled metadata row")
		}
		seen[label] = true
	}
	out := map[string]bool{}
	for _, label := range platform.ParseDisabledLabels(s) {
		out[label] = true
	}
	return out, nil
}

// InspectLocalWatch verifies configuration and loaded state, not a persistent
// PID: an idle StartInterval job can be loaded without any running process.
func InspectLocalWatch(ctx context.Context, engine, home string, run LocalCommand) (LocalObservation, error) {
	result := LocalObservation{Check: "local-watch", State: "observed"}
	if run == nil {
		run = localCommand
	}
	disabled, err := localDisabled(ctx, run)
	if err != nil {
		return result, err
	}
	if disabled[Label] {
		result.State, result.Detail = "not-applicable", "local watch is operator-disabled; deliberate stop preserved"
		return result, nil
	}
	path := filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
	b, err := readLocalRegular(path, 64*1024)
	if errors.Is(err, os.ErrNotExist) {
		result.Concern, result.Detail = true, "local metadata watch is not installed"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	configurationDigest := sha256.Sum256(b)
	b, err = run(ctx, "/usr/bin/plutil", b, "-convert", "json", "-o", "-", "-")
	if err != nil {
		return result, err
	}
	var plist struct {
		Label                string
		Program              string
		ProgramArguments     []string
		WorkingDirectory     string
		RunAtLoad            bool
		StartInterval        int
		KeepAlive            any
		EnvironmentVariables any
	}
	if err = json.Unmarshal(b, &plist); err != nil {
		return result, err
	}
	if plist.Label != Label || plist.Program != "" || !slices.Equal(plist.ProgramArguments, []string{engine, "liveness-watch", "run", "--local"}) || plist.WorkingDirectory != home || !plist.RunAtLoad || plist.StartInterval != StartInterval || plist.KeepAlive != nil || plist.EnvironmentVariables != nil {
		result.Concern, result.Detail = true, "watch configuration is legacy or differs from the closed local metadata schedule"
		return result, nil
	}
	b, err = run(ctx, "/bin/launchctl", nil, "print", fmt.Sprintf("gui/%d/%s", os.Getuid(), Label))
	if err != nil {
		listed, listErr := run(ctx, "/bin/launchctl", nil, "list")
		if listErr != nil {
			return result, fmt.Errorf("local watch loaded state unavailable: %w", errors.Join(err, listErr))
		}
		loaded, parseErr := localWatchListed(string(listed), Label)
		if parseErr != nil {
			return result, parseErr
		}
		if !loaded {
			result.Concern, result.Detail = true, "exact local metadata schedule is configured and enabled but not loaded"
			return result, nil
		}
		return result, fmt.Errorf("launchd listed the local watch but its exact loaded state was unavailable: %w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(b)), fmt.Sprintf("gui/%d/%s = {", os.Getuid(), Label)) {
		return result, errors.New("unrecognized loaded-job metadata")
	}
	// A new plist on disk does not prove launchd consumed it. Bind readback to
	// the loaded argv/cwd as well, without requiring an always-running PID.
	var loadedArgs []string
	var loadedHome string
	insideArgs := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "arguments = {" {
			if loadedArgs != nil {
				return result, errors.New("duplicate loaded arguments")
			}
			loadedArgs = []string{}
			insideArgs = true
			continue
		}
		if insideArgs {
			if line == "}" {
				insideArgs = false
			} else {
				loadedArgs = append(loadedArgs, line)
			}
			continue
		}
		if value, ok := strings.CutPrefix(line, "working directory = "); ok {
			if loadedHome != "" {
				return result, errors.New("duplicate loaded cwd")
			}
			loadedHome = value
		}
	}
	if insideArgs || !slices.Equal(loadedArgs, []string{engine, "liveness-watch", "run", "--local"}) || loadedHome != home {
		return result, errors.New("loaded watch configuration differs from local schedule")
	}
	result.ConfigurationSHA256 = hex.EncodeToString(configurationDigest[:])
	result.Detail = "exact local metadata schedule is loaded; no persistent PID required"
	return result, nil
}

func localWatchListed(output, label string) (bool, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.Join(strings.Fields(lines[0]), " ") != "PID Status Label" {
		return false, errors.New("unrecognized launchctl list metadata; local watch state remains unknown")
	}
	listed := false
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) != 3 || (fields[0] != "-" && !isDecimalPID(fields[0])) || !isSignedLaunchctlStatus(fields[1]) || fields[2] == "" {
			return false, errors.New("malformed launchctl list metadata; local watch state remains unknown")
		}
		if fields[2] == label {
			listed = true
		}
	}
	return listed, nil
}

func isSignedLaunchctlStatus(value string) bool {
	digits := strings.TrimPrefix(value, "-")
	if !isDecimalPID(digits) {
		return false
	}
	_, err := strconv.ParseInt(value, 10, 32)
	return err == nil
}

func isDecimalPID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func NativeLocalProbes(engine, home string, run LocalCommand) []LocalProbe {
	if run == nil {
		run = localCommand
	}
	return []LocalProbe{
		{Check: "local-watch", Read: func(ctx context.Context) (LocalObservation, error) { return InspectLocalWatch(ctx, engine, home, run) }},
		{Check: "launchd-disabled", Read: func(ctx context.Context) (LocalObservation, error) {
			disabled, err := localDisabled(ctx, run)
			count := 0
			for label := range disabled {
				if platform.ManagedLabel(label) {
					count++
				}
			}
			return LocalObservation{Check: "launchd-disabled", State: "observed", Concern: count > 0, Detail: fmt.Sprintf("%d managed disabled overrides observed; intentional stops remain unchanged", count)}, err
		}},
		{Check: "menubar", Read: func(ctx context.Context) (LocalObservation, error) {
			o := LocalObservation{Check: "menubar", State: "not-applicable", Detail: "optional menubar is not configured"}
			if _, err := os.Lstat(MenubarQuarantineMarkerPath(home)); err == nil {
				o.Detail = "menubar owner quarantine preserved"
				return o, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return o, err
			}
			if _, err := readLocalRegular(filepath.Join(home, "Library", "LaunchAgents", MenubarLabel+".plist"), 64*1024); errors.Is(err, os.ErrNotExist) {
				return o, nil
			} else if err != nil {
				return o, err
			}
			disabled, err := localDisabled(ctx, run)
			if err != nil {
				return o, err
			}
			if disabled[MenubarLabel] {
				o.Detail = "menubar operator-disabled; stop preserved"
				return o, nil
			}
			b, err := run(ctx, "/bin/launchctl", nil, "print", fmt.Sprintf("gui/%d/%s", os.Getuid(), MenubarLabel))
			if err != nil {
				return o, err
			}
			if !strings.HasPrefix(strings.TrimSpace(string(b)), fmt.Sprintf("gui/%d/%s = {", os.Getuid(), MenubarLabel)) {
				return o, errors.New("unrecognized menubar metadata")
			}
			o.State, o.Detail = "observed", "configured menubar job is loaded; UI responsiveness not tested"
			return o, nil
		}},
		{Check: "memory-metadata", Read: func(ctx context.Context) (LocalObservation, error) {
			o := LocalObservation{Check: "memory-metadata", State: "observed"}
			b, err := run(ctx, "/usr/bin/vm_stat", nil)
			if err != nil {
				return o, err
			}
			if !strings.HasPrefix(string(b), "Mach Virtual Memory Statistics:") {
				return o, errors.New("unrecognized VM metadata")
			}
			for _, line := range strings.Split(string(b), "\n") {
				if value, ok := strings.CutPrefix(line, "Pages free:"); ok {
					n, e := strconv.ParseUint(strings.Trim(strings.TrimSpace(value), "."), 10, 64)
					if e != nil {
						return o, e
					}
					o.Detail = fmt.Sprintf("VM free pages observed: %d; memory pressure and inference readiness are not inferred", n)
					return o, nil
				}
			}
			return o, errors.New("missing VM free-page metadata")
		}},
	}
}

// InstallLocal is explicit per-user provisioning. It never clears OS disabled
// intent, changes another label, or falls back to an unresolved PATH command.
func InstallLocal(ctx context.Context, engine, home string, run LocalCommand) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("local watch installation requires macOS")
	}
	if os.Geteuid() == 0 {
		return "", errors.New("local watch installation requires the ordinary GUI owner")
	}
	if err := validateLocalEngine(engine); err != nil {
		return "", err
	}
	if !filepath.IsAbs(home) {
		return "", errors.New("absolute local HOME required")
	}
	info, err := os.Lstat(home)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || st.Uid != uint32(os.Getuid()) {
		return "", errors.New("owned nonsymlink HOME required")
	}
	if run == nil {
		run = localCommand
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	disabled, err := localDisabled(ctx, run)
	if err != nil {
		return "", fmt.Errorf("local watch intent unavailable: %w", err)
	}
	if disabled[Label] {
		return "", errors.New("local watch is operator-disabled; no automatic restoration")
	}
	parent := filepath.Join(home, "Library", "LaunchAgents")
	for _, p := range []string{filepath.Join(home, "Library"), parent} {
		i, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			if e = os.Mkdir(p, 0o755); e != nil {
				return "", e
			}
			i, e = os.Lstat(p)
		}
		if e != nil {
			return "", e
		}
		if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("unsafe local LaunchAgents parent")
		}
	}
	path := filepath.Join(parent, Label+".plist")
	previous, err := readLocalRegular(path, 64*1024)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err == nil {
		converted, e := run(ctx, "/usr/bin/plutil", previous, "-convert", "json", "-o", "-", "-")
		if e != nil {
			return "", e
		}
		var old struct{ Label string }
		if e = json.Unmarshal(converted, &old); e != nil || old.Label != Label {
			return "", errors.New("existing plist is not the exact owned watch label")
		}
	}
	// Escape all variable XML values; do not interpolate HOME into a shell.
	content := LocalPlistContent(engine, home)
	stage, err := os.CreateTemp(parent, ".liveness-local-*")
	if err != nil {
		return "", err
	}
	// Retain stage on failure rather than unlinking an unknown replacement.
	debt := stage.Name()
	_, writeErr := io.WriteString(stage, content)
	syncErr := stage.Sync()
	closeErr := stage.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return "", fmt.Errorf("local watch stage retained at %s: %w", debt, err)
	}
	if err = os.Rename(debt, path); err != nil {
		return "", fmt.Errorf("local watch stage retained at %s: %w", debt, err)
	}
	// The existing exact-label unload/load lifecycle is retained, but all calls
	// are bounded and readback is independent of an idle job having a PID.
	_, _ = run(ctx, "/bin/launchctl", nil, "unload", path)
	if _, err = run(ctx, "/bin/launchctl", nil, "load", path); err != nil {
		return "", fmt.Errorf("local watch plist retained; load unverified: %w", err)
	}
	o, err := InspectLocalWatch(ctx, engine, home, run)
	if err != nil {
		return "", fmt.Errorf("local watch plist retained; readback unavailable: %w", err)
	}
	if o.State != "observed" || o.Concern {
		return "", errors.New("local watch installation did not verify")
	}
	return "local metadata watch installed and verified; Ma'at recording is local, Ra delivery not attempted", nil
}
