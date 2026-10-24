package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func openTestArchive(t *testing.T) (*Archive, string) {
	t.Helper()
	home := t.TempDir()
	archive, err := Open(home)
	if err != nil {
		t.Fatalf("Open(test home): %v", err)
	}
	t.Cleanup(func() { _ = archive.Close() })
	return archive, home
}

func archiveRecord(source, instance, nativeID, content string) seshat.SourceRecord {
	return seshat.SourceRecord{
		Source: source, InstanceID: instance, NativeItemID: nativeID,
		Schema: "fixture-v1", ExtractedAt: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC),
		Title: "Local reference", Summary: "A bounded fixture", Content: content,
		References: []seshat.KIReference{{Type: "source", Value: source}, {Type: "file", Value: "fixture.txt"}},
	}
}

func TestArchiveStableIdentityVersionsAndCrossSourceDedup(t *testing.T) {
	archive, _ := openTestArchive(t)
	ctx := context.Background()
	first := archiveRecord("claude", "profile-a", "session-1", "same full local content")
	run, err := archive.ImportSource(ctx, first.Source, []seshat.SourceRecord{first}, nil)
	if err != nil || run.State != ImportComplete || run.Added != 1 {
		t.Fatalf("first import = %+v, %v", run, err)
	}
	same := first
	run, err = archive.ImportSource(ctx, same.Source, []seshat.SourceRecord{same}, nil)
	if err != nil || run.Unchanged != 1 || run.Versioned != 0 {
		t.Fatalf("repeat import = %+v, %v", run, err)
	}
	second := archiveRecord("claude", "profile-a", "session-1", "revised full local content")
	run, err = archive.ImportSource(ctx, second.Source, []seshat.SourceRecord{second}, nil)
	if err != nil || run.Versioned != 1 {
		t.Fatalf("revised import = %+v, %v", run, err)
	}
	id := recordID(first)
	history, err := archive.History(ctx, id)
	if err != nil || len(history) != 2 || history[0].Revision != 2 || history[1].Revision != 1 {
		t.Fatalf("history = %+v, %v", history, err)
	}
	if history[0].ContentDigest == history[1].ContentDigest || history[0].Content != second.Content {
		t.Fatalf("revision digests/content do not preserve the edit: %+v", history)
	}
	crossSource := archiveRecord("gemini", "account-a", "conversation-7", second.Content)
	if _, err := archive.ImportSource(ctx, crossSource.Source, []seshat.SourceRecord{crossSource}, nil); err != nil {
		t.Fatal(err)
	}
	page, err := archive.Search(ctx, SearchQuery{Limit: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("deduplicated search = %+v, %v", page, err)
	}
	if len(page.Items[0].Origins) != 2 || len(page.Items[0].EquivalentIDs) != 2 {
		t.Fatalf("cross-source provenance was not retained: %+v", page.Items[0])
	}
}

func TestArchivePartialImportPreservesRowsWithoutAdvancingCursor(t *testing.T) {
	archive, _ := openTestArchive(t)
	item := archiveRecord("claude", "profile-a", "partial-session", "readable item")
	run, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, errors.New("source path contains a credential fixture"))
	if err != nil || run.State != ImportPartial || run.ErrorCode == "" {
		t.Fatalf("partial import = %+v, %v", run, err)
	}
	var cursorCount int
	if err := archive.db.QueryRow(`SELECT COUNT(*) FROM source_cursors WHERE source='claude'`).Scan(&cursorCount); err != nil || cursorCount != 0 {
		t.Fatalf("partial import advanced cursor: count=%d err=%v", cursorCount, err)
	}
	if _, err := archive.Detail(context.Background(), recordID(item), 0); err != nil {
		t.Fatalf("readable partial item was not retained: %v", err)
	}
	var runError string
	if err := archive.db.QueryRow(`SELECT error_code FROM import_runs WHERE id=?`, run.ID).Scan(&runError); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(runError, "credential fixture") {
		t.Fatal("raw adapter error was retained")
	}
}

