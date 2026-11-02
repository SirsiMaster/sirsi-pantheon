//go:build darwin

package trashreceipt

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func prepareReceiptHome(t *testing.T, names ...string) (string, string, string) {
	t.Helper()
	home, root := ownedTrash(t)
	receipts := filepath.Join(home, ".sirsi", "trash-receipts")
	if err := os.MkdirAll(receipts, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		put(t, filepath.Join(root, name))
	}
	return home, root, receipts
}

func openTestHistory(t *testing.T, root, id string) HistoryDetail {
	t.Helper()
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	return historyDetailAtFD(fd, id)
}

func journalFactory(root string, fault func(string) error, configure func(*durableJournal)) func(Manifest) (operationJournal, error) {
	return func(m Manifest) (operationJournal, error) {
		j, err := newOperationJournalAt(root, m, fault)
		if err != nil {
			return nil, err
		}
		if configure != nil {
			configure(j)
		}
		return j, nil
	}
}

func TestTrashJournalPersistsExactIntentAttemptsAndNewestFirstPages(t *testing.T) {
	home, root, receipts := prepareReceiptHome(t, "first")
	firstManifest := mustPreview(t, home)
	first := applyHomeWithJournal(home, firstManifest, true, nil, journalFactory(receipts, nil, nil))
	if first.Status != "completed" || first.OperationID == "" || len(first.Errors) != 0 {
		t.Fatalf("first operation: %+v", first)
	}
	put(t, filepath.Join(root, "second"))
	secondManifest := mustPreview(t, home)
	second := applyHomeWithJournal(home, secondManifest, true, nil, journalFactory(receipts, nil, nil))
	if second.Status != "completed" || second.OperationID == "" {
		t.Fatalf("second operation: %+v", second)
	}

	page := historyPageAt(receipts, 1, "")
	if page.Incomplete || len(page.Items) != 1 || page.Items[0].OperationID != second.OperationID || page.NextCursor == "" {
		t.Fatalf("newest page: %+v", page)
	}
	older := historyPageAt(receipts, 1, page.NextCursor)
	if older.Incomplete || len(older.Items) != 1 || older.Items[0].OperationID != first.OperationID || older.NextCursor != "" {
		t.Fatalf("older page: %+v", older)
	}
	detail := openTestHistory(t, receipts, second.OperationID)
	if detail.State != "completed" || detail.IntegrityError != "" || detail.TerminalReport == nil ||
		!reflect.DeepEqual(detail.ReviewedManifest, secondManifest) || len(detail.Attempts) != 1 ||
		detail.Attempts[0].State != "unlink_succeeded" || detail.TerminalReport.OperationID != second.OperationID {
		t.Fatalf("exact durable detail: %+v", detail)
	}
}

