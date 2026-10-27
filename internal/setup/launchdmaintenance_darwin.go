//go:build darwin

package setup

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

// These exact source revisions were independently inspected for the existing
// bounded legacy maintenance profile. A version label never selects a profile.
const (
	maintenanceLegacySourceCurrent = "ba2a07bb464bbfd68808254a7d489769f122149b"
	maintenanceLegacySource121     = "22944b2458f377a9f3e5e10c63c3442cb3ea83c3"
)

func maintenanceLegacySourceProfile(info *debug.BuildInfo) (string, error) {
	if info == nil || info.Main.Path != "github.com/SirsiMaster/sirsi-pantheon" {
		return "", errors.New("main module identity unavailable")
	}
	var revision, modified string
	hasRevision, hasModified := false, false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if hasRevision {
				return "", errors.New("duplicate VCS revision setting")
			}
			revision, hasRevision = setting.Value, true
		case "vcs.modified":
			if hasModified {
				return "", errors.New("duplicate VCS modified setting")
			}
			modified, hasModified = setting.Value, true
		}
	}
	if !hasRevision || !hasModified || modified != "false" {
		return "", errors.New("clean VCS source settings unavailable")
	}
	switch revision {
	case maintenanceLegacySourceCurrent, maintenanceLegacySource121:
		return revision, nil
	default:
		return "", errors.New("source revision is not an inspected maintenance profile")
	}
}

func maintenanceRelayUser() (int, int, error) {
	return servicedrain.MaintenanceRelayIdentity()
}

func maintenancePayload(path string) (string, error) {
	_, hash, err := serviceArtifactPolicy(path, false, func(fd int) error {
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Nlink != 1 || !routerstore.MaintenanceMetadataNoACL(fd) {
			return errors.New("legacy maintenance: retained singly linked payload required")
		}
		return rootRelayArtifactPolicy(false, st)
	})
	return hash, err
}

func (b nativeServiceBackend) maintenancePreview(ctx context.Context, legacy string) (MaintenancePlan, error) {
	var plan MaintenancePlan
	if err := rootRelayOwner(os.Getuid(), os.Geteuid()); err != nil {
		return plan, err
	}
	if err := maintenanceEnabled(ctx); err != nil {
		return plan, err
	}
	p, err := b.inspectRootRelay(ctx)
	if err != nil {
		return plan, err
	}
	uid, _, err := maintenanceRelayUser()
	if err != nil {
		return plan, err
	}
	k, err := unix.SysctlKinfoProc("kern.proc.pid", p.PID)
	if err != nil || k == nil || k.Eproc.Pcred.P_ruid != uint32(uid) {
		return plan, errors.New("legacy maintenance: selected PID is not the exact configured service user")
	}
	start, err := servicedrain.MaintenanceStart(p.PID)
	if err != nil {
		return plan, err
	}
	if err := servicedrain.MaintenanceEndpointAbsent(uid, p.PID); err != nil {
		return plan, err // Never weaken a modern/occupied producer's strict path.
	}
	legacyHash, err := maintenancePayload(legacy)
	if err != nil {
		return plan, fmt.Errorf("legacy maintenance: recoverable signed old payload required: %w", err)
	}
	info, err := buildinfo.ReadFile(legacy)
	if err != nil {
		return plan, fmt.Errorf("legacy maintenance: read old payload source identity: %w", err)
	}
	legacySource, err := maintenanceLegacySourceProfile(info)
	if err != nil {
		return plan, fmt.Errorf("legacy maintenance: old payload source profile refused: %w", err)
	}
	loaded, err := maintenanceImage(ctx, legacy, p.PID)
	if err != nil {
		return plan, err
	}
	if hash, err := maintenancePayload(legacy); err != nil || hash != legacyHash {
		return plan, errors.New("legacy maintenance: retained old payload changed")
	}
	replacement, err := maintenanceImage(ctx, rootRelayProgram, 0)
	if err != nil {
		return plan, err
	}
	if loaded == replacement {
		return plan, errors.New("legacy maintenance: old loaded image is not a distinct replacement transition")
	}
	if err := maintenanceReplacementCapability(ctx); err != nil {
		return plan, err
	}
	root, st, err := maintenanceSpool(0, 0)
	if err != nil {
		return plan, err
	}
	_ = root.Close()
	final, err := b.inspectRootRelay(ctx)
	finalStart, startErr := servicedrain.MaintenanceStart(p.PID)
	if err != nil || final != p || startErr != nil || finalStart != start {
		return plan, errors.New("legacy maintenance: selected loaded instance/artifacts changed during preview")
	}
	plan = MaintenancePlan{Profile: RootRelayMaintenanceProfile, Original: p, LegacyPayload: legacy,
		LegacySHA256: legacyHash, LoadedCDHash: loaded, KernelStart: start, ReplacementCDHash: replacement,
		LegacySource: legacySource, RootDevice: uint64(st.Dev), RootInode: uint64(st.Ino),
		WriterScope: "cooperative-publication-only; uncovered-writers-and-old-remote-outcomes-unknown"}
	plan.SHA256 = maintenanceDigest(plan)
	return plan, plan.validate()
}

