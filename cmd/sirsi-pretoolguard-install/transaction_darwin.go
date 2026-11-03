//go:build darwin

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const leaf = "sirsi-display-power-guard"
const maxArtifact = 128 << 20

const provenance = "com.apple.provenance"
const maxMetadata = 1 << 20
const maxJournalRecords = 8
const maxJournalBytes = 65536

func permittedXattrs(b []byte) (bool, error) {
	if len(b) == 0 {
		return false, nil
	}
	if b[len(b)-1] != 0 {
		return false, errors.New("malformed xattr list")
	}
	seen := false
	for _, name := range bytes.Split(b[:len(b)-1], []byte{0}) {
		if string(name) != provenance || seen {
			return false, errors.New("unknown or duplicate xattr")
		}
		seen = true
	}
	return seen, nil
}

// This parser is only valid for the exact single-attribute fgetattrlist query
// below: no RETURNED_ATTRS or PACK_INVAL_ATTRS, with REPORT_FULLSIZE. In XNU's
// corresponding compatibility path, unsupported vnode attributes cause an
// error, while a supported NULL va_acl is packed as a zero-length reference.
func validateACLResponse(buf []byte, queryErr error) error {
	if queryErr != nil {
		return fmt.Errorf("ACL query failed: %w", queryErr)
	}
	if len(buf) < 12 {
		return errors.New("invalid ACL response")
	}
	length := binary.LittleEndian.Uint32(buf[:4])
	offset := int32(binary.LittleEndian.Uint32(buf[4:8]))
	size := binary.LittleEndian.Uint32(buf[8:12])
	if length > uint32(len(buf)) || length < 12 || offset != 8 {
		return errors.New("invalid ACL response")
	}
	if size == 0 {
		if length != 12 {
			return errors.New("invalid empty ACL response")
		}
		return nil
	}
	// A 44-byte kauth_filesec containing KAUTH_FILESEC_NOACL is the other
	// explicit no-ACL representation. A zero-entry ACL is NOT no ACL.
	if size != 44 || length != 56 {
		return errors.New("ACL present or invalid ACL response")
	}
	sec := buf[12:56]
	if binary.LittleEndian.Uint32(sec[0:4]) != 0x012cc16d || binary.LittleEndian.Uint32(sec[36:40]) != ^uint32(0) || binary.LittleEndian.Uint32(sec[40:44]) != 0 {
		return errors.New("ACL cannot be retained")
	}
	return nil
}

// Provenance is read only. No metadata is copied, removed or rewritten.
// Unknown xattrs, flags, ACLs, unreadable metadata and races fail closed.
func readMetadata(dir int, name string) (metadata, error) {
	var result metadata
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return result, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return result, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Flags != 0 {
		return result, errors.New("non-regular file or flags cannot be retained")
	}
	n, err := unix.Flistxattr(fd, nil)
	if err != nil {
		return result, fmt.Errorf("xattr query failed: %w", err)
	}
	if n < 0 || n > maxMetadata {
		return result, errors.New("xattr list too large")
	}
	list := make([]byte, n+1)
	got, err := unix.Flistxattr(fd, list)
	if err != nil || got != n {
		return result, errors.New("xattr list changed or unreadable")
	}
	result.ProvenancePresent, err = permittedXattrs(list[:got])
	if err != nil {
		return result, err
	}
	if result.ProvenancePresent {
		size, e := unix.Fgetxattr(fd, provenance, nil)
		if e != nil || size < 0 || size > maxMetadata {
			return result, errors.New("provenance unreadable or too large")
		}
		value := make([]byte, size+1)
		got, e = unix.Fgetxattr(fd, provenance, value)
		if e != nil || got != size {
			return result, errors.New("provenance changed or unreadable")
		}
		sum := sha256.Sum256(value[:got])
		result.ProvenanceSHA256 = hex.EncodeToString(sum[:])
	}
	// Do not add RETURNED_ATTRS or PACK_INVAL_ATTRS: those change the
	// unsupported-attribute behavior on which zero-length interpretation rests.
	var buf [8192]byte
	if err = validateACLResponse(buf[:], queryACL(fd, buf[:])); err != nil {
		return result, err
	}
	var end unix.Stat_t
	if err = unix.Fstat(fd, &end); err != nil {
		return result, err
	}
	if st.Dev != end.Dev || st.Ino != end.Ino || st.Ctim != end.Ctim || st.Flags != end.Flags || st.Mode != end.Mode {
		return result, errors.New("metadata changed during read")
	}
	return result, nil
}

