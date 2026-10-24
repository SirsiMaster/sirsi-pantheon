package modelrouter

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	decisionRecordLimit  = 2048
	decisionJournalLimit = 256 << 10
	decisionLockWait     = 250 * time.Millisecond
	decisionLockRetry    = 10 * time.Millisecond
)

// DecisionRecord is the redacted, bounded evidence for one route or completion.
// It intentionally has no prompt, output, provider error text, or credential field.
type DecisionRecord struct {
	At                time.Time    `json:"at"`
	Task              TaskClass    `json:"task"`
	Privacy           PrivacyClass `json:"privacy"`
	Latency           LatencyClass `json:"latency"`
	MinContextTokens  int          `json:"min_context_tokens"`
	RequestedOverride Lane         `json:"requested_override,omitempty"`
	InitialLane       Lane         `json:"initial_lane,omitempty"`
	FinalLane         Lane         `json:"final_lane,omitempty"`
	Provider          string       `json:"provider,omitempty"`
	Model             string       `json:"model,omitempty"`
	Reason            string       `json:"reason,omitempty"`
	Outcome           string       `json:"outcome"`
	FailureCategory   string       `json:"failure_category,omitempty"`
}

// DecisionSink receives one bounded routing or execution decision.
type DecisionSink interface {
	Record(DecisionRecord) error
}

// PrivateDecisionSink stores a small owner-local JSONL journal under ~/.sirsi.
// An explicit home is accepted for tests; production passes an empty value.
type PrivateDecisionSink struct {
	Home           string
	lockWait       time.Duration
	afterSizeCheck func(needsTruncate bool)
}

func NewPrivateDecisionSink(home string) *PrivateDecisionSink {
	return &PrivateDecisionSink{Home: home}
}

func (s *PrivateDecisionSink) Record(record DecisionRecord) error {
	if record.At.IsZero() {
		record.At = time.Now().UTC()
	}
	record.Provider = decisionLabel(record.Provider)
	record.Model = decisionLabel(record.Model)
	record.Reason = decisionReason(record.Reason)
	record.Outcome = decisionLabel(record.Outcome)
	record.FailureCategory = decisionLabel(record.FailureCategory)
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("model-router: encode decision record")
	}
	line = append(line, '\n')
	if len(line) > decisionRecordLimit {
		return fmt.Errorf("model-router: decision record exceeds size limit")
	}
	home := s.Home
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("model-router: resolve private decision directory")
		}
	}
	homeFD, parentFD, journalDirFD, err := openDecisionDirectories(home)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(homeFD); _ = unix.Close(parentFD); _ = unix.Close(journalDirFD) }()
	lockFD, err := openDecisionFile(journalDirFD, "decisions.lock")
	if err != nil {
		return fmt.Errorf("model-router: open private decision lock")
	}
	defer unix.Close(lockFD)
	wait := s.lockWait
	if wait <= 0 {
		wait = decisionLockWait
	}
	if err := acquireDecisionLock(lockFD, journalDirFD, "decisions.lock", wait); err != nil {
		return err
	}
	defer func() { _ = unix.Flock(lockFD, unix.LOCK_UN) }()

	fd, err := openDecisionFile(journalDirFD, "decisions.jsonl")
	if err != nil {
		return fmt.Errorf("model-router: open private decision journal")
	}
	f := os.NewFile(uintptr(fd), "model-router decisions")
	if f == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("model-router: open private decision journal")
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	if err := validateDecisionDirectories(homeFD, parentFD, journalDirFD); err != nil {
		return err
	}
	if err := validateDecisionBinding(journalDirFD, "decisions.jsonl", fd); err != nil {
		return err
	}
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil || !decisionJournalFileIsPrivate(&info, uint32(os.Geteuid())) {
		return fmt.Errorf("model-router: decision journal must be an owned regular 0600 single-link file")
	}
	needsTruncate := info.Size+int64(len(line)) > decisionJournalLimit
	if s.afterSizeCheck != nil {
		s.afterSizeCheck(needsTruncate)
	}
	if err := validateDecisionDirectories(homeFD, parentFD, journalDirFD); err != nil {
		return err
	}
	if err := validateDecisionBinding(journalDirFD, "decisions.jsonl", fd); err != nil {
		return err
	}
	if err := unix.Fstat(fd, &info); err != nil || !decisionJournalFileIsPrivate(&info, uint32(os.Geteuid())) {
		return fmt.Errorf("model-router: decision journal must be an owned regular 0600 single-link file")
	}
	needsTruncate = info.Size+int64(len(line)) > decisionJournalLimit
	if needsTruncate {
		if err := f.Truncate(0); err != nil {
			return fmt.Errorf("model-router: bound private decision journal")
		}
	}
	if _, err := f.Seek(0, 2); err != nil {
		return fmt.Errorf("model-router: seek private decision journal")
	}
	written, writeErr := f.Write(line)
	if writeErr == nil && written != len(line) {
		writeErr = errors.New("short write")
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	closed = true
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("model-router: write private decision journal")
	}
	return nil
}