func maintenanceReplacementCapability(ctx context.Context) error {
	// A known signed engine's fixed read-only verb only. Never a shell, old
	// payload, service entry, arbitrary executable or caller-provided command.
	raw, err := rootRelayRead(ctx, nil, rootRelayProgram, "router", "relay", "maintenance-capabilities")
	if err != nil {
		return errors.New("legacy maintenance: replacement does not implement startup-held protocol; no retirement")
	}
	contract, err := servicedrain.DecodeMaintenance[routerstore.RootMaintenanceContract](raw)
	if err != nil || contract != routerstore.RootMaintenanceCapability() {
		return errors.New("legacy maintenance: replacement startup/publication implementation is not this reviewed contract")
	}
	return nil
}

func (b nativeServiceBackend) maintenanceOperations() maintenanceOperations {
	if b.maintenance != nil {
		return *b.maintenance
	}
	j := maintenanceJournal{directory: "/Library/LaunchDaemons", owner: 0}
	native := b.relayDependencies()
	var observed servicedrain.MaintenanceReady
	return maintenanceOperations{
		appLoad:        j.appState,
		appPersist:     j.persistAppState,
		appSync:        j.syncAppState,
		restorePreview: b.maintenanceRestorePreview,
		restorePrepare: b.maintenanceRestorePrepare,
		abortVerify:    b.maintenanceAbortVerify,
		abortFinish: func(ctx context.Context, s MaintenanceStatus) error {
			if err := b.maintenanceAbortVerify(ctx, s); err != nil {
				return err
			}
			if err := maintenanceMarker(s, true); err != nil {
				return err
			}
			return maintenanceClearPreparation(s)
		},
		preparedSuccessor: func(ctx context.Context, s MaintenanceStatus) (bool, error) {
			start, err := servicedrain.MaintenanceStart(s.Plan.Original.PID)
			if err == nil {
				if start == s.Plan.KernelStart {
					return false, nil
				}
				return false, errors.New("legacy maintenance: old PID reused during preparation recovery")
			}
			if !errors.Is(err, unix.ESRCH) {
				return false, err
			}
			if err := b.maintenanceHeldSuccessor(ctx, s); err != nil {
				return false, err
			}
			return true, nil
		},
		prepare: maintenancePrepare,
		pending: func(s MaintenanceStatus) (bool, error) {
			p, err := maintenanceReadPreparation()
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			if p.ID != s.ID || p.Plan != s.Plan {
				return false, errors.New("legacy maintenance: pending preparation identity differs")
			}
			return true, nil
		},
		clearPreparation: maintenanceClearPreparation,
		loadRecovery: func(id, digest string) (MaintenanceStatus, error) {
			current, err := j.load()
			if err != nil {
				return current, err
			}
			if current.ID == id && current.Plan.SHA256 == digest {
				return current, nil
			}
			if current.ID != "" && current.Phase != "completed" && current.Phase != "aborted" {
				return current, errors.New("legacy maintenance: another durable transaction owns recovery")
			}
			prepared, err := maintenanceReadPreparation()
			if err != nil {
				return current, err
			}
			if prepared.ID != id || prepared.Plan.SHA256 != digest || prepared.Revision != current.Revision+1 {
				return current, errors.New("legacy maintenance: exact durable prepare reconstruction refused")
			}
			if err := j.persist(current.Revision, prepared); err != nil {
				return current, err
			}
			return j.load()
		},
		preview: b.maintenancePreview, load: func() (MaintenanceStatus, error) {
			if err := rootRelayOwner(os.Getuid(), os.Geteuid()); err != nil {
				return MaintenanceStatus{}, err
			}
			current, err := j.load()
			if err != nil {
				return current, err
			}
			if current.ID == "" || current.Phase == "completed" || current.Phase == "aborted" {
				prepared, err := maintenanceReadPreparation()
				if errors.Is(err, os.ErrNotExist) {
					return current, nil
				}
				if err != nil {
					return current, err
				}
				if current.ID != prepared.ID {
					prepared.NextAction = "Durable preparation requires exact reconstruction or verified abort; private journal not yet prepared"
					return prepared, nil
				}
			}
			return current, nil
		}, persist: j.persist,
		lock: func(op func() error) error {
			if err := rootRelayOwner(os.Getuid(), os.Geteuid()); err != nil {
				return err
			}
			return withServiceLock(rootRelayPlist, op)
		},
		exclude: func(ctx context.Context, s MaintenanceStatus, op func() error) error {
			if s.Phase != "activated" && s.Phase != "completed" {
				if err := maintenanceMarker(s, false); err != nil {
					return err
				}
			}
			return routerstore.WithSpoolQuiescence(ctx, rootRelayPhysicalSpool, op)
		},
		revalidate: func(ctx context.Context, p MaintenancePlan, original bool) error {
			if err := maintenanceEnabled(ctx); err != nil {
				return err
			}
			if original {
				fresh, err := b.maintenancePreview(ctx, p.LegacyPayload)
				if err != nil || fresh != p {
					return errors.Join(errors.New("legacy maintenance: old image or selected profile changed"), err)
				}
				processes, err := rootRelayRead(ctx, nil, "/bin/ps", "-axo", "pid=,ppid=")
				if err != nil {
					return err
				}
				return rootRelayChildrenClear(processes, p.Original.PID)
			}
			artifacts, err := b.rootRelayArtifacts(ctx)
			if err != nil || !sameRootRelayArtifacts(artifacts, p.Original) {
				return errors.New("legacy maintenance: replacement/plist changed; retain startup hold")
			}
			if hash, err := maintenanceImage(ctx, rootRelayProgram, 0); err != nil || hash != p.ReplacementCDHash {
				return errors.New("legacy maintenance: replacement signature changed")
			}
			root, _, err := maintenanceSpool(p.RootDevice, p.RootInode)
			if err == nil {
				err = root.Close()
			}
			return errors.Join(err, maintenanceReplacementCapability(ctx))
		},
		retire: func(ctx context.Context, p MaintenancePlan) error {
			_, err := native.read(ctx, nil, "/bin/launchctl", "bootout", rootRelayTarget)
			return err
		},
		retirement: maintenanceRetirement,
		preserve:   maintenancePreserve,
		bootstrap: func(ctx context.Context, s MaintenanceStatus) error {
			loaded, err := maintenanceJobListedWith(ctx, native.read)
			if err != nil {
				return err
			}
			if loaded {
				return b.maintenanceHeldSuccessor(ctx, s)
			}
			_, err = native.read(ctx, nil, "/bin/launchctl", "bootstrap", "system", rootRelayPlist)
			return err
		},
		resume: func(ctx context.Context, s MaintenanceStatus) error {
			loaded, err := maintenanceJobListed(ctx)
			if err != nil || loaded {
				return err // Existing selected job: inspect it, never duplicate bootstrap.
			}
			state, err := maintenanceRetirement(ctx, s.Plan)
			if err != nil || state != "retired" {
				return errors.Join(errors.New("legacy maintenance: absent-job recovery is not positively established"), err)
			}
			if err := maintenanceMarker(s, false); err != nil {
				return err
			}
			_, err = native.read(ctx, nil, "/bin/launchctl", "bootstrap", "system", rootRelayPlist)
			return err // One explicit recovery attempt, not a timer/retry loop.
		},
		ready: func(ctx context.Context, s MaintenanceStatus) (maintenanceReady, error) {
			for {
				p, err := b.inspectRootRelay(ctx)
				if err == nil {
					if !sameRootRelayArtifacts(p, s.Plan.Original) || (p.PID == s.Plan.Original.PID && p.Started == s.Plan.Original.Started) {
						return maintenanceReady{}, errors.New("legacy maintenance: unexpected successor/artifacts")
					}
					if hash, err := maintenanceImage(ctx, rootRelayProgram, p.PID); err != nil || hash != s.Plan.ReplacementCDHash {
						return maintenanceReady{}, errors.New("legacy maintenance: successor loaded image not the selected replacement")
					}
					uid, _, err := maintenanceRelayUser()
					if err != nil {
						return maintenanceReady{}, err
					}
					start, err := servicedrain.MaintenanceStart(p.PID)
					if err != nil {
						return maintenanceReady{}, err
					}
					observed, err = servicedrain.ReadMaintenance(ctx, uid, p.PID, start, nil)
					if err == nil {
						if observed.Marker != maintenanceMarkerValue(s) || observed.Preserved != s.Preserved {
							return maintenanceReady{}, errors.New("legacy maintenance: readiness transaction/root/custody mismatch")
						}
						return maintenanceReady{p, observed.Generation, observed.State == "maintenance_ready_not_drained"}, nil
					}
				}
				timer := time.NewTimer(100 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return maintenanceReady{}, errors.Join(errors.New("legacy maintenance: successor readiness unverified; hold retained"), err, ctx.Err())
				case <-timer.C:
				}
			}
		},
		activate: func(ctx context.Context, s MaintenanceStatus, ready maintenanceReady) error {
			root, _, err := maintenanceSpool(s.Plan.RootDevice, s.Plan.RootInode)
			if err != nil {
				return err
			}
			if err := root.Close(); err != nil {
				return err
			}
			uid, _, err := maintenanceRelayUser()
			if err != nil || observed.Generation != ready.Generation || observed.PID != ready.Instance.PID || observed.Marker != maintenanceMarkerValue(s) {
				return errors.New("legacy maintenance: activation observation changed")
			}
			if observed.State == "active_with_old_outcomes_unknown" {
				fresh, err := servicedrain.ReadMaintenance(ctx, uid, observed.PID, observed.Started, nil)
				if err != nil || fresh != observed {
					return errors.New("legacy maintenance: prior activation unverified")
				}
				return nil
			}
			observed, err = servicedrain.ReadMaintenance(ctx, uid, observed.PID, observed.Started, &observed)
			return err
		},
		finish: func(ctx context.Context, s MaintenanceStatus) error {
			uid, _, err := maintenanceRelayUser()
			if err != nil {
				return err
			}
			fresh, err := servicedrain.ReadMaintenance(ctx, uid, observed.PID, observed.Started, nil)
			if err != nil || fresh != observed || fresh.State != "active_with_old_outcomes_unknown" {
				return errors.New("legacy maintenance: final activation readback unavailable; publication hold retained")
			}
			return maintenanceMarker(s, true)
		},
	}
}

