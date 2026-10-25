package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const AppUpdateManifestSchema = "sirsi.maat.app-update-action.v1"

var appUpdateVersionRE = regexp.MustCompile(`^0\.[0-9]+\.[0-9]+$`)

// AppUpdateManifest binds the one supported whole-app update to the exact
// staged, signed PKG and the canonical app/helper target. Its dependencies are
// a closed list, not caller supplied installer instructions.
type AppUpdateManifest struct {
	Schema                        string    `json:"schema"`
	StoreUUID                     string    `json:"store_uuid"`
	UID                           uint32    `json:"uid"`
	Release                       string    `json:"release"`
	PackageName                   string    `json:"package_name"`
	PackageURL                    string    `json:"package_url"`
	PackagePath                   string    `json:"package_path"`
	PackageSHA256                 string    `json:"package_sha256"`
	PackageDevice                 uint64    `json:"package_device"`
	PackageInode                  uint64    `json:"package_inode"`
	PackageBytes                  int64     `json:"package_bytes"`
	InstallerIdentity             string    `json:"installer_identity"`
	Notarization                  string    `json:"notarization"`
	TargetBundleID                string    `json:"target_bundle_id"`
	TargetAppPath                 string    `json:"target_app_path"`
	TargetHelperPath              string    `json:"target_helper_path"`
	ExpectedInstalledHelperSHA256 string    `json:"expected_installed_helper_sha256"`
	CurrentRelease                string    `json:"current_release"`
	CurrentHelperSHA256           string    `json:"current_helper_sha256"`
	Dependencies                  []string  `json:"dependencies"`
	ObservedAtUTC                 time.Time `json:"observed_at_utc"`
}

func (m AppUpdateManifest) validate(fresh bool) error {
	if m.Schema != AppUpdateManifestSchema || !validUUID(m.StoreUUID) || m.UID != uint32(os.Geteuid()) ||
		!appUpdateVersionRE.MatchString(m.Release) || !appUpdateVersionRE.MatchString(m.CurrentRelease) ||
		m.Release == m.CurrentRelease ||
		m.PackageName != "SirsiPantheon-"+m.Release+"-arm64.pkg" || m.PackageURL != "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v"+m.Release+"/"+m.PackageName || !filepathIsCanonicalAbsolute(m.PackagePath) ||
		filepath.Base(m.PackagePath) != m.PackageName || !strings.HasPrefix(filepath.Base(filepath.Dir(m.PackagePath)), "sirsi-maintenance-installer-") ||
		!validDigest(m.PackageSHA256) || m.PackageDevice == 0 || m.PackageInode == 0 || m.PackageBytes <= 0 || m.PackageBytes > 1<<30 ||
		m.InstallerIdentity != "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)" ||
		m.Notarization != "Notarized Developer ID" || m.TargetBundleID != "ai.sirsi.pantheon" ||
		m.TargetAppPath != "/Applications/Pantheon.app" || m.TargetHelperPath != "/usr/local/libexec/sirsi-pantheon/sirsi" ||
		!validDigest(m.ExpectedInstalledHelperSHA256) || !validDigest(m.CurrentHelperSHA256) ||
		len(m.Dependencies) != 2 || m.Dependencies[0] != m.TargetAppPath || m.Dependencies[1] != m.TargetHelperPath ||
		m.ObservedAtUTC.IsZero() || m.ObservedAtUTC.Location() != time.UTC {
		return errors.New("maat memory: invalid whole-app update manifest")
	}
	if fresh && (time.Since(m.ObservedAtUTC) > 2*time.Minute || time.Until(m.ObservedAtUTC) > 10*time.Second) {
		return errors.New("maat memory: whole-app update observation is stale")
	}
	return nil
}

