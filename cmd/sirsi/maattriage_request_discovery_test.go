package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatTriageRequestDiscoveryActualCLIContinuationAndChangedRestart(t *testing.T) {
	_, report, journal, reads := triageMemoryFixture(t)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{triageRequestTestID, "790c358a-d905-4546-871d-de40a76cc0ac"} {
		report.Timestamp = time.Now().UTC().Add(time.Duration(i) * time.Nanosecond)
		if _, err := runTriageFixture(t, "--confirm=false", "--prepare-record-request", id); err != nil {
			t.Fatal(err)
		}
		if _, err := runTriageFixture(t, "--record-request", id, "--confirm"); err != nil {
			t.Fatal(err)
		}
	}
	if *reads != 2 || journal.calls != 2 {
		t.Fatal("fixture did not record both actual CLI observations", *reads, journal.calls)
	}
	seen := map[string]bool{}
	cursor := ""
	snapshot := ""
	query := ""
	firstCursor := ""
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= 8 {
			t.Fatal("CLI continuation failed to terminate")
		}
		out, err := runTriageFixture(t, "--confirm=false", "--inspect-record-requests", "--request-limit", "1", "--request-cursor", cursor)
		var page maat.DiagnosticDiscoveryPage
		if err != nil || json.Unmarshal([]byte(out), &page) != nil {
			t.Fatal(out, err)
		}
		if pageNumber == 0 {
			snapshot = page.History.Snapshot
			query = page.History.QueryID
			firstCursor = page.History.NextCursor
		}
		if page.History.Snapshot != snapshot || page.History.QueryID != query || page.History.Returned != 1 || page.History.CountScope != "page" || page.History.AfterCursor != (pageNumber > 0) {
			t.Fatal("CLI lost snapshot/count semantics", page.History)
		}
		for _, r := range page.Records {
			key := "record:" + r.Intent.ID
			if seen[key] {
				t.Fatal("duplicate CLI record")
			}
			seen[key] = true
		}
		for _, r := range page.Requests {
			key := "request:" + r.Request.ID
			if seen[key] {
				t.Fatal("duplicate CLI request")
			}
			seen[key] = true
		}
		if page.History.Coverage == maat.HistoryCoverageComplete {
			if page.History.NextCursor != "" {
				t.Fatal("complete page retained continuation")
			}
			break
		}
		if page.History.Coverage != maat.HistoryCoveragePartial || page.History.NextCursor == "" {
			t.Fatal("partial CLI page has no continuation")
		}
		cursor = page.History.NextCursor
	}
	if len(seen) != 4 || *reads != 2 || journal.calls != 2 || firstCursor == "" {
		t.Fatal("CLI discovery omitted or replayed rows", len(seen), *reads, journal.calls)
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := store.PrepareDiagnosticRequest("890c358a-d905-4546-871d-de40a76cc0ac")
	closeErr := store.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	out, err := runTriageFixture(t, "--inspect-record-requests", "--request-limit", "1", "--request-cursor", firstCursor)
	var changed struct {
		ErrorCode string `json:"error_code"`
		Restart   bool   `json:"restart_required"`
	}
	if err == nil || json.Unmarshal([]byte(out), &changed) != nil || changed.ErrorCode != "snapshot_changed" || !changed.Restart {
		t.Fatal("CLI changed read did not require restart", out, err)
	}
	out, err = runTriageFixture(t, "--inspect-record-requests", "--request-limit", "1", "--request-cursor=")
	var fresh maat.DiagnosticDiscoveryPage
	if err != nil || json.Unmarshal([]byte(out), &fresh) != nil || fresh.History.AfterCursor || fresh.History.Snapshot == snapshot {
		t.Fatal("CLI did not restart fresh", out, err)
	}
	if *reads != 2 || journal.calls != 2 {
		t.Fatal("restart observed/appended")
	}
}

func TestMaatTriageExplicitEmptyRequestSelectorsNeverFallBackToLegacy(t *testing.T) {
	for _, name := range []string{"prepare-record-request", "record-request", "read-record-request", "resolve-record-request"} {
		for _, confirm := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/confirm_%t", name, confirm), func(t *testing.T) {
				home, _, journal, reads := triageMemoryFixture(t)
				if _, err := maat.InitializeDefaultStore(); err != nil {
					t.Fatal(err)
				}
				before := triageOwnedFixtureFiles(t, home)
				if _, err := runTriageFixture(t, "--"+name+"=", fmt.Sprintf("--confirm=%t", confirm)); err == nil {
					t.Fatal("explicit empty request accepted")
				}
				if *reads != 0 || journal.calls != 0 {
					t.Fatal("explicit empty selector fell into legacy work", *reads, journal.calls)
				}
				if after := triageOwnedFixtureFiles(t, home); !reflect.DeepEqual(before, after) {
					t.Fatal("invalid selector mutated owned fixture/WAL")
				}
			})
		}
	}
}

func triageOwnedFixtureFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected owned fixture leaf")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = fmt.Sprintf("%o:%x", info.Mode().Perm(), sha256.Sum256(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