func (b nativeServiceBackend) maintenanceAbortVerify(ctx context.Context, s MaintenanceStatus) error {
	if s.Phase != "prepared" && s.Phase != "abort_requested" && s.Phase != "aborted" {
		return errors.New("legacy maintenance: abort is only pre-retirement")
	}
	root, _, err := maintenanceSpool(s.Plan.RootDevice, s.Plan.RootInode)
	if err != nil {
		return err
	}
	if err := root.Close(); err != nil {
		return err
	}
	start, err := servicedrain.MaintenanceStart(s.Plan.Original.PID)
	if err != nil || start != s.Plan.KernelStart {
		return errors.New("legacy maintenance: original runtime not alive; no blind abort or revival")
	}
	if hash, err := maintenanceImage(ctx, s.Plan.LegacyPayload, s.Plan.Original.PID); err != nil || hash != s.Plan.LoadedCDHash {
		return errors.New("legacy maintenance: original loaded image changed; abort refused")
	}
	raw, err := b.relayDependencies().read(ctx, nil, "/bin/launchctl", "print", rootRelayTarget)
	if err != nil {
		return err
	}
	ids := regexp.MustCompile(`(?m)^\s*pid = ([0-9]+)\s*$`).FindAllSubmatch(raw, -1)
	if len(ids) != 1 || string(ids[0][1]) != strconv.Itoa(s.Plan.Original.PID) {
		return errors.New("legacy maintenance: original no longer owns exact selected job")
	}
	return nil
}

