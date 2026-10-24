package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"golang.org/x/sys/unix"
)

var (
	maatRepairDefaultStoreInitialize = maat.InitializeDefaultStore
	maatRepairDefaultStoreInspect    = maat.InspectDefaultStore
	maatRepairBuildLivenessManifest  = buildLivenessWatchManifest
	maatRepairObserveLiveness        = observeLivenessWatch
	maatRepairInstallLiveness        = installLivenessWatchManifest
	maatRepairRevalidateExecutable   = revalidateLivenessExecutable
	maatRepairCurrentLocalEngine     = setup.CurrentPackagedMaatEngine
	maatRepairVerifyLocalEngine      = setup.VerifyPackagedMaatEngine
	maatRepairUserHomeDir            = os.UserHomeDir
	maatRepairInspectLocalWatch      = liveness.InspectLocalWatch
	maatRepairInstallLocalWatch      = liveness.InstallLocal
	maatRepairReadLivenessFinding    = readActionableLivenessFinding
)

type maatLivenessStore interface {
	Close() error
	PreflightLivenessWatch(maat.LivenessWatchManifest) (maat.PreflightReceipt, error)
	BeginLivenessWatchAttempt(maat.LivenessWatchManifest, maat.PreflightReceipt) (maat.LivenessWatchAttempt, error)
	PendingLivenessWatchAttempt() (maat.LivenessWatchAttempt, error)
	RecordLivenessWatchOutcome(maat.LivenessWatchAttempt, maat.LivenessWatchObservation, string) (maat.LivenessWatchOutcome, error)
	BeginLivenessWatchProjection(maat.LivenessWatchAttempt, maat.LivenessWatchOutcome, maat.Decision) (maat.LivenessWatchProjectionDebt, error)
	PersistLivenessWatchProjectionOutcome(maat.LivenessWatchProjectionDebt) error
	PendingLivenessWatchProjectionDebts() ([]maat.LivenessWatchProjectionDebt, error)
	MarkLivenessWatchProjectionReadBack(maat.LivenessWatchProjectionDebt) error
}

type idempotentMaatJournal interface {
	AppendIdempotent(maat.Decision) error
}

var maatRepairOpenDefaultStore = func() (maatLivenessStore, error) { return maat.OpenDefaultStore() }

type livenessFindingIdentity struct {
	Check    string                   `json:"check"`
	Severity guard.DiagnosticSeverity `json:"severity"`
	Message  string                   `json:"message"`
	Detail   string                   `json:"detail"`
}

func buildLivenessWatchManifest(finding guard.DiagnosticFinding, storeUUID string) (maat.LivenessWatchManifest, error) {
	uid, euid := os.Getuid(), os.Geteuid()
	if uid != euid {
		return maat.LivenessWatchManifest{}, errors.New("Ma'at liveness-watch requires matching real and effective user identities")
	}
	executable, err := maatRepairCurrentLocalEngine()
	if err != nil {
		return maat.LivenessWatchManifest{}, fmt.Errorf("resolve canonical running local Sirsi engine: %w", err)
	}
	if err := maatRepairVerifyLocalEngine(executable); err != nil {
		return maat.LivenessWatchManifest{}, fmt.Errorf("verify canonical installed Sirsi engine: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return maat.LivenessWatchManifest{}, err
	}
	executable = filepath.Clean(executable)
	fd, err := unix.Open(executable, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return maat.LivenessWatchManifest{}, fmt.Errorf("open canonical Sirsi executable: %w", err)
	}
	file := os.NewFile(uintptr(fd), executable)
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return maat.LivenessWatchManifest{}, err
	}
	var identity unix.Stat_t
	if err := unix.Fstat(fd, &identity); err != nil {
		return maat.LivenessWatchManifest{}, err
	}
	if !stat.Mode().IsRegular() || !maat.LivenessExecutableIdentityAllowed(uint32(uid), identity.Uid, uint32(identity.Mode)) || identity.Nlink != 1 {
		return maat.LivenessWatchManifest{}, errors.New("canonical Sirsi executable must be actor- or root-owned, singly linked, executable, and not group/other writable")
	}
	executableHash := sha256.New()
	if _, err := io.Copy(executableHash, file); err != nil {
		return maat.LivenessWatchManifest{}, fmt.Errorf("hash canonical Sirsi executable: %w", err)
	}
	workDirectory, err := maatRepairUserHomeDir()
	if err != nil {
		return maat.LivenessWatchManifest{}, fmt.Errorf("resolve local liveness-watch HOME: %w", err)
	}
	if !filepath.IsAbs(workDirectory) || filepath.Clean(workDirectory) != workDirectory {
		return maat.LivenessWatchManifest{}, errors.New("local liveness-watch HOME must be canonical and absolute")
	}
	homeFD, err := unix.Open(workDirectory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return maat.LivenessWatchManifest{}, fmt.Errorf("inspect local liveness-watch HOME: %w", err)
	}
	defer unix.Close(homeFD)
	var homeIdentity unix.Stat_t
	if err := unix.Fstat(homeFD, &homeIdentity); err != nil {
		return maat.LivenessWatchManifest{}, fmt.Errorf("stat local liveness-watch HOME: %w", err)
	}
	if homeIdentity.Mode&unix.S_IFMT != unix.S_IFDIR || homeIdentity.Uid != uint32(uid) {
		return maat.LivenessWatchManifest{}, errors.New("local liveness-watch HOME must be an existing nonsymlink directory owned by the acting user")
	}
	plistPath := filepath.Join(workDirectory, "Library", "LaunchAgents", liveness.Label+".plist")
	plistDigest := sha256.Sum256([]byte(liveness.LocalPlistContent(executable, workDirectory)))
	findingBytes, err := json.Marshal(livenessFindingIdentity{Check: finding.Check, Severity: finding.Severity, Message: finding.Message, Detail: finding.Detail})
	if err != nil {
		return maat.LivenessWatchManifest{}, err
	}
	findingDigest := sha256.Sum256(findingBytes)
	manifest := maat.LivenessWatchManifest{
		Schema: maat.LivenessWatchManifestSchema, UID: uint32(uid), Domain: "gui/" + strconv.Itoa(uid),
		ExecutableOwnerUID: identity.Uid, ExecutableOwnerBound: true,
		ExecutablePath: executable, ExecutableDevice: uint64(identity.Dev), ExecutableInode: uint64(identity.Ino),
		ExecutableMode: uint32(identity.Mode), ExecutableSize: stat.Size(), ExecutableSHA256: hex.EncodeToString(executableHash.Sum(nil)),
		WorkDirectory: workDirectory, ManagedLabel: liveness.Label, TargetPlistPath: plistPath,
		ExpectedPlistSHA256: hex.EncodeToString(plistDigest[:]), FindingCheck: finding.Check,
		FindingSeverity: int(finding.Severity), FindingEvidenceSHA256: hex.EncodeToString(findingDigest[:]),
		FindingObservedAtUTC: time.Now().UTC(), StoreUUID: storeUUID,
	}
	return manifest, manifest.ValidateFresh()
}