func TestTrashJournalFailuresFenceUnlinksAndPreserveOutcomeDebt(t *testing.T) {
	cases := []struct {
		name       string
		stage      string
		configure  func(*durableJournal)
		indexed    bool
		deleted    bool
		wantState  string
		wantStatus string
	}{
		{name: "intent-write", stage: "intent", wantState: "unknown", wantStatus: "review_required"},
		{name: "index-fsync", stage: "index-fsync", indexed: true, wantState: "unknown", wantStatus: "review_required"},
		{name: "intent-fsync", stage: "intent-fsync", indexed: true, wantState: "unknown", wantStatus: "review_required"},
		{name: "attempt-start-sync", configure: func(j *durableJournal) { j.syncFn = func(int) error { return errors.New("injected fsync failure") } }, wantState: "not_finalized", wantStatus: "review_required"},
		{name: "attempt-start-short-write", configure: func(j *durableJournal) {
			writes := 0
			j.writeFn = func(_ int, data []byte) (int, error) {
				writes++
				if writes == 1 {
					return 1, nil
				}
				return 0, nil
			}
		}, wantState: "unknown", wantStatus: "review_required"},
		{name: "attempt-result", stage: "attempt-result", deleted: true, wantState: "not_finalized", wantStatus: "partial"},
		{name: "attempt-result-fsync", configure: func(j *durableJournal) {
			calls := 0
			j.syncFn = func(fd int) error {
				calls++
				if calls == 2 {
					return errors.New("injected result fsync failure")
				}
				return unix.Fsync(fd)
			}
		}, deleted: true, wantState: "not_finalized", wantStatus: "partial"},
		{name: "terminal", stage: "terminal", deleted: true, wantState: "not_finalized", wantStatus: "partial"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, root, receipts := prepareReceiptHome(t, "owned")
			manifest := mustPreview(t, home)
			fault := func(stage string) error {
				if stage == tc.stage && tc.stage != "" {
					return errors.New("injected " + stage + " failure")
				}
				return nil
			}
			report := applyHomeWithJournal(home, manifest, true, nil, journalFactory(receipts, fault, tc.configure))
			if report.Status != tc.wantStatus || len(report.Errors) == 0 {
				t.Fatalf("failure report: %+v", report)
			}
			_, statErr := os.Lstat(filepath.Join(root, "owned"))
			wasDeleted := os.IsNotExist(statErr)
			if wasDeleted != tc.deleted {
				t.Fatalf("deleted=%t want %t (stat error %v), report=%+v", wasDeleted, tc.deleted, statErr, report)
			}
			if report.OperationID == "" {
				if !tc.indexed && tc.name != "intent-write" {
					t.Fatal("persisted intent did not return its operation identity")
				}
				page := historyPageAt(receipts, 20, "")
				if len(page.Items) != 1 || page.Items[0].State != "unknown" || page.Items[0].StartedAt == "" {
					t.Fatalf("indexed orphan intent is visible: %+v", page)
				}
				return
			}
			detail := openTestHistory(t, receipts, report.OperationID)
			if detail.State != tc.wantState {
				t.Fatalf("history state %q want %q: %+v", detail.State, tc.wantState, detail)
			}
			if tc.deleted && (len(report.Deleted) != 1 || report.Deleted[0] != "owned") {
				t.Fatalf("immediate report lost known deleted path: %+v", report)
			}
			if !tc.deleted && len(report.Deleted) != 0 {
				t.Fatalf("no-delete failure acquired a deleted prefix: %+v", report)
			}
		})
	}
}

func TestTrashJournalTerminalAndCloseErrorsCannotReturnSuccess(t *testing.T) {
	for _, stage := range []string{"terminal", "close"} {
		t.Run(stage, func(t *testing.T) {
			home, _, receipts := prepareReceiptHome(t, "owned")
			manifest := mustPreview(t, home)
			fault := func(current string) error {
				if current == stage {
					return errors.New("injected " + stage + " failure")
				}
				return nil
			}
			report := applyHomeWithJournal(home, manifest, true, nil, journalFactory(receipts, fault, nil))
			if report.Status == "completed" || len(report.Errors) == 0 || len(report.Deleted) != 1 || report.Deleted[0] != "owned" {
				t.Fatalf("durability failure must retain prefix and avoid success: %+v", report)
			}
		})
	}
}

func TestTrashHistoryCorruptIndexRowsDoNotHideValidNeighbors(t *testing.T) {
	home, _, receipts := prepareReceiptHome(t, "first")
	manifest := mustPreview(t, home)
	first := applyHomeWithJournal(home, manifest, true, nil, journalFactory(receipts, nil, nil))
	index := filepath.Join(receipts, "history-index.jsonl")
	f, err := os.OpenFile(index, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{bad-row}\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(filepath.Dir(filepath.Dir(receipts)), ".Trash", "second"))
	secondManifest := mustPreview(t, home)
	second := applyHomeWithJournal(home, secondManifest, true, nil, journalFactory(receipts, nil, nil))
	page := historyPageAt(receipts, 20, "")
	ids := []string{}
	for _, item := range page.Items {
		ids = append(ids, item.OperationID)
	}
	if !page.Incomplete || len(page.Issues) == 0 || !containsString(ids, first.OperationID) || !containsString(ids, second.OperationID) {
		t.Fatalf("valid neighbors remain visible around corrupt row: %+v", page)
	}
}

func TestTrashHistoryNonRegularLeavesFailWithoutBlocking(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			index := filepath.Join(root, "history-index.jsonl")
			switch kind {
			case "fifo":
				if err := unix.Mkfifo(index, 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(root, "target")
				if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, index); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				target := filepath.Join(root, "target")
				if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, index); err != nil {
					t.Fatal(err)
				}
			}
			page := requireBoundedResult(t, func() HistoryPage { return historyPageAt(root, 20, "") })
			if !page.Incomplete {
				t.Fatalf("non-regular index accepted: %+v", page)
			}
			if _, err := os.Lstat(index); err != nil {
				t.Fatalf("substituted index was removed: %v", err)
			}
			if kind != "fifo" {
				data, err := os.ReadFile(filepath.Join(root, "target"))
				if err != nil || string(data) != "untouched" {
					t.Fatalf("substituted target changed: %q %v", data, err)
				}
			}
		})
	}
}