// The Security framework call runs only in a bounded, joined canonical probe
// child. No goroutine is abandoned around an uninterruptible C call. This is
// an owned read-only helper process, never a signal to a selected service.
func maintenanceImage(ctx context.Context, path string, pid int) (string, error) {
	if pid != 0 && !maintenanceValidLoadedPID(pid) {
		return "", errors.New("legacy maintenance: invalid selected image PID")
	}
	probe, err := CurrentPackagedMaatEngine()
	if err != nil {
		return "", err
	}
	before, err := maintenancePayload(probe)
	if err != nil {
		return "", err
	}
	if _, err := rootRelayRead(ctx, nil, "/usr/bin/codesign", "--verify", "--strict", "-R", horusSigningRequirement, probe); err != nil {
		return "", err
	}
	raw, err := rootRelayRead(ctx, nil, probe, "router", "relay", "maintenance-image", "--payload", path, "--pid", strconv.Itoa(pid))
	if err != nil {
		return "", err
	}
	identity, err := servicedrain.DecodeMaintenance[MaintenanceImageIdentity](raw)
	if err != nil || identity.PID != pid || len(identity.CDHash) != 40 {
		return "", errors.New("legacy maintenance: invalid bounded image probe response")
	}
	if after, err := maintenancePayload(probe); err != nil || after != before {
		return "", errors.New("legacy maintenance: probe engine changed")
	}
	return identity.CDHash, nil
}

