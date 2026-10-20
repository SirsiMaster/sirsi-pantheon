//go:build unix

package desktopcustody

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const maxSleepRecordBytes = 16 << 10

// FileSleepJournal retains the directory descriptor for its whole lifetime;
// all leaves are no-follow, immutable, complete-write/fsync files.
type FileSleepJournal struct {
	rootFD int
	mu     sync.Mutex
}

func NewFileSleepJournal(root string) (*FileSleepJournal, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("sleep custody: state directory must be absolute")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("sleep custody: open state directory: %w", err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		if err != nil {
			return nil, fmt.Errorf("sleep custody: stat state directory: %w", err)
		}
		return nil, errors.New("sleep custody: state directory is not private retained directory")
	}
	return &FileSleepJournal{rootFD: fd}, nil
}

func (j *FileSleepJournal) Close() error {
	if j == nil || j.rootFD < 0 {
		return nil
	}
	err := unix.Close(j.rootFD)
	j.rootFD = -1
	return err
}

var sleepOrdinal = map[SleepPhase]string{
	SleepPrepared: "00-prepared", WakeScheduled: "10-wake-scheduled", WakeReadBack: "20-wake-readback", SleepRequested: "30-sleep-requested", SleepInvoked: "35-native-invoked", SleepNativeReturned: "36-native-returned", ReturnPending: "40-return-pending", SleepVerified: "50-verified-return", SleepFailed: "90-failed", SleepUnknown: "91-unknown",
}

func (j *FileSleepJournal) BeginSleep(e SleepEvent) error {
	if e.Phase != SleepPrepared {
		return errors.New("sleep custody: first event must be prepared")
	}
	if j == nil || j.rootFD < 0 || !validSleepEvent(e) {
		return errors.New("sleep custody: invalid prepared event")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return j.withLock(func() error {
		// Both claims survive process exit. Partial persistence burns authority
		// conservatively; it never authorizes a second dispatch after a crash.
		if err := j.create(sleepClaim("nonce", e.Issuer, e.Nonce), b); err != nil {
			return fmt.Errorf("sleep custody: admission nonce already consumed or unavailable: %w", err)
		}
		if err := j.create(sleepClaim("host", e.PlatformUUID, e.BootID), b); err != nil {
			return fmt.Errorf("sleep custody: host has an unreconciled sleep transaction: %w", err)
		}
		return j.create(sleepLeaf(e.RequestID, e.Phase), b)
	})
}
func (j *FileSleepJournal) AppendSleep(e SleepEvent) error {
	if e.Phase == SleepPrepared {
		return errors.New("sleep custody: prepared event is create-only")
	}
	return j.write(e)
}

func (j *FileSleepJournal) ReadSleep(id string) ([]SleepEvent, error) {
	if j == nil || j.rootFD < 0 || !identifierRE.MatchString(id) {
		return nil, errors.New("sleep custody: invalid journal request")
	}
	var events []SleepEvent
	err := j.withLock(func() error {
		for _, p := range []SleepPhase{SleepPrepared, WakeScheduled, WakeReadBack, SleepRequested, SleepInvoked, SleepNativeReturned, ReturnPending, SleepVerified, SleepFailed, SleepUnknown} {
			b, err := j.read(sleepLeaf(id, p))
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read %s: %w", p, err)
			}
			var e SleepEvent
			if err := decodeSleepExact(b, &e); err != nil {
				return fmt.Errorf("decode %s: %w", p, err)
			}
			if e.Phase != p || e.RequestID != id || e.At.IsZero() {
				return errors.New("sleep custody: event identity is invalid")
			}
			events = append(events, e)
		}
		return validateSleepEvents(events)
	})
	return events, err
}

