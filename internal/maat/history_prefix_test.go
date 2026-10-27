//go:build !windows

package maat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func appendPrefixFixture(t *testing.T, path string, decision Decision) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(decision); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func historyPrefixCursorFixture(t *testing.T, path, query string, limit int) *historyCursor {
	t.Helper()
	file, size, err := openHistoryPrefix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := digestBytes(raw)
	return &historyCursor{Version: historyCursorVersion, Snapshot: file.prefixSnapshotID(size, digest), PrefixBytes: size, PrefixSHA: digest, QueryKey: query, Phase: "scan", Limit: limit}
}

func drainPrefixQuery(t *testing.T, journal *FileDecisionJournal, query HistoryQuery, appendEach bool) HistoryPage {
	t.Helper()
	for calls := 0; calls < 100; calls++ {
		page, err := journal.QueryHistory(query)
		if err != nil {
			t.Fatal(err)
		}
		if page.VerificationBytes <= 0 && page.Cursor != "" {
			t.Fatal("continuation omitted separate prefix verification byte count")
		}
		if page.Coverage == HistoryCoverageComplete {
			return page
		}
		if len(page.Candidates) != 0 || page.Cursor == "" {
			t.Fatal("unverified/partial query exposed candidates or no cursor")
		}
		query.Cursor = page.Cursor
		if appendEach {
			appendPrefixFixture(t, journal.Path, historyFixtureScreen(t, time.Now().UTC().Format(time.RFC3339Nano), "append:between-phase"))
		}
	}
	t.Fatal("history query did not finish within its existing continuation bound")
	return HistoryPage{}
}

func TestHistoryFrozenPrefixAllowsBetweenPhaseAppendsAndFreshQuerySeesThem(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: filepath.Join(root, "decisions.jsonl")}
	first := historyFixtureScreen(t, "2026-10-27T12:00:00Z", "frozen:first")
	appendHistoryFixtures(t, journal, first)
	page, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "frozen-prefix", Limit: 5, RowBudget: 1})
	if err != nil || page.Coverage != HistoryCoveragePartial || page.Cursor == "" || page.VerificationBytes <= 0 {
		t.Fatalf("anchor page=%+v err=%v", page, err)
	}
	state, err := decodeHistoryCursor(page.Cursor)
	if err != nil || state == nil || state.Phase != "scan" || !validHistoryDigest(state.PrefixSHA) || state.Snapshot != page.Snapshot || state.Snapshot == "" {
		t.Fatalf("anchor did not establish a resumable prefix identity: state=%+v snapshot=%q err=%v", state, page.Snapshot, err)
	}
	appendPrefixFixture(t, journal.Path, historyFixtureScreen(t, "2026-10-27T12:01:00Z", "appended:after-anchor"))
	continued := drainPrefixQuery(t, journal, HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "frozen-prefix", Limit: 5, RowBudget: 1, Cursor: page.Cursor}, true)
	if len(continued.Candidates) != 1 || continued.Candidates[0].Decision.Evidence != first.Evidence {
		t.Fatalf("frozen query included tail append: %+v", continued.Candidates)
	}
	fresh := drainPrefixQuery(t, journal, HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "fresh-prefix", Limit: 20}, false)
	if len(fresh.Candidates) < 2 {
		t.Fatalf("fresh query did not observe appended records: %+v", fresh.Candidates)
	}
}