func TestArchiveSensitivityAdmissionPrecedesRecordAndFTSWrite(t *testing.T) {
	archive, _ := openTestArchive(t)
	fixtures := []struct {
		name string
		edit func(*seshat.SourceRecord, string)
	}{
		{name: "source", edit: func(item *seshat.SourceRecord, secret string) { item.Source = "api_key=" + secret }},
		{name: "unvalidated_source", edit: func(item *seshat.SourceRecord, _ string) { item.Source = "Unvalidated source label" }},
		{name: "instance", edit: func(item *seshat.SourceRecord, secret string) { item.InstanceID = "api_key=" + secret }},
		{name: "native_id", edit: func(item *seshat.SourceRecord, secret string) { item.NativeItemID = "api_key=" + secret }},
		{name: "schema", edit: func(item *seshat.SourceRecord, secret string) { item.Schema = "api_key=" + secret }},
		{name: "reference", edit: func(item *seshat.SourceRecord, secret string) {
			item.References = append(item.References, seshat.KIReference{Type: "api_key", Value: secret})
		}},
		{name: "content", edit: func(item *seshat.SourceRecord, secret string) { item.Content = "api_key=" + secret }},
	}
	for index, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			secret := "FixtureSecret" + string(rune('A'+index)) + "91A3!"
			item := archiveRecord("claude", "profile-a", "secret-session-"+fixture.name, "safe content")
			fixture.edit(&item, secret)
			// ImportSource's route key is the raw source identity; a secret-bearing
			// source is intentionally accepted only into an opaque withheld record.
			run, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil)
			if err != nil || run.Withheld != 1 || strings.Contains(run.Source, secret) {
				t.Fatalf("sensitive import = %+v, %v", run, err)
			}
			var joined string
			err = archive.db.QueryRow(`SELECT
				COALESCE((SELECT group_concat(id||source||instance_id||native_item_id||title||summary||withheld_reason) FROM records),'') ||
				COALESCE((SELECT group_concat(record_id||content_digest||predecessor_digest||title||summary||content||references_json) FROM versions),'') ||
				COALESCE((SELECT group_concat(record_id||source||instance_id||native_item_id||reference_json) FROM origins),'') ||
				COALESCE((SELECT group_concat(record_id||source||instance_id||native_item_id||reference_json) FROM version_occurrences),'') ||
				COALESCE((SELECT group_concat(record_digest||safe_source||reason) FROM withheld_observations),'') ||
				COALESCE((SELECT group_concat(source||error_code) FROM import_runs),'') ||
				COALESCE((SELECT group_concat(source||cursor) FROM source_cursors),'') ||
				COALESCE((SELECT group_concat(title||summary||content||references_text) FROM knowledge_fts),'')`).Scan(&joined)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(joined, secret) {
				t.Fatal("sensitive source or identity text reached SQLite or FTS")
			}
			var storedSource, storedInstance, storedNative string
			var versions, origins int
			if err := archive.db.QueryRow(`SELECT source,instance_id,native_item_id FROM records WHERE id=?`, recordID(item)).Scan(&storedSource, &storedInstance, &storedNative); err != nil {
				t.Fatal(err)
			}
			if storedSource != "withheld" && storedSource != "claude" || !strings.HasPrefix(storedInstance, "opaque:") || !strings.HasPrefix(storedNative, "opaque:") {
				t.Fatalf("withheld identity was not safely projected: %q %q %q", storedSource, storedInstance, storedNative)
			}
			if err := archive.db.QueryRow(`SELECT COUNT(*) FROM versions WHERE record_id=?`, recordID(item)).Scan(&versions); err != nil {
				t.Fatal(err)
			}
			if err := archive.db.QueryRow(`SELECT COUNT(*) FROM origins WHERE record_id=?`, recordID(item)).Scan(&origins); err != nil {
				t.Fatal(err)
			}
			var withheldObservations int
			if err := archive.db.QueryRow(`SELECT COUNT(*) FROM withheld_observations WHERE record_digest=?`, recordID(item)).Scan(&withheldObservations); err != nil {
				t.Fatal(err)
			}
			if versions != 0 || origins != 0 || withheldObservations != 1 {
				t.Fatalf("withheld rows retained source history/provenance: versions=%d origins=%d observations=%d", versions, origins, withheldObservations)
			}
			detail, err := archive.Detail(context.Background(), recordID(item), 0)
			if err != nil || detail.Content != "" || detail.WithheldReason == "" {
				t.Fatalf("withheld detail = %+v, %v", detail, err)
			}
			projection, err := json.Marshal(detail)
			if err != nil || strings.Contains(string(projection), secret) {
				t.Fatalf("withheld projection leaked input: %s (%v)", projection, err)
			}
			page, err := archive.Search(context.Background(), SearchQuery{WithheldOnly: true, Limit: 20})
			encoded, marshalErr := json.Marshal(page)
			if err != nil || marshalErr != nil || strings.Contains(string(encoded), secret) {
				t.Fatalf("withheld listing leaked input: %s (%v, %v)", encoded, err, marshalErr)
			}
		})
	}
}

