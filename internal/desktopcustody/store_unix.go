//go:build unix

package desktopcustody

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const maxRecordBytes = 16 << 10

// FileStore is a retained-root, no-follow, create-only custody store. Grants
// and revocations are individual immutable leaves so a restart never loses an
// owner-use revocation or turns it back into consent.
type FileStore struct {
	rootFD int
	mu     sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("desktop custody: state directory must be absolute")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("desktop custody: open state directory: %w", err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(fd)
		if err != nil {
			return nil, fmt.Errorf("desktop custody: stat state directory: %w", err)
		}
		return nil, errors.New("desktop custody: state path is not a directory")
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		return nil, errors.New("desktop custody: state directory must be owner-only")
	}
	return &FileStore{rootFD: fd}, nil
}

// WithLock serializes decisions across all processes that use this state root.
// The lock stays held through the final durable revocation read and fake/real
// actuator call; callers cannot cache a successful check outside it.
func (s *FileStore) WithLock(fn func() error) error {
	if s == nil || s.rootFD < 0 {
		return errors.New("state store is unavailable")
	}
	// flock coordinates independently opened descriptors. This mutex closes the
	// same-open-file-description case too: BSD flock treats a duplicated/shared
	// description as already locked, so it is not a substitute for in-process
	// serialization.
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := unix.Flock(s.rootFD, unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock state directory: %w", err)
	}
	fnErr := fn()
	if err := unix.Flock(s.rootFD, unix.LOCK_UN); err != nil {
		if fnErr != nil {
			return fmt.Errorf("%w; desktop custody unlock: %v", fnErr, err)
		}
		return fmt.Errorf("unlock state directory: %w", err)
	}
	return fnErr
}

func (s *FileStore) CreateGrant(g Grant) error {
	if err := validateStoredGrant(g); err != nil {
		return err
	}
	b, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal grant: %w", err)
	}
	return s.create("grant-"+g.ID+".json", b)
}

func (s *FileStore) LoadGrant(id string) (Grant, error) {
	if !identifierRE.MatchString(id) {
		return Grant{}, errors.New("invalid grant id")
	}
	b, err := s.read("grant-" + id + ".json")
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return Grant{}, ErrAbsent
		}
		return Grant{}, err
	}
	var g Grant
	if err := decodeExact(b, &g); err != nil {
		return Grant{}, fmt.Errorf("decode grant: %w", err)
	}
	if g.ID != id {
		return Grant{}, errors.New("grant identity does not match its retained name")
	}
	if err := validateStoredGrant(g); err != nil {
		return Grant{}, err
	}
	return g, nil
}

func (s *FileStore) CreateRevocation(id, reason string, at time.Time) error {
	if !identifierRE.MatchString(id) || strings.TrimSpace(reason) == "" || at.IsZero() {
		return errors.New("invalid revocation")
	}
	b, err := json.Marshal(Revocation{GrantID: id, Reason: reason, At: at.UTC()})
	if err != nil {
		return fmt.Errorf("marshal revocation: %w", err)
	}
	err = s.create("revoke-"+id+".json", b)
	if errors.Is(err, unix.EEXIST) {
		return nil
	}
	return err
}

func (s *FileStore) Revocation(id string) (Revocation, bool, error) {
	if !identifierRE.MatchString(id) {
		return Revocation{}, false, errors.New("invalid grant id")
	}
	b, err := s.read("revoke-" + id + ".json")
	if errors.Is(err, unix.ENOENT) {
		return Revocation{}, false, nil
	}
	if err != nil {
		return Revocation{}, false, err
	}
	var r Revocation
	if err := decodeExact(b, &r); err != nil {
		return Revocation{}, false, fmt.Errorf("decode revocation: %w", err)
	}
	if r.GrantID != id || strings.TrimSpace(r.Reason) == "" || r.At.IsZero() {
		return Revocation{}, false, errors.New("revocation identity is invalid")
	}
	return r, true, nil
}

func (s *FileStore) Close() error {
	if s == nil || s.rootFD < 0 {
		return nil
	}
	err := unix.Close(s.rootFD)
	s.rootFD = -1
	return err
}

func (s *FileStore) create(name string, content []byte) error {
	if s == nil || s.rootFD < 0 {
		return errors.New("state store is unavailable")
	}
	fd, err := unix.Openat(s.rootFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
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
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 {
		if err != nil {
			return fmt.Errorf("stat new record: %w", err)
		}
		return errors.New("new record identity is invalid")
	}
	for written := 0; written < len(content); {
		n, writeErr := unix.Write(fd, content[written:])
		if writeErr != nil {
			return fmt.Errorf("write record: %w", writeErr)
		}
		if n <= 0 {
			return errors.New("short record write")
		}
		written += n
	}
	if err := unix.Fsync(fd); err != nil {
		return fmt.Errorf("fsync record: %w", err)
	}
	if err := unix.Close(fd); err != nil {
		return fmt.Errorf("close record: %w", err)
	}
	closed = true
	if err := unix.Fsync(s.rootFD); err != nil {
		return fmt.Errorf("fsync state directory: %w", err)
	}
	return nil
}

func (s *FileStore) read(name string) ([]byte, error) {
	if s == nil || s.rootFD < 0 {
		return nil, errors.New("state store is unavailable")
	}
	fd, err := unix.Openat(s.rootFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, fmt.Errorf("stat record: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 || st.Size < 1 || st.Size > maxRecordBytes {
		return nil, errors.New("record identity is invalid")
	}
	b := make([]byte, st.Size)
	for off := 0; off < len(b); {
		n, readErr := unix.Read(fd, b[off:])
		if readErr != nil {
			return nil, fmt.Errorf("read record: %w", readErr)
		}
		if n == 0 {
			return nil, errors.New("short record read")
		}
		off += n
	}
	var extra [1]byte
	if n, err := unix.Read(fd, extra[:]); err != nil || n != 0 {
		if err != nil {
			return nil, fmt.Errorf("read record eof: %w", err)
		}
		return nil, errors.New("record changed while read")
	}
	return b, nil
}

func decodeExact(b []byte, dst any) error {
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

func validateStoredGrant(g Grant) error {
	if !identifierRE.MatchString(g.ID) || !identifierRE.MatchString(g.Host) || !identifierRE.MatchString(g.Owner) ||
		g.UnattendedStart.IsZero() || g.UnattendedEnd.IsZero() || g.ExpiresAt.IsZero() {
		return errors.New("stored grant is malformed")
	}
	start, end, expiry := g.UnattendedStart.UTC(), g.UnattendedEnd.UTC(), g.ExpiresAt.UTC()
	if !start.Before(end) || end.Sub(start) > maxUnattendedWindow || end.After(expiry) {
		return errors.New("stored grant has an invalid unattended window")
	}
	return nil
}