func TestHistoryFrozenPrefixAllowsFirstCanonicalAppendLockAndFreshQuerySeesTail(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: filepath.Join(root, "decisions.jsonl")}
	first := historyFixtureScreen(t, "2026-10-27T12:00:00Z", "lock:first")
	appendPrefixFixture(t, journal.Path, first) // Seed without creating the canonical mutation lock.
	lockPath := filepath.Join(root, ".decisions.jsonl.lock")
	if _, err := os.Lstat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mutation lock already exists before canonical append: %v", err)
	}
	page, err := journal.QueryHistory(HistoryQuery{QueryKey: "frozen-before-first-lock", Limit: 5, RowBudget: 1,
		Match: func(decision Decision) bool { return decision.Evidence == first.Evidence },
	})
	if err != nil || page.Coverage != HistoryCoveragePartial || page.Cursor == "" {
		t.Fatalf("initial frozen page=%+v err=%v", page, err)
	}
	tail := historyFixtureScreen(t, "2026-10-27T13:00:00Z", "lock:tail")
	if err := journal.Append(tail); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(lockPath); err != nil {
		t.Fatalf("canonical append did not create its mutation lock: %v", err)
	}
	continued := drainPrefixQuery(t, journal, HistoryQuery{QueryKey: "frozen-before-first-lock", Limit: 5, RowBudget: 1,
		Cursor: page.Cursor, Match: func(decision Decision) bool { return decision.Evidence == first.Evidence },
	}, false)
	if len(continued.Candidates) != 1 || continued.Candidates[0].Decision.Evidence != first.Evidence {
		t.Fatalf("frozen query changed across first lock creation: %+v", continued.Candidates)
	}
	fresh := drainPrefixQuery(t, journal, HistoryQuery{QueryKey: "fresh-after-first-lock", Limit: 5, RowBudget: 1,
		Match: func(decision Decision) bool { return decision.Evidence == tail.Evidence },
	}, false)
	if len(fresh.Candidates) != 1 || fresh.Candidates[0].Decision.Evidence != tail.Evidence {
		t.Fatalf("fresh query did not observe canonical append tail: %+v", fresh.Candidates)
	}
}

func TestHistoryPrefixRetainedDirectoryIdentityAllowsSiblingAddition(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "decisions.jsonl")
	if err := os.WriteFile(path, []byte("prefix\n"), 0600); err != nil {
		t.Fatal(err)
	}
	file, size, err := openHistoryPrefix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	if err := os.WriteFile(filepath.Join(root, "unrelated-sibling"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := file.verify(size); err != nil {
		t.Fatalf("unrelated parent entry changed retained directory identity: %v", err)
	}
}

type historyTruncateRestoreReader struct {
	file      *os.File
	original  []byte
	firstSize int
	delivered int
	truncated bool
	observed  bool
}

func (reader *historyTruncateRestoreReader) Read(buffer []byte) (int, error) {
	if !reader.truncated {
		remaining := reader.firstSize - reader.delivered
		if len(buffer) > remaining {
			buffer = buffer[:remaining]
		}
		n, err := reader.file.Read(buffer)
		reader.delivered += n
		if reader.delivered == reader.firstSize {
			reader.truncated = true
		}
		return n, err
	}
	offset, err := reader.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	if err := reader.file.Truncate(0); err != nil {
		return 0, err
	}
	n, readErr := reader.file.Read(buffer) // Observe physical EOF at the old offset.
	reader.observed = errors.Is(readErr, io.EOF)
	if _, err := reader.file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := reader.file.Write(reader.original); err != nil {
		return 0, err
	}
	if err := reader.file.Sync(); err != nil {
		return 0, err
	}
	if _, err := reader.file.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	reader.truncated = false
	return n, readErr
}

func TestHistoryPrefixPassRejectsTransientPhysicalEOFBeforeFrozenBoundary(t *testing.T) {
	first, err := json.Marshal(historyFixtureScreen(t, "2026-10-27T12:00:00Z", "eof:parsed-before-truncate"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(historyFixtureScreen(t, "2026-10-27T12:01:00Z", "eof:restored-tail"))
	if err != nil {
		t.Fatal(err)
	}
	first = append(first, '\n')
	tail := append(second, '\n')
	original := append(append([]byte(nil), first...), tail...)
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "journal.jsonl"), os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(original); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	source := &historyTruncateRestoreReader{file: file, original: original, firstSize: len(first)}
	ctx := context.Background()
	state := &historyCursor{Version: historyCursorVersion, QueryKey: "transient-eof", Phase: "scan", Limit: 5}
	var done bool
	digest, passErr := historyPrefixPassReader(source, int64(len(original)), ctx, time.Now().Add(time.Second), func(reader *bufio.Reader) error {
		var scanErr error
		done, _, _, scanErr = scanHistoryChunkReader(ctx, time.Now().Add(time.Second), reader, state, int64(len(original)), 20, HistoryScopeSystemOne, "", nil, nil, nil)
		return scanErr
	})
	var cursor string
	if passErr == nil && done {
		cursor, err = encodeHistoryCursor(state)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	restored, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatalf("fixture failed to restore original bytes before pass drain: equal=%v err=%v", bytes.Equal(restored, original), err)
	}
	if !errors.Is(passErr, ErrHistorySnapshotChanged) || done || digest != "" || cursor != "" || !source.observed {
		t.Fatalf("transient EOF accepted incomplete query: done=%v digest=%q cursor=%q observed=%v err=%v", done, digest, cursor, source.observed, passErr)
	}
}

func TestHistoryVerificationDeadlineRefusesZeroProgressCursor(t *testing.T) {
	state := &historyCursor{Version: historyCursorVersion, Snapshot: "prefix", PrefixBytes: 10, PrefixSHA: digestBytes([]byte("prefix")), QueryKey: "deadline", Phase: "scan", Limit: 5}
	previous, err := encodeHistoryCursor(state)
	if err != nil {
		t.Fatal(err)
	}
	if cursor, err := historyCursorAfterDeadline(state, previous); cursor != "" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("zero-progress deadline returned cursor=%q err=%v", cursor, err)
	}
	progressed := *state
	progressed.Offset = 1
	cursor, err := historyCursorAfterDeadline(&progressed, previous)
	if err != nil || cursor == "" {
		t.Fatalf("verified progress was not resumable after deadline: cursor=%q err=%v", cursor, err)
	}
}

func TestSmallHistoryQueryCompletesAfterVerifiedPhasesInOneCall(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: filepath.Join(root, "decisions.jsonl")}
	decision := historyFixtureScreen(t, "2026-10-27T12:00:00Z", "small:complete")
	appendHistoryFixtures(t, journal, decision)
	info, err := os.Stat(journal.Path)
	if err != nil {
		t.Fatal(err)
	}
	page, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "small-complete", Limit: 5})
	if err != nil || page.Coverage != HistoryCoverageComplete || page.Cursor != "" || len(page.Candidates) != 1 || page.Candidates[0].Decision.Evidence != decision.Evidence {
		t.Fatalf("small query did not complete in one verified request: page=%+v err=%v", page, err)
	}
	if page.VerificationBytes < info.Size() {
		t.Fatalf("verification bytes=%d anchored size=%d", page.VerificationBytes, info.Size())
	}
}