func TestArchiveWithholdingPreservesAdmittedHistoryWithoutRawWithheldIdentity(t *testing.T) {
	archive, _ := openTestArchive(t)
	item := archiveRecord("claude", "profile-a", "identity-transition", "previously admitted safe body")
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	secret := "FixtureIdentityTransition91A3!"
	item.Schema = "api_key=" + secret
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	var joined string
	if err := archive.db.QueryRow(`SELECT
		COALESCE((SELECT group_concat(id||source||instance_id||native_item_id||title||summary||withheld_reason) FROM records),'') ||
		COALESCE((SELECT group_concat(record_id||content_digest||predecessor_digest||title||summary||content||references_json) FROM versions),'') ||
		COALESCE((SELECT group_concat(record_id||source||instance_id||native_item_id||reference_json) FROM origins),'') ||
		COALESCE((SELECT group_concat(record_id||source||instance_id||native_item_id||reference_json) FROM version_occurrences),'') ||
		COALESCE((SELECT group_concat(record_digest||safe_source||reason) FROM withheld_observations),'') ||
		COALESCE((SELECT group_concat(title||summary||content||references_text) FROM knowledge_fts),'')`).Scan(&joined); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(joined, secret) || !strings.Contains(joined, "previously admitted safe body") {
		t.Fatal("withheld transition leaked the triggering identity or destroyed admitted history")
	}
	var versions, origins int
	if err := archive.db.QueryRow(`SELECT COUNT(*) FROM versions WHERE record_id=?`, recordID(item)).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := archive.db.QueryRow(`SELECT COUNT(*) FROM origins WHERE record_id=?`, recordID(item)).Scan(&origins); err != nil {
		t.Fatal(err)
	}
	if versions != 1 || origins != 1 {
		t.Fatalf("withheld transition did not preserve admitted versions/origins: %d/%d", versions, origins)
	}
}