type record struct {
	Version           int      `json:"version"`
	Transaction       string   `json:"transaction"`
	Target            string   `json:"target"`
	Phase             string   `json:"phase"`
	Original          identity `json:"original"`
	Installed         identity `json:"installed"`
	Backup            identity `json:"backup"`
	OriginalMetadata  metadata `json:"original_metadata"`
	InstalledMetadata metadata `json:"installed_metadata"`
}

// All directory traversal below is relative to opened, no-follow directories.
// Same-UID writers must cooperate; this is not an adversarial same-UID lock.
func targetDirectory(selector string) (string, uint32, error) {
	ruid, euid := os.Getuid(), os.Geteuid()
	if ruid != euid {
		return "", 0, errors.New("real/effective UID mismatch")
	}
	if selector == "codex-system" {
		if ruid != 0 {
			return "", 0, errors.New("system target requires real root custody")
		}
		return "/private/etc/codex/hooks", 0, nil
	}
	if ruid == 0 {
		return "", 0, errors.New("user target must run as its real non-root account")
	}
	u, err := user.LookupId(strconv.Itoa(ruid))
	if err != nil || u.Uid != strconv.Itoa(ruid) || !filepath.IsAbs(u.HomeDir) || filepath.Clean(u.HomeDir) != u.HomeDir || u.HomeDir == "/" {
		return "", 0, errors.New("account home lookup failed")
	}
	part := ".codex"
	if selector == "claude-user" {
		part = ".claude"
	}
	return filepath.Join(u.HomeDir, part, "hooks"), uint32(ruid), nil
}

func openDirectory(path string, owner uint32) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if component == "" || component == "." || component == ".." {
			unix.Close(fd)
			return -1, errors.New("invalid directory component")
		}
		next, e := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
		var st unix.Stat_t
		if e = unix.Fstat(fd, &st); e != nil || (st.Uid != 0 && st.Uid != owner) || st.Mode&0022 != 0 {
			unix.Close(fd)
			return -1, errors.New("untrusted directory ownership or permissions")
		}
	}
	return fd, nil
}

func snapshot(dir int, name string) (identity, error) {
	var id identity
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return id, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	var a, b unix.Stat_t
	if err = unix.Fstat(fd, &a); err != nil {
		return id, err
	}
	if a.Mode&unix.S_IFMT != unix.S_IFREG || a.Nlink != 1 || a.Size < 0 || a.Size > maxArtifact {
		return id, errors.New("object must be bounded single-link regular file")
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, maxArtifact+1)); err != nil {
		return id, err
	}
	if err = unix.Fstat(fd, &b); err != nil {
		return id, err
	}
	if a.Dev != b.Dev || a.Ino != b.Ino || a.Size != b.Size || a.Mtim != b.Mtim || a.Ctim != b.Ctim || a.Uid != b.Uid || a.Gid != b.Gid || a.Mode != b.Mode || a.Nlink != b.Nlink || a.Flags != b.Flags {
		return id, errors.New("object changed during read")
	}
	return identity{uint64(a.Dev), a.Ino, a.Uid, a.Gid, uint32(a.Mode & 07777), hex.EncodeToString(h.Sum(nil))}, nil
}

func syncFile(fd int) error { _, err := unix.FcntlInt(uintptr(fd), unix.F_FULLFSYNC, 0); return err }
func syncDir(fd int) error  { return unix.Fsync(fd) }

