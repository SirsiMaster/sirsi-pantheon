package maat

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	defaultStoreName   = "failure-memory"
	defaultBindingName = "failure-memory.binding.jsonl"
	defaultLockName    = "failure-memory.bootstrap.lock"
	genesisName        = "genesis.json"
	bindingSchema      = "sirsi.maat.failure-memory.binding.v1"
	genesisSchema      = "sirsi.maat.failure-memory.genesis.v1"
)

type DefaultStoreState string

const (
	DefaultStoreUninitialized DefaultStoreState = "uninitialized"
	DefaultStoreReady         DefaultStoreState = "ready"
	DefaultStoreIncomplete    DefaultStoreState = "incomplete"
)

// DefaultStoreStatus is a read-only projection used by native Ma'at controls.
// Incomplete state is never repaired or converted into a new registry.
type DefaultStoreStatus struct {
	State     DefaultStoreState `json:"state"`
	StoreUUID string            `json:"store_uuid,omitempty"`
	Detail    string            `json:"detail,omitempty"`
}

type storeBinding struct {
	Schema    string    `json:"schema"`
	StoreUUID string    `json:"store_uuid"`
	UID       uint32    `json:"uid"`
	State     string    `json:"state"`
	Recorded  time.Time `json:"recorded_at_utc"`
}

type storeGenesis struct {
	Schema            string    `json:"schema"`
	StoreUUID         string    `json:"store_uuid"`
	UID               uint32    `json:"uid"`
	InitializedAtUTC  time.Time `json:"initialized_at_utc"`
	ImportedIncidents bool      `json:"imported_incidents"`
}

func DefaultStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("maat memory: resolve home: %w", err)
	}
	return filepath.Join(home, ".sirsi", "maat", defaultStoreName), nil
}

// InspectDefaultStore distinguishes genuine first use from any state that
// could represent interrupted initialization, loss, substitution, or damage.
// It never creates or repairs state.
func InspectDefaultStore() DefaultStoreStatus {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, Detail: "cannot resolve the per-user home directory"}
	}
	return inspectDefaultStoreAt(home)
}

func inspectDefaultStoreAt(home string) DefaultStoreStatus {
	parentFD, err := openDefaultStoreParent(home, false)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return DefaultStoreStatus{State: DefaultStoreUninitialized}
		}
		return DefaultStoreStatus{State: DefaultStoreIncomplete, Detail: "the per-user Ma'at directory is not safely inspectable"}
	}
	defer unix.Close(parentFD)
	return inspectStoreAt(parentFD)
}

// OpenDefaultStore opens only a previously initialized, matching default
// registry. It does not adopt an existing directory or create first-use state.
func OpenDefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("maat memory: resolve home: %w", err)
	}
	return openDefaultStoreAt(home)
}

func openDefaultStoreAt(home string) (*Store, error) {
	parentFD, err := openDefaultStoreParent(home, false)
	if err != nil {
		return nil, fmt.Errorf("maat memory: default store parent unavailable: %w", err)
	}
	defer unix.Close(parentFD)
	lockFD, err := openBootstrapLock(parentFD, false)
	if err != nil {
		return nil, fmt.Errorf("maat memory: default store initialization lock unavailable: %w", err)
	}
	defer unix.Close(lockFD)
	if err := unix.Flock(lockFD, unix.LOCK_SH); err != nil {
		return nil, fmt.Errorf("maat memory: lock default store binding: %w", err)
	}
	defer func() {
		// Closing lockFD below releases the lock even if this best-effort unlock fails.
		_ = unix.Flock(lockFD, unix.LOCK_UN)
	}()
	status := inspectStoreAt(parentFD)
	if status.State != DefaultStoreReady {
		return nil, fmt.Errorf("maat memory: default store is %s: %s", status.State, status.Detail)
	}
	rootFD, err := openPrivateDirectoryAt(parentFD, defaultStoreName, false)
	if err != nil {
		return nil, fmt.Errorf("maat memory: open bound store root: %w", err)
	}
	store, err := openStoreFDExisting(rootFD)
	if err != nil {
		return nil, err
	}
	store.storeUUID = status.StoreUUID
	return store, nil
}