// readActionableLivenessFinding is deliberately narrower than the general
// doctor. It reads only the bounded local-watch contract needed by this closed
// action; errors and operator-disabled state are refusals, never healthy data.
func readActionableLivenessFinding() (guard.DiagnosticFinding, bool, error) {
	engine, err := maatRepairCurrentLocalEngine()
	if err != nil {
		return guard.DiagnosticFinding{}, false, fmt.Errorf("resolve canonical installed Sirsi engine: %w", err)
	}
	if err := maatRepairVerifyLocalEngine(engine); err != nil {
		return guard.DiagnosticFinding{}, false, fmt.Errorf("verify canonical installed Sirsi engine: %w", err)
	}
	home, err := maatRepairUserHomeDir()
	if err != nil {
		return guard.DiagnosticFinding{}, false, fmt.Errorf("resolve local liveness-watch HOME: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	observation, err := maatRepairInspectLocalWatch(ctx, engine, home, nil)
	if err != nil {
		return guard.DiagnosticFinding{}, false, fmt.Errorf("bounded local liveness-watch preflight unavailable: %w", err)
	}
	if observation.Check != "local-watch" || observation.State != "observed" {
		return guard.DiagnosticFinding{}, false, errors.New("bounded local liveness-watch preflight is unknown or operator-disabled")
	}
	if !observation.Concern {
		return guard.DiagnosticFinding{}, false, nil
	}
	return guard.DiagnosticFinding{
		Check: maatRepairLivenessWatchCheck, Severity: guard.SeverityWarn,
		Message: observation.Detail, Detail: "bounded local watch metadata finding",
	}, true, nil
}

func sameLivenessWatchConsent(a, b maat.LivenessWatchManifest) bool {
	a.FindingObservedAtUTC, b.FindingObservedAtUTC = time.Time{}, time.Time{}
	return a == b
}

func installLivenessWatchManifest(manifest maat.LivenessWatchManifest) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("Ma'at liveness-watch installation is macOS only")
	}
	if err := revalidateLivenessExecutable(manifest); err != nil {
		return "", err
	}
	return installLivenessWatchWithLocalInstaller(manifest, maatRepairInstallLocalWatch)
}

func installLivenessWatchWithLocalInstaller(
	manifest maat.LivenessWatchManifest,
	install func(context.Context, string, string, liveness.LocalCommand) (string, error),
) (string, error) {
	if install == nil {
		return "", errors.New("local liveness-watch installer is unavailable")
	}
	return install(context.Background(), manifest.ExecutablePath, manifest.WorkDirectory, nil)
}

