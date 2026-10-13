package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// This is an attempt receipt, not a second queue or permanent server dedupe.
// It never stores the prompt and never authorizes retry, even across clock skew.
type sharedGemmaAttempt struct {
	Key        string `json:"request_key"`
	Actor      string `json:"actor"`
	Digest     string `json:"prompt_sha256"`
	Title      string `json:"search_title"`
	FirstSend  string `json:"prepared_utc"`
	State      string `json:"state"`
	OriginalID string `json:"original_id,omitempty"`
}

type sharedGemmaReceipt struct {
	meta         sharedGemmaAttempt
	file, parent *os.File
	path, leaf   string
	existing     bool
}

func sendSharedGemmaAttempt(home, actor, key, prompt string, output io.Writer, send func(string) (string, error)) (string, error) {
	r, err := prepareSharedGemmaReceipt(home, actor, key, prompt)
	if err != nil {
		return "", err
	}
	defer r.close()
	if _, err := fmt.Fprintf(output, "%s; request-key=%s; exact search title=%q; receipt=%s\n", sharedGemmaPrivacy, key, r.meta.Title, r.path); err != nil {
		return "", err
	}
	if r.existing {
		return r.meta.OriginalID, fmt.Errorf("request key already retained: never resend; resume known original with --resume, otherwise reconcile the exact search title manually")
	}
	r.meta.State = "pending"
	if err := r.persist(); err != nil {
		return "", err
	}
	id, sendErr := send(r.meta.Title)
	r.meta.OriginalID = id
	if id != "" {
		r.meta.State = "submitted"
	}
	if err := r.persist(); err != nil {
		return id, fmt.Errorf("retain send outcome: %w (send error: %v); do not resend", err, sendErr)
	}
	return id, sendErr
}

func prepareSharedGemmaReceipt(home, actor, key, prompt string) (*sharedGemmaReceipt, error) {
	b, err := hex.DecodeString(key)
	if err != nil || len(b) != 16 || hex.EncodeToString(b) != key {
		return nil, fmt.Errorf("request key must be 32 lowercase hexadecimal characters")
	}
	root := filepath.Join(home, ".sirsi", "gemma-shared-attempts")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	pfd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open attempt directory: %w", err)
	}
	r := &sharedGemmaReceipt{parent: os.NewFile(uintptr(pfd), root), leaf: key + ".json", path: filepath.Join(root, key+".json")}
	fail := func(e error) (*sharedGemmaReceipt, error) { r.close(); return nil, e }
	ps, err := r.parent.Stat()
	if err != nil || ps.Mode().Perm() != 0700 {
		return fail(fmt.Errorf("attempt directory must be mode0700: %v", err))
	}
	digest := sha256.Sum256([]byte(prompt))
	r.meta = sharedGemmaAttempt{Key: key, Actor: actor, Digest: hex.EncodeToString(digest[:]), Title: "Gemma shared " + key + " " + hex.EncodeToString(digest[:]), FirstSend: time.Now().UTC().Format(time.RFC3339Nano), State: "prepared"}
	fd, err := unix.Openat(pfd, r.leaf, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err == unix.EEXIST {
		r.existing = true
		fd, err = unix.Openat(pfd, r.leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		return fail(fmt.Errorf("create/read attempt receipt: %w", err))
	}
	r.file = os.NewFile(uintptr(fd), r.path)
	fs, err := r.file.Stat()
	if err != nil || !fs.Mode().IsRegular() || fs.Mode().Perm() != 0600 {
		return fail(fmt.Errorf("attempt receipt must be regular mode0600: %v", err))
	}
	var identity unix.Stat_t
	if err := unix.Fstat(fd, &identity); err != nil || identity.Nlink != 1 {
		return fail(fmt.Errorf("attempt receipt must have one link: %v", err))
	}
	if r.existing {
		data, err := io.ReadAll(io.LimitReader(r.file, 4097))
		if err != nil || len(data) > 4096 {
			return fail(fmt.Errorf("invalid existing attempt receipt: %v", err))
		}
		var old sharedGemmaAttempt
		if err := json.Unmarshal(data, &old); err != nil {
			return fail(fmt.Errorf("unreadable attempt; do not resend: %w", err))
		}
		if old.Key != r.meta.Key || old.Actor != actor || old.Digest != r.meta.Digest || old.Title != r.meta.Title {
			return fail(fmt.Errorf("request key conflicts with retained actor/prompt; do not resend"))
		}
		r.meta = old
		return r, nil
	}
	if err := r.persist(); err != nil {
		return fail(err)
	}
	return r, nil
}

func (r *sharedGemmaReceipt) persist() error {
	// Never unlink or replace a public leaf on failure. Retain explicit debt.
	var original, named unix.Stat_t
	if err := unix.Fstat(int(r.file.Fd()), &original); err != nil {
		return err
	}
	if err := unix.Fstatat(int(r.parent.Fd()), r.leaf, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if original.Dev != named.Dev || original.Ino != named.Ino || named.Mode&unix.S_IFMT != unix.S_IFREG || named.Nlink != 1 {
		return fmt.Errorf("receipt identity changed; retained debt, do not send/resend")
	}
	ps, err := r.parent.Stat()
	if err != nil {
		return err
	}
	current, err := os.Stat(filepath.Dir(r.path))
	if err != nil || !os.SameFile(ps, current) {
		return fmt.Errorf("attempt parent changed; retained debt, do not send/resend")
	}
	data, err := json.Marshal(r.meta)
	if err != nil {
		return err
	}
	if err := r.file.Truncate(0); err != nil {
		return err
	}
	n, err := r.file.WriteAt(append(data, '\n'), 0)
	if err != nil {
		return err
	}
	if n != len(data)+1 {
		return io.ErrShortWrite
	}
	if err := r.file.Sync(); err != nil {
		return err
	}
	if err := r.parent.Sync(); err != nil {
		return err
	}
	if err := unix.Fstatat(int(r.parent.Fd()), r.leaf, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if original.Dev != named.Dev || original.Ino != named.Ino || named.Mode&unix.S_IFMT != unix.S_IFREG || named.Nlink != 1 {
		return fmt.Errorf("receipt changed during persistence; retained debt, do not send/resend")
	}
	current, err = os.Stat(filepath.Dir(r.path))
	if err != nil || !os.SameFile(ps, current) {
		return fmt.Errorf("attempt parent changed during persistence; do not send/resend")
	}
	return nil
}

func (r *sharedGemmaReceipt) close() {
	if r.file != nil {
		_ = r.file.Close()
	}
	if r.parent != nil {
		_ = r.parent.Close()
	}
}