func TestArchiveSafeWithheldSafeCycleRestoresCurrentSearchWithoutChangingOrigins(t *testing.T) {
	archive, _ := openTestArchive(t)
	ctx := context.Background()
	item := archiveRecord("claude", "profile-a", "recoverable-session", "safe retained body")
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	firstRefs := append([]seshat.KIReference(nil), item.References...)
	item.References = append(item.References, seshat.KIReference{Type: "file", Value: "second-reference.txt"})
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	item.Schema = "api_key=WithheldSchemaSecret931!"
	if run, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil || run.Withheld != 1 {
		t.Fatalf("withhold refreshed source = %+v, %v", run, err)
	}
	item.Schema = "fixture-v1"
	item.References = firstRefs
	// Revision two has different references. Returning to revision one's
	// references is a new immutable occurrence, not an unchanged current item.
	if run, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil || run.Versioned != 1 || run.Unchanged != 0 || run.Added != 0 {
		t.Fatalf("safe recovery import = %+v, %v", run, err)
	}
	if run, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil || run.Unchanged != 1 || run.Versioned != 0 || run.Added != 0 {
		t.Fatalf("exact safe recovery repeat = %+v, %v", run, err)
	}
	page, err := archive.Search(ctx, SearchQuery{Text: "safe retained body"})
	if err != nil || len(page.Items) != 1 || page.Items[0].WithheldReason != "" {
		t.Fatalf("recovered item search = %+v, %v", page, err)
	}
	var versions, origins, occurrences, fts int
	for query, destination := range map[string]*int{
		`SELECT COUNT(*) FROM versions WHERE record_id=?`:                     &versions,
		`SELECT COUNT(*) FROM origins WHERE record_id=?`:                      &origins,
		`SELECT COUNT(*) FROM version_occurrences WHERE record_id=?`:          &occurrences,
		`SELECT COUNT(*) FROM knowledge_fts WHERE record_id=? AND revision=3`: &fts,
	} {
		if err := archive.db.QueryRow(query, recordID(item)).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	if versions != 3 || origins != 2 || occurrences != 3 || fts != 1 {
		t.Fatalf("safe cycle changed retained version/provenance state: versions=%d origins=%d occurrences=%d fts=%d", versions, origins, occurrences, fts)
	}
}

func TestArchiveVersionDetailReturnsOnlyItsOwnProvenance(t *testing.T) {
	archive, _ := openTestArchive(t)
	ctx := context.Background()
	item := archiveRecord("claude", "profile-a", "version-provenance", "version one content")
	item.References = []seshat.KIReference{{Type: "file", Value: "file-a"}}
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	item.Content = "version two content"
	item.References = []seshat.KIReference{{Type: "file", Value: "file-b"}}
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	first, err := archive.Detail(ctx, recordID(item), 1)
	if err != nil || len(first.Origins) != 1 || first.Origins[0].Reference != "file:file-a" {
		t.Fatalf("revision 1 provenance = %+v, %v", first.Origins, err)
	}
	second, err := archive.Detail(ctx, recordID(item), 2)
	if err != nil || len(second.Origins) != 1 || second.Origins[0].Reference != "file:file-b" {
		t.Fatalf("revision 2 provenance = %+v, %v", second.Origins, err)
	}
}

func TestArchiveSameContentReferenceAndSchemaRevisionKeepsVersionProvenance(t *testing.T) {
	archive, _ := openTestArchive(t)
	ctx := context.Background()
	item := archiveRecord("claude", "profile-a", "same-content-provenance", "unchanged full content")
	item.Schema = "schema-a"
	item.References = []seshat.KIReference{{Type: "file", Value: "ref-a"}}
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	firstObserved := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, err := archive.db.Exec(`UPDATE version_occurrences SET observed_at=? WHERE record_id=? AND revision=1`, firstObserved.Format(time.RFC3339Nano), recordID(item)); err != nil {
		t.Fatal(err)
	}
	item.Schema = "schema-b"
	item.References = []seshat.KIReference{{Type: "file", Value: "ref-b"}}
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	secondObserved := time.Now().UTC().Add(-time.Second).Truncate(time.Microsecond)
	if _, err := archive.db.Exec(`UPDATE version_occurrences SET observed_at=? WHERE record_id=? AND revision=2`, secondObserved.Format(time.RFC3339Nano), recordID(item)); err != nil {
		t.Fatal(err)
	}
	first, err := archive.Detail(ctx, recordID(item), 1)
	if err != nil || first.Revision != 1 || first.ContentDigest == "" || len(first.Origins) != 1 || first.Origins[0].Reference != "file:ref-a" || !first.Origins[0].ObservedAt.Equal(firstObserved) {
		t.Fatalf("revision 1 provenance = %+v, %v", first, err)
	}
	second, err := archive.Detail(ctx, recordID(item), 2)
	if err != nil || second.Revision != 2 || second.ContentDigest != first.ContentDigest || len(second.Origins) != 1 || second.Origins[0].Reference != "file:ref-b" || !second.Origins[0].ObservedAt.Equal(secondObserved) {
		t.Fatalf("revision 2 provenance = %+v, %v", second, err)
	}
}

func TestArchiveOpenFreshAndExistingCompletesInBoundedChild(t *testing.T) {
	home := t.TempDir()
	for _, mode := range []string{"fresh", "existing"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestArchiveOpenBoundedChildHelper$")
			cmd.Env = append(os.Environ(), "SIRSI_ARCHIVE_OPEN_CHILD_HOME="+home, "SIRSI_ARCHIVE_OPEN_CHILD_MODE="+mode)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("archive %s Open/import/search/close/reopen exceeded watchdog: %s", mode, output)
			}
			if err != nil {
				t.Fatalf("archive %s child failed: %v\n%s", mode, err, output)
			}
		})
	}
}