// openDecisionDirectories accepts the normal 0755 ~/.sirsi directory but
// creates a dedicated 0700 child. Descriptor-relative operations preserve the
// parent's mode and prevent symlink substitution at either directory level.
func openDecisionDirectories(home string) (int, int, int, error) {
	uid := uint32(os.Geteuid())
	homeFD, err := unix.Open(home, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, -1, -1, fmt.Errorf("model-router: open owner decision home")
	}
	var homeStat unix.Stat_t
	if err := unix.Fstat(homeFD, &homeStat); err != nil || !decisionParentIsSafe(&homeStat, uid) {
		_ = unix.Close(homeFD)
		return -1, -1, -1, fmt.Errorf("model-router: decision home must be owned and not writable by others")
	}
	parentFD, err := unix.Openat(homeFD, ".sirsi", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		if mkdirErr := unix.Mkdirat(homeFD, ".sirsi", 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
			_ = unix.Close(homeFD)
			return -1, -1, -1, fmt.Errorf("model-router: create owner decision parent")
		}
		parentFD, err = unix.Openat(homeFD, ".sirsi", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		_ = unix.Close(homeFD)
		return -1, -1, -1, fmt.Errorf("model-router: open owner decision parent")
	}
	var parentStat unix.Stat_t
	if err := unix.Fstat(parentFD, &parentStat); err != nil || !decisionParentIsSafe(&parentStat, uid) {
		_ = unix.Close(parentFD)
		_ = unix.Close(homeFD)
		return -1, -1, -1, fmt.Errorf("model-router: decision parent must be owned and not writable by others")
	}
	if err := unix.Mkdirat(parentFD, "model-router", 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		_ = unix.Close(parentFD)
		_ = unix.Close(homeFD)
		return -1, -1, -1, fmt.Errorf("model-router: create private decision subdirectory")
	}
	journalDirFD, err := unix.Openat(parentFD, "model-router", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Close(parentFD)
		_ = unix.Close(homeFD)
		return -1, -1, -1, fmt.Errorf("model-router: open private decision subdirectory")
	}
	var journalStat, boundStat unix.Stat_t
	if err := unix.Fstat(journalDirFD, &journalStat); err != nil || !decisionDirectoryIsPrivate(&journalStat, uid) ||
		unix.Fstatat(parentFD, "model-router", &boundStat, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameDecisionInode(&journalStat, &boundStat) {
		_ = unix.Close(journalDirFD)
		_ = unix.Close(parentFD)
		_ = unix.Close(homeFD)
		return -1, -1, -1, fmt.Errorf("model-router: decision subdirectory identity or permissions are invalid")
	}
	return homeFD, parentFD, journalDirFD, nil
}

func decisionParentIsSafe(stat *unix.Stat_t, uid uint32) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Uid == uid && stat.Mode&0o022 == 0
}

func decisionDirectoryIsPrivate(stat *unix.Stat_t, uid uint32) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Uid == uid && stat.Mode&0o777 == 0o700
}

func decisionJournalFileIsPrivate(stat *unix.Stat_t, uid uint32) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Uid == uid && stat.Mode&0o777 == 0o600 && stat.Nlink == 1
}

func sameDecisionInode(a, b *unix.Stat_t) bool { return a.Dev == b.Dev && a.Ino == b.Ino }

