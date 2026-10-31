package maat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type historySnapshotTime struct {
	Sec  int64
	Nsec int64
}

type historySnapshotStat struct {
	Dev, Ino  uint64
	Atimespec historySnapshotTime
	Ctimespec historySnapshotTime
}

type historySnapshotInfo struct {
	size    int64
	mode    os.FileMode
	modTime time.Time
	sys     any
}

func (info historySnapshotInfo) Name() string       { return "decisions.jsonl" }
func (info historySnapshotInfo) Size() int64        { return info.size }
func (info historySnapshotInfo) Mode() os.FileMode  { return info.mode }
func (info historySnapshotInfo) ModTime() time.Time { return info.modTime }
func (info historySnapshotInfo) IsDir() bool        { return info.mode.IsDir() }
func (info historySnapshotInfo) Sys() any           { return info.sys }

func TestHistorySnapshotIdentityIgnoresReadAtimeButBindsInodeAndCtime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	base := historySnapshotInfo{
		size: 10, mode: 0o600, modTime: time.Unix(10, 20).UTC(),
		sys: historySnapshotStat{Dev: 1, Ino: 2, Atimespec: historySnapshotTime{Sec: 3}, Ctimespec: historySnapshotTime{Sec: 4}},
	}
	changedAtime := base
	changedAtime.sys = historySnapshotStat{Dev: 1, Ino: 2, Atimespec: historySnapshotTime{Sec: 30}, Ctimespec: historySnapshotTime{Sec: 4}}
	if historySnapshotID(path, base) != historySnapshotID(path, changedAtime) {
		t.Fatal("read-time access timestamp changed the continuation snapshot")
	}
	changedCtime := base
	changedCtime.sys = historySnapshotStat{Dev: 1, Ino: 2, Atimespec: historySnapshotTime{Sec: 3}, Ctimespec: historySnapshotTime{Sec: 5}}
	if historySnapshotID(path, base) == historySnapshotID(path, changedCtime) {
		t.Fatal("same-size, same-mtime rewrite was not distinguished by change time")
	}
	changedInode := base
	changedInode.sys = historySnapshotStat{Dev: 1, Ino: 3, Atimespec: historySnapshotTime{Sec: 3}, Ctimespec: historySnapshotTime{Sec: 4}}
	if historySnapshotID(path, base) == historySnapshotID(path, changedInode) {
		t.Fatal("replacement inode was not distinguished")
	}
}

