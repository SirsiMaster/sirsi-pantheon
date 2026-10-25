//go:build darwin && cgo

package knowledge

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// Closing any descriptor for the database inode must not drop another Archive's
// process-owned POSIX locks. A second connection in this process cannot prove
// that invariant: the contender must be a separate process.
func TestArchiveClosingOtherHandlePreservesCrossProcessWriteLock(t *testing.T) {
	a, home := openTestArchive(t)
	if _, err := a.db.Exec(`INSERT INTO archive_meta(key,value) VALUES ('lock-close-fixture','initial')`); err != nil {
		t.Fatal(err)
	}
	b, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	bClosed := false
	defer func() {
		if !bClosed {
			_ = b.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var writeTx *sql.Tx
	defer func() {
		if writeTx != nil {
			_ = writeTx.Rollback()
		}
	}()

	// The child opens first: Open initializes schema, so opening under A's
	// lock would turn SQLITE_BUSY into a generic initialization error instead
	// of testing a real competing write. Readiness is an explicit pipe barrier.
	runArchiveLockCloseChild(t, home, "busy", func() {
		writeTx, err = a.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writeTx.ExecContext(ctx, `UPDATE archive_meta SET value='held-a' WHERE key='lock-close-fixture'`); err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
		bClosed = true
	})
	var held string
	if err := writeTx.QueryRowContext(ctx, `SELECT value FROM archive_meta WHERE key='lock-close-fixture'`).Scan(&held); err != nil || held != "held-a" {
		t.Fatalf("held transaction changed: value=%q err=%v", held, err)
	}
	if err := writeTx.Commit(); err != nil {
		t.Fatal(err)
	}
	writeTx = nil

	// A fresh process must now commit, not merely time out or return an unknown
	// error. This also prevents a permanently refused writer from passing.
	runArchiveLockCloseChild(t, home, "commit", nil)
	var final string
	if err := a.db.QueryRowContext(ctx, `SELECT value FROM archive_meta WHERE key='lock-close-fixture'`).Scan(&final); err != nil || final != "child-commit" {
		t.Fatalf("fresh child write not retained: value=%q err=%v", final, err)
	}
}

func runArchiveLockCloseChild(t *testing.T, home, mode string, afterReady func()) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestArchiveLockCloseChildHelper$", "-test.count=1")
	// No inherited helper selectors, user archive configuration or router state.
	// This package has no CLI TestMain and needs no MAAT_SUBMIT_HELPER bypass.
	cmd.Env = []string{"HOME=" + home, "TMPDIR=" + t.TempDir(), "SIRSI_ARCHIVE_LOCK_CHILD_HOME=" + home, "SIRSI_ARCHIVE_LOCK_CHILD_MODE=" + mode}
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		cancel() // Only this owned inert child; never a process group or service.
		if !waited {
			_ = cmd.Wait()
		}
	}()
	// Closing the read end on deadline also bounds a stuck readiness read;
	// there is no detached goroutine left waiting on a child pipe.
	stopRead := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	defer stopRead()
	reader := bufio.NewReader(stdout)
	ready, err := reader.ReadString('\n')
	if err != nil || ready != "archive-lock-ready\n" {
		t.Fatalf("%s child not ready: line=%q err=%v", mode, ready, err)
	}
	if afterReady != nil {
		afterReady()
	}
	if _, err := io.WriteString(stdin, "write\n"); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	output, readErr := io.ReadAll(io.LimitReader(reader, 4097))
	waitErr := cmd.Wait()
	waited = true
	if ctx.Err() != nil || readErr != nil || waitErr != nil || len(output) > 4096 {
		t.Fatalf("%s child failed: context=%v read=%v wait=%v stdout=%q stderr=%q", mode, ctx.Err(), readErr, waitErr, output, stderr.String())
	}
	want := "archive-lock-busy-code-5\n"
	if mode == "commit" {
		want = "archive-lock-committed\n"
	}
	if !strings.HasPrefix(string(output), want) {
		t.Fatalf("%s child missing verified outcome: %q", mode, output)
	}
}

func TestArchiveLockCloseChildHelper(t *testing.T) {
	home := os.Getenv("SIRSI_ARCHIVE_LOCK_CHILD_HOME")
	if home == "" {
		return
	}
	mode := os.Getenv("SIRSI_ARCHIVE_LOCK_CHILD_MODE")
	if mode != "busy" && mode != "commit" {
		t.Fatal("invalid owned lock child mode")
	}
	a, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := a.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout=100`); err != nil {
		t.Fatal(err)
	}
	fmt.Println("archive-lock-ready")
	var instruction [6]byte
	if _, err := io.ReadFull(os.Stdin, instruction[:]); err != nil || string(instruction[:]) != "write\n" {
		t.Fatalf("missing owned write barrier: %v", err)
	}
	writeTx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin competing transaction: %v", err)
	}
	defer writeTx.Rollback()
	result, writeErr := writeTx.ExecContext(ctx, `UPDATE archive_meta SET value=? WHERE key='lock-close-fixture'`, "child-"+mode)
	if mode == "busy" {
		var sqliteErr *sqlite.Error
		if ctx.Err() != nil || !errors.As(writeErr, &sqliteErr) || sqliteErr.Code()&0xff != 5 {
			t.Fatalf("held A lock must produce SQLITE_BUSY (5), not success/context/unknown: %v", writeErr)
		}
		if err := writeTx.Rollback(); err != nil {
			t.Fatal(err)
		}
		fmt.Println("archive-lock-busy-code-5")
		return
	}
	if writeErr != nil {
		t.Fatalf("fresh child write after A commit: %v", writeErr)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		t.Fatalf("fresh child affected rows=%d err=%v", count, err)
	}
	if err := writeTx.Commit(); err != nil {
		t.Fatal(err)
	}
	fmt.Println("archive-lock-committed")
}