// InitializeDefaultStore is an explicit first-use action. It never imports
// prior incidents and refuses every existing or interrupted binding/root.
func InitializeDefaultStore() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("maat memory: resolve home: %w", err)
	}
	return initializeDefaultStoreAt(home)
}

func initializeDefaultStoreAt(home string) (string, error) {
	parentFD, err := openDefaultStoreParent(home, true)
	if err != nil {
		return "", fmt.Errorf("maat memory: create private default-store parent: %w", err)
	}
	defer unix.Close(parentFD)
	lockFD, err := openBootstrapLock(parentFD, true)
	if err != nil {
		return "", err
	}
	defer unix.Close(lockFD)
	if err := unix.Flock(lockFD, unix.LOCK_EX); err != nil {
		return "", fmt.Errorf("maat memory: serialize default-store initialization: %w", err)
	}
	defer func() {
		// Closing lockFD below releases the lock even if this best-effort unlock fails.
		_ = unix.Flock(lockFD, unix.LOCK_UN)
	}()
	status := inspectStoreAt(parentFD)
	if status.State != DefaultStoreUninitialized {
		return "", fmt.Errorf("maat memory: initialize refused for %s store: %s", status.State, status.Detail)
	}
	if err := unix.Fstatat(parentFD, defaultStoreName, &unix.Stat_t{}, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		return "", errors.New("maat memory: unbound failure-memory directory will not be adopted")
	} else if !errors.Is(err, unix.ENOENT) {
		return "", fmt.Errorf("maat memory: inspect unbound store path: %w", err)
	}
	uid := uint32(os.Geteuid())
	storeUUID, err := newStoreUUID()
	if err != nil {
		return "", err
	}
	initial := storeBinding{Schema: bindingSchema, StoreUUID: storeUUID, UID: uid, State: "initializing", Recorded: time.Now().UTC()}
	if err := createBinding(parentFD, initial); err != nil {
		return "", fmt.Errorf("maat memory: persist initializing binding: %w", err)
	}
	if err := unix.Mkdirat(parentFD, defaultStoreName, 0o700); err != nil {
		return "", fmt.Errorf("maat memory: create store root after binding: %w", err)
	}
	if err := unix.Fsync(parentFD); err != nil {
		return "", fmt.Errorf("maat memory: sync store-root creation: %w", err)
	}
	rootFD, err := openPrivateDirectoryAt(parentFD, defaultStoreName, false)
	if err != nil {
		return "", fmt.Errorf("maat memory: open newly created store root: %w", err)
	}
	genesis := storeGenesis{Schema: genesisSchema, StoreUUID: storeUUID, UID: uid, InitializedAtUTC: time.Now().UTC(), ImportedIncidents: false}
	genesisBytes, err := json.Marshal(genesis)
	if err == nil {
		genesisBytes = append(genesisBytes, '\n')
		err = createOrVerifyRaw(rootFD, genesisName, genesisBytes)
	}
	if err == nil {
		err = unix.Fsync(rootFD)
	}
	if err == nil {
		err = validateGenesis(rootFD, storeUUID, uid)
	}
	if err == nil {
		var store *Store
		store, err = openStoreFD(rootFD)
		if err == nil {
			store.storeUUID = storeUUID
			err = store.Close()
		} else {
			// openStoreFD consumes rootFD on both paths.
			rootFD = -1
		}
		if store != nil {
			rootFD = -1
		}
	}
	if rootFD >= 0 {
		_ = unix.Close(rootFD)
	}
	if err != nil {
		return "", fmt.Errorf("maat memory: genesis initialization incomplete and will not be reset: %w", err)
	}
	if err := validateDefaultStoreContents(parentFD, storeUUID, uid); err != nil {
		return "", fmt.Errorf("maat memory: initialized store layout is incomplete and will not be committed: %w", err)
	}
	committed := initial
	committed.State = "committed"
	committed.Recorded = time.Now().UTC()
	if err := appendBinding(parentFD, committed); err != nil {
		return "", fmt.Errorf("maat memory: store exists but binding commit is incomplete; do not retry installation: %w", err)
	}
	status = inspectStoreAt(parentFD)
	if status.State != DefaultStoreReady || status.StoreUUID != storeUUID {
		return "", errors.New("maat memory: committed store failed binding/genesis readback")
	}
	return storeUUID, nil
}