func validateDecisionDirectories(homeFD, parentFD, journalDirFD int) error {
	var openedHome, namedParent, openedParent, namedJournal, openedJournal unix.Stat_t
	uid := uint32(os.Geteuid())
	if unix.Fstat(homeFD, &openedHome) != nil || !decisionParentIsSafe(&openedHome, uid) ||
		unix.Fstatat(homeFD, ".sirsi", &namedParent, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		unix.Fstat(parentFD, &openedParent) != nil || !decisionParentIsSafe(&openedParent, uid) ||
		!sameDecisionInode(&openedParent, &namedParent) ||
		unix.Fstatat(parentFD, "model-router", &namedJournal, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		unix.Fstat(journalDirFD, &openedJournal) != nil || !decisionDirectoryIsPrivate(&openedJournal, uid) ||
		!sameDecisionInode(&openedJournal, &namedJournal) {
		return fmt.Errorf("model-router: decision directory identity or permissions are invalid")
	}
	return nil
}

func openDecisionFile(dirFD int, name string) (int, error) {
	return unix.Openat(dirFD, name, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0o600)
}

func validateDecisionBinding(dirFD int, name string, fd int) error {
	var opened, bound unix.Stat_t
	if unix.Fstat(fd, &opened) != nil || !decisionJournalFileIsPrivate(&opened, uint32(os.Geteuid())) ||
		unix.Fstatat(dirFD, name, &bound, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameDecisionInode(&opened, &bound) {
		return fmt.Errorf("model-router: decision file identity or permissions are invalid")
	}
	return nil
}

func acquireDecisionLock(fd, dirFD int, name string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		if err := validateDecisionBinding(dirFD, name, fd); err != nil {
			return fmt.Errorf("model-router: decision lock identity or permissions are invalid")
		}
		err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			if err := validateDecisionBinding(dirFD, name, fd); err != nil {
				_ = unix.Flock(fd, unix.LOCK_UN)
				return fmt.Errorf("model-router: decision lock identity or permissions are invalid")
			}
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("model-router: acquire private decision lock")
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("model-router: decision lock contention exceeded bounded wait")
		}
		time.Sleep(decisionLockRetry)
	}
}

type DecisionRecordError struct{ err error }

func (e *DecisionRecordError) Error() string { return "model-router: decision evidence write failed" }
func (e *DecisionRecordError) Unwrap() error { return e.err }

func (rt *Router) recordDecision(req Request, d Decision, outcome string, cause error) error {
	if rt.decisionSink == nil {
		return nil
	}
	rec := DecisionRecord{
		At: time.Now().UTC(), Task: safeTask(req.Task), Privacy: safePrivacy(req.Privacy),
		Latency: safeLatency(req.Latency), MinContextTokens: max(0, req.MinContextTokens),
		RequestedOverride: safeLane(req.Override), InitialLane: safeLane(d.InitialLane),
		FinalLane: safeLane(d.Lane), Reason: d.Rationale, Outcome: outcome,
		FailureCategory: d.FailureCategory,
	}
	if d.Provider != nil {
		rec.Provider = d.Provider.Name()
	}
	if d.ProviderName != "" {
		rec.Provider = d.ProviderName
	}
	rec.Model = d.Model
	if rec.FailureCategory == "" && cause != nil {
		rec.FailureCategory = failureCategory(cause)
	}
	rec.Provider = decisionLabel(rec.Provider)
	rec.Model = decisionLabel(rec.Model)
	rec.Reason = decisionReason(rec.Reason)
	rec.Outcome = decisionLabel(rec.Outcome)
	rec.FailureCategory = decisionLabel(rec.FailureCategory)
	if err := rt.decisionSink.Record(rec); err != nil {
		d.FailureCategory = "decision_record_failure"
		return &DecisionRecordError{err: err}
	}
	return nil
}

func decisionLabel(value string) string {
	lower := strings.ToLower(value)
	if len(value) > 64 || strings.ContainsAny(value, "@=\\\r\n\t ") || strings.Contains(lower, "://") || strings.Contains(lower, "bearer") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "sk-") {
		return "redacted"
	}
	var b strings.Builder
	for _, r := range value {
		if b.Len() >= 96 {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._:-", r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func decisionReason(value string) string {
	var b strings.Builder
	for _, r := range value {
		if b.Len() >= 160 {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune(" ._:-=+();", r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func safeTask(v TaskClass) TaskClass {
	if v == TaskGeneration || v == TaskJudgment || v == TaskExtraction || v == TaskEmbedding {
		return v
	}
	return ""
}
func safePrivacy(v PrivacyClass) PrivacyClass {
	if v == PrivacyLocalOnly || v == PrivacyShareable {
		return v
	}
	return ""
}
func safeLatency(v LatencyClass) LatencyClass {
	if v == LatencyInteractive || v == LatencyBackground {
		return v
	}
	return ""
}
func safeLane(v Lane) Lane {
	if v == LaneLocal || v == LaneRemote {
		return v
	}
	return ""
}