// InspectRootMaintenanceImage is the fixed read-only canonical probe entry.
// It is not a service action or a general executable runner.
func InspectRootMaintenanceImage(path string, pid int) (MaintenanceImageIdentity, error) {
	var result MaintenanceImageIdentity
	if err := rootRelayOwner(os.Getuid(), os.Geteuid()); err != nil {
		return result, err
	}
	if pid != 0 && !maintenanceValidLoadedPID(pid) {
		return result, errors.New("legacy maintenance: invalid selected image PID")
	}
	before, err := maintenancePayload(path)
	if err != nil {
		return result, err
	}
	start := ""
	if pid > 1 {
		start, err = servicedrain.MaintenanceStart(pid)
		if err != nil {
			return result, err
		}
	}
	hash, err := maintenanceSignedImage(path, pid)
	if err != nil {
		return result, err
	}
	if pid > 1 {
		finalStart, err := servicedrain.MaintenanceStart(pid)
		if err != nil || finalStart != start {
			return result, errors.New("legacy maintenance: selected PID changed during image verification")
		}
	}
	if after, err := maintenancePayload(path); err != nil || before != after {
		return result, errors.New("legacy maintenance: inspected payload changed")
	}
	return MaintenanceImageIdentity{PID: pid, CDHash: hash, KernelStart: start}, nil
}

// InspectRootLoadedMaintenanceImage returns only the kernel-reported identity
// for one PID. It performs no candidate-path lookup or lifecycle action. The
// start identity is sampled on both sides of csops to detect PID replacement.
func InspectRootLoadedMaintenanceImage(pid int) (MaintenanceImageIdentity, error) {
	var result MaintenanceImageIdentity
	if err := rootRelayOwner(os.Getuid(), os.Geteuid()); err != nil {
		return result, err
	}
	return maintenanceLoadedImageIdentity(pid, servicedrain.MaintenanceStart, maintenanceKernelCDHash)
}

func maintenanceEnabled(ctx context.Context) error {
	raw, err := rootRelayRead(ctx, nil, "/bin/launchctl", "print-disabled", "system")
	if err != nil {
		return errors.New("legacy maintenance: system disabled intent unavailable; no lifecycle mutation")
	}
	return maintenanceDisabledIntent(raw)
}

func maintenanceDisabledIntent(raw []byte) error {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "disabled services = {" || strings.TrimSpace(lines[len(lines)-1]) != "}" {
		return errors.New("legacy maintenance: unknown system disabled intent")
	}
	pattern := regexp.MustCompile(`^"([^"\\]+)"\s*=>\s*(disabled|enabled|true|false)$`)
	seen := map[string]bool{}
	for _, line := range lines[1 : len(lines)-1] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := pattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(parts) != 3 || len(strings.Fields(parts[1])) != 1 || seen[parts[1]] {
			return errors.New("legacy maintenance: malformed system disabled intent")
		}
		seen[parts[1]] = true
		if parts[1] == rootRelayLabel && (parts[2] == "disabled" || parts[2] == "true") {
			return errors.New("legacy maintenance: selected relay is OS-disabled; preserve intent, no bootstrap or enable")
		}
	}
	return nil
}

// A successful complete native list plus kernel absence establishes retirement.
// Failed launchctl print or elapsed time never does.
func maintenanceRetirement(ctx context.Context, p MaintenancePlan) (string, error) {
	return maintenanceRetirementWith(ctx, p, servicedrain.MaintenanceStart,
		func(pid int) error { return unix.Kill(pid, 0) }, maintenanceJobListed)
}

