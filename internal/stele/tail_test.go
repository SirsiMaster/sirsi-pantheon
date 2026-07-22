package stele

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLedger builds a ledger file from raw lines and returns its path.
func writeLedger(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stele.jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
	return path
}

func entryLine(seq int, eventType, origBytes string) string {
	return fmt.Sprintf(`{"seq":%d,"deity":"rtk","type":%q,"data":{"original_bytes":%q},"ts":"2026-07-15T00:00:00Z"}`,
		seq, eventType, origBytes)
}

func TestTailByTypeFrom(t *testing.T) {
	t.Run("filters to the requested type", func(t *testing.T) {
		path := writeLedger(t,
			entryLine(1, TypeRTKFilter, "100"),
			entryLine(2, TypeThothSync, "999"),
			entryLine(3, TypeRTKFilter, "200"),
		)
		got, err := TailByTypeFrom(path, TypeRTKFilter, 1<<20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 rtk entries, got %d", len(got))
		}
		if got[0].Data["original_bytes"] != "100" || got[1].Data["original_bytes"] != "200" {
			t.Errorf("wrong entries or order: %+v", got)
		}
	})

	t.Run("skips the partial line a mid-file seek lands on", func(t *testing.T) {
		// The window must cut into the middle of entry 1, so its truncated
		// remainder is discarded rather than parsed as a whole record.
		full := entryLine(1, TypeRTKFilter, "111")
		last := entryLine(2, TypeRTKFilter, "222")
		path := writeLedger(t, full, last)

		window := int64(len(last) + 10) // lands inside line 1
		got, err := TailByTypeFrom(path, TypeRTKFilter, window)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want only the whole trailing entry, got %d: %+v", len(got), got)
		}
		if got[0].Data["original_bytes"] != "222" {
			t.Errorf("want the last entry, got %+v", got[0])
		}
	})

	t.Run("window larger than the file reads every entry", func(t *testing.T) {
		path := writeLedger(t, entryLine(1, TypeRTKFilter, "5"))
		got, err := TailByTypeFrom(path, TypeRTKFilter, 1<<30)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1, got %d", len(got))
		}
	})

	t.Run("a corrupt line is skipped, not fatal", func(t *testing.T) {
		path := writeLedger(t,
			entryLine(1, TypeRTKFilter, "10"),
			`{"seq":2,"type":"rtk_filter",BROKEN`,
			entryLine(3, TypeRTKFilter, "30"),
		)
		got, err := TailByTypeFrom(path, TypeRTKFilter, 1<<20)
		if err != nil {
			t.Fatalf("a torn line must not fail the read: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want the 2 readable entries, got %d", len(got))
		}
	})

	t.Run("no matching entries returns empty, not an error", func(t *testing.T) {
		path := writeLedger(t, entryLine(1, TypeThothSync, "1"))
		got, err := TailByTypeFrom(path, TypeRTKFilter, 1<<20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("want none, got %d", len(got))
		}
	})

	t.Run("a missing ledger errors rather than reporting zero savings", func(t *testing.T) {
		_, err := TailByTypeFrom(filepath.Join(t.TempDir(), "absent.jsonl"), TypeRTKFilter, 1<<20)
		if err == nil {
			t.Fatal("want an error for a missing ledger")
		}
		if !strings.Contains(err.Error(), "stele") {
			t.Errorf("want a stele-scoped error, got %v", err)
		}
	})
}