func inspectStoreAt(parentFD int) DefaultStoreStatus {
	data, bindingErr := readPrivateRegularAt(parentFD, defaultBindingName, 8192)
	var pathStat unix.Stat_t
	pathErr := unix.Fstatat(parentFD, defaultStoreName, &pathStat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(bindingErr, unix.ENOENT) {
		if errors.Is(pathErr, unix.ENOENT) {
			return DefaultStoreStatus{State: DefaultStoreUninitialized}
		}
		return DefaultStoreStatus{State: DefaultStoreIncomplete, Detail: "an unbound store path exists and cannot be adopted"}
	}
	if bindingErr != nil {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, Detail: "binding is not a safe regular file"}
	}
	bindings, err := decodeBindings(data)
	if err != nil {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, Detail: "binding is corrupt or initialization was interrupted"}
	}
	last := bindings[len(bindings)-1]
	if last.State != "committed" || len(bindings) != 2 {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, StoreUUID: last.StoreUUID, Detail: "initialization is incomplete; inspect and re-observe, never reset or replay"}
	}
	if pathErr != nil || pathStat.Mode&unix.S_IFMT != unix.S_IFDIR || pathStat.Uid != uint32(os.Geteuid()) || pathStat.Mode&0o077 != 0 {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, StoreUUID: last.StoreUUID, Detail: "committed store root is missing or not private and owner-controlled"}
	}
	rootFD, err := openPrivateDirectoryAt(parentFD, defaultStoreName, false)
	if err != nil {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, StoreUUID: last.StoreUUID, Detail: "committed store root cannot be opened safely"}
	}
	defer unix.Close(rootFD)
	if err := validateGenesis(rootFD, last.StoreUUID, last.UID); err != nil {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, StoreUUID: last.StoreUUID, Detail: "committed store genesis does not match its outside binding"}
	}
	if err := validateDefaultStoreContents(parentFD, last.StoreUUID, last.UID); err != nil {
		return DefaultStoreStatus{State: DefaultStoreIncomplete, StoreUUID: last.StoreUUID, Detail: "committed store layout is incomplete or not safely owner-controlled"}
	}
	return DefaultStoreStatus{State: DefaultStoreReady, StoreUUID: last.StoreUUID}
}

func validateDefaultStoreContents(parentFD int, storeUUID string, uid uint32) error {
	if uid != uint32(os.Geteuid()) || !validUUID(storeUUID) {
		return errors.New("maat memory: store owner or UUID does not match the current binding")
	}
	rootFD, err := openPrivateDirectoryAt(parentFD, defaultStoreName, false)
	if err != nil {
		return err
	}
	defer unix.Close(rootFD)
	for _, name := range []string{"evidence", "incidents", "transitions", "writes", "attempts"} {
		fd, err := openStoreDirectory(rootFD, name, false)
		if err != nil {
			return err
		}
		_ = unix.Close(fd)
	}
	lockFD, err := unix.Openat(rootFD, ".lock", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("maat memory: open existing registry lock: %w", err)
	}
	defer unix.Close(lockFD)
	return requireRegular(lockFD)
}