func TestHistoryFrozenPrefixIgnoresUnterminatedAppendedEOFRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: filepath.Join(root, "decisions.jsonl")}
	decision := historyFixtureScreen(t, "2026-10-27T12:00:00Z", "eof:anchored")
	appendHistoryFixtures(t, journal, decision)
	anchor, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "eof-prefix", Limit: 5, RowBudget: 1})
	if err != nil || anchor.Cursor == "" {
		t.Fatalf("anchor=%+v err=%v", anchor, err)
	}
	f, err := os.OpenFile(journal.Path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"time":"unfinished`); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	page := drainPrefixQuery(t, journal, HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "eof-prefix", Limit: 5, Cursor: anchor.Cursor}, false)
	if len(page.Candidates) != 1 || page.Candidates[0].Decision.Evidence != decision.Evidence {
		t.Fatalf("unterminated appended tail affected anchored query: %+v", page.Candidates)
	}
}

func TestHistoryFrozenPrefixRejectsSameInodeEditDespiteRestoredMtime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "decisions.jsonl")
	journal := &FileDecisionJournal{Path: path}
	appendHistoryFixtures(t, journal, historyFixtureScreen(t, "2026-10-27T12:00:00Z", "aba:original"))
	page, err := journal.QueryHistory(HistoryQuery{QueryKey: "aba", Limit: 1, RowBudget: 1})
	if err != nil || page.Cursor == "" {
		t.Fatalf("anchor page=%+v err=%v", page, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	needle := []byte("aba:original")
	replacement := []byte("aba:mutated!")
	if len(needle) != len(replacement) {
		t.Fatal("fixture mutation must preserve length")
	}
	for i := 0; i <= len(raw)-len(needle); i++ {
		if string(raw[i:i+len(needle)]) == string(needle) {
			copy(raw[i:i+len(needle)], replacement)
			break
		}
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	changed, err := journal.QueryHistory(HistoryQuery{QueryKey: "aba", Limit: 1, Cursor: page.Cursor})
	if !errors.Is(err, ErrHistorySnapshotChanged) || len(changed.Candidates) != 0 || changed.Cursor != "" {
		t.Fatalf("mutated prefix returned page=%+v err=%v", changed, err)
	}
}

func TestHistoryPrefixPassDoesNotAcceptTentativeBytesWhenDigestChanges(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "decisions.jsonl")
	original := []byte("original-prefix")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	file, size, err := openHistoryPrefix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	expected := digestBytes(original)
	changed := []byte("mutated--prefix")
	if len(changed) != len(original) {
		t.Fatal("mutation fixture must preserve prefix length")
	}
	_, err = historyPrefixPass(file, size, expected, context.Background(), time.Now().Add(time.Second), func(reader *bufio.Reader) error {
		if err := os.WriteFile(path, changed, 0600); err != nil {
			return err
		}
		_, err := io.CopyN(io.Discard, reader, size)
		return err
	})
	if !errors.Is(err, ErrHistorySnapshotChanged) {
		t.Fatalf("changed in-pass prefix was accepted: %v", err)
	}
}

func TestHistoryOlderPageRemainsFrozenWhileFreshQuerySeesAppendedResolution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: filepath.Join(root, "decisions.jsonl")}
	first := historyFixtureScreen(t, "2026-10-27T12:00:00Z", "page:first")
	second := historyFixtureScreen(t, "2026-10-27T11:00:00Z", "page:second")
	appendHistoryFixtures(t, journal, first, second)
	firstPage := drainPrefixQuery(t, journal, HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "older", Limit: 1, RowBudget: 1}, true)
	if !firstPage.HasMore || firstPage.NextPageCursor == "" || len(firstPage.Candidates) != 1 {
		t.Fatalf("first page=%+v", firstPage)
	}
	appendPrefixFixture(t, journal.Path, Decision{Time: "2026-10-27T13:00:00Z", Host: "fixture", Kind: "diagnostic owner acceptance", Requester: "fixture", Assessed: "resolution", Determination: "accepted", Why: "new tail link", ResolutionFor: first.Evidence})
	older := drainPrefixQuery(t, journal, HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "older", Limit: 1, RowBudget: 1, Cursor: firstPage.NextPageCursor}, true)
	if len(older.Candidates) != 1 || older.Candidates[0].Decision.Evidence != second.Evidence || len(older.Links.Acceptances) != 0 {
		t.Fatalf("older page changed from its frozen prefix: %+v", older)
	}
	fresh := drainPrefixQuery(t, journal, HistoryQuery{Scope: HistoryScopeAll, QueryKey: "fresh-resolution", Limit: 10,
		Match: func(d Decision) bool { return d.Evidence == first.Evidence },
	}, false)
	if len(fresh.Candidates) != 1 || len(fresh.Links.Acceptances) != 1 || fresh.Links.Acceptances[0].ResolutionFor != first.Evidence {
		t.Fatalf("fresh query did not observe tail resolution: %+v", fresh)
	}
}

