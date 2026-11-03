//go:build darwin

package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestACLAttributeResponseABI(t *testing.T) {
	// Synthetic buffers only: length, attrreference, optional kauth_filesec.
	noACL := make([]byte, 12)
	binary.LittleEndian.PutUint32(noACL[0:4], 12)
	binary.LittleEndian.PutUint32(noACL[4:8], 8)
	sentinel := make([]byte, 56)
	binary.LittleEndian.PutUint32(sentinel[0:4], 56)
	binary.LittleEndian.PutUint32(sentinel[4:8], 8)
	binary.LittleEndian.PutUint32(sentinel[8:12], 44)
	binary.LittleEndian.PutUint32(sentinel[12:16], 0x012cc16d)
	binary.LittleEndian.PutUint32(sentinel[48:52], ^uint32(0))
	clone := func(b []byte) []byte { return append([]byte(nil), b...) }
	zeroEntry := clone(sentinel)
	binary.LittleEndian.PutUint32(zeroEntry[48:52], 0)
	populated := make([]byte, 80)
	copy(populated, sentinel)
	binary.LittleEndian.PutUint32(populated[0:4], 80)
	binary.LittleEndian.PutUint32(populated[8:12], 68)
	binary.LittleEndian.PutUint32(populated[48:52], 1)
	flagged := clone(sentinel)
	binary.LittleEndian.PutUint32(flagged[52:56], 1)
	badMagic := clone(sentinel)
	binary.LittleEndian.PutUint32(badMagic[12:16], 0)
	badOffset := clone(noACL)
	binary.LittleEndian.PutUint32(badOffset[4:8], 0)
	extra := clone(noACL)
	binary.LittleEndian.PutUint32(extra[0:4], 16)
	truncated := clone(sentinel)
	binary.LittleEndian.PutUint32(truncated[0:4], 57)
	for _, tc := range []struct {
		name string
		b    []byte
		err  error
		ok   bool
	}{
		{"supported null ACL", noACL, nil, true},
		{"no ACL sentinel", sentinel, nil, true},
		{"unsupported query", noACL, unix.EINVAL, false},
		{"permission error", noACL, unix.EACCES, false},
		{"empty ACL is not absent", zeroEntry, nil, false},
		{"populated ACL", populated, nil, false},
		{"flags", flagged, nil, false},
		{"bad magic", badMagic, nil, false},
		{"bad offset", badOffset, nil, false},
		{"trailing or truncated empty", extra, nil, false},
		{"reported truncated", truncated, nil, false},
		{"short buffer", noACL[:11], nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateACLResponse(tc.b, tc.err)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

// These fixtures create only inert text files below t.TempDir. They do not call
// execute, install, codesign, a guard, or any real hook path.
func TestFixtureSwapRollbackAndMismatch(t *testing.T) {
	root := t.TempDir()
	privatePath := filepath.Join(root, "private")
	if err := os.Mkdir(privatePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, leaf), []byte("original fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(privatePath, "object"), []byte("replacement fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(privatePath, "journal"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(p)
	d, err := unix.Open(privatePath, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(d)
	original, err := snapshot(p, leaf)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := snapshot(d, "object")
	if err != nil {
		t.Fatal(err)
	}
	rec := record{Version: 2, Transaction: strings.Repeat("a", 64), Target: "codex-user", Phase: "install-prepared", Original: original, Installed: staged, Backup: original}
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	if err = unix.RenameatxNp(p, leaf, d, "object", unix.RENAME_SWAP); err != nil {
		t.Fatal(err)
	}
	if err = verifyPair(p, d, staged, original); err != nil {
		t.Fatal(err)
	}
	if err = verifyPair(p, d, original, staged); err == nil {
		t.Fatal("mismatched pair accepted")
	}
	rec.Phase = "installed"
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	got, err := readJournal(d)
	if err != nil || got != rec {
		t.Fatalf("journal: %+v %v", got, err)
	}
	rec.Phase = "rollback-prepared"
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	if err = unix.RenameatxNp(p, leaf, d, "object", unix.RENAME_SWAP); err != nil {
		t.Fatal(err)
	}
	if err = verifyPair(p, d, original, staged); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(privatePath, "journal"), []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readJournal(d); err == nil {
		t.Fatal("torn journal accepted")
	}
}

func TestFixtureExplicitRollback(t *testing.T) {
	if !aclInspectionAvailable {
		t.Skip("real ACL metadata inspection requires cgo")
	}
	root := t.TempDir()
	id := strings.Repeat("c", 64)
	dir := filepath.Join(root, ".sirsi-pretoolguard-"+id)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, leaf), []byte("installed fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "object"), []byte("original fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journal"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(p)
	d, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(d)
	installed, err := snapshot(p, leaf)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := snapshot(d, "object")
	if err != nil {
		t.Fatal(err)
	}
	rec := record{Version: 2, Transaction: id, Target: "codex-user", Phase: "installed", Original: backup, Installed: installed, Backup: backup}
	rec.InstalledMetadata, err = readMetadata(p, leaf)
	if err != nil {
		t.Fatal(err)
	}
	rec.OriginalMetadata, err = readMetadata(d, "object")
	if err != nil {
		t.Fatal(err)
	}
	rec.Phase = "install-prepared"
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	rec.Phase = "installed"
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(backup)
	r := request{action: "rollback", target: "codex-user", transaction: id, backup: string(b)}
	wrong := installed
	wrong.Inode++
	if err = rollback(r, p, uint32(os.Getuid()), wrong); err == nil {
		t.Fatal("wrong installed identity accepted")
	}
	if err = verifyPair(p, d, installed, backup); err != nil {
		t.Fatal(err)
	}
	if err = rollback(r, p, uint32(os.Getuid()), installed); err != nil {
		t.Fatal(err)
	}
	if err = verifyPair(p, d, backup, installed); err != nil {
		t.Fatal(err)
	}
	got, err := readJournal(d)
	if err != nil || got.Phase != "rolled-back" {
		t.Fatalf("journal: %+v %v", got, err)
	}
	if err = rollback(r, p, uint32(os.Getuid()), installed); err == nil {
		t.Fatal("repeat rollback accepted")
	}
}

// Recovery observes fixtures only; it never invokes the installer or a hook.
func TestPreparedRecoveryOutcomes(t *testing.T) {
	if !aclInspectionAvailable {
		t.Skip("real ACL metadata inspection requires cgo")
	}
	for _, tc := range []struct {
		phase   string
		swapped bool
		want    string
	}{
		{"install-prepared", false, "install-aborted"},
		{"install-prepared", true, "installed"},
		{"rollback-prepared", false, "rollback-aborted"},
		{"rollback-prepared", true, "rolled-back"},
	} {
		t.Run(tc.phase+tc.want, func(t *testing.T) {
			root := t.TempDir()
			private := filepath.Join(root, "private")
			if err := os.Mkdir(private, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, leaf), []byte("one"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(private, "object"), []byte("two"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(private, "journal"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			p, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(p)
			d, err := unix.Open(private, unix.O_RDONLY|unix.O_DIRECTORY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(d)
			a, err := snapshot(p, leaf)
			if err != nil {
				t.Fatal(err)
			}
			b, err := snapshot(d, "object")
			if err != nil {
				t.Fatal(err)
			}
			original, installed := a, b
			if (tc.phase == "install-prepared" && tc.swapped) || (tc.phase == "rollback-prepared" && !tc.swapped) {
				original, installed = b, a
			}
			rec := record{Version: 2, Transaction: strings.Repeat("d", 64), Target: "codex-user", Phase: "install-prepared", Original: original, Installed: installed, Backup: original}
			am, err := readMetadata(p, leaf)
			if err != nil {
				t.Fatal(err)
			}
			bm, err := readMetadata(d, "object")
			if err != nil {
				t.Fatal(err)
			}
			rec.OriginalMetadata, rec.InstalledMetadata = am, bm
			if original == b {
				rec.OriginalMetadata, rec.InstalledMetadata = bm, am
			}
			if err = appendRecord(d, rec); err != nil {
				t.Fatal(err)
			}
			if tc.phase == "rollback-prepared" {
				rec.Phase = "installed"
				if err = appendRecord(d, rec); err != nil {
					t.Fatal(err)
				}
				rec.Phase = "rollback-prepared"
				if err = appendRecord(d, rec); err != nil {
					t.Fatal(err)
				}
			}
			wrong := a
			wrong.Inode++
			if err = recoverPrepared(p, d, rec, wrong, b); err == nil {
				t.Fatal("incorrect proof accepted")
			}
			if err = recoverPrepared(p, d, rec, a, b); err != nil {
				t.Fatal(err)
			}
			got, err := readJournal(d)
			if err != nil || got.Phase != tc.want {
				t.Fatalf("got %+v: %v", got, err)
			}
			if err = recoverPrepared(p, d, got, wrong, b); err == nil {
				t.Fatal("terminal recovery accepted wrong proof")
			}
			if err = recoverPrepared(p, d, got, a, b); err != nil {
				t.Fatal("terminal acknowledgement:", err)
			}
			again, err := readJournal(d)
			if err != nil || again != got {
				t.Fatal("terminal acknowledgement changed journal", err)
			}
			if err = verifyPair(p, d, a, b); err != nil {
				t.Fatal("recovery changed fixture pair:", err)
			}
		})
	}
}

type fixtureWriter struct {
	n   int
	err error
}

func (w fixtureWriter) Write(p []byte) (int, error) {
	if w.n < 0 {
		return len(p), w.err
	}
	return w.n, w.err
}

func TestTransactionAnnouncementFailures(t *testing.T) {
	if reportTransaction(fixtureWriter{n: 0, err: errors.New("fixture failure")}, "id", "/inert") == nil {
		t.Fatal("write failure accepted")
	}
	if !errors.Is(reportTransaction(fixtureWriter{n: 1}, "id", "/inert"), io.ErrShortWrite) {
		t.Fatal("short write accepted")
	}
	if err := reportTransaction(fixtureWriter{n: -1}, "id", "/inert"); err != nil {
		t.Fatal(err)
	}
}

func TestJournalCapacityReservesTerminal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "journal"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	d, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(d)
	a := identity{Device: 1, Inode: 1, SHA256: strings.Repeat("a", 64)}
	b := identity{Device: 1, Inode: 2, SHA256: strings.Repeat("b", 64)}
	rec := record{Version: 2, Transaction: strings.Repeat("f", 64), Target: "codex-user", Phase: "install-prepared", Original: a, Backup: a, Installed: b}
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	rec.Phase = "installed"
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		rec.Phase = "rollback-prepared"
		if err = appendRecord(d, rec); err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			if err = appendRecord(d, rec); err == nil {
				t.Fatal("prepared append consumed terminal reservation")
			}
		}
		rec.Phase = "rollback-aborted"
		if err = appendRecord(d, rec); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(filepath.Join(root, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	rec.Phase = "rollback-prepared"
	if err = appendRecord(d, rec); err == nil {
		t.Fatal("ninth record accepted")
	}
	after, err := os.ReadFile(filepath.Join(root, "journal"))
	if err != nil || string(after) != string(before) {
		t.Fatal("capacity refusal changed journal", err)
	}
	if _, err = readJournal(d); err != nil {
		t.Fatal("full journal became unreadable", err)
	}
}

func TestMetadataXattrFailsClosed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "object")
	if err := os.WriteFile(path, []byte("inert"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(path, "org.sirsi.fixture", []byte("metadata"), 0); err != nil {
		t.Fatal(err)
	}
	d, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(d)
	if _, err = readMetadata(d, "object"); err == nil {
		t.Fatal("xattr accepted")
	}
}

// Synthetic lists exercise policy without attempting to create or modify the
// protected system attribute, even on a temporary file.
func TestProvenanceAllowlistAndJournalProof(t *testing.T) {
	for _, tc := range []struct {
		input       string
		present, ok bool
	}{
		{"", false, true},
		{"com.apple.provenance\x00", true, true},
		{"com.apple.provenance", false, false},
		{"com.apple.provenance\x00unknown\x00", false, false},
		{"com.apple.provenance\x00com.apple.provenance\x00", false, false},
	} {
		present, err := permittedXattrs([]byte(tc.input))
		if (err == nil) != tc.ok || present != tc.present {
			t.Fatalf("list %q: present=%v err=%v", tc.input, present, err)
		}
	}
	good := metadata{ProvenancePresent: true, ProvenanceSHA256: strings.Repeat("a", 64)}
	b, _ := json.Marshal(good)
	if !metadataJSONValid(b) {
		t.Fatal("valid proof rejected")
	}
	for _, bad := range []string{`{}`, `{"provenance_present":true,"provenance_sha256":""}`, `{"provenance_present":false,"provenance_sha256":"` + strings.Repeat("a", 64) + `"}`} {
		if metadataJSONValid([]byte(bad)) {
			t.Fatalf("invalid proof accepted: %s", bad)
		}
	}
	changed := good
	changed.ProvenanceSHA256 = strings.Repeat("b", 64)
	if compareMetadataPair(good, metadata{}, good, metadata{}) != nil {
		t.Fatal("unchanged proof rejected")
	}
	if compareMetadataPair(changed, metadata{}, good, metadata{}) == nil || compareMetadataPair(metadata{}, changed, metadata{}, good) == nil || compareMetadataPair(metadata{}, metadata{}, good, metadata{}) == nil {
		t.Fatal("changed or missing provenance accepted")
	}
}

func TestJournalRejectsChangedProvenanceProof(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "journal"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	d, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(d)
	a := identity{Device: 1, Inode: 1, SHA256: strings.Repeat("a", 64)}
	b := identity{Device: 1, Inode: 2, SHA256: strings.Repeat("b", 64)}
	rec := record{Version: 2, Transaction: strings.Repeat("f", 64), Target: "codex-user", Phase: "install-prepared", Original: a, Backup: a, Installed: b, OriginalMetadata: metadata{ProvenancePresent: true, ProvenanceSHA256: strings.Repeat("c", 64)}}
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	rec.Phase = "installed"
	rec.OriginalMetadata.ProvenanceSHA256 = strings.Repeat("d", 64)
	if err = appendRecord(d, rec); err != nil {
		t.Fatal(err)
	}
	if _, err = readJournal(d); err == nil {
		t.Fatal("changed journal proof accepted")
	}
}
