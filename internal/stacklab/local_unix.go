//go:build unix

package stacklab

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var errLocalMissing = errors.New("local Stack Lab preferences are not initialized")

func localRoot(home string, create bool) (int, error) {
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		return -1, err
	}
	parent := filepath.Join(home, ".sirsi")
	root := filepath.Join(parent, "stacklab")
	for _, p := range []string{parent, root} {
		if create {
			if err = os.Mkdir(p, 0o700); err != nil && !os.IsExist(err) {
				return -1, err
			}
		}
		st, e := os.Lstat(p)
		if e != nil {
			if !create && os.IsNotExist(e) {
				return -1, errLocalMissing
			}
			return -1, e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0o022 != 0 {
			return -1, errors.New("Stack Lab local directory is linked or writable by another user/group")
		}
	}
	fd, err := unix.Open(root, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil || st.Uid != uint32(os.Getuid()) || st.Mode&0o777 != 0o700 {
		_ = unix.Close(fd)
		return -1, errors.New("Stack Lab local root must be private and owned")
	}
	return fd, nil
}

func readLocalAt(fd int) (LocalState, error) {
	n, err := unix.Openat(fd, "state.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return defaultLocalState(), nil
	}
	if err != nil {
		return LocalState{}, err
	}
	f := os.NewFile(uintptr(n), "state.json")
	defer func() { _ = f.Close() }()
	var st unix.Stat_t
	if err = unix.Fstat(n, &st); err != nil {
		return LocalState{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Getuid()) || st.Mode&0o777 != 0o600 || st.Nlink != 1 || st.Size > 4096 {
		return LocalState{}, errors.New("Stack Lab state must be private regular bounded data")
	}
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return LocalState{}, err
	}
	var value LocalState
	keys := json.NewDecoder(bytes.NewReader(data))
	token, err := keys.Token()
	if err != nil || token != json.Delim('{') {
		return value, errors.New("Stack Lab state must be an exact object")
	}
	seen := map[string]bool{}
	for keys.More() {
		token, err = keys.Token()
		if err != nil {
			return value, err
		}
		key, ok := token.(string)
		if !ok || seen[key] || key != "schema" && key != "enabled" && key != "provider" {
			return value, errors.New("Stack Lab state has duplicate/unknown keys")
		}
		seen[key] = true
		var raw json.RawMessage
		if err = keys.Decode(&raw); err != nil {
			return value, err
		}
		if key == "enabled" && string(raw) != "true" && string(raw) != "false" {
			return value, errors.New("Stack Lab enabled must be explicit boolean")
		}
	}
	if len(seen) != 3 {
		return value, errors.New("Stack Lab state fields missing")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&value); err != nil {
		return value, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return value, errors.New("Stack Lab state has trailing data")
	}
	return value, value.Validate()
}

func InspectLocal(home string) (LocalState, error) {
	fd, err := localRoot(home, false)
	if errors.Is(err, errLocalMissing) {
		return defaultLocalState(), nil
	}
	if err != nil {
		return LocalState{}, err
	}
	defer func() { _ = unix.Close(fd) }()
	value, err := readLocalAt(fd)
	if err != nil {
		return value, err
	}
	return value, localRootContinuity(home, fd)
}

func localRootContinuity(home string, fd int) error {
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		return err
	}
	var opened, named unix.Stat_t
	if err = unix.Fstat(fd, &opened); err != nil {
		return err
	}
	if err = unix.Lstat(filepath.Join(home, ".sirsi", "stacklab"), &named); err != nil {
		return err
	}
	if named.Mode&unix.S_IFMT != unix.S_IFDIR || opened.Dev != named.Dev || opened.Ino != named.Ino {
		return errors.New("Stack Lab local root changed; no update is claimed")
	}
	return nil
}

func SetLocal(home string, enabled bool, provider string) (LocalState, error) {
	want := LocalState{Schema: LocalSchema, Enabled: enabled, Provider: provider}
	if err := want.Validate(); err != nil {
		return want, err
	}
	fd, err := localRoot(home, true)
	if err != nil {
		return want, err
	}
	defer func() { _ = unix.Close(fd) }()
	lock, err := unix.Openat(fd, ".lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return want, err
	}
	defer func() { _ = unix.Close(lock) }()
	var st unix.Stat_t
	if err = unix.Fstat(lock, &st); err != nil {
		return want, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Getuid()) || st.Mode&0o777 != 0o600 || st.Nlink != 1 {
		return want, errors.New("Stack Lab config lock is not private regular data")
	}
	if err = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return want, err
	}
	if _, err = readLocalAt(fd); err != nil {
		return want, err
	} // Never overwrite corrupt/unknown state.
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return want, err
	}
	if err = localRootContinuity(home, fd); err != nil {
		return want, err
	}
	leaf := ".state-" + hex.EncodeToString(nonce[:])
	out, err := unix.Openat(fd, leaf, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return want, err
	}
	f := os.NewFile(uintptr(out), leaf)
	data, _ := json.Marshal(want)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return want, err
	} // Retained temp is honest debt, not deleted by name.
	if err = unix.Renameat(fd, leaf, fd, "state.json"); err != nil {
		return want, err
	}
	if err = unix.Fsync(fd); err != nil {
		return want, err
	}
	got, err := readLocalAt(fd)
	if err != nil {
		return want, err
	}
	if got != want {
		return got, errors.New("Stack Lab local readback changed")
	}
	return got, localRootContinuity(home, fd)
}