func TestHistoryPrefixRetainedIdentityRejectsReplacementHardlinkAndTruncation(t *testing.T) {
	for _, mutation := range []string{"replacement", "hardlink", "truncation", "symlink", "mode-change", "parent-replacement", "parent-mode-change"} {
		t.Run(mutation, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "decisions.jsonl")
			if err := os.WriteFile(path, []byte("prefix"), 0600); err != nil {
				t.Fatal(err)
			}
			file, size, err := openHistoryPrefix(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.close()
			switch mutation {
			case "replacement":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("prefix"), 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".link"); err != nil {
					t.Fatal(err)
				}
			case "mode-change":
				if err := os.Chmod(path, 0640); err != nil {
					t.Fatal(err)
				}
			case "truncation":
				if err := os.Truncate(path, size-1); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".target", path); err != nil {
					t.Fatal(err)
				}
			case "parent-replacement":
				if err := os.Rename(root, root+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("prefix"), 0600); err != nil {
					t.Fatal(err)
				}
			case "parent-mode-change":
				if err := os.Chmod(root, 0750); err != nil {
					t.Fatal(err)
				}
			}
			if err := file.verify(size); !errors.Is(err, ErrHistorySnapshotChanged) {
				t.Fatalf("verify=%v, want snapshot refusal", err)
			}
		})
	}
}

func TestHistoryPrefixPassHashesSameForwardReaderAndHonorsCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "decisions.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 23_563_095)), 0600); err != nil {
		t.Fatal(err)
	}
	file, size, err := openHistoryPrefix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := historyPrefixPass(file, size, "", ctx, time.Now().Add(time.Second), func(reader *bufio.Reader) error { _, err := io.CopyN(io.Discard, reader, 1); return err }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled prefix pass error=%v", err)
	}
	ctx = context.Background()
	digest, err := historyPrefixPass(file, size, "", ctx, time.Now().Add(2*time.Second), nil)
	if err != nil || !validHistoryDigest(digest) {
		t.Fatalf("large private fixture digest=%q err=%v", digest, err)
	}
}