func (m AppUpdateManifest) Digest() (string, error) {
	if err := m.validate(false); err != nil {
		return "", err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("maat memory: encode whole-app update manifest: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

type AppUpdateAttempt struct {
	Schema                 string            `json:"schema"`
	ID                     string            `json:"id"`
	Manifest               AppUpdateManifest `json:"manifest"`
	ActionManifestSHA256   string            `json:"action_manifest_sha256"`
	PreflightReceipt       PreflightReceipt  `json:"preflight_receipt"`
	PreflightReceiptSHA256 string            `json:"preflight_receipt_sha256"`
	RegistrySnapshotSHA256 string            `json:"registry_snapshot_sha256"`
	StartedAtUTC           time.Time         `json:"started_at_utc"`
}

type AppUpdateObservation struct {
	Release                 string    `json:"release"`
	BundleID                string    `json:"bundle_id"`
	AppPath                 string    `json:"app_path"`
	HelperPath              string    `json:"helper_path"`
	HelperSHA256            string    `json:"helper_sha256"`
	AppSignatureVerified    bool      `json:"app_signature_verified"`
	HelperSignatureVerified bool      `json:"helper_signature_verified"`
	DependenciesPresent     bool      `json:"dependencies_present"`
	ObservedAtUTC           time.Time `json:"observed_at_utc"`
}

type AppUpdateOutcome struct {
	Schema        string               `json:"schema"`
	AttemptID     string               `json:"attempt_id"`
	Determination string               `json:"determination"`
	Observation   AppUpdateObservation `json:"observation"`
	RecordedAtUTC time.Time            `json:"recorded_at_utc"`
}

var ErrNoIncompleteAppUpdateAttempt = errors.New("maat memory: no incomplete whole-app update attempt")

func (s *Store) PreflightAppUpdate(manifest AppUpdateManifest) (PreflightReceipt, error) {
	if s == nil || manifest.StoreUUID != s.storeUUID {
		return PreflightReceipt{}, errors.New("maat memory: whole-app update is not bound to this initialized store")
	}
	if err := manifest.validate(true); err != nil {
		return PreflightReceipt{}, err
	}
	digest, err := manifest.Digest()
	if err != nil {
		return PreflightReceipt{}, err
	}
	return s.preflightBound(Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-whole-app"}, digest, resolveMemoryGuard)
}

// BeginAppUpdateAttempt makes the complete reviewed receipt and exact PKG
// identity durable under the existing Store lock before Installer handoff.
// The caller must not hold or re-enter this Store from an external callback.
func (s *Store) BeginAppUpdateAttempt(manifest AppUpdateManifest, receipt PreflightReceipt) (AppUpdateAttempt, error) {
	if s == nil || s.attemptsFD < 0 || manifest.StoreUUID != s.storeUUID || manifest.validate(true) != nil ||
		receipt.validate() != nil || receipt.Decision != PreflightPass {
		return AppUpdateAttempt{}, errors.New("maat memory: whole-app update requires a passing exact-scope preflight")
	}
	digest, err := manifest.Digest()
	if err != nil || receipt.Action != (Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-whole-app"}) || receipt.ActionManifestSHA256 != digest {
		return AppUpdateAttempt{}, errors.New("maat memory: preflight receipt does not bind the exact whole-app update")
	}
	receiptDigest, err := receipt.Digest()
	if err != nil {
		return AppUpdateAttempt{}, err
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return AppUpdateAttempt{}, err
	}
	defer s.unlock()
	current, err := s.preflightBoundLocked(receipt.Action, digest, resolveMemoryGuard)
	if err != nil || current.Decision != PreflightPass || current.ActionManifestSHA256 != receipt.ActionManifestSHA256 || current.RegistrySnapshotSHA256 != receipt.RegistrySnapshotSHA256 {
		return AppUpdateAttempt{}, errors.New("maat memory: whole-app update preflight is stale or no longer passing")
	}
	if _, err := s.pendingLivenessWatchAttemptLocked(); err != nil && !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		return AppUpdateAttempt{}, fmt.Errorf("maat memory: shared attempt history is unverifiable: %w", err)
	}
	if _, err := s.pendingAppUpdateAttemptLocked(); err == nil {
		return AppUpdateAttempt{}, errors.New("maat memory: an incomplete whole-app update handoff must be inspected before another update")
	} else if !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		return AppUpdateAttempt{}, err
	}
	id, err := newStoreUUID()
	if err != nil {
		return AppUpdateAttempt{}, err
	}
	attempt := AppUpdateAttempt{Schema: MemorySchema, ID: id, Manifest: manifest, ActionManifestSHA256: digest,
		PreflightReceipt: receipt, PreflightReceiptSHA256: receiptDigest, RegistrySnapshotSHA256: receipt.RegistrySnapshotSHA256,
		StartedAtUTC: time.Now().UTC()}
	data, err := json.Marshal(attempt)
	if err != nil {
		return AppUpdateAttempt{}, err
	}
	if err := s.createOrVerify("attempts", s.attemptsFD, id+".app-update-started.json", data); err != nil {
		return AppUpdateAttempt{}, fmt.Errorf("maat memory: persist durable whole-app update intent: %w", err)
	}
	return attempt, nil
}

func (s *Store) PendingAppUpdateAttempt() (AppUpdateAttempt, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return AppUpdateAttempt{}, err
	}
	defer s.unlock()
	return s.pendingAppUpdateAttemptLocked()
}

func (s *Store) pendingAppUpdateAttemptLocked() (AppUpdateAttempt, error) {
	if _, err := s.verifyWriteJournal(); err != nil {
		return AppUpdateAttempt{}, fmt.Errorf("maat memory: whole-app update history is unverifiable: %w", err)
	}
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return AppUpdateAttempt{}, err
	}
	started := map[string]AppUpdateAttempt{}
	completed := map[string]AppUpdateOutcome{}
	for _, name := range names {
		switch {
		case strings.HasSuffix(name, ".app-update-started.json"):
			id := strings.TrimSuffix(name, ".app-update-started.json")
			if !validUUID(id) {
				return AppUpdateAttempt{}, errors.New("maat memory: malformed whole-app update attempt namespace")
			}
			data, err := readExact(s.attemptsFD, name)
			if err != nil {
				return AppUpdateAttempt{}, err
			}
			var attempt AppUpdateAttempt
			if json.Unmarshal(data, &attempt) != nil || validateAppUpdateAttempt(attempt, id, s.storeUUID) != nil {
				return AppUpdateAttempt{}, errors.New("maat memory: invalid whole-app update attempt record")
			}
			started[id] = attempt
		case strings.HasSuffix(name, ".app-update-outcome.json"):
			id := strings.TrimSuffix(name, ".app-update-outcome.json")
			if !validUUID(id) {
				return AppUpdateAttempt{}, errors.New("maat memory: malformed whole-app update outcome namespace")
			}
			data, err := readExact(s.attemptsFD, name)
			if err != nil {
				return AppUpdateAttempt{}, err
			}
			var outcome AppUpdateOutcome
			if json.Unmarshal(data, &outcome) != nil || outcome.Schema != MemorySchema || outcome.AttemptID != id || outcome.Determination != "installed" || outcome.RecordedAtUTC.IsZero() {
				return AppUpdateAttempt{}, errors.New("maat memory: invalid whole-app update outcome record")
			}
			completed[id] = outcome
		case strings.HasSuffix(name, ".started.json"), strings.HasSuffix(name, ".outcome.json"),
			strings.HasSuffix(name, ".projection.json"), strings.HasSuffix(name, ".projection-receipt.json"):
			// The liveness-watch owner validates its own closed record shapes.
		default:
			return AppUpdateAttempt{}, errors.New("maat memory: unrecognized attempt namespace entry")
		}
	}
	var pending *AppUpdateAttempt
	for id, attempt := range started {
		if outcome, ok := completed[id]; ok {
			if validateAppUpdateOutcome(outcome, attempt) != nil {
				return AppUpdateAttempt{}, errors.New("maat memory: whole-app update outcome does not bind its attempt")
			}
			continue
		}
		if pending != nil {
			return AppUpdateAttempt{}, errors.New("maat memory: multiple incomplete whole-app updates require inspection")
		}
		pending = &attempt
	}
	for id := range completed {
		if _, ok := started[id]; !ok {
			return AppUpdateAttempt{}, errors.New("maat memory: whole-app update outcome has no matching attempt")
		}
	}
	if pending == nil {
		return AppUpdateAttempt{}, ErrNoIncompleteAppUpdateAttempt
	}
	return *pending, nil
}