func TestArchiveOpenBoundedChildHelper(t *testing.T) {
	home := os.Getenv("SIRSI_ARCHIVE_OPEN_CHILD_HOME")
	if home == "" {
		return
	}
	mode := os.Getenv("SIRSI_ARCHIVE_OPEN_CHILD_MODE")
	if mode == "fresh" {
		if _, err := os.Stat(filepath.Join(home, ".sirsi", "maat", "knowledge", "archive.sqlite3")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("fresh fixture archive already exists: %v", err)
		}
	} else if mode == "existing" {
		if _, err := os.Stat(filepath.Join(home, ".sirsi", "maat", "knowledge", "archive.sqlite3")); err != nil {
			t.Fatalf("existing fixture archive missing: %v", err)
		}
	} else {
		t.Fatalf("unknown child mode %q", mode)
	}
	item := archiveRecord("claude", "bounded-open-profile", "bounded-open-item", "bounded child archive record")
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	page, err := archive.Search(context.Background(), SearchQuery{Text: "bounded child archive record", Limit: 5})
	if err != nil || len(page.Items) != 1 {
		_ = archive.Close()
		t.Fatalf("search after open = %+v, %v", page, err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	archive, err = Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if _, err := archive.Detail(context.Background(), page.Items[0].ID, 0); err != nil {
		t.Fatalf("detail after reopen = %v", err)
	}
}

func TestArchiveWithheldEmptyBatchSourceIsSafe(t *testing.T) {
	archive, _ := openTestArchive(t)
	secret := "api_key=EmptyBatchSecret731!"
	run, err := archive.ImportSource(context.Background(), secret, nil, nil)
	if err != nil || run.Source != "withheld" || run.State != ImportUnverifiable {
		t.Fatalf("sensitive empty-batch source = %+v, %v", run, err)
	}
	var values string
	err = archive.db.QueryRow(`SELECT group_concat(source||error_code) FROM import_runs`).Scan(&values)
	if err != nil || strings.Contains(values, secret) {
		t.Fatalf("sensitive empty-batch source reached coverage: %q (%v)", values, err)
	}
}

func TestArchiveSourceTransactionRollsBackAsAUnit(t *testing.T) {
	archive, _ := openTestArchive(t)
	if _, err := archive.db.Exec(`CREATE TRIGGER reject_origin BEFORE INSERT ON origins BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	item := archiveRecord("claude", "profile-a", "rollback-session", "transactional source text")
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err == nil {
		t.Fatal("origin trigger did not abort import")
	}
	for _, table := range []string{"records", "versions", "origins", "import_runs", "source_cursors"} {
		var count int
		query := `SELECT COUNT(*) FROM ` + table
		if table == "source_cursors" {
			query += ` WHERE source='claude'`
		}
		if err := archive.db.QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("transaction left %d rows in %s (err=%v)", count, table, err)
		}
	}
}

func TestArchiveOwnerAnnotationIsSeparateAndLeavesProjectionDebt(t *testing.T) {
	archive, _ := openTestArchive(t)
	item := archiveRecord("claude", "profile-a", "review-session", "source evidence remains immutable")
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	annotationID, err := archive.RecordOwnerAnnotation(context.Background(), recordID(item), 1, contentDigest(item), ResolutionReviewed, "Owner reviewed the retained source evidence.")
	if err != nil || annotationID == 0 {
		t.Fatalf("owner annotation = %d, %v", annotationID, err)
	}
	detail, err := archive.Detail(context.Background(), recordID(item), 0)
	if err != nil || detail.Resolution != ResolutionReviewed || len(detail.Annotations) != 1 {
		t.Fatalf("owner resolution projection = %+v, %v", detail, err)
	}
	if detail.Content != item.Content {
		t.Fatal("owner review changed immutable source content")
	}
	coverage, err := archive.Coverage(context.Background(), 50)
	if err != nil || coverage.PendingProjectionDebt != 1 {
		t.Fatalf("pending projection debt = %+v, %v", coverage, err)
	}
	pending, err := archive.PendingProjectionAnnotations(context.Background(), 10)
	if err != nil || len(pending) != 1 || pending[0].ID != annotationID || pending[0].RecordID != recordID(item) || pending[0].Text != "Owner reviewed the retained source evidence." {
		t.Fatalf("pending projection event = %+v, %v", pending, err)
	}
	if err := archive.MarkProjected(context.Background(), annotationID); err != nil {
		t.Fatal(err)
	}
	pending, err = archive.PendingProjectionAnnotations(context.Background(), 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("acknowledged projection remains pending: %+v, %v", pending, err)
	}
	coverage, err = archive.Coverage(context.Background(), 50)
	if err != nil || coverage.PendingProjectionDebt != 0 {
		t.Fatalf("projected coverage = %+v, %v", coverage, err)
	}
	var versionCount int
	if err := archive.db.QueryRow(`SELECT COUNT(*) FROM versions WHERE record_id=?`, recordID(item)).Scan(&versionCount); err != nil || versionCount != 1 {
		t.Fatalf("annotation changed source version history: count=%d err=%v", versionCount, err)
	}
	if _, err := archive.RecordOwnerAnnotation(context.Background(), recordID(item), 1, contentDigest(item), ResolutionAccepted, "password=KnownSecret73!"); err == nil {
		t.Fatal("sensitive owner note was retained")
	}
}

func TestArchiveOwnerDecisionRefusesReviewedVersionDrift(t *testing.T) {
	archive, _ := openTestArchive(t)
	ctx := context.Background()
	item := archiveRecord("claude", "profile-a", "review-drift", "reviewed revision one")
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	reviewed, err := archive.Detail(ctx, recordID(item), 0)
	if err != nil {
		t.Fatal(err)
	}
	item.Content = "unseen revision two"
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.RecordOwnerAnnotation(ctx, reviewed.ID, reviewed.Revision, reviewed.ContentDigest, ResolutionAccepted, "Reviewed only the displayed revision."); err == nil {
		t.Fatal("owner decision accepted after current revision changed")
	}
	current, err := archive.Detail(ctx, reviewed.ID, 0)
	if err != nil || current.Resolution != ResolutionOpen || len(current.Annotations) != 0 {
		t.Fatalf("revision drift changed current resolution: %+v, %v", current, err)
	}
	// The stale approval must stay refused even if submitted twice. Only a
	// fresh review of the newly read current detail can authorize that version.
	if _, err := archive.RecordOwnerAnnotation(ctx, reviewed.ID, reviewed.Revision, reviewed.ContentDigest, ResolutionAccepted, "Reviewed only the displayed revision."); err == nil {
		t.Fatal("repeated stale owner decision accepted after current revision changed")
	}
	reviewed = current
	if _, err := archive.RecordOwnerAnnotation(ctx, reviewed.ID, reviewed.Revision, reviewed.ContentDigest, ResolutionAccepted, "Reviewed the freshly reopened current revision."); err != nil {
		t.Fatalf("exact reviewed version was not accepted: %v", err)
	}
	item.Content = "unseen revision three"
	if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	current, err = archive.Detail(ctx, reviewed.ID, 0)
	if err != nil || current.Resolution != ResolutionOpen {
		t.Fatalf("new version inherited prior acceptance: %+v, %v", current, err)
	}
	historical, err := archive.Detail(ctx, reviewed.ID, reviewed.Revision)
	if err != nil || historical.Resolution != ResolutionAccepted {
		t.Fatalf("accepted historical revision lost its decision: %+v, %v", historical, err)
	}
}

func TestArchiveSearchAndHistoryCursorsAreStableAndBounded(t *testing.T) {
	archive, _ := openTestArchive(t)
	for _, nativeID := range []string{"page-a", "page-b"} {
		item := archiveRecord("claude", "profile-a", nativeID, "unique content "+nativeID)
		if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
			t.Fatal(err)
		}
	}
	first, err := archive.Search(context.Background(), SearchQuery{Limit: 1})
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first search page = %+v, %v", first, err)
	}
	second, err := archive.Search(context.Background(), SearchQuery{Limit: 1, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second search page = %+v, %v", second, err)
	}
	item := archiveRecord("claude", "profile-a", "history-page", "revision one")
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"revision two", "revision three"} {
		item.Content = body
		if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
			t.Fatal(err)
		}
	}
	history, err := archive.HistoryPage(context.Background(), recordID(item), 1, "")
	if err != nil || len(history.Items) != 1 || history.NextCursor == "" {
		t.Fatalf("first history page = %+v, %v", history, err)
	}
	older, err := archive.HistoryPage(context.Background(), recordID(item), 1, history.NextCursor)
	if err != nil || len(older.Items) != 1 || older.Items[0].Revision >= history.Items[0].Revision {
		t.Fatalf("older history page = %+v, %v", older, err)
	}
	item.Content = "revision four"
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.HistoryPage(context.Background(), recordID(item), 1, history.NextCursor); err == nil {
		t.Fatal("stale history cursor survived an archive generation change")
	}
}

func TestArchiveSearchCursorInvalidatesWhenCoverageChangesWithoutItems(t *testing.T) {
	archive, _ := openTestArchive(t)
	ctx := context.Background()
	for _, nativeID := range []string{"coverage-page-a", "coverage-page-b"} {
		item := archiveRecord("claude", "profile-a", nativeID, "coverage snapshot "+nativeID)
		if _, err := archive.ImportSource(ctx, item.Source, []seshat.SourceRecord{item}, nil); err != nil {
			t.Fatal(err)
		}
	}
	first, err := archive.Search(ctx, SearchQuery{Limit: 1})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first search page = %+v, %v", first, err)
	}
	if _, err := archive.ImportSource(ctx, "claude", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Search(ctx, SearchQuery{Limit: 1, Cursor: first.NextCursor}); err == nil {
		t.Fatal("search cursor survived a coverage-only generation change")
	}
}

func TestArchiveSearchSnapshotDoesNotMixConcurrentConnectionCommit(t *testing.T) {
	archive, home := openTestArchive(t)
	ctx := context.Background()
	first := archiveRecord("claude", "profile-a", "snapshot-a", "snapshot original row")
	if _, err := archive.ImportSource(ctx, first.Source, []seshat.SourceRecord{first}, nil); err != nil {
		t.Fatal(err)
	}
	writer, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writeResult := make(chan error, 1)
	started := make(chan struct{})
	tx, err := archive.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := archive.searchSnapshotWithHook(ctx, SearchQuery{Limit: 10}, tx, func() {
		go func() {
			writeTx, beginErr := writer.db.BeginTx(ctx, nil)
			if beginErr != nil {
				close(started)
				writeResult <- beginErr
				return
			}
			if _, updateErr := writeTx.ExecContext(ctx, `UPDATE records SET withheld_reason='sensitivity_rule' WHERE id=?`, recordID(first)); updateErr != nil {
				_ = writeTx.Rollback()
				close(started)
				writeResult <- updateErr
				return
			}
			if _, updateErr := writeTx.ExecContext(ctx, `UPDATE archive_meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); updateErr != nil {
				_ = writeTx.Rollback()
				close(started)
				writeResult <- updateErr
				return
			}
			close(started)
			writeResult <- writeTx.Commit()
		}()
		<-started
	})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 1 || page.Withheld != 0 || page.Items[0].ID != recordID(first) || page.Items[0].WithheldReason != "" {
		_ = tx.Rollback()
		t.Fatalf("page mixed a concurrent second-connection commit: %+v", page)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-writeResult:
		if err != nil {
			t.Fatalf("waiting concurrent writer did not commit after the snapshot ended: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("concurrent writer did not finish after the read snapshot ended")
	}
	updated, err := archive.Search(ctx, SearchQuery{Limit: 10})
	if err != nil || updated.Total != 0 || updated.Withheld != 1 || len(updated.Items) != 0 {
		t.Fatalf("post-commit search = %+v, %v", updated, err)
	}
}

func TestArchiveConcurrentRepeatImportsRemainSingleIdentity(t *testing.T) {
	archive, _ := openTestArchive(t)
	item := archiveRecord("claude", "profile-a", "concurrent-session", "concurrently observed source")
	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var records, versions, origins int
	for query, target := range map[string]*int{
		`SELECT COUNT(*) FROM records`:  &records,
		`SELECT COUNT(*) FROM versions`: &versions,
		`SELECT COUNT(*) FROM origins`:  &origins,
	} {
		if err := archive.db.QueryRow(query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if records != 1 || versions != 1 || origins != 1 {
		t.Fatalf("concurrent duplicate identity counts records=%d versions=%d origins=%d", records, versions, origins)
	}
}

func TestLegacyMigrationImportsAllSnapshotsIdempotentlyAndPreservesFiles(t *testing.T) {
	archive, home := openTestArchive(t)
	legacyDir := filepath.Join(home, ".config", "seshat", "store")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	items := []seshat.KnowledgeItem{{
		ID: "legacy-item-a", Title: "Retained legacy item", Summary: "An old summary", Content: "old full body",
		References: []seshat.KIReference{{Type: "source", Value: "claude"}, {Type: "conversation_id", Value: "legacy-session"}},
	}}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(legacyDir, "latest.json"), filepath.Join(legacyDir, "ingestion_2026-10-24.json")}
	for _, path := range paths {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := make(map[string]string, len(paths))
	for _, path := range paths {
		contents, _ := os.ReadFile(path)
		digest := sha256.Sum256(contents)
		before[path] = hex.EncodeToString(digest[:])
	}
	runs, err := archive.MigrateLegacy(context.Background(), legacyDir)
	if err != nil || len(runs) == 0 {
		t.Fatalf("legacy migration = %+v, %v", runs, err)
	}
	var revisions int
	if err := archive.db.QueryRow(`SELECT COUNT(*) FROM versions`).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatalf("legacy source histories = %d, %v", revisions, err)
	}
	secondRuns, err := archive.MigrateLegacy(context.Background(), legacyDir)
	if err != nil || len(secondRuns) != 0 {
		t.Fatalf("idempotent migration = %+v, %v", secondRuns, err)
	}
	for _, path := range paths {
		contents, _ := os.ReadFile(path)
		digest := sha256.Sum256(contents)
		if hex.EncodeToString(digest[:]) != before[path] {
			t.Fatalf("legacy source file was modified: %s", filepath.Base(path))
		}
	}
}

func TestArchivePathCreatesPrivateChildWithoutChangingSirsiParent(t *testing.T) {
	archive, home := openTestArchive(t)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	sirsiPath := filepath.Join(home, ".sirsi")
	info, err := os.Stat(sirsiPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("new .sirsi mode = %o", info.Mode().Perm())
	}
	archiveDir := filepath.Join(home, ".sirsi", "maat", "knowledge")
	dirInfo, err := os.Stat(archiveDir)
	if err != nil || dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("archive directory is not private: info=%v err=%v", dirInfo, err)
	}
	fileInfo, err := os.Stat(filepath.Join(archiveDir, "archive.sqlite3"))
	if err != nil || fileInfo.Mode().Perm()&0o077 != 0 || fileInfo.Mode().IsRegular() == false {
		t.Fatalf("archive file is not private regular file: info=%v err=%v", fileInfo, err)
	}
}

func TestArchivePathPreservesExisting0755SirsiAndRejectsLinks(t *testing.T) {
	home := t.TempDir()
	sirsiPath := filepath.Join(home, ".sirsi")
	if err := os.Mkdir(sirsiPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sirsiPath, 0o755); err != nil {
		t.Fatal(err)
	}
	initialInfo, err := os.Stat(sirsiPath)
	if err != nil || initialInfo.Mode().Perm() != 0o755 {
		t.Fatalf("fixture requires existing 0755 .sirsi: info=%v err=%v", initialInfo, err)
	}
	archive, err := Open(home)
	if err != nil {
		t.Fatalf("open below existing 0755 .sirsi: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sirsiPath)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("existing .sirsi mode changed: info=%v err=%v", info, err)
	}

	symlinkHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(symlinkHome, ".sirsi"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(symlinkHome, ".sirsi", "maat")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(symlinkHome); err == nil {
		t.Fatal("archive followed a substituted parent symlink")
	}

	linkHome := t.TempDir()
	knowledgeDir := filepath.Join(linkHome, ".sirsi", "maat", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(original, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, filepath.Join(knowledgeDir, "archive.sqlite3")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(linkHome); err == nil {
		t.Fatal("archive accepted a hard-linked database target")
	}
	contents, err := os.ReadFile(original)
	if err != nil || string(contents) != "unrelated" {
		t.Fatalf("hard-link refusal changed unrelated sentinel: %q err=%v", contents, err)
	}
}

func TestArchiveCorruptionDoesNotFallBackToLegacySnapshots(t *testing.T) {
	home := t.TempDir()
	legacyDir := filepath.Join(home, ".config", "seshat", "store")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`[{"id":"legacy-only","title":"must not be projected","summary":"retained only in legacy input","references":[]}]`)
	legacyPath := filepath.Join(legacyDir, "latest.json")
	if err := os.WriteFile(legacyPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(home, ".sirsi", "maat", "knowledge")
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "archive.sqlite3"), []byte("not a SQLite archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(home, "must not be projected"); err == nil {
		t.Fatal("corrupt canonical archive fell back to legacy latest.json")
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil || string(after) != string(legacy) {
		t.Fatalf("failed open modified legacy history: %q (%v)", after, err)
	}
}