func historyFixtureScreen(t *testing.T, at, evidence string) Decision {
	t.Helper()
	verdict, err := Screen(SystemOneScreen{
		Subject:       VerdictSubject{Kind: "commit", Repo: "fixture/repo", Ref: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		FeatherWeight: 80, Confidence: 1,
		Floor: FloorResult{Passed: true, Checks: []FloorCheck{{Name: "fixture", Passed: true}}},
		Model: ModelStamp{Provider: "local:fixture", Version: "fixture", Local: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Decision{
		Time: at, Host: "fixture", Kind: "system one screen", Requester: "fixture",
		Assessed: "fixture screen", Determination: string(verdict.Gate), Why: "inert history fixture",
		Evidence: evidence, SystemOne: &verdict,
	}
}

func appendHistoryFixtures(t *testing.T, journal *FileDecisionJournal, decisions ...Decision) {
	t.Helper()
	for _, decision := range decisions {
		if err := journal.Append(decision); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQueryHistoryCountsExactEvidenceAcrossKindsOutsideSelectedWindow(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	target := historyFixtureScreen(t, "2026-10-20T00:00:00Z", "screen:collision")
	rows := []Decision{target}
	for index := 0; index < 105; index++ {
		rows = append(rows, historyFixtureScreen(t,
			time.Date(2026, 10, 21, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"screen:outside-window:"+strconv.Itoa(index)))
	}
	rows = append(rows, Decision{Time: "2026-10-01T00:00:00Z", Host: "fixture", Kind: "reservation",
		Requester: "fixture", Assessed: "same exact evidence under another kind", Determination: "grant",
		Why: "inert collision", Evidence: target.Evidence})
	appendHistoryFixtures(t, journal, rows...)
	page, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "all-kind-collision", Limit: 1,
		Match: func(decision Decision) bool { return decision.Evidence == target.Evidence }})
	if err != nil || page.Coverage != HistoryCoverageComplete || len(page.Candidates) != 1 {
		t.Fatalf("targeted history page: coverage=%q candidates=%d err=%v", page.Coverage, len(page.Candidates), err)
	}
	if page.Candidates[0].SystemOneSourceCount != 1 || page.Candidates[0].EvidenceMatchCount != 2 {
		t.Fatalf("exact evidence counts omitted the old different-kind collision: %+v", page.Candidates[0])
	}
}

func TestQueryHistoryRetainsRepairLinksWhenTypedAcceptanceIsRejected(t *testing.T) {
	for _, pending := range []bool{false, true} {
		kind, determination := "diagnostic verified repair", "resolved"
		if pending {
			kind, determination = "diagnostic repair attempt", "started"
		}
		t.Run(kind, func(t *testing.T) {
			journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
			source := historyFixtureScreen(t, "2026-10-31T12:00:00Z", "screen:rejected-acceptance")
			acceptance, err := BuildBinaryDriftInactiveDisposition("fixture owner", BinaryDriftInactiveDisposition{
				OriginEvidence: source.Evidence, Host: "wrong-host",
				ObservedFromUTC:    time.Date(2026, 10, 31, 12, 0, 1, 0, time.UTC),
				ObservedThroughUTC: time.Date(2026, 10, 31, 12, 0, 2, 0, time.UTC),
				ObservationSHA256:  strings.Repeat("b", 64),
			})
			if err != nil {
				t.Fatal(err)
			}
			repair := Decision{Time: "2026-10-31T12:00:03Z", Host: "fixture", Kind: kind, Requester: "fixture",
				Resource: "binary-drift", Determination: determination, Why: "inert retained repair", Evidence: "repair:" + kind, OriginEvidence: source.Evidence}
			appendHistoryFixtures(t, journal, source, acceptance, repair)
			page, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "rejected-typed-" + kind, Limit: 1,
				MatchState: func(_ Decision, state HistoryMatchState) bool {
					return state.RepairOutcome != nil || state.RepairAttempt != nil
				}})
			if err != nil || page.Coverage != HistoryCoverageComplete || len(page.Candidates) != 1 {
				t.Fatalf("rejected typed acceptance suppressed paged repair state: page=%+v err=%v", page, err)
			}
			if pending && (len(page.Links.RepairAttempts) != 1 || page.Links.RepairAttempts[0].Evidence != repair.Evidence) {
				t.Fatalf("pending attempt missing after rejected acceptance: %+v", page.Links)
			}
			if !pending && (len(page.Links.RepairOutcomes) != 1 || page.Links.RepairOutcomes[0].Evidence != repair.Evidence) {
				t.Fatalf("verified outcome missing after rejected acceptance: %+v", page.Links)
			}
		})
	}
}

func TestQueryHistoryScopesBeforePageLimitAndFindsOldSystemOne(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	old := historyFixtureScreen(t, "2026-09-01T00:00:00Z", "screen:old")
	appendHistoryFixtures(t, journal, old)
	for index := 0; index < 60; index++ {
		appendHistoryFixtures(t, journal, Decision{
			Time: time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "fixture reservation",
			Determination: "grant", Why: "unrelated", Evidence: "reservation:" + string(rune('a'+index%26)),
		})
	}
	page, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "system-one", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if page.Coverage != HistoryCoverageComplete || len(page.Candidates) != 1 || page.Candidates[0].Decision.Evidence != old.Evidence {
		t.Fatalf("scoped history = coverage %q, candidates %+v", page.Coverage, page.Candidates)
	}
	if page.HasMore || page.NextPageCursor != "" {
		t.Fatalf("single scoped result unexpectedly has continuation: %+v", page)
	}
}

func TestQueryHistoryUsesStablePhysicalTieAndPlatformSnapshotScope(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	at := "2026-10-24T12:00:00Z"
	appendHistoryFixtures(t, journal,
		historyFixtureScreen(t, at, "screen:first"),
		historyFixtureScreen(t, at, "screen:second"),
		historyFixtureScreen(t, at, "screen:third"),
	)
	first, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "stable", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Candidates) != 2 || !first.HasMore || first.Candidates[0].Decision.Evidence != "screen:first" || first.Candidates[1].Decision.Evidence != "screen:second" {
		t.Fatalf("first equal-time page = %+v", first)
	}
	second, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "stable", Limit: 2, Cursor: first.NextPageCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Candidates) != 1 || second.Candidates[0].Decision.Evidence != "screen:third" || second.HasMore {
		t.Fatalf("second equal-time page = %+v", second)
	}
	appendHistoryFixtures(t, journal, historyFixtureScreen(t, "2026-10-25T00:00:00Z", "screen:appended"))
	continued, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "stable", Limit: 2, Cursor: first.NextPageCursor})
	if runtime.GOOS == "windows" {
		if !errors.Is(err, ErrHistorySnapshotChanged) {
			t.Fatalf("legacy Windows continuation error = %v, want snapshot drift", err)
		}
		return
	}
	if err != nil || len(continued.Candidates) != 1 || continued.Candidates[0].Decision.Evidence != "screen:third" {
		t.Fatalf("frozen older page changed after append: %+v / %v", continued, err)
	}
	fresh, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "stable-fresh", Limit: 2})
	if err != nil || len(fresh.Candidates) != 2 || fresh.Candidates[0].Decision.Evidence != "screen:appended" {
		t.Fatalf("fresh query did not see appended row: %+v / %v", fresh, err)
	}
}