func (s *Store) RecordAppUpdateInstalled(attempt AppUpdateAttempt, observation AppUpdateObservation) (AppUpdateOutcome, error) {
	if s == nil || s.attemptsFD < 0 || validateAppUpdateAttempt(attempt, attempt.ID, s.storeUUID) != nil {
		return AppUpdateOutcome{}, errors.New("maat memory: invalid whole-app update attempt reference")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return AppUpdateOutcome{}, err
	}
	defer s.unlock()
	pending, err := s.pendingAppUpdateAttemptLocked()
	if err != nil || pending.ID != attempt.ID {
		return AppUpdateOutcome{}, errors.New("maat memory: whole-app update is not the unique incomplete attempt")
	}
	if !appUpdateObservationMatches(attempt.Manifest, observation) {
		return AppUpdateOutcome{}, errors.New("maat memory: installed target, signature, dependency, or version readback does not match the reviewed update; handoff remains unresolved")
	}
	if !observation.ObservedAtUTC.After(attempt.StartedAtUTC) || time.Until(observation.ObservedAtUTC) < -10*time.Second {
		return AppUpdateOutcome{}, errors.New("maat memory: installed update readback is not a fresh post-handoff observation")
	}
	outcome := AppUpdateOutcome{Schema: MemorySchema, AttemptID: attempt.ID, Determination: "installed", Observation: observation, RecordedAtUTC: time.Now().UTC()}
	data, err := json.Marshal(outcome)
	if err != nil {
		return AppUpdateOutcome{}, err
	}
	if err := s.createOrVerify("attempts", s.attemptsFD, attempt.ID+".app-update-outcome.json", data); err != nil {
		return AppUpdateOutcome{}, fmt.Errorf("maat memory: installed update readback remains unresolved: %w", err)
	}
	return outcome, nil
}