func maintenanceRetirementWith(ctx context.Context, p MaintenancePlan,
	startIdentity func(int) (string, error), exists func(int) error,
	jobListed func(context.Context) (bool, error),
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "outcome_unknown", err
	}
	if p.Original.PID <= 0 {
		return "outcome_unknown", errors.New("legacy maintenance: positive original PID required")
	}
	start, err := startIdentity(p.Original.PID)
	if err == nil {
		if start == p.KernelStart {
			return "original_alive", nil
		}
		return "pid_reused_or_changed", errors.New("legacy maintenance: original PID identity changed")
	}
	// Darwin may report EIO when kern.proc.pid returns an empty result. Neither
	// EIO nor an identity-reader ESRCH alone establishes process absence.
	if !errors.Is(err, unix.ESRCH) && !errors.Is(err, unix.EIO) {
		return "outcome_unknown", err
	}
	if probeErr := exists(p.Original.PID); !errors.Is(probeErr, unix.ESRCH) {
		if probeErr == nil {
			probeErr = errors.New("legacy maintenance: selected PID remains present but its start identity is unavailable")
		}
		return "outcome_unknown", errors.Join(err, probeErr)
	}
	listed, err := jobListed(ctx)
	if err != nil {
		return "outcome_unknown", err
	}
	if listed {
		return "selected_job_still_loaded_or_changed", nil
	}
	return "retired", nil
}

func maintenanceJobListed(ctx context.Context) (bool, error) {
	return maintenanceJobListedWith(ctx, rootRelayRead)
}

func maintenanceJobListedWith(ctx context.Context, read func(context.Context, []byte, string, ...string) ([]byte, error)) (bool, error) {
	// Scope the native query explicitly. `launchctl list` inherits a caller's
	// bootstrap namespace and cannot establish absence from the system domain.
	raw, err := read(ctx, nil, "/bin/launchctl", "print", "system")
	if err != nil {
		return false, err
	}
	return maintenanceSystemJobListed(raw)
}

func (b nativeServiceBackend) maintenanceHeldSuccessor(ctx context.Context, s MaintenanceStatus) error {
	p, err := b.inspectRootRelay(ctx)
	if err != nil {
		return err
	}
	if !sameRootRelayArtifacts(p, s.Plan.Original) || p.PID == s.Plan.Original.PID {
		return errors.New("legacy maintenance: prepared recovery selected an unexpected process/artifact")
	}
	if hash, err := maintenanceImage(ctx, rootRelayProgram, p.PID); err != nil || hash != s.Plan.ReplacementCDHash {
		return errors.New("legacy maintenance: held successor loaded image is not approved replacement")
	}
	uid, _, err := maintenanceRelayUser()
	if err != nil {
		return err
	}
	start, err := servicedrain.MaintenanceStart(p.PID)
	if err != nil {
		return err
	}
	ready, err := servicedrain.ReadMaintenance(ctx, uid, p.PID, start, nil)
	if err != nil || ready.State != "maintenance_ready_not_drained" || ready.Marker != maintenanceMarkerValue(s) {
		return errors.Join(errors.New("legacy maintenance: prepared successor is not this transaction's held producer"), err)
	}
	return nil
}

func maintenanceSystemJobListed(raw []byte) (bool, error) {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "system = {" || strings.TrimSpace(lines[len(lines)-1]) != "}" {
		return false, errors.New("legacy maintenance: native system job enumeration unknown")
	}
	found, services, inside, closed := false, false, false, false
	seenLabels := make(map[string]bool)
	depth := 1
	for _, line := range lines[1 : len(lines)-1] {
		line = strings.TrimSpace(line)
		if !inside {
			if depth == 1 && line == "services = {" {
				if services {
					return false, errors.New("legacy maintenance: duplicate system services inventory")
				}
				services, inside = true, true
				continue
			}
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth < 1 {
				return false, errors.New("legacy maintenance: incomplete system domain inventory")
			}
			continue
		}
		if line == "}" {
			inside, closed = false, true
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return false, errors.New("legacy maintenance: incomplete system job enumeration")
		}
		if fields[0] != "-" {
			if pid, err := strconv.Atoi(fields[0]); err != nil || pid < 0 {
				return false, errors.New("legacy maintenance: malformed job PID")
			}
		}
		if fields[1] != "-" && fields[1] != "(pe)" {
			if _, err := strconv.Atoi(fields[1]); err != nil {
				return false, errors.New("legacy maintenance: malformed job status")
			}
		}
		if seenLabels[fields[2]] {
			return false, errors.New("legacy maintenance: ambiguous system job label")
		}
		seenLabels[fields[2]] = true
		if fields[2] == rootRelayLabel {
			if found {
				return false, errors.New("legacy maintenance: ambiguous selected job")
			}
			found = true
		}
	}
	if !services || !closed || inside || depth != 1 {
		return false, errors.New("legacy maintenance: incomplete selected system services inventory")
	}
	return found, nil
}