func TestTrashHistoryOperationLeafSubstitutionsAreVisibleUnknownRows(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			manifest := Manifest{Schema: ManifestSchema, UID: uint32(os.Geteuid()),
				Root:       Identity{Device: 1, Inode: 2, UID: uint32(os.Geteuid()), Mode: 0o040700, Type: "directory"},
				Containers: []Entry{}, Entries: []Entry{}}
			digest, err := manifestDigest(manifest)
			if err != nil {
				t.Fatal(err)
			}
			id := strings.Repeat("a", 32)
			started := "2026-11-01T12:00:00Z"
			row, err := json.Marshal(historyIndexRow{OperationID: id, StartedAt: started, ManifestSHA: digest})
			if err != nil {
				t.Fatal(err)
			}
			row = append(row, '\n')
			if err := os.WriteFile(filepath.Join(root, "history-index.jsonl"), row, 0o600); err != nil {
				t.Fatal(err)
			}
			leaf := filepath.Join(root, id+".jsonl")
			var target string
			switch kind {
			case "fifo":
				if err := unix.Mkfifo(leaf, 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target = filepath.Join(root, "target")
				if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, leaf); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				target = filepath.Join(root, "target")
				if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, leaf); err != nil {
					t.Fatal(err)
				}
			}
			page := requireBoundedResult(t, func() HistoryPage { return historyPageAt(root, 20, "") })
			if len(page.Items) != 1 || page.Items[0].State != "unknown" || page.Items[0].StartedAt != started || !page.Incomplete || len(page.Issues) == 0 {
				t.Fatalf("substituted operation file not surfaced: %+v", page)
			}
			if target != "" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "preserve" {
					t.Fatalf("substituted target changed: %q %v", data, err)
				}
			}
			if _, err := os.Lstat(leaf); err != nil {
				t.Fatalf("substituted operation leaf was removed: %v", err)
			}
		})
	}
}

func TestTrashHistoryIndexAppendRejectsSubstitutionsWithoutMutation(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			index := filepath.Join(root, "history-index.jsonl")
			var target string
			switch kind {
			case "fifo":
				if err := unix.Mkfifo(index, 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink", "hardlink":
				target = filepath.Join(root, "target")
				if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					if err := os.Symlink(target, index); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Link(target, index); err != nil {
					t.Fatal(err)
				}
			}
			rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(rootFD)
			err = requireBoundedResult(t, func() error {
				return appendHistoryIndex(rootFD, strings.Repeat("d", 32), time.Now().UTC(), strings.Repeat("e", 64), nil)
			})
			if err == nil {
				t.Fatal("substituted history index accepted for append")
			}
			if _, err := os.Lstat(index); err != nil {
				t.Fatalf("substituted index was removed: %v", err)
			}
			if target != "" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "preserve" {
					t.Fatalf("substituted target changed: %q %v", data, err)
				}
			}
		})
	}
}

func TestTrashHistoryIndexLockIsNonblocking(t *testing.T) {
	root := t.TempDir()
	index := filepath.Join(root, "history-index.jsonl")
	if err := os.WriteFile(index, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(index, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	err = requireBoundedResult(t, func() error {
		return appendHistoryIndex(rootFD, strings.Repeat("b", 32), time.Now().UTC(), strings.Repeat("c", 64), nil)
	})
	if err == nil {
		t.Fatal("contended history index lock was accepted")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Fatalf("lock contention lacks visible retry guidance: %v", err)
	}
}

func requireBoundedResult[T any](t *testing.T, operation func() T) T {
	t.Helper()
	result := make(chan T, 1)
	go func() { result <- operation() }()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("history operation did not return within one second")
		var zero T
		return zero
	}
}

func TestTrashHistoryUnfinishedDoesNotClaimInterrupted(t *testing.T) {
	home, _, receipts := prepareReceiptHome(t, "owned")
	manifest := mustPreview(t, home)
	j, err := newOperationJournalAt(receipts, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := j.OperationID()
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	detail := openTestHistory(t, receipts, id)
	if detail.State != "not_finalized" || detail.TerminalReport != nil || detail.IntegrityError != "" ||
		len(detail.Attempts) != 1 || detail.Attempts[0].State != "not_attempted" || strings.Contains(detail.State, "interrupt") {
		t.Fatalf("unfinished operation was guessed: %+v", detail)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