func syncJournal(dir int) error {
	fd, err := unix.Openat(dir, "journal", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return syncFile(fd)
}

func reportTransaction(w io.Writer, id, path string) error {
	message := fmt.Sprintf("transaction=%s retained-directory=%s\n", id, path)
	n, err := io.WriteString(w, message)
	if err != nil {
		return err
	}
	if n != len(message) {
		return io.ErrShortWrite
	}
	return nil
}

func appendRecord(dir int, rec record) error {
	fd, err := unix.Openat(dir, "journal", unix.O_RDWR|unix.O_APPEND|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), "journal")
	defer f.Close()
	existing, err := io.ReadAll(io.LimitReader(f, maxJournalBytes+1))
	if err != nil || len(existing) > maxJournalBytes {
		return errors.New("journal unreadable or too large")
	}
	count := bytes.Count(existing, []byte{'\n'})
	if len(existing) > 0 {
		if _, err = readJournal(dir); err != nil {
			return err
		}
	} else if rec.Phase != "install-prepared" {
		return errors.New("journal must start prepared")
	}
	required := 1
	if rec.Phase == "install-prepared" || rec.Phase == "rollback-prepared" {
		required = 2
	}
	if count+required > maxJournalRecords {
		return errors.New("journal capacity exhausted; cannot reserve terminal record")
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	// Reserve byte capacity as well as a record slot for the terminal record.
	reserve := 0
	if required == 2 {
		reserve = len(b) + len("rollback-aborted")
	}
	if len(existing)+len(b)+reserve > maxJournalBytes {
		return errors.New("journal byte capacity exhausted")
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	return syncFile(fd)
}

func readJournal(dir int) (record, error) {
	var last record
	fd, err := unix.Openat(dir, "journal", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return last, err
	}
	f := os.NewFile(uintptr(fd), "journal")
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxJournalBytes+1))
	if err != nil || len(b) == 0 || len(b) > maxJournalBytes || b[len(b)-1] != '\n' {
		return record{}, errors.New("incomplete journal")
	}
	lines := bytes.Split(b[:len(b)-1], []byte{'\n'})
	if len(lines) > maxJournalRecords {
		return record{}, errors.New("invalid journal")
	}
	var first record
	for i, line := range lines {
		var next record
		var fields map[string]json.RawMessage
		if json.Unmarshal(line, &next) != nil || json.Unmarshal(line, &fields) != nil || !metadataJSONValid(fields["original_metadata"]) || !metadataJSONValid(fields["installed_metadata"]) {
			return record{}, errors.New("invalid journal")
		}
		if i == 0 {
			first = next
			if next.Phase != "install-prepared" {
				return record{}, errors.New("invalid initial journal phase")
			}
		} else {
			if next.Version != first.Version || next.Transaction != first.Transaction || next.Target != first.Target || next.Original != first.Original || next.Installed != first.Installed || next.Backup != first.Backup || next.OriginalMetadata != first.OriginalMetadata || next.InstalledMetadata != first.InstalledMetadata {
				return record{}, errors.New("journal identity changed")
			}
			valid := (last.Phase == "install-prepared" && (next.Phase == "installed" || next.Phase == "install-aborted")) || (last.Phase == "installed" && next.Phase == "rollback-prepared") || (last.Phase == "rollback-prepared" && (next.Phase == "rolled-back" || next.Phase == "rollback-aborted")) || (last.Phase == "rollback-aborted" && next.Phase == "rollback-prepared")
			if !valid {
				return record{}, errors.New("invalid journal transition")
			}
		}
		last = next
	}
	if first.Version != 2 || !digest.MatchString(first.Transaction) || first.Original != first.Backup || first.Original == first.Installed || first.Original.Inode == 0 || first.Installed.Inode == 0 {
		return record{}, errors.New("invalid journal identities")
	}
	return last, nil
}

func verifyMetadataPair(parent, private int, target, saved metadata) error {
	a, err := readMetadata(parent, leaf)
	if err != nil {
		return fmt.Errorf("target metadata: %w", err)
	}
	b, err := readMetadata(private, "object")
	if err != nil {
		return fmt.Errorf("retained metadata: %w", err)
	}
	return compareMetadataPair(a, b, target, saved)

}

func compareMetadataPair(a, b, target, saved metadata) error {
	if a != target || b != saved {
		return errors.New("provenance metadata changed; retain both objects")
	}
	return nil
}

func verifyPair(parent, private int, target, saved identity) error {
	a, err := snapshot(parent, leaf)
	if err != nil {
		return err
	}
	b, err := snapshot(private, "object")
	if err != nil {
		return err
	}
	if a != target || b != saved {
		return fmt.Errorf("identity mismatch; stop and retain both objects: target=%+v retained=%+v", a, b)
	}
	return nil
}

func execute(r request) error {
	path, owner, err := targetDirectory(r.target)
	if err != nil {
		return err
	}
	parent, err := openDirectory(path, owner)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	current, _ := parseIdentity(r.current)
	if current.UID != owner {
		return errors.New("target owner does not match custody")
	}
	if r.action == "rollback" || r.action == "recover" {
		return rollback(r, parent, owner, current)
	}
	return install(r, path, parent, owner, current)
}