func (j *FileSleepJournal) write(e SleepEvent) error {
	if j == nil || j.rootFD < 0 || !validSleepEvent(e) {
		return errors.New("sleep custody: invalid journal event")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return j.withLock(func() error {
		if err := j.create(sleepLeaf(e.RequestID, e.Phase), b); err != nil {
			return err
		}
		if e.Phase != SleepVerified && e.Phase != SleepFailed {
			return nil
		}
		// A failed pre-invocation transaction can release its host fence. An
		// invoked/unknown transaction retains that fence until verified return.
		if e.Phase == SleepFailed {
			for _, p := range []SleepPhase{SleepInvoked, SleepUnknown} {
				if _, err := j.read(sleepLeaf(e.RequestID, p)); err == nil {
					return errors.New("sleep custody: invoked or unknown transaction cannot release a failed fence")
				} else if !errors.Is(err, unix.ENOENT) {
					return err
				}
			}
		}
		name := sleepClaim("host", e.PlatformUUID, e.BootID)
		data, err := j.read(name)
		if err != nil {
			return fmt.Errorf("sleep custody: read owned host fence: %w", err)
		}
		var claim SleepEvent
		if err := decodeSleepExact(data, &claim); err != nil || claim.RequestID != e.RequestID || claim.Nonce != e.Nonce || claim.AdmissionID != e.AdmissionID {
			return errors.New("sleep custody: host fence belongs to another transaction")
		}
		if err := unix.Unlinkat(j.rootFD, name, 0); err != nil {
			return fmt.Errorf("sleep custody: release owned host fence: %w", err)
		}
		return unix.Fsync(j.rootFD)
	})
}

func sleepClaim(kind, identity, nonce string) string {
	sum := sha256.Sum256([]byte("pantheon.sleep-custody.v1\x00" + identity + "\x00" + nonce))
	return "sleep-" + kind + "-" + hex.EncodeToString(sum[:]) + ".claim"
}
func (j *FileSleepJournal) withLock(fn func() error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := unix.Flock(j.rootFD, unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock state directory: %w", err)
	}
	err := fn()
	unlockErr := unix.Flock(j.rootFD, unix.LOCK_UN)
	if unlockErr != nil {
		if err != nil {
			return fmt.Errorf("%w; unlock state directory: %v", err, unlockErr)
		}
		return fmt.Errorf("unlock state directory: %w", unlockErr)
	}
	return err
}
func (j *FileSleepJournal) create(name string, content []byte) error {
	fd, err := unix.Openat(j.rootFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = unix.Close(fd)
		}
	}()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("stat new record: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 {
		return errors.New("new record identity is invalid")
	}
	for wrote := 0; wrote < len(content); {
		n, e := unix.Write(fd, content[wrote:])
		if e != nil {
			return fmt.Errorf("write record: %w", e)
		}
		if n <= 0 {
			return errors.New("short record write")
		}
		wrote += n
	}
	if err := unix.Fsync(fd); err != nil {
		return fmt.Errorf("fsync record: %w", err)
	}
	if err := unix.Close(fd); err != nil {
		return fmt.Errorf("close record: %w", err)
	}
	closed = true
	if err := unix.Fsync(j.rootFD); err != nil {
		return fmt.Errorf("fsync state directory: %w", err)
	}
	return nil
}
func (j *FileSleepJournal) read(name string) ([]byte, error) {
	fd, err := unix.Openat(j.rootFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, fmt.Errorf("stat record: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 || st.Size < 1 || st.Size > maxSleepRecordBytes {
		return nil, errors.New("record identity is invalid")
	}
	b := make([]byte, st.Size)
	for off := 0; off < len(b); {
		n, e := unix.Read(fd, b[off:])
		if e != nil {
			return nil, fmt.Errorf("read record: %w", e)
		}
		if n == 0 {
			return nil, errors.New("short record read")
		}
		off += n
	}
	var extra [1]byte
	if n, e := unix.Read(fd, extra[:]); e != nil || n != 0 {
		if e != nil {
			return nil, fmt.Errorf("read record eof: %w", e)
		}
		return nil, errors.New("record changed while read")
	}
	return b, nil
}
func sleepLeaf(id string, p SleepPhase) string {
	return "sleep-" + id + "-" + sleepOrdinal[p] + ".json"
}
func decodeSleepExact(b []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing record JSON")
	}
	return nil
}
func validSleepEvent(e SleepEvent) bool {
	return sleepOrdinal[e.Phase] != "" && identifierRE.MatchString(e.RequestID) && identifierRE.MatchString(e.Host) && identifierRE.MatchString(e.Owner) && e.AdmissionID != "" && e.Issuer != "" && e.PlatformUUID != "" && e.BootID != "" && e.Operation == SleepOperation && e.OwnerTag == "pantheon-sleep-"+e.RequestID && e.ReviewDigest != "" && !e.At.IsZero() && e.Nonce != "" && e.CustodyGeneration != "" && !e.NotBefore.IsZero() && e.NotAfter.After(e.NotBefore) && e.NotAfter.Sub(e.NotBefore) <= maxSleepWindow
}
func validateSleepEvents(events []SleepEvent) error {
	if len(events) == 0 {
		return nil
	}
	first := events[0]
	if first.Phase != SleepPrepared || !validSleepEvent(first) {
		return errors.New("sleep custody: invalid prepared root")
	}
	seen := map[SleepPhase]bool{}
	for _, e := range events {
		if !validSleepEvent(e) || seen[e.Phase] || e.RequestID != first.RequestID || e.Host != first.Host || e.Owner != first.Owner || e.AdmissionID != first.AdmissionID || e.Issuer != first.Issuer || e.PlatformUUID != first.PlatformUUID || e.BootID != first.BootID || e.Operation != first.Operation || e.OwnerTag != first.OwnerTag || e.ReviewDigest != first.ReviewDigest || e.Nonce != first.Nonce || e.CustodyGeneration != first.CustodyGeneration || !e.NotBefore.Equal(first.NotBefore) || !e.NotAfter.Equal(first.NotAfter) {
			return errors.New("sleep custody: substituted or duplicate journal event")
		}
		seen[e.Phase] = true
	}
	if seen[WakeScheduled] && !seen[SleepPrepared] || seen[WakeReadBack] && !seen[WakeScheduled] || seen[SleepRequested] && !seen[WakeReadBack] || seen[SleepInvoked] && !seen[SleepRequested] || seen[SleepNativeReturned] && !seen[SleepInvoked] || seen[ReturnPending] && !seen[SleepNativeReturned] {
		return errors.New("sleep custody: invalid event transition")
	}
	if seen[SleepVerified] && (!seen[ReturnPending] || seen[SleepFailed] || seen[SleepUnknown]) || (seen[SleepFailed] || seen[SleepUnknown]) && seen[SleepVerified] {
		return errors.New("sleep custody: conflicting terminal event")
	}
	return nil
}