func TestQueryHistoryBudgetContinuationFindsLateMatchWithoutProvisionalEmpty(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	for index := 0; index < 8; index++ {
		appendHistoryFixtures(t, journal, Decision{
			Time: time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "fixture reservation",
			Determination: "grant", Why: "unrelated", Evidence: "reservation:budget",
		})
	}
	old := historyFixtureScreen(t, "2026-09-01T00:00:00Z", "screen:late")
	appendHistoryFixtures(t, journal, old)
	var cursor string
	for attempt := 0; attempt < 40; attempt++ {
		page, err := journal.QueryHistory(HistoryQuery{
			Scope: HistoryScopeSystemOne, QueryKey: "budget", Limit: 3, Cursor: cursor,
			Context: context.Background(), ByteBudget: 1, RowBudget: 1, TimeBudget: time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 0 && (page.Coverage != HistoryCoveragePartial || page.ScannedRecords != 1 || len(page.Candidates) != 0) {
			t.Fatalf("first bounded chunk did not remain explicitly empty/incomplete: %+v", page)
		}
		if page.Coverage == HistoryCoverageComplete {
			if len(page.Candidates) != 1 || page.Candidates[0].Decision.Evidence != old.Evidence {
				t.Fatalf("completed continuation = %+v", page)
			}
			return
		}
		if len(page.Candidates) != 0 || page.Cursor == "" {
			t.Fatalf("partial scan exposed provisional results or lost cursor: %+v", page)
		}
		cursor = page.Cursor
	}
	t.Fatal("bounded continuation did not complete")
}

func TestQueryHistoryCancellationDuringVerifiedPassReturnsNoCursorOrProvisionalCases(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	appendHistoryFixtures(t, journal,
		historyFixtureScreen(t, "2026-10-03T00:00:00Z", "screen:materialize-new"),
		historyFixtureScreen(t, "2026-10-02T00:00:00Z", "screen:materialize-old"),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	match := func(Decision) bool {
		if calls.Add(1) == 3 { // two scan predicates, then final-page revalidation
			cancel()
		}
		return true
	}
	first, err := journal.QueryHistory(HistoryQuery{
		Scope: HistoryScopeSystemOne, QueryKey: "materialize-cancel", Limit: 2, Context: ctx,
		Match: match,
	})
	if runtime.GOOS == "windows" {
		if err != nil || first.Cursor == "" || len(first.Candidates) != 0 {
			t.Fatalf("legacy Windows cancellation did not return an empty continuation: %+v / %v", first, err)
		}
	} else if !errors.Is(err, context.Canceled) || first.Cursor != "" || len(first.Candidates) != 0 {
		t.Fatalf("canceled pass returned progress or provisional cases: %+v / %v", first, err)
	}
	complete, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "materialize-cancel", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	for calls := 0; complete.Coverage != HistoryCoverageComplete && calls < 10; calls++ {
		complete, err = journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "materialize-cancel", Limit: 2, Cursor: complete.Cursor})
		if err != nil {
			t.Fatal(err)
		}
	}
	if complete.Coverage != HistoryCoverageComplete || len(complete.Candidates) != 2 {
		t.Fatalf("resumed materialization did not complete: %+v", complete)
	}
}

func TestQueryHistoryRetainsBoundedCandidatesAndChargesMaterialization(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	for index := 0; index < 27; index++ {
		appendHistoryFixtures(t, journal, historyFixtureScreen(t,
			time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano), "screen:bounded:"+strconv.Itoa(index)))
	}
	const limit, rowsPerRead = 3, 4
	var cursor string
	var materializationCalls atomic.Int32
	match := func(Decision) bool { materializationCalls.Add(1); return true }
	for attempt := 0; attempt < 100; attempt++ {
		page, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "bounded-retention", Limit: limit,
			Cursor: cursor, RowBudget: rowsPerRead, TimeBudget: time.Second, Match: match})
		if err != nil {
			t.Fatal(err)
		}
		if page.ScannedRecords > rowsPerRead || len(page.Candidates) > limit {
			t.Fatalf("request exceeded its row/page retention bound: %+v", page)
		}
		if page.Coverage == HistoryCoverageComplete {
			if page.MatchCount != 27 || len(page.Candidates) != limit || !page.HasMore || materializationCalls.Load() <= 27 {
				t.Fatalf("materialization was not counted/revalidated after full bounded coverage: %+v", page)
			}
			return
		}
		if len(page.Candidates) != 0 || page.Cursor == "" {
			t.Fatalf("partial query exposed provisional records: %+v", page)
		}
		state, err := decodeHistoryCursor(page.Cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Candidates) > limit+1 || len(state.Links) > limit+1 || len(state.Issues) > maxRetainedJournalIssues {
			t.Fatalf("continuation retained unbounded page data: %+v", state)
		}
		cursor = page.Cursor
	}
	t.Fatal("bounded materialization failed to make finite progress")
}