func revalidateLivenessExecutable(manifest maat.LivenessWatchManifest) error {
	fd, err := unix.Open(manifest.ExecutablePath, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), manifest.ExecutablePath)
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	var identity unix.Stat_t
	if err := unix.Fstat(fd, &identity); err != nil {
		return err
	}
	executableOwnerUID := manifest.UID
	if manifest.ExecutableOwnerBound {
		executableOwnerUID = manifest.ExecutableOwnerUID
	}
	if !stat.Mode().IsRegular() || identity.Uid != executableOwnerUID || !maat.LivenessExecutableIdentityAllowed(manifest.UID, identity.Uid, uint32(identity.Mode)) || uint64(identity.Dev) != manifest.ExecutableDevice || identity.Ino != manifest.ExecutableInode || uint32(identity.Mode) != manifest.ExecutableMode || stat.Size() != manifest.ExecutableSize || identity.Nlink != 1 {
		return errors.New("canonical Sirsi executable identity changed after review; no installation was attempted")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != manifest.ExecutableSHA256 {
		return errors.New("canonical Sirsi executable bytes changed after review; no installation was attempted")
	}
	return nil
}

func observeLivenessWatch(manifest maat.LivenessWatchManifest) (maat.LivenessWatchObservation, error) {
	if runtime.GOOS != "darwin" {
		return maat.LivenessWatchObservation{}, errors.New("Ma'at liveness-watch observation is macOS only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return observeLivenessWatchWithLocalInspector(ctx, manifest, maatRepairInspectLocalWatch)
}

func observeLivenessWatchWithLocalInspector(
	ctx context.Context,
	manifest maat.LivenessWatchManifest,
	inspect func(context.Context, string, string, liveness.LocalCommand) (liveness.LocalObservation, error),
) (maat.LivenessWatchObservation, error) {
	if inspect == nil {
		return maat.LivenessWatchObservation{}, errors.New("local liveness-watch inspector is unavailable")
	}
	path := filepath.Join(manifest.WorkDirectory, "Library", "LaunchAgents", liveness.Label+".plist")
	expected := sha256.Sum256([]byte(liveness.LocalPlistContent(manifest.ExecutablePath, manifest.WorkDirectory)))
	if !filepath.IsAbs(manifest.WorkDirectory) || filepath.Clean(manifest.WorkDirectory) != manifest.WorkDirectory ||
		manifest.TargetPlistPath != path || hex.EncodeToString(expected[:]) != manifest.ExpectedPlistSHA256 {
		return maat.LivenessWatchObservation{}, errors.New("reviewed local watch identity does not match canonical engine and HOME")
	}
	local, err := inspect(ctx, manifest.ExecutablePath, manifest.WorkDirectory, nil)
	if err != nil {
		return maat.LivenessWatchObservation{}, fmt.Errorf("inspect exact local metadata watch: %w", err)
	}
	if local.Check != "local-watch" || local.State != "observed" || local.Concern {
		return maat.LivenessWatchObservation{}, fmt.Errorf("local metadata watch is not verified: %s", local.Detail)
	}
	if local.ConfigurationSHA256 != manifest.ExpectedPlistSHA256 {
		return maat.LivenessWatchObservation{}, errors.New("local metadata watch plist bytes differ from the reviewed manifest")
	}
	return maat.LivenessWatchObservation{
		UID: manifest.UID, Domain: manifest.Domain, Label: manifest.ManagedLabel,
		PlistPath: path, PlistSHA256: local.ConfigurationSHA256, LaunchdLoaded: true,
	}, nil
}

func runMaatLivenessWatchAction() error {
	modes := 0
	for _, enabled := range []bool{maatRepairInitializeFailureMemory, maatRepairInspectFailureMemory, maatRepairReobserveFailureMemory} {
		if enabled {
			modes++
		}
	}
	if modes > 1 {
		return errors.New("Ma'at liveness-watch accepts only one failure-memory action at a time")
	}
	if maatRepairInspectFailureMemory {
		return inspectMaatLivenessMemory()
	}
	if maatRepairInitializeFailureMemory {
		return initializeMaatLivenessMemory()
	}
	if maatRepairReobserveFailureMemory {
		return reobserveMaatLivenessAttempt()
	}
	return installMaatLivenessWatch()
}

func inspectMaatLivenessMemory() error {
	status := maatRepairDefaultStoreInspect()
	type inspectResult struct {
		maat.DefaultStoreStatus
		IncompleteAttempt bool `json:"incomplete_attempt"`
		ProjectionDebt    bool `json:"projection_debt"`
	}
	result := inspectResult{DefaultStoreStatus: status}
	if status.State == maat.DefaultStoreReady {
		store, err := maatRepairOpenDefaultStore()
		if err != nil {
			return fmt.Errorf("inspect initialized Ma'at failure memory: %w", err)
		}
		defer store.Close()
		_, err = store.PendingLivenessWatchAttempt()
		if err == nil {
			result.IncompleteAttempt = true
		} else if !errors.Is(err, maat.ErrNoIncompleteLivenessWatchAttempt) {
			result.State = maat.DefaultStoreIncomplete
			result.Detail = "liveness-watch attempt history is unverifiable; inspect only, with no repair or replay"
		}
		debts, debtErr := store.PendingLivenessWatchProjectionDebts()
		if debtErr != nil {
			result.State = maat.DefaultStoreIncomplete
			result.Detail = "liveness-watch journal projection is unverifiable; inspect only, with no repair or replay"
		} else {
			result.ProjectionDebt = len(debts) != 0
		}
	}
	if JsonOutput || maatJSON {
		return emitJSON(result)
	}
	fmt.Printf("Ma'at failure memory: %s\n", status.State)
	if status.Detail != "" {
		fmt.Println(status.Detail)
	}
	if result.IncompleteAttempt {
		fmt.Println("An earlier liveness-watch attempt is incomplete. Inspect and re-observe it; installation will not replay automatically.")
	}
	if result.ProjectionDebt {
		fmt.Println("A durable liveness-watch outcome still needs Casebook projection. Re-observe and reconcile it; installation will not replay.")
	}
	return nil
}

func initializeMaatLivenessMemory() error {
	if !maatRepairConfirm {
		return errors.New("Ma'at failure-memory initialization is explicit; review the first-use action and rerun with --confirm")
	}
	status := maatRepairDefaultStoreInspect()
	if status.State != maat.DefaultStoreUninitialized {
		return fmt.Errorf("Ma'at will initialize only genuine first use; current store is %s: %s", status.State, status.Detail)
	}
	journal, err := newMaatDecisionJournal()
	if err != nil {
		return fmt.Errorf("open Ma'at journal before first-use initialization: %w", err)
	}
	if _, err := journal.Recent(1); err != nil {
		return fmt.Errorf("validate Ma'at journal before first-use initialization: %w", err)
	}
	storeUUID, err := maatRepairDefaultStoreInitialize()
	if err != nil {
		return err
	}
	decision := maat.Decision{
		Kind: "failure memory initialization", Requester: "sirsi maat repair liveness-watch",
		Resource: "per-user local failure memory", Assessed: "explicit first-use initialization",
		Determination: "initialized", Why: "A private owner-only genesis was created with no incidents imported.",
		Evidence: "maat-failure-memory-store:" + storeUUID,
	}
	if err := journal.Append(decision); err != nil {
		return fmt.Errorf("failure memory initialized but Ma'at journal outcome is incomplete; inspect locally before any liveness repair: %w", err)
	}
	if JsonOutput || maatJSON {
		return emitJSON(map[string]any{"status": "initialized", "store_uuid": storeUUID, "imported_incidents": false})
	}
	fmt.Println("Ma'at initialized private local failure memory. No prior incidents were imported. Review the liveness-watch finding again before repair.")
	return nil
}

func reobserveMaatLivenessAttempt() error {
	if !maatRepairConfirm {
		return errors.New("re-observing an incomplete Ma'at action requires explicit --confirm; installation will not replay")
	}
	journal, err := newMaatDecisionJournal()
	if err != nil {
		return fmt.Errorf("open Ma'at journal before re-observation: %w", err)
	}
	if _, err := journal.Recent(1); err != nil {
		return fmt.Errorf("validate Ma'at journal before re-observation: %w", err)
	}
	store, err := maatRepairOpenDefaultStore()
	if err != nil {
		return err
	}
	defer store.Close()
	debts, err := store.PendingLivenessWatchProjectionDebts()
	if err != nil {
		return fmt.Errorf("inspect liveness-watch projection debt: %w", err)
	}
	if len(debts) > 1 {
		return errors.New("multiple liveness-watch projection debts require individual review; no action was taken")
	}
	if len(debts) == 1 {
		return reconcileMaatLivenessProjection(store, journal, debts[0])
	}
	attempt, err := store.PendingLivenessWatchAttempt()
	if err != nil {
		return err
	}
	observation, observationErr := maatRepairObserveLiveness(attempt.Manifest)
	observationDetail := "exact managed watch remains unproven after the interrupted attempt"
	if observationErr != nil {
		observationDetail = observationErr.Error()
	}
	var installErr string
	if observationErr != nil {
		installErr = "re-observation: " + observationErr.Error()
	}
	outcome, err := maat.NewLivenessWatchOutcome(attempt, observation, installErr)
	if err != nil {
		return fmt.Errorf("re-observation could not form a terminal outcome; do not replay installation: %w", err)
	}
	if outcome.Determination == "resolved" {
		observationDetail = "exact managed plist and loaded launchd label were observed"
	}
	determination := "failed"
	if outcome.Determination == "resolved" {
		determination = "resolved"
	}
	decision, err := maat.BuildDiagnosticRepairDecision("sirsi maat repair liveness-watch re-observe", maat.DiagnosticRepair{
		Check: maatRepairLivenessWatchCheck, Operation: "re-observe the exact prior liveness-watch attempt without replay",
		Before: "incomplete attempt " + attempt.ID, After: observationDetail,
		Determination: determination, Detail: "Attempt evidence was retained; any subsequent installation requires a new owner review and consent.",
	})
	if err != nil {
		return err
	}
	decision.Evidence = fmt.Sprintf("maat-liveness-watch-attempt:%s:%s", attempt.ID, decision.Evidence)
	debt, err := store.BeginLivenessWatchProjection(attempt, outcome, decision)
	if err != nil {
		return fmt.Errorf("re-observation found no durable projection intent; installation was not replayed: %w", err)
	}
	if err := store.PersistLivenessWatchProjectionOutcome(debt); err != nil {
		return fmt.Errorf("re-observed outcome remains in projection debt; installation was not replayed: %w", err)
	}
	if err := appendLivenessProjectionOnce(journal, debt); err != nil {
		return fmt.Errorf("re-observation is retained as durable projection debt; installation was not replayed: %w", err)
	}
	if err := store.MarkLivenessWatchProjectionReadBack(debt); err != nil {
		return fmt.Errorf("Casebook append was read back but its Ma'at projection receipt remains pending; installation was not replayed: %w", err)
	}
	return renderLivenessProjectionResult(attempt.ID, outcome, "re-observed attempt")
}

func appendLivenessProjectionOnce(journal maat.DecisionJournal, debt maat.LivenessWatchProjectionDebt) error {
	idempotent, ok := journal.(idempotentMaatJournal)
	if !ok {
		return errors.New("Ma'at journal backend cannot prove idempotent projection readback")
	}
	return idempotent.AppendIdempotent(debt.Decision)
}

func boundedLivenessObservationError(err error) string {
	if err == nil {
		return ""
	}
	const maxObservationError = 4096
	message := err.Error()
	if len(message) > maxObservationError {
		message = message[:maxObservationError]
	}
	return message
}

func reconcileMaatLivenessProjection(store maatLivenessStore, journal maat.DecisionJournal, debt maat.LivenessWatchProjectionDebt) error {
	if err := store.PersistLivenessWatchProjectionOutcome(debt); err != nil {
		return fmt.Errorf("durable projection outcome could not be read back; debt retained and installation was not replayed: %w", err)
	}
	if err := appendLivenessProjectionOnce(journal, debt); err != nil {
		return fmt.Errorf("Casebook projection remains pending; debt retained and installation was not replayed: %w", err)
	}
	if err := store.MarkLivenessWatchProjectionReadBack(debt); err != nil {
		return fmt.Errorf("Casebook decision was read back but historical projection receipt remains pending; installation was not replayed: %w", err)
	}

	// Historical mutation debt is now closed independently of present health.
	// A fresh observation is an ordinary append-only fact, never a prerequisite
	// for retaining that receipt or a reason to replay the installer.
	observation, observeErr := maatRepairObserveLiveness(debt.Attempt.Manifest)
	observedAt := time.Now().UTC()
	state := "unverifiable"
	var after string
	if observeErr != nil {
		after = "current watch could not be verified: " + boundedLivenessObservationError(observeErr)
	} else if exactLivenessObservation(debt.Attempt.Manifest, observation) {
		state, after = "exact", "exact managed plist and loaded launchd label observed"
	} else {
		state, after = "drifted", "current observation differs from the reviewed managed watch"
	}
	after += " at " + observedAt.Format(time.RFC3339Nano)
	currentDecision := maat.Decision{
		Time: observedAt.Format(time.RFC3339Nano), Kind: "liveness-watch observation",
		Requester: "sirsi maat repair liveness-watch projection reconcile", Resource: maatRepairLivenessWatchCheck,
		Assessed: "fresh read-only managed watch observation", Determination: state,
		Why: fmt.Sprintf("%s; historical outcome %s at %s remains unchanged; installer was not replayed",
			after, debt.Outcome.Determination, debt.Outcome.RecordedAtUTC.UTC().Format(time.RFC3339Nano)),
		Evidence: fmt.Sprintf("maat-liveness-watch-attempt:%s:current:%s", debt.Attempt.ID, observedAt.Format(time.RFC3339Nano)),
	}
	if err := journal.Append(currentDecision); err != nil {
		return renderLivenessProjectionCurrentResult(debt, "unknown", observedAt, currentDecision.Evidence, false, err)
	}
	if observeErr != nil {
		return renderLivenessProjectionCurrentResult(debt, state, observedAt, currentDecision.Evidence, true, observeErr)
	}
	return renderLivenessProjectionCurrentResult(debt, state, observedAt, currentDecision.Evidence, true, nil)
}

func exactLivenessObservation(manifest maat.LivenessWatchManifest, observation maat.LivenessWatchObservation) bool {
	return observation.UID == manifest.UID && observation.Domain == manifest.Domain && observation.Label == manifest.ManagedLabel &&
		observation.PlistPath == manifest.TargetPlistPath && observation.PlistSHA256 == manifest.ExpectedPlistSHA256 && observation.LaunchdLoaded
}

func renderLivenessProjectionResult(attemptID string, outcome maat.LivenessWatchOutcome, source string) error {
	if JsonOutput || maatJSON {
		if err := emitJSON(map[string]any{"status": outcome.Determination, "attempt_id": attemptID, "journal_projected": true, "installation_replayed": false}); err != nil {
			return err
		}
		if outcome.Determination != "resolved" {
			return errors.New("Ma'at re-observation retained an incomplete outcome; a new repair requires fresh review and confirmation")
		}
		return nil
	}
	fmt.Printf("Ma'at %s %s: %s. Installation was not replayed.\n", source, attemptID, outcome.Determination)
	if outcome.Determination != "resolved" {
		return errors.New("Ma'at re-observation retained an incomplete outcome; a new repair requires fresh review and confirmation")
	}
	return nil
}

func renderLivenessProjectionCurrentResult(debt maat.LivenessWatchProjectionDebt, state string, observedAt time.Time, evidence string, recorded bool, factErr error) error {
	status := "projection_reconciled"
	if !recorded {
		status = "historical_projection_complete_current_unknown"
	}
	result := map[string]any{
		"status": status, "attempt_id": debt.Attempt.ID,
		"historical_outcome": debt.Outcome.Determination, "historical_recorded_at_utc": debt.Outcome.RecordedAtUTC,
		"current_state": state, "current_observed_at_utc": observedAt,
		"current_fact_recorded": recorded, "current_evidence": evidence,
		"historical_journal_projected": true, "installation_replayed": false,
	}
	if factErr != nil {
		result["current_error"] = boundedLivenessObservationError(factErr)
	}
	if JsonOutput || maatJSON {
		if err := emitJSON(result); err != nil {
			return err
		}
	} else {
		fmt.Printf("Ma'at projection debt %s reconciled: historical %s remains recorded at %s; current watch state is %s as of %s (fact recorded: %t). Installation was not replayed.\n",
			debt.Attempt.ID, debt.Outcome.Determination, debt.Outcome.RecordedAtUTC.UTC().Format(time.RFC3339Nano), state, observedAt.Format(time.RFC3339Nano), recorded)
	}
	if !recorded {
		return fmt.Errorf("historical projection is complete, but current observation is unknown and its fact was not confirmed recorded; a new repair requires fresh review and confirmation: %w", factErr)
	}
	if debt.Outcome.Determination != "resolved" {
		return errors.New("historical repair outcome remains incomplete; the current observation does not upgrade it, and any new repair requires fresh review and confirmation")
	}
	if state != "exact" {
		return errors.New("historical projection is complete, but the current watch is not verified; a new repair requires fresh review and confirmation")
	}
	return nil
}

func installMaatLivenessWatch() error {
	if !maatRepairConfirm {
		return errors.New("Ma'at liveness-watch installation changes a managed LaunchAgent; review the exact finding and rerun with --confirm")
	}
	firstFinding, found, err := maatRepairReadLivenessFinding()
	if err != nil {
		return fmt.Errorf("Ma'at liveness-watch review: bounded local observation: %w", err)
	}
	if !found {
		return errors.New("Ma'at liveness-watch review: no actionable local-watch concern exists; no launchd state changed")
	}
	journal, err := newMaatDecisionJournal()
	if err != nil {
		return fmt.Errorf("open Ma'at journal before failure-memory preflight: %w", err)
	}
	if _, err := journal.Recent(1); err != nil {
		return fmt.Errorf("validate Ma'at journal before failure-memory preflight: %w", err)
	}
	store, err := maatRepairOpenDefaultStore()
	if err != nil {
		status := maatRepairDefaultStoreInspect()
		if status.State == maat.DefaultStoreUninitialized {
			return errors.New("Ma'at local failure memory needs explicit first-use initialization in the Ma'at app before this repair; no launchd state changed")
		}
		return fmt.Errorf("Ma'at liveness-watch failure-memory preflight is %s; inspect/re-observe it in Ma'at, never reset or replay: %w", status.State, err)
	}
	defer store.Close()
	if pending, pendingErr := store.PendingLivenessWatchAttempt(); pendingErr == nil {
		return fmt.Errorf("Ma'at liveness-watch has incomplete attempt %s; inspect and re-observe it before new consent; installation was not replayed", pending.ID)
	} else if !errors.Is(pendingErr, maat.ErrNoIncompleteLivenessWatchAttempt) {
		return fmt.Errorf("Ma'at liveness-watch attempt history is unverifiable: %w", pendingErr)
	}
	firstManifest, err := maatRepairBuildLivenessManifest(firstFinding, storeUUIDFromDefaultPath())
	if err != nil {
		return fmt.Errorf("bind reviewed liveness-watch action: %w", err)
	}
	currentFinding, found, err := maatRepairReadLivenessFinding()
	if err != nil {
		return fmt.Errorf("Ma'at liveness-watch consent recheck: bounded local observation: %w", err)
	}
	if !found {
		return errors.New("Ma'at liveness-watch consent recheck changed; no launchd state changed")
	}
	currentManifest, err := maatRepairBuildLivenessManifest(currentFinding, firstManifest.StoreUUID)
	if err != nil || !sameLivenessWatchConsent(firstManifest, currentManifest) {
		return errors.New("Ma'at liveness-watch finding or canonical action identity changed after review; no launchd state changed")
	}
	receipt, err := store.PreflightLivenessWatch(currentManifest)
	if err != nil {
		return fmt.Errorf("Ma'at liveness-watch failure-memory preflight is unverifiable: %w", err)
	}
	if err := maat.ProjectFailureMemoryPreflight(journal, receipt); err != nil {
		return fmt.Errorf("retain Ma'at liveness-watch preflight before action: %w", err)
	}
	if receipt.Decision != maat.PreflightPass {
		return fmt.Errorf("Ma'at liveness-watch failure-memory preflight %s; installation refused", receipt.Decision)
	}
	attempt, err := store.BeginLivenessWatchAttempt(currentManifest, receipt)
	if err != nil {
		return fmt.Errorf("persist Ma'at liveness-watch attempt start: %w", err)
	}
	startDecision := maat.Decision{Kind: "diagnostic repair attempt", Requester: "sirsi maat repair liveness-watch", Resource: maatRepairLivenessWatchCheck, Assessed: "durable attempt start", Determination: "started", Why: "The exact reviewed manifest and failure-memory preflight are retained before the installer call.", Evidence: "maat-liveness-watch-attempt:" + attempt.ID}
	if err := journal.Append(startDecision); err != nil {
		return fmt.Errorf("attempt start is durable but the decision journal refused it; no installer call occurred; inspect/re-observe before new consent: %w", err)
	}
	lastFinding, found, err := maatRepairReadLivenessFinding()
	if err != nil {
		return finishUnstartedLivenessAttempt(store, journal, attempt, currentManifest, fmt.Errorf("final consent recheck could not inspect local watch: %w", err))
	}
	lastManifest, manifestErr := maatRepairBuildLivenessManifest(lastFinding, currentManifest.StoreUUID)
	if !found || manifestErr != nil || !sameLivenessWatchConsent(currentManifest, lastManifest) {
		return finishUnstartedLivenessAttempt(store, journal, attempt, currentManifest, errors.New("finding or action identity changed at the final consent recheck"))
	}
	if err := maatRepairRevalidateExecutable(currentManifest); err != nil {
		return finishUnstartedLivenessAttempt(store, journal, attempt, currentManifest, err)
	}
	installMessage, installErr := maatRepairInstallLiveness(currentManifest)
	observation, observationErr := maatRepairObserveLiveness(currentManifest)
	postDescription := "exact local metadata schedule and loaded state observed after installation"
	postOK := observationErr == nil && exactLivenessObservation(currentManifest, observation)
	if observationErr != nil {
		postDescription = "post-install local observation unavailable: " + observationErr.Error()
	} else if !postOK {
		postDescription = "post-install local observation differs from the exact reviewed schedule"
	}
	installErrText := ""
	if installErr != nil {
		installErrText = installErr.Error()
	}
	if observationErr != nil {
		installErrText = strings.TrimSpace(installErrText + "; exact post-install observation failed: " + observationErr.Error())
		observation = maat.LivenessWatchObservation{}
	}
	storeOutcome, persistErr := maat.NewLivenessWatchOutcome(attempt, observation, installErrText)
	if persistErr != nil {
		return fmt.Errorf("liveness-watch installer may have changed the system but no terminal outcome could be formed; do not replay: %w", persistErr)
	}
	determination := "failed"
	detail := "the exact managed watch observation or existing diagnostic postcondition did not pass"
	if storeOutcome.Determination == "resolved" && postOK {
		determination, detail = "resolved", installMessage
	} else if installErr != nil {
		detail = "bounded installer returned an error: " + installErr.Error()
	} else if !postOK {
		detail = postDescription
	} else if observationErr != nil {
		detail = observationErr.Error()
	}
	decision, recordErr := maat.BuildDiagnosticRepairDecision("sirsi maat repair liveness-watch", maat.DiagnosticRepair{
		Check:     maatRepairLivenessWatchCheck,
		Operation: "install the exact reviewed managed per-user liveness LaunchAgent from the canonical Sirsi executable",
		Before:    maatRepairFindingDescription(firstFinding), After: postDescription, Determination: determination,
		Detail: fmt.Sprintf("attempt %s; durable result %s; %s", attempt.ID, storeOutcome.Determination, detail),
	})
	if recordErr != nil {
		return recordErr
	}
	decision.Evidence = fmt.Sprintf("maat-liveness-watch-attempt:%s:%s", attempt.ID, decision.Evidence)
	debt, debtErr := store.BeginLivenessWatchProjection(attempt, storeOutcome, decision)
	if debtErr != nil {
		return fmt.Errorf("installer may have changed the system; terminal result could not be bound to projection debt; do not replay: %w", debtErr)
	}
	if err := store.PersistLivenessWatchProjectionOutcome(debt); err != nil {
		return fmt.Errorf("liveness-watch projection debt is durable but its outcome readback failed; inspect and reconcile without replay: %w", err)
	}
	if err := appendLivenessProjectionOnce(journal, debt); err != nil {
		return fmt.Errorf("liveness-watch outcome is durable but Casebook projection remains pending; inspect and reconcile without replay: %w", err)
	}
	if err := store.MarkLivenessWatchProjectionReadBack(debt); err != nil {
		return fmt.Errorf("Casebook decision is read back but projection receipt remains pending; inspect and reconcile without replay: %w", err)
	}
	decision = debt.Decision
	result := &output.CommandResult{
		Command: "sirsi maat repair liveness-watch", BriefTitle: "Ma'at local liveness watch",
		Status: determination, Summary: "Ma'at recorded the liveness-watch attempt and its exact post-action observation.",
		Evidence:    []output.Evidence{{Label: "Failure-memory attempt", Value: attempt.ID}, {Label: "Post-repair", Value: postDescription}, {Label: "Ma'at receipt", Value: decision.Evidence}},
		NextActions: []output.NextAction{{Label: "Review Ma'at Casebook", Command: "sirsi maat casebook", Description: "Inspect the retained attempt, preflight, and post-action outcome."}},
	}
	if determination != "resolved" {
		result.Status = "error"
		result.Summary = "The liveness-watch action is incomplete. Ma'at retained the attempt; inspect and re-observe before any new consent."
		result.Errors = []string{detail}
		result.Render()
		return errors.New("Ma'at liveness-watch action did not satisfy every exact postcondition")
	}
	result.Render()
	return nil
}

func finishUnstartedLivenessAttempt(store maatLivenessStore, journal maat.DecisionJournal, attempt maat.LivenessWatchAttempt, manifest maat.LivenessWatchManifest, cause error) error {
	observation, observeErr := maatRepairObserveLiveness(manifest)
	installErr := "installer was not called: " + cause.Error()
	if observeErr != nil {
		installErr += "; current exact observation unavailable: " + observeErr.Error()
		observation = maat.LivenessWatchObservation{}
	}
	outcome, err := maat.NewLivenessWatchOutcome(attempt, observation, installErr)
	if err != nil {
		return fmt.Errorf("%v; durable attempt remains incomplete for safe re-observation: %w", cause, err)
	}
	decision, journalErr := maat.BuildDiagnosticRepairDecision("sirsi maat repair liveness-watch", maat.DiagnosticRepair{
		Check: maatRepairLivenessWatchCheck, Operation: "final consent recheck before liveness-watch installer",
		Before: "durable attempt " + attempt.ID, After: "installer not called; action stopped before effects",
		Determination: "failed", Detail: "attempt result " + outcome.Determination + "; " + cause.Error(),
	})
	if journalErr != nil {
		return fmt.Errorf("%v; stopped before installer; projection decision could not be built: %w", cause, journalErr)
	}
	decision.Evidence = fmt.Sprintf("maat-liveness-watch-attempt:%s:%s", attempt.ID, decision.Evidence)
	debt, err := store.BeginLivenessWatchProjection(attempt, outcome, decision)
	if err != nil {
		return fmt.Errorf("%v; installer was not called and projection debt could not be retained: %w", cause, err)
	}
	if err := store.PersistLivenessWatchProjectionOutcome(debt); err != nil {
		return fmt.Errorf("%v; installer was not called and projection outcome readback failed: %w", cause, err)
	}
	if err := appendLivenessProjectionOnce(journal, debt); err != nil {
		return fmt.Errorf("%v; installer was not called; projection remains pending for Ma'at reconciliation: %w", cause, err)
	}
	if err := store.MarkLivenessWatchProjectionReadBack(debt); err != nil {
		return fmt.Errorf("%v; installer was not called; journal readback succeeded but projection receipt is pending: %w", cause, err)
	}
	return fmt.Errorf("%w; installer was not called", cause)
}

func storeUUIDFromDefaultPath() string {
	status := maatRepairDefaultStoreInspect()
	if status.State != maat.DefaultStoreReady {
		return ""
	}
	return status.StoreUUID
}
