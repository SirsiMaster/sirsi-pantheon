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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type nativeServiceBackend struct{ relay *rootRelayDependencies }

func NativeServiceBackend() ServiceBackend { return nativeServiceBackend{} }

// Closed initial maintenance scope. Registry policy further restricts the actor;
// wildcard/router/wake/model-engine targets are deliberately not supported.
func serviceLocation(label string) (target, plist string, err error) {
	var dir, domain string
	switch label {
	case rootRelayLabel:
		if err := rootRelayOwner(os.Getuid(), os.Geteuid()); err != nil {
			return "", "", err
		}
		return rootRelayTarget, rootRelayPlist, nil
	case "ai.sirsi.apollo.observer", "ai.sirsi.apollo-chat", "ai.sirsi.gemma-broker":
		if os.Geteuid() == 0 || os.Getuid() != os.Geteuid() {
			return "", "", errors.New("service lifecycle: user agent requires its native unprivileged owner")
		}
		home, e := os.UserHomeDir()
		if e != nil {
			return "", "", e
		}
		dir, domain = filepath.Join(home, "Library", "LaunchAgents"), "gui/"+strconv.Itoa(os.Getuid())
	case "ai.sirsi.mercury-monitor", "ai.sirsi.mercury-chip":
		if os.Geteuid() != 0 {
			return "", "", errors.New("service lifecycle: existing privileged controller required; no elevation or prompt fallback")
		}
		dir, domain = "/Library/LaunchDaemons", "system"
	default:
		return "", "", errors.New("service lifecycle: label outside the explicit maintenance scope")
	}
	return domain + "/" + label, filepath.Join(dir, label+".plist"), nil
}

func serviceRead(ctx context.Context, executable string, args ...string) ([]byte, error) {
	return serviceReadInput(ctx, nil, executable, args...)
}

func serviceReadInput(ctx context.Context, input []byte, executable string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var b serviceOutput
	cmd.Stdout = &b
	cmd.Stderr = io.Discard // launchd environments and credential values never enter a receipt
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("service lifecycle: %s failed: %w", filepath.Base(executable), err)
	}
	return b.Bytes(), nil
}

type serviceOutput struct{ bytes.Buffer }

func (b *serviceOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 256*1024 {
		return 0, errors.New("service lifecycle: native response exceeds bound")
	}
	return b.Buffer.Write(p)
}

func serviceFileHash(path string) (string, error) {
	_, hash, err := serviceArtifact(path, false)
	return hash, err
}

// Capture plist semantics and its digest from the same descriptor bytes.
func serviceArtifact(path string, capture bool) ([]byte, string, error) {
	return serviceArtifactPolicy(path, capture, nil)
}

func serviceArtifactPolicy(path string, capture bool, policy func(int) error) ([]byte, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, "", errors.New("service lifecycle: canonical absolute artifact path required")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || parent != filepath.Dir(path) {
		return nil, "", errors.New("service lifecycle: symlinked artifact parent refused")
	}
	// Resolve every component relative to a retained no-follow directory. A
	// leaf-only O_NOFOLLOW would still follow a substituted ancestor.
	parentFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	for _, component := range strings.Split(strings.TrimPrefix(filepath.Dir(path), "/"), "/") {
		if component == "" {
			continue
		}
		next, e := unix.Openat(parentFD, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(parentFD)
		if e != nil {
			return nil, "", e
		}
		parentFD = next
	}
	defer unix.Close(parentFD)
	fd, err := unix.Openat(parentFD, filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, "", err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	before, err := f.Stat()
	limit := int64(256 << 20)
	if capture {
		limit = 1 << 20
	}
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, "", errors.New("service lifecycle: bounded regular artifact required")
	}
	if policy != nil {
		if err := policy(fd); err != nil {
			return nil, "", err
		}
	}
	h := sha256.New()
	var snapshot bytes.Buffer
	var sink io.Writer = h
	if capture {
		sink = io.MultiWriter(h, &snapshot)
	}
	n, err := io.Copy(sink, io.LimitReader(f, limit+1))
	if err != nil || n != before.Size() || n > limit {
		return nil, "", errors.New("service lifecycle: artifact read failed or size changed")
	}
	after, e1 := f.Stat()
	named, e2 := os.Lstat(path)
	if e1 != nil || e2 != nil || !named.Mode().IsRegular() || !os.SameFile(before, after) || !os.SameFile(after, named) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		return nil, "", errors.New("service lifecycle: artifact identity changed during inspection")
	}
	if policy != nil {
		if err := policy(fd); err != nil {
			return nil, "", err
		}
	}
	return snapshot.Bytes(), hex.EncodeToString(h.Sum(nil)), nil
}