func install(r request, path string, parent int, owner uint32, original identity) error {
	observed, err := snapshot(parent, leaf)
	if err != nil {
		return err
	}
	if observed != original {
		return errors.New("original identity mismatch")
	}
	originalMeta, err := readMetadata(parent, leaf)
	if err != nil {
		return fmt.Errorf("original metadata: %w", err)
	}
	if err = checkExpectedProvenance(r.originalProvenance, originalMeta); err != nil {
		return err
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	id := hex.EncodeToString(random[:])
	name := ".sirsi-pretoolguard-" + id
	if err = unix.Mkdirat(parent, name, 0700); err != nil {
		return err
	}
	if err = reportTransaction(os.Stderr, id, filepath.Join(path, name)); err != nil {
		return fmt.Errorf("transaction identification write failed; no staging attempted: %w", err)
	}
	if err = syncDir(parent); err != nil {
		return err
	}
	private, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(private)
	journal, err := unix.Openat(private, "journal", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	if err = syncFile(journal); err != nil {
		unix.Close(journal)
		return err
	}
	unix.Close(journal)
	if err = syncDir(private); err != nil {
		return err
	}
	src, err := unix.Open(r.artifact, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	input := os.NewFile(uintptr(src), "artifact")
	defer input.Close()
	var st unix.Stat_t
	if unix.Fstat(src, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Size < 0 || st.Size > maxArtifact {
		return errors.New("invalid artifact")
	}
	fd, err := unix.Openat(private, "object", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	out := os.NewFile(uintptr(fd), "object")
	n, copyErr := io.Copy(out, io.LimitReader(input, maxArtifact+1))
	if copyErr == nil && n > maxArtifact {
		copyErr = errors.New("artifact too large")
	}
	if copyErr == nil {
		copyErr = unix.Fchown(fd, int(original.UID), int(original.GID))
	}
	if copyErr == nil {
		copyErr = unix.Fchmod(fd, original.Mode)
	}
	if copyErr == nil {
		copyErr = syncFile(fd)
	}
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	if err = syncDir(private); err != nil {
		return err
	}
	staged, err := snapshot(private, "object")
	if err != nil {
		return err
	}
	if staged.SHA256 != r.replacement || staged.Device != original.Device || staged.UID != original.UID || staged.GID != original.GID || staged.Mode != original.Mode {
		return errors.New("staged artifact identity mismatch")
	}
	requirement := fmt.Sprintf(`identifier "%s" and anchor apple generic and certificate leaf[subject.OU] = "%s" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists`, r.identifier, r.team)
	cmd := exec.Command("/usr/bin/codesign", "--verify", "--strict", "--test-requirement", "="+requirement, filepath.Join(path, name, "object"))
	if output, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("signature verification failed: %w: %s", e, output)
	}
	if err = verifyPair(parent, private, original, staged); err != nil {
		return err
	}
	stagedMeta, err := readMetadata(private, "object")
	if err != nil {
		return fmt.Errorf("replacement metadata: %w", err)
	}
	if err = verifyMetadataPair(parent, private, originalMeta, stagedMeta); err != nil {
		return err
	}
	if err = verifyPair(parent, private, original, staged); err != nil {
		return err
	}
	rec := record{Version: 2, Transaction: id, Target: r.target, Phase: "install-prepared", Original: original, Installed: staged, Backup: original, OriginalMetadata: originalMeta, InstalledMetadata: stagedMeta}
	if err = appendRecord(private, rec); err != nil {
		return err
	}
	if err = unix.RenameatxNp(parent, leaf, private, "object", unix.RENAME_SWAP); err != nil {
		return err
	}
	if err = verifyPair(parent, private, staged, original); err != nil {
		return err
	}
	if err = verifyMetadataPair(parent, private, stagedMeta, originalMeta); err != nil {
		return err
	}
	if err = verifyPair(parent, private, staged, original); err != nil {
		return err
	}
	if err = syncDir(parent); err != nil {
		return err
	}
	if err = syncDir(private); err != nil {
		return err
	}
	rec.Phase = "installed"
	if err = appendRecord(private, rec); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(rec)
}

func rollback(r request, parent int, owner uint32, current identity) error {
	name := ".sirsi-pretoolguard-" + r.transaction
	private, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(private)
	var st unix.Stat_t
	if unix.Fstat(private, &st) != nil || st.Uid != owner || st.Mode&0777 != 0700 {
		return errors.New("private directory custody mismatch")
	}
	rec, err := readJournal(private)
	if err != nil {
		return err
	}
	backup, _ := parseIdentity(r.backup)
	if rec.Version != 2 || rec.Transaction != r.transaction || rec.Target != r.target {
		return errors.New("transaction journal mismatch")
	}
	if r.action == "recover" {
		return recoverPrepared(parent, private, rec, current, backup)
	}
	if (rec.Phase != "installed" && rec.Phase != "rollback-aborted") || rec.Installed != current || rec.Backup != backup || rec.Original != backup {
		return errors.New("rollback journal or expected identities mismatch")
	}
	if err = verifyPair(parent, private, current, backup); err != nil {
		return err
	}
	if err = verifyMetadataPair(parent, private, rec.InstalledMetadata, rec.OriginalMetadata); err != nil {
		return err
	}
	if err = verifyPair(parent, private, current, backup); err != nil {
		return err
	}
	rec.Phase = "rollback-prepared"
	if err = appendRecord(private, rec); err != nil {
		return err
	}
	if err = unix.RenameatxNp(parent, leaf, private, "object", unix.RENAME_SWAP); err != nil {
		return err
	}
	if err = verifyPair(parent, private, backup, current); err != nil {
		return err
	}
	if err = verifyMetadataPair(parent, private, rec.OriginalMetadata, rec.InstalledMetadata); err != nil {
		return err
	}
	if err = verifyPair(parent, private, backup, current); err != nil {
		return err
	}
	if err = syncDir(parent); err != nil {
		return err
	}
	if err = syncDir(private); err != nil {
		return err
	}
	rec.Phase = "rolled-back"
	if err = appendRecord(private, rec); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(rec)
}

// Recovery never swaps. It proves which pair currently exists, syncs both
// directories, then records the corresponding outcome. A torn journal is not
// repaired automatically; an append/sync failure remains an uncertain result.
func recoverPrepared(parent, private int, rec record, current, retained identity) error {
	var phase string
	switch rec.Phase {
	case "install-prepared":
		if current == rec.Original && retained == rec.Installed {
			phase = "install-aborted"
		}
		if current == rec.Installed && retained == rec.Original {
			phase = "installed"
		}
	case "rollback-prepared":
		if current == rec.Installed && retained == rec.Original {
			phase = "rollback-aborted"
		}
		if current == rec.Original && retained == rec.Installed {
			phase = "rolled-back"
		}
	case "installed", "rollback-aborted":
		if current == rec.Installed && retained == rec.Original {
			phase = rec.Phase
		}
	case "install-aborted", "rolled-back":
		if current == rec.Original && retained == rec.Installed {
			phase = rec.Phase
		}
	default:
		return errors.New("journal phase cannot be recovered")
	}
	if phase == "" {
		return errors.New("expected identities do not describe the journal outcome")
	}
	if err := verifyPair(parent, private, current, retained); err != nil {
		return err
	}
	targetMeta, retainedMeta := rec.OriginalMetadata, rec.InstalledMetadata
	if current == rec.Installed {
		targetMeta, retainedMeta = retainedMeta, targetMeta
	}
	if err := verifyMetadataPair(parent, private, targetMeta, retainedMeta); err != nil {
		return err
	}
	if err := syncDir(parent); err != nil {
		return err
	}
	if err := syncDir(private); err != nil {
		return err
	}
	if err := verifyPair(parent, private, current, retained); err != nil {
		return err
	}
	if err := verifyMetadataPair(parent, private, targetMeta, retainedMeta); err != nil {
		return err
	}
	if rec.Phase == phase {
		if err := syncJournal(private); err != nil {
			return err
		}
		last, err := readJournal(private)
		if err != nil {
			return err
		}
		if last != rec {
			return errors.New("journal changed during terminal acknowledgement")
		}
		if err = verifyPair(parent, private, current, retained); err != nil {
			return err
		}
		if err = verifyMetadataPair(parent, private, targetMeta, retainedMeta); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(rec)
	}
	rec.Phase = phase
	if err := appendRecord(private, rec); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(rec)
}
