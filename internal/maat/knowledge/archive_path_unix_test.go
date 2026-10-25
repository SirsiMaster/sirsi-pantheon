//go:build !windows

package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestPreparedArchiveRefusesLeafAndParentReplacementBeforeSQLiteOpen(t *testing.T) {
	t.Run("leaf symlink to moved original", func(t *testing.T) {
		home := t.TempDir()
		archive, err := Open(home)
		if err != nil {
			t.Fatal(err)
		}
		item := archiveRecord("claude", "profile", "continuity-session", "retained before path replacement")
		if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}

		location, err := prepareSecureArchiveLocation(home)
		if err != nil {
			t.Fatal(err)
		}
		defer location.Close()
		moved := filepath.Join(t.TempDir(), "moved-original.sqlite3")
		if err := os.Rename(location.path, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(moved, location.path); err != nil {
			t.Fatal(err)
		}
		before := fileDigest(t, moved)
		if _, err := openPreparedArchive(location); err == nil {
			t.Fatal("open accepted a leaf symlink to the moved canonical database")
		}
		if got := fileDigest(t, moved); got != before {
			t.Fatal("refused leaf substitution changed the moved database")
		}
	})

	t.Run("leaf hardlink to unrelated file", func(t *testing.T) {
		home := t.TempDir()
		archive, err := Open(home)
		if err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		location, err := prepareSecureArchiveLocation(home)
		if err != nil {
			t.Fatal(err)
		}
		defer location.Close()
		moved := filepath.Join(t.TempDir(), "moved-original.sqlite3")
		if err := os.Rename(location.path, moved); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(t.TempDir(), "unrelated-sentinel")
		if err := os.WriteFile(sentinel, []byte("outside sentinel"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(sentinel, location.path); err != nil {
			t.Fatal(err)
		}
		beforeSentinel := fileDigest(t, sentinel)
		beforeMoved := fileDigest(t, moved)
		if _, err := openPreparedArchive(location); err == nil {
			t.Fatal("open accepted a database hardlink to an unrelated file")
		}
		if fileDigest(t, sentinel) != beforeSentinel || fileDigest(t, moved) != beforeMoved {
			t.Fatal("refused hardlink substitution changed the sentinel or moved database")
		}
	})

	t.Run("parent replaced by symlink", func(t *testing.T) {
		home := t.TempDir()
		archive, err := Open(home)
		if err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		location, err := prepareSecureArchiveLocation(home)
		if err != nil {
			t.Fatal(err)
		}
		defer location.Close()
		knowledgeDir := filepath.Dir(location.path)
		outside := t.TempDir()
		movedOriginal := filepath.Join(outside, "moved-knowledge")
		if err := os.Rename(knowledgeDir, movedOriginal); err != nil {
			t.Fatal(err)
		}
		movedArchive := filepath.Join(movedOriginal, "archive.sqlite3")
		beforeMoved := fileDigest(t, movedArchive)
		replacement := filepath.Join(outside, "replacement")
		if err := os.Mkdir(replacement, 0o700); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(replacement, "archive.sqlite3")
		if err := os.WriteFile(sentinel, []byte("outside sentinel"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(replacement, knowledgeDir); err != nil {
			t.Fatal(err)
		}
		before := fileDigest(t, sentinel)
		if _, err := openPreparedArchive(location); err == nil {
			t.Fatal("open accepted a replaced archive parent")
		}
		if got := fileDigest(t, sentinel); got != before {
			t.Fatal("refused parent substitution changed the outside sentinel")
		}
		if got := fileDigest(t, movedArchive); got != beforeMoved {
			t.Fatal("refused parent substitution changed the moved original archive")
		}
	})
}

func TestPreparedArchiveRechecksNamespaceAfterSQLiteDescriptorOpen(t *testing.T) {
	home := t.TempDir()
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	location, err := prepareSecureArchiveLocation(home)
	if err != nil {
		t.Fatal(err)
	}
	defer location.Close()

	outside := t.TempDir()
	movedOriginal := filepath.Join(outside, "moved-original.sqlite3")
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("outside sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeSentinel := fileDigest(t, sentinel)
	beforeOriginal := fileDigest(t, location.path)
	_, err = openPreparedArchiveWithHook(location, func() error {
		if err := os.Rename(location.path, movedOriginal); err != nil {
			return err
		}
		return os.Symlink(sentinel, location.path)
	})
	if err == nil {
		t.Fatal("SQLite open accepted a namespace substitution before its first query")
	}
	if got := fileDigest(t, sentinel); got != beforeSentinel {
		t.Fatal("namespace substitution changed the unrelated sentinel")
	}
	if got := fileDigest(t, movedOriginal); got != beforeOriginal {
		t.Fatal("namespace substitution changed the moved archive before refusal")
	}
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if _, statErr := os.Lstat(movedOriginal + suffix); !os.IsNotExist(statErr) {
			t.Fatalf("SQLite created a path-derived sidecar after namespace substitution: %s", suffix)
		}
	}
}

func TestPreparedArchiveRefusesParentSubstitutionAfterSQLiteDescriptorOpen(t *testing.T) {
	home := t.TempDir()
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	location, err := prepareSecureArchiveLocation(home)
	if err != nil {
		t.Fatal(err)
	}
	defer location.Close()
	parent := filepath.Dir(location.path)
	outside := t.TempDir()
	movedParent := filepath.Join(outside, "moved-knowledge")
	replacement := filepath.Join(outside, "replacement")
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(replacement, "archive.sqlite3")
	if err := os.WriteFile(sentinel, []byte("unrelated sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeSentinel := fileDigest(t, sentinel)
	beforeOriginal := fileDigest(t, location.path)
	_, err = openPreparedArchiveWithHook(location, func() error {
		if err := os.Rename(parent, movedParent); err != nil {
			return err
		}
		return os.Symlink(replacement, parent)
	})
	if err == nil {
		t.Fatal("SQLite open accepted parent substitution before its first query")
	}
	if fileDigest(t, sentinel) != beforeSentinel || fileDigest(t, filepath.Join(movedParent, "archive.sqlite3")) != beforeOriginal {
		t.Fatal("parent substitution changed the sentinel or moved archive")
	}
}

func TestArchiveReopenPreservesExistingHistory(t *testing.T) {
	home := t.TempDir()
	item := archiveRecord("claude", "profile", "reopen-session", "history survives a normal close and reopen")
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	archive, err = Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	got, err := archive.Detail(context.Background(), recordID(item), 0)
	if err != nil || got.Content != item.Content {
		t.Fatalf("existing archive continuity = %+v, %v", got, err)
	}
}

func TestArchiveUsesDurableSQLiteJournal(t *testing.T) {
	archive, _ := openTestArchive(t)
	var journalMode string
	if err := archive.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil || journalMode != "delete" {
		t.Fatalf("journal_mode = %q, %v; want delete", journalMode, err)
	}
	var synchronous int
	if err := archive.db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil || synchronous != 2 {
		t.Fatalf("synchronous = %d, %v; want FULL (2)", synchronous, err)
	}
	if runtime.GOOS == "darwin" {
		var fullfsync int
		if err := archive.db.QueryRow(`PRAGMA fullfsync`).Scan(&fullfsync); err != nil || fullfsync != 1 {
			t.Fatalf("fullfsync = %d, %v; want enabled on macOS", fullfsync, err)
		}
	}
}

func TestArchiveRecoversProcessInterruptedTransaction(t *testing.T) {
	const crashHomeEnv = "SIRSI_TEST_MAAT_CRASH_HOME"
	if home := os.Getenv(crashHomeEnv); home != "" {
		archive, err := Open(home)
		if err != nil {
			os.Exit(2)
		}
		conn, err := archive.db.Conn(context.Background())
		if err != nil {
			os.Exit(3)
		}
		if _, err := conn.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
			os.Exit(4)
		}
		if _, err := conn.ExecContext(context.Background(), `UPDATE archive_meta SET value='interrupted' WHERE key='schema'`); err != nil {
			os.Exit(5)
		}
		journalPath := filepath.Join(home, ".sirsi", "maat", "knowledge", "archive.sqlite3-journal")
		journal, err := os.Stat(journalPath)
		if err != nil || !journal.Mode().IsRegular() || journal.Size() == 0 {
			os.Exit(6)
		}
		// Deliberately bypass database/connection cleanup to model process loss
		// with a hot rollback journal. This is not a power-loss simulation.
		os.Exit(0)
	}

	archive, home := openTestArchive(t)
	item := archiveRecord("claude", "profile", "crash-recovery", "committed history survives interrupted transaction")
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestArchiveRecoversProcessInterruptedTransaction$")
	command.Env = append(os.Environ(), crashHomeEnv+"="+home)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("interrupted transaction fixture failed (exit %v): %s", err, strings.TrimSpace(string(output)))
	}

	recovered, err := Open(home)
	if err != nil {
		t.Fatalf("reopen after interrupted transaction: %v", err)
	}
	defer recovered.Close()
	var schema string
	if err := recovered.db.QueryRow(`SELECT value FROM archive_meta WHERE key='schema'`).Scan(&schema); err != nil || schema != archiveSchemaVersion {
		t.Fatalf("recovered schema = %q, %v", schema, err)
	}
	got, err := recovered.Detail(context.Background(), recordID(item), 0)
	if err != nil || got.Content != item.Content {
		t.Fatalf("committed history after recovery = %+v, %v", got, err)
	}
}

func TestArchivePathRejectsLinkedSQLiteSidecars(t *testing.T) {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		t.Run(suffix, func(t *testing.T) {
			home := t.TempDir()
			archive, err := Open(home)
			if err != nil {
				t.Fatal(err)
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			database := filepath.Join(home, ".sirsi", "maat", "knowledge", "archive.sqlite3")
			sentinel := filepath.Join(t.TempDir(), "sentinel")
			if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(sentinel, database+suffix); err != nil {
				t.Fatal(err)
			}
			before := fileDigest(t, sentinel)
			if _, err := Open(home); err == nil {
				t.Fatal("archive accepted a linked SQLite sidecar")
			}
			if got := fileDigest(t, sentinel); got != before {
				t.Fatal("sidecar refusal changed unrelated target")
			}
		})
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", filepath.Base(path), err)
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