func TestQueryHistoryDefersResidualMaterializationAndRetainsSelectedPage(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	for index := 0; index < 4; index++ {
		appendHistoryFixtures(t, journal, historyFixtureScreen(t,
			time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano), "screen:residual:"+strconv.Itoa(index)))
	}
	file, err := os.Open(journal.Path)
	if err != nil {
		t.Fatal(err)
	}
	const limit, rowsPerRead = 3, 4
	state := historyPrefixCursorFixture(t, journal.Path, "residual-budget", limit)
	done, _, _, scanErr := scanHistoryChunk(context.Background(), time.Now().Add(time.Second), file, state,
		defaultHistoryByteBudget, 100, HistoryScopeSystemOne, "", nil, nil, nil)
	closeErr := file.Close()
	if scanErr != nil || closeErr != nil || !done || len(state.Candidates) != limit+1 {
		t.Fatalf("prepare bounded selected references: done=%v scan=%v close=%v state=%+v", done, scanErr, closeErr, state)
	}
	// Leave two physical rows in the link scan. They consume half of the
	// four-row request before its three-row selected page is materialized.
	state.Phase, state.Offset, state.Links = "links", state.Candidates[1].Order.Offset, makeHistoryLinkRefs(state.Candidates)
	third := state.Candidates[2]
	cursor, err := encodeHistoryCursor(state)
	if err != nil {
		t.Fatal(err)
	}
	query := HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: state.QueryKey, Limit: limit, Cursor: cursor, RowBudget: rowsPerRead, TimeBudget: time.Second}
	partial, err := journal.QueryHistory(query)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Coverage != HistoryCoveragePartial || len(partial.Candidates) != 0 || partial.Cursor == "" || partial.NextPageCursor != "" || partial.HasMore || partial.ScannedRecords > rowsPerRead {
		t.Fatalf("residual budget published a short complete page: %+v", partial)
	}
	resumed, err := decodeHistoryCursor(partial.Cursor)
	if err != nil || resumed == nil || resumed.Phase != "materialize" || len(resumed.Candidates) != limit+1 || resumed.Candidates[2] != third || resumed.After != nil {
		t.Fatalf("continuation lost the pending third candidate or advanced the page: %+v / %v", resumed, err)
	}
	query.Cursor = partial.Cursor
	complete, err := journal.QueryHistory(query)
	if err != nil {
		t.Fatal(err)
	}
	if complete.Coverage != HistoryCoverageComplete || len(complete.Candidates) != limit || complete.Candidates[2].Decision.Evidence != "screen:residual:1" || complete.MatchCount != 4 || !complete.HasMore || complete.NextPageCursor == "" || complete.ScannedRecords > rowsPerRead {
		t.Fatalf("fresh four-row budget did not materialize the full three-case page: %+v", complete)
	}
	// Full-budget insufficiency retains the separate atomic short-page
	// contract; it must not turn into a continuation that cannot progress.
	query.RowBudget = 1
	short, err := journal.QueryHistory(query)
	if err != nil || short.Coverage != HistoryCoverageComplete || len(short.Candidates) != 1 || short.ScannedRecords != 1 || !short.HasMore {
		t.Fatalf("fresh tiny budget lost atomic short-page progress: %+v / %v", short, err)
	}
}