func openDefaultStoreParent(home string, create bool) (int, error) {
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return -1, errors.New("maat memory: home path is not canonical")
	}
	homeFD, err := openAbsoluteDirectory(home)
	if err != nil {
		return -1, err
	}
	defer unix.Close(homeFD)
	if err := requireOwnedDirectory(homeFD, false); err != nil {
		return -1, fmt.Errorf("maat memory: home directory identity: %w", err)
	}
	sirsiFD, err := openOrCreateSharedDirectory(homeFD, ".sirsi", create)
	if err != nil {
		return -1, err
	}
	defer unix.Close(sirsiFD)
	return openOrCreateSharedDirectory(sirsiFD, "maat", create)
}

// Shared HOME ancestors may retain ordinary 0755 permissions and unrelated
// files. They must still be current-user owned and not writable by group/other;
// the actual failure-memory root and every record remain private.
func openOrCreateSharedDirectory(parentFD int, name string, create bool) (int, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if err = unix.Mkdirat(parentFD, name, 0o700); err == nil {
			err = unix.Fsync(parentFD)
			if err == nil {
				fd, err = unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
	}
	if err != nil {
		return -1, fmt.Errorf("maat memory: open shared parent directory %q: %w", name, err)
	}
	if err := requireOwnedSharedDirectory(fd); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("maat memory: shared parent directory %q: %w", name, err)
	}
	return fd, nil
}

func requireOwnedSharedDirectory(fd int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if !sharedDirectoryIdentityAllowed(uint32(os.Geteuid()), stat.Uid, uint32(stat.Mode)) {
		return errors.New("expected current-user-owned non-group/other-writable directory")
	}
	return nil
}

func sharedDirectoryIdentityAllowed(actorUID, ownerUID, mode uint32) bool {
	return mode&uint32(unix.S_IFMT) == uint32(unix.S_IFDIR) && ownerUID == actorUID && mode&0o022 == 0
}

func openAbsoluteDirectory(path string) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if component == "" {
			continue
		}
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return -1, openErr
		}
		fd = next
	}
	return fd, nil
}

func openOrCreatePrivateDirectory(parentFD int, name string, create bool) (int, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if err = unix.Mkdirat(parentFD, name, 0o700); err == nil {
			err = unix.Fsync(parentFD)
			if err == nil {
				fd, err = unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
	}
	if err != nil {
		return -1, fmt.Errorf("maat memory: open private directory %q: %w", name, err)
	}
	if err := requireOwnedDirectory(fd, true); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("maat memory: private directory %q: %w", name, err)
	}
	return fd, nil
}

func openPrivateDirectoryAt(parentFD int, name string, create bool) (int, error) {
	return openOrCreatePrivateDirectory(parentFD, name, create)
}

func requireOwnedDirectory(fd int, private bool) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || (private && stat.Mode&0o077 != 0) {
		return errors.New("expected current-user-owned private directory")
	}
	return nil
}