func (b nativeServiceBackend) Inspect(ctx context.Context, label string) (ServiceInstance, error) {
	if label == rootRelayLabel {
		return b.inspectRootRelay(ctx)
	}
	target, plist, err := serviceLocation(label)
	if err != nil {
		return ServiceInstance{}, err
	}
	p := ServiceInstance{Label: label, Target: target, Plist: plist}
	snapshot, hash, err := serviceArtifact(plist, true)
	p.PlistSHA256 = hash
	if err != nil {
		return p, err
	}
	decoded, err := serviceReadInput(ctx, snapshot, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	var config struct {
		Label            string
		Program          string
		ProgramArguments []string
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(decoded, &config); err != nil || config.Label != label {
		return p, errors.New("service lifecycle: plist label does not match selected service")
	}
	p.Program = config.Program
	if p.Program == "" && len(config.ProgramArguments) > 0 {
		p.Program = config.ProgramArguments[0]
	}
	if p.ProgramSHA256, err = serviceFileHash(p.Program); err != nil {
		return p, err
	}
	state, err := serviceRead(ctx, "/bin/launchctl", "print", target)
	if err != nil {
		return p, err
	}
	m := regexp.MustCompile(`(?m)^\s*pid = ([0-9]+)\s*$`).FindSubmatch(state)
	if len(m) != 2 {
		return p, errors.New("service lifecycle: selected job has no running instance")
	}
	p.PID, err = strconv.Atoi(string(m[1]))
	if err != nil || p.PID <= 1 {
		return p, errors.New("service lifecycle: invalid native job PID")
	}
	started, err := serviceRead(ctx, "/bin/ps", "-p", strconv.Itoa(p.PID), "-o", "lstart=")
	if err != nil {
		return p, err
	}
	p.Started = strings.TrimSpace(string(started))
	final, err := serviceFileHash(plist)
	if err != nil || final != p.PlistSHA256 {
		return p, errors.New("service lifecycle: plist changed during inspection")
	}
	return p, validateServiceInstance(p)
}

func (b nativeServiceBackend) Refresh(ctx context.Context, p ServiceInstance) error {
	if p.Label == rootRelayLabel {
		return b.refreshRootRelay(ctx, p)
	}
	return withServiceLock(p.Plist, func() error {
		// Two cooperative callers can inspect the same old instance before
		// either retires it. Serialize and recheck inside the lock so the loser
		// cannot stop the winner's newly started service.
		current, err := b.Inspect(ctx, p.Label)
		if err != nil {
			return err
		}
		if current != p {
			return errors.New("service lifecycle: selected instance changed before locked apply")
		}
		return b.refreshLocked(ctx, p)
	})
}

const (
	rootRelayLabel   = "ai.sirsi.router.relay"
	rootRelayTarget  = "system/ai.sirsi.router.relay"
	rootRelayPlist   = "/Library/LaunchDaemons/ai.sirsi.router.relay.plist"
	rootRelayProgram = "/usr/local/libexec/sirsi-pantheon/sirsi"
	rootRelaySpool   = "/var/sirsipantheon/relay"
)

// Only the relay uses these instance-owned seams. Existing service profiles
// retain their implementation; tests never replace process-wide dependencies.
type rootRelayDependencies struct {
	uid, euid  func() int
	read       func(context.Context, []byte, string, ...string) ([]byte, error)
	artifact   func(string, bool) ([]byte, string, error)
	lock       func(string, func() error) error
	wait       func(context.Context, time.Duration) error
	spoolClear func(context.Context) error
}

func (b nativeServiceBackend) relayDependencies() rootRelayDependencies {
	if b.relay != nil {
		return *b.relay
	}
	return rootRelayDependencies{
		uid: os.Getuid, euid: os.Geteuid, read: rootRelayRead, lock: withServiceLock,
		spoolClear: func(ctx context.Context) error { return rootRelaySpoolQuiescentAt(ctx, rootRelayPhysicalSpool, nil) },
		artifact: func(path string, capture bool) ([]byte, string, error) {
			return serviceArtifactPolicy(path, capture, func(fd int) error {
				var st unix.Stat_t
				if err := unix.Fstat(fd, &st); err != nil {
					return err
				}
				return rootRelayArtifactPolicy(capture, st)
			})
		},
		wait: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func rootRelayArtifactPolicy(plist bool, st unix.Stat_t) error {
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != 0 {
		return errors.New("service lifecycle: root-owned regular relay artifact required")
	}
	if plist {
		if st.Mode&07777 != 0600 {
			return errors.New("service lifecycle: credential-bearing root relay plist requires exact mode 0600")
		}
	} else if st.Mode&0022 != 0 || st.Mode&0111 == 0 {
		return errors.New("service lifecycle: protected relay program must be executable without group/other write")
	}
	return nil
}

// Do not embed bytes.Buffer: its promoted ReadFrom would let exec's io.Copy
// bypass Write's bound on a native process listing.
type rootRelayOutput struct{ data bytes.Buffer }

func (b *rootRelayOutput) Write(p []byte) (int, error) {
	if b.data.Len()+len(p) > 256*1024 {
		return 0, errors.New("service lifecycle: relay native response exceeds bound")
	}
	return b.data.Write(p)
}

func rootRelayRead(ctx context.Context, input []byte, executable string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var out rootRelayOutput
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("service lifecycle: relay %s failed: %w", filepath.Base(executable), err)
	}
	return out.data.Bytes(), nil
}

func rootRelayOwner(uid, euid int) error {
	if uid != 0 || euid != 0 {
		return errors.New("service lifecycle: root relay requires native real/effective root; no elevation, prompt or user-agent fallback")
	}
	return nil
}

type rootRelayConfiguration struct {
	Label            string
	Program          string
	ProgramArguments []string
	UserName         string
}

func validateRootRelayConfiguration(config rootRelayConfiguration) error {
	args := config.ProgramArguments
	// Exact six-argument deployed profile, independently observed in native
	// metadata. No loose binary, shell wrapper, alternate spool or extra flags.
	if config.Label != rootRelayLabel || config.UserName != "_sirsipantheon" || (config.Program != "" && config.Program != rootRelayProgram) || len(args) != 6 ||
		args[0] != rootRelayProgram || args[1] != "router" || args[2] != "relay" || args[3] != "serve" ||
		args[4] != "--spool" || args[5] != rootRelaySpool {
		return errors.New("service lifecycle: relay plist program/argv outside protected native profile")
	}
	return nil
}

func (b nativeServiceBackend) rootRelayArtifacts(ctx context.Context) (ServiceInstance, error) {
	d := b.relayDependencies()
	p := ServiceInstance{Label: rootRelayLabel, Target: rootRelayTarget, Plist: rootRelayPlist, Program: rootRelayProgram}
	if err := rootRelayOwner(d.uid(), d.euid()); err != nil {
		return p, err
	}
	snapshot, hash, err := d.artifact(rootRelayPlist, true)
	if err != nil {
		return p, err
	}
	p.PlistSHA256 = hash
	decoded, err := d.read(ctx, snapshot, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	if err != nil {
		return p, err
	}
	var config rootRelayConfiguration
	if err := json.Unmarshal(decoded, &config); err != nil {
		return p, errors.New("service lifecycle: invalid relay plist")
	}
	if err := validateRootRelayConfiguration(config); err != nil {
		return p, err
	}
	_, p.ProgramSHA256, err = d.artifact(rootRelayProgram, false)
	return p, err
}

func (b nativeServiceBackend) inspectRootRelay(ctx context.Context) (ServiceInstance, error) {
	p, err := b.rootRelayArtifacts(ctx)
	if err != nil {
		return p, err
	}
	d := b.relayDependencies()
	state, err := d.read(ctx, nil, "/bin/launchctl", "print", rootRelayTarget)
	if err != nil {
		return p, err
	}
	matches := regexp.MustCompile(`(?m)^\s*pid = ([0-9]+)\s*$`).FindAllSubmatch(state, -1)
	if len(matches) != 1 {
		return p, errors.New("service lifecycle: relay requires one running native instance")
	}
	p.PID, err = strconv.Atoi(string(matches[0][1]))
	if err != nil || p.PID <= 1 {
		return p, errors.New("service lifecycle: invalid relay PID")
	}
	started, err := d.read(ctx, nil, "/bin/ps", "-p", strconv.Itoa(p.PID), "-o", "lstart=")
	if err != nil {
		return p, err
	}
	p.Started = strings.TrimSpace(string(started))
	final, err := b.rootRelayArtifacts(ctx)
	if err != nil || !sameRootRelayArtifacts(p, final) {
		return p, errors.New("service lifecycle: relay artifacts changed during inspection")
	}
	return p, validateServiceInstance(p)
}

func sameRootRelayArtifacts(a, b ServiceInstance) bool {
	return a.Label == b.Label && a.Target == b.Target && a.Plist == b.Plist && a.Program == b.Program &&
		a.PlistSHA256 == b.PlistSHA256 && a.ProgramSHA256 == b.ProgramSHA256
}

func rootRelayChildrenClear(snapshot []byte, pid int) error {
	seen := map[int]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(snapshot)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return errors.New("service lifecycle: unknown relay process enumeration")
		}
		child, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		if e1 != nil || e2 != nil || child < 0 || parent < 0 || seen[child] {
			return errors.New("service lifecycle: invalid relay process enumeration")
		}
		seen[child] = true
		if parent == pid {
			return errors.New("service lifecycle: relay has direct children; owning application must drain before maintenance")
		}
	}
	if !seen[pid] {
		return errors.New("service lifecycle: relay instance absent from process enumeration")
	}
	return nil
}

func (b nativeServiceBackend) refreshRootRelay(ctx context.Context, p ServiceInstance) error {
	d := b.relayDependencies()
	if err := rootRelayOwner(d.uid(), d.euid()); err != nil {
		return err
	}
	if err := validateServiceInstance(p); err != nil || p.Label != rootRelayLabel || p.Target != rootRelayTarget || p.Plist != rootRelayPlist || p.Program != rootRelayProgram {
		return errors.New("service lifecycle: relay instance outside exact system profile")
	}
	return d.lock(rootRelayPlist, func() error {
		// The lock serializes cooperating maintainers, not the relay itself.
		// Repeated checks catch observed drift/children; the owner --drained
		// assertion remains essential because a child may spawn after the last
		// sample. This is not an atomic kernel drain/bootout guarantee.
		for pass := 0; pass < 2; pass++ {
			current, err := b.inspectRootRelay(ctx)
			if err != nil {
				return err
			}
			if current != p {
				return errors.New("service lifecycle: relay instance changed before locked bootout")
			}
			processes, err := d.read(ctx, nil, "/bin/ps", "-axo", "pid=,ppid=")
			if err != nil {
				return errors.New("service lifecycle: relay child enumeration failed; no retirement performed")
			}
			if err := rootRelayChildrenClear(processes, p.PID); err != nil {
				return err
			}
			if d.spoolClear == nil {
				return errors.New("service lifecycle: relay spool quiescence probe unavailable")
			}
			if err := d.spoolClear(ctx); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, bootoutErr := d.read(ctx, nil, "/bin/launchctl", "bootout", rootRelayTarget); bootoutErr != nil {
			// A failed/timed-out command may already have retired the job. Only
			// inspect on this path: failed print is NOT proof of absence and does
			// not authorize bootstrap, repeat bootout or an inferred recovery.
			probe, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			current, probeErr := b.inspectRootRelay(probe)
			if probeErr != nil {
				return fmt.Errorf("service lifecycle: relay retirement OUTCOME UNKNOWN; read-only probe failed; owning application recovery required: %w", errors.Join(bootoutErr, probeErr))
			}
			if current != p {
				return fmt.Errorf("service lifecycle: relay changed concurrently after failed bootout; no further mutation performed: %w", bootoutErr)
			}
			return fmt.Errorf("service lifecycle: bootout failed; original selected relay instance remains alive; no further mutation performed: %w", bootoutErr)
		}
		// Once retired, cancellation of the requester must not abandon recovery.
		// Recovery is still finite, exact-artifact and single-label only.
		recovery, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var last error
		for attempt := 0; attempt < 20; attempt++ {
			artifacts, err := b.rootRelayArtifacts(recovery)
			if err != nil {
				return fmt.Errorf("service lifecycle: relay retired; artifact revalidation unavailable; owning application recovery required: %w", err)
			}
			if !sameRootRelayArtifacts(p, artifacts) {
				return errors.New("service lifecycle: relay retired but artifacts changed; owning application recovery required")
			}
			if _, last = d.read(recovery, nil, "/bin/launchctl", "bootstrap", "system", rootRelayPlist); last == nil {
				for read := 0; read < 40; read++ {
					artifacts, err := b.rootRelayArtifacts(recovery)
					if err != nil {
						return fmt.Errorf("service lifecycle: relay restart artifact revalidation unavailable; owning application recovery required: %w", err)
					}
					if !sameRootRelayArtifacts(p, artifacts) {
						return errors.New("service lifecycle: relay restart artifacts changed; owning application recovery required")
					}
					after, err := b.inspectRootRelay(recovery)
					if err == nil {
						if !sameRootRelayArtifacts(p, after) || (after.PID == p.PID && after.Started == p.Started) {
							return errors.New("service lifecycle: relay restart returned unexpected instance/artifacts")
						}
						confirmed, err := b.inspectRootRelay(recovery)
						if err != nil || confirmed != after {
							return errors.New("service lifecycle: relay instance drifted during restart readback")
						}
						return nil
					}
					if err := d.wait(recovery, 250*time.Millisecond); err != nil {
						return fmt.Errorf("service lifecycle: relay recovery readback failed: %w", err)
					}
				}
				return errors.New("service lifecycle: relay retired but restart readback exhausted; owning application recovery required")
			}
			if err := d.wait(recovery, 250*time.Millisecond); err != nil {
				return fmt.Errorf("service lifecycle: relay bootstrap recovery failed: %w", err)
			}
		}
		return fmt.Errorf("service lifecycle: relay bootstrap retries exhausted; owning application recovery required: %w", last)
	})
}

func withServiceLock(plist string, operation func() error) error {
	path := filepath.Join(filepath.Dir(plist), "."+filepath.Base(plist)+".lifecycle-lock")
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return errors.New("service lifecycle: private owned regular lock required")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("service lifecycle: selected service maintenance already in progress: %w", err)
	}
	// Closing the retained descriptor (already deferred above) releases the
	// lock on every exit; do not add a second fallible unlock operation.
	return operation()
}

func (b nativeServiceBackend) refreshLocked(ctx context.Context, p ServiceInstance) error {
	// Resolve again rather than accepting caller-supplied domains or paths.
	target, plist, err := serviceLocation(p.Label)
	if err != nil || target != p.Target || plist != p.Plist {
		return errors.New("service lifecycle: target does not match native service location")
	}
	if _, err = serviceRead(ctx, "/bin/launchctl", "bootout", target); err != nil {
		return err // unlike bulk refresh, never ignore a failed retirement
	}
	domain := strings.TrimSuffix(target, "/"+p.Label)
	for attempt := 0; attempt < 20; attempt++ {
		if _, err = serviceRead(ctx, "/bin/launchctl", "bootstrap", domain, plist); err == nil {
			for read := 0; read < 40; read++ {
				if _, inspectErr := b.Inspect(ctx, p.Label); inspectErr == nil {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(250 * time.Millisecond):
				}
			}
			return errors.New("service lifecycle: selected service did not become running")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return err
}