func TestHistoryMaterializationChargesOneAtomicBundleAndDefersRest(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	appendHistoryFixtures(t, journal,
		historyFixtureScreen(t, "2026-10-03T00:00:00Z", "screen:bundle-first"),
		historyFixtureScreen(t, "2026-10-02T00:00:00Z", "screen:bundle-second"))
	file, err := os.Open(journal.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	state := &historyCursor{Version: historyCursorVersion, Snapshot: historySnapshotID(journal.Path, info), QueryKey: "bundle", Phase: "scan", Limit: 2}
	done, _, _, err := scanHistoryChunk(context.Background(), time.Now().Add(time.Second), file, state, defaultHistoryByteBudget, 100,
		HistoryScopeSystemOne, "", nil, nil, nil)
	if err != nil || !done {
		t.Fatalf("prepare retained page: done=%v err=%v", done, err)
	}
	state.Links = makeHistoryLinkRefs(state.Candidates)
	query := HistoryQuery{Scope: HistoryScopeSystemOne, Limit: 2, ByteBudget: 1, RowBudget: 1}
	page := HistoryPage{}
	limited, bytesRead, rowsRead, err := materializeHistoryPage(context.Background(), time.Now().Add(time.Second), file,
		journal.Path, state.Snapshot, state, query, &page, 1, 1)
	if err != nil || !limited || len(page.Candidates) != 1 || rowsRead != 1 || bytesRead != state.Candidates[0].Order.Size {
		t.Fatalf("materialization did not charge one bounded record and defer the page: limited=%v bytes=%d rows=%d candidates=%d err=%v", limited, bytesRead, rowsRead, len(page.Candidates), err)
	}
	if bytesRead > maxHistoryRecordBytes+1 {
		t.Fatal("atomic progress exceeded the admitted record-size bound")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page = HistoryPage{}
	_, bytesRead, rowsRead, err = materializeHistoryPage(ctx, time.Now().Add(time.Second), file, journal.Path, state.Snapshot, state, query, &page, 1, 1)
	if !errors.Is(err, context.Canceled) || bytesRead != 0 || rowsRead != 0 || len(page.Candidates) != 0 {
		t.Fatalf("precanceled materialization did work: bytes=%d rows=%d err=%v", bytesRead, rowsRead, err)
	}
}

func TestQueryHistoryResumesPastOversizedRecordWithinBudgets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(strings.Repeat("x", 1<<20) + "\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	want := historyFixtureScreen(t, "2026-09-01T00:00:00Z", "screen:after-oversized")
	encoded, err := json.Marshal(want)
	if err != nil {
		file.Close()
		t.Fatal(err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	journal := &FileDecisionJournal{Path: path}
	var cursor string
	for attempt := 0; attempt < 40; attempt++ {
		page, err := journal.QueryHistory(HistoryQuery{
			Scope: HistoryScopeSystemOne, QueryKey: "oversized", Limit: 3, Cursor: cursor,
			ByteBudget: 64 << 10, RowBudget: 4, TimeBudget: time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if page.ScannedBytes > (64<<10)+(maxHistoryRecordBytes+(64<<10)) {
			t.Fatalf("oversized row exceeded one bounded-record allowance: %d bytes", page.ScannedBytes)
		}
		if page.Coverage == HistoryCoverageComplete {
			if page.Integrity.InvalidCount != 1 || len(page.Integrity.Issues) != 1 || page.Integrity.Issues[0].Reason != "oversized record; digest covers bounded prefix" || len(page.Candidates) != 1 || page.Candidates[0].Decision.Evidence != want.Evidence {
				t.Fatalf("completed oversized continuation = %+v", page)
			}
			return
		}
		if len(page.Candidates) != 0 || page.Cursor == "" {
			t.Fatalf("partial oversized scan exposed provisional history or lost cursor: %+v", page)
		}
		cursor = page.Cursor
	}
	t.Fatal("oversized row continuation did not complete")
}

func TestQueryHistoryCanceledScanReturnsNoProvisionalResultsAndCanResume(t *testing.T) {
	journal := &FileDecisionJournal{Path: filepath.Join(t.TempDir(), "decisions.jsonl")}
	for index := 0; index < 20; index++ {
		appendHistoryFixtures(t, journal, Decision{
			Time: time.Date(2026, 10, 1, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Host: "fixture", Kind: "reservation", Requester: "fixture", Assessed: "reservation",
			Determination: "grant", Why: "unrelated", Evidence: "reservation:cancel",
		})
	}
	appendHistoryFixtures(t, journal, historyFixtureScreen(t, "2026-09-01T00:00:00Z", "screen:after-cancel"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	partial, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "cancel", Limit: 5, Context: ctx})
	if runtime.GOOS == "windows" {
		if err != nil || partial.Cursor == "" || len(partial.Candidates) != 0 {
			t.Fatalf("legacy Windows canceled scan did not return an empty continuation: %+v / %v", partial, err)
		}
	} else if !errors.Is(err, context.Canceled) || partial.Cursor != "" || len(partial.Candidates) != 0 {
		t.Fatalf("canceled scan returned progress: %+v / %v", partial, err)
	}
	complete, err := journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "cancel", Limit: 5, Context: context.Background()})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; complete.Coverage != HistoryCoverageComplete && attempt < 10; attempt++ {
		complete, err = journal.QueryHistory(HistoryQuery{Scope: HistoryScopeSystemOne, QueryKey: "cancel", Limit: 5, Cursor: complete.Cursor, Context: context.Background()})
		if err != nil {
			t.Fatal(err)
		}
	}
	if complete.Coverage != HistoryCoverageComplete || len(complete.Candidates) != 1 || complete.Candidates[0].Decision.Evidence != "screen:after-cancel" {
		t.Fatalf("resumed canceled scan = %+v", complete)
	}
}