func openBootstrapLock(parentFD int, create bool) (int, error) {
	flags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if create {
		flags |= unix.O_CREAT
	}
	fd, err := unix.Openat(parentFD, defaultLockName, flags, 0o600)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		return -1, errors.New("maat memory: bootstrap lock identity is not private and owner-controlled")
	}
	if err := unix.Fsync(parentFD); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func createBinding(parentFD int, binding storeBinding) error {
	data, err := encodeBinding(binding)
	if err != nil {
		return err
	}
	fd, err := unix.Openat(parentFD, defaultBindingName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	writeErr := writeDescriptor(fd, data)
	if writeErr == nil {
		writeErr = unix.Fsync(fd)
	}
	closeErr := unix.Close(fd)
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := unix.Fsync(parentFD); err != nil {
		return err
	}
	actual, err := readExact(parentFD, defaultBindingName)
	if err != nil || !bytes.Equal(actual, data) {
		return errors.New("maat memory: initializing binding readback mismatch")
	}
	return nil
}

func appendBinding(parentFD int, binding storeBinding) error {
	data, err := encodeBinding(binding)
	if err != nil {
		return err
	}
	fd, err := unix.Openat(parentFD, defaultBindingName, unix.O_WRONLY|unix.O_APPEND|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		return errors.New("maat memory: binding changed before commit")
	}
	if err := writeDescriptor(fd, data); err != nil {
		return err
	}
	if err := unix.Fsync(fd); err != nil {
		return err
	}
	actual, err := readExact(parentFD, defaultBindingName)
	if err != nil {
		return err
	}
	bindings, err := decodeBindings(actual)
	if err != nil || len(bindings) != 2 || bindings[1] != binding {
		return errors.New("maat memory: committed binding readback mismatch")
	}
	return unix.Fsync(parentFD)
}

func encodeBinding(binding storeBinding) ([]byte, error) {
	if binding.Schema != bindingSchema || !validUUID(binding.StoreUUID) || binding.UID != uint32(os.Geteuid()) || (binding.State != "initializing" && binding.State != "committed") || binding.Recorded.IsZero() {
		return nil, errors.New("maat memory: invalid default-store binding")
	}
	data, err := json.Marshal(binding)
	return append(data, '\n'), err
}

func decodeBindings(data []byte) ([]storeBinding, error) {
	if len(data) == 0 || len(data) > 8192 || data[len(data)-1] != '\n' {
		return nil, errors.New("maat memory: invalid binding length or torn tail")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) < 1 || len(lines) > 2 {
		return nil, errors.New("maat memory: invalid binding state count")
	}
	bindings := make([]storeBinding, 0, len(lines))
	for _, line := range lines {
		var binding storeBinding
		if err := json.Unmarshal(line, &binding); err != nil || binding.Schema != bindingSchema || !validUUID(binding.StoreUUID) || binding.UID != uint32(os.Geteuid()) || binding.Recorded.IsZero() {
			return nil, errors.New("maat memory: invalid binding record")
		}
		canonical, err := json.Marshal(binding)
		if err != nil || !bytes.Equal(canonical, line) {
			return nil, errors.New("maat memory: noncanonical binding record")
		}
		bindings = append(bindings, binding)
	}
	if bindings[0].State != "initializing" || (len(bindings) == 2 && (bindings[1].State != "committed" || bindings[0].StoreUUID != bindings[1].StoreUUID || bindings[0].UID != bindings[1].UID || !bindings[1].Recorded.After(bindings[0].Recorded))) {
		return nil, errors.New("maat memory: invalid binding transition")
	}
	return bindings, nil
}

func validateGenesis(rootFD int, storeUUID string, uid uint32) error {
	data, err := readPrivateRegularAt(rootFD, genesisName, 8192)
	if err != nil {
		return err
	}
	var genesis storeGenesis
	if err := json.Unmarshal(data, &genesis); err != nil || genesis.Schema != genesisSchema || genesis.StoreUUID != storeUUID || genesis.UID != uid || genesis.ImportedIncidents || genesis.InitializedAtUTC.IsZero() {
		return errors.New("maat memory: genesis identity mismatch")
	}
	canonical, err := json.Marshal(genesis)
	if err != nil || !bytes.Equal(append(canonical, '\n'), data) {
		return errors.New("maat memory: noncanonical genesis record")
	}
	return nil
}

func readPrivateRegularAt(parentFD int, name string, max int64) ([]byte, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return nil, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Uid != uint32(os.Geteuid()) || before.Mode&0o077 != 0 || before.Size < 0 || before.Size > max {
		return nil, errors.New("maat memory: expected private owner-controlled regular file")
	}
	data := make([]byte, before.Size)
	for offset := 0; offset < len(data); {
		n, readErr := unix.Read(fd, data[offset:])
		if readErr != nil {
			return nil, readErr
		}
		if n == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		offset += n
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return nil, err
	}
	if before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode || before.Uid != after.Uid || before.Nlink != after.Nlink || before.Size != after.Size {
		return nil, errors.New("maat memory: private file changed during read")
	}
	return data, nil
}

func writeDescriptor(fd int, data []byte) error {
	for len(data) > 0 {
		n, err := unix.Write(fd, data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func newStoreUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("maat memory: generate store identity: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}