func validateAppUpdateAttempt(attempt AppUpdateAttempt, id, storeUUID string) error {
	digest, err := attempt.Manifest.Digest()
	receiptDigest, receiptErr := attempt.PreflightReceipt.Digest()
	if err != nil || receiptErr != nil || attempt.Schema != MemorySchema || attempt.ID != id || attempt.Manifest.StoreUUID != storeUUID ||
		digest != attempt.ActionManifestSHA256 || attempt.PreflightReceipt.Decision != PreflightPass ||
		attempt.PreflightReceipt.Action != (Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-whole-app"}) ||
		attempt.PreflightReceipt.ActionManifestSHA256 != digest || receiptDigest != attempt.PreflightReceiptSHA256 ||
		attempt.RegistrySnapshotSHA256 != attempt.PreflightReceipt.RegistrySnapshotSHA256 || attempt.StartedAtUTC.IsZero() || attempt.StartedAtUTC.Location() != time.UTC {
		return errors.New("maat memory: invalid whole-app update attempt identity")
	}
	return nil
}

func validateAppUpdateOutcome(outcome AppUpdateOutcome, attempt AppUpdateAttempt) error {
	if outcome.Schema != MemorySchema || outcome.AttemptID != attempt.ID || outcome.Determination != "installed" || outcome.RecordedAtUTC.IsZero() ||
		outcome.RecordedAtUTC.Location() != time.UTC || !outcome.RecordedAtUTC.After(outcome.Observation.ObservedAtUTC) ||
		!outcome.Observation.ObservedAtUTC.After(attempt.StartedAtUTC) || !appUpdateObservationMatches(attempt.Manifest, outcome.Observation) {
		return errors.New("maat memory: invalid whole-app update outcome identity")
	}
	return nil
}

func appUpdateObservationMatches(manifest AppUpdateManifest, observation AppUpdateObservation) bool {
	return observation.Release == manifest.Release && observation.BundleID == manifest.TargetBundleID &&
		observation.AppPath == manifest.TargetAppPath && observation.HelperPath == manifest.TargetHelperPath &&
		observation.HelperSHA256 == manifest.ExpectedInstalledHelperSHA256 && observation.AppSignatureVerified &&
		observation.HelperSignatureVerified && observation.DependenciesPresent && !observation.ObservedAtUTC.IsZero()
}
