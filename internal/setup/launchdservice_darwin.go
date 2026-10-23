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

type nativeServiceBackend struct{}

func NativeServiceBackend() ServiceBackend { return nativeServiceBackend{} }

// Closed initial maintenance scope. Registry policy further restricts the actor;
// wildcard/router/wake/model-engine targets are deliberately not supported.
func serviceLocation(label string) (target, plist string, err error) {
	var dir, domain string
	switch label {
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
	return snapshot.Bytes(), hex.EncodeToString(h.Sum(nil)), nil
}

func (nativeServiceBackend) Inspect(ctx context.Context, label string) (ServiceInstance, error) {
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
	defer unix.Flock(fd, unix.LOCK_UN)
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
