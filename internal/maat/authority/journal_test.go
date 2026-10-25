package authority

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

func mkJournalReq(resource, holder string) schedule.Reservation {
	return schedule.Reservation{Resource: resource, Holder: holder, Regime: schedule.RegimeQuiet, LeaseTTLSec: 120}
}

// A crash mid-Write leaves a partial last line. readAllLocked must treat an
// unterminated, unparsable last line as torn rather than fail every future
// Append/Since/Head (ADR-120 review P2-a).
func TestFileJournal_TornTailIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j, err := OpenFileJournal(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := j.Append(mkJournalReq("studio", "claude-io")); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if _, err := j.Append(mkJournalReq("m5", "claude-io")); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open for torn write: %v", err)
	}
	if _, err := f.WriteString(`{"seq":3,"reservation":{"resource"`); err != nil {
		t.Fatalf("torn write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := OpenFileJournal(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	head, err := reopened.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head != 2 {
		t.Fatalf("want head 2 after a torn tail, got %d", head)
	}
	entry, err := reopened.Append(mkJournalReq("studio", "claude-io"))
	if err != nil {
		t.Fatalf("append after torn tail: %v", err)
	}
	if entry.Seq != 3 {
		t.Fatalf("want the torn entry's seq reused, got %d", entry.Seq)
	}
}
