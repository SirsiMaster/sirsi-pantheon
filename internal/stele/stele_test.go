package stele

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestAppendAndRead(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.jsonl")

	ledger, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Write 3 entries
	for i, deity := range []string{"ra", "maat", "thoth"} {
		err := ledger.Append(deity, TypeToolUse, "test", map[string]string{"seq": string(rune('0' + i))})
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	// Read all entries using a reader
	reader := &Reader{
		stelePath:  path,
		offsetPath: filepath.Join(t.TempDir(), "test.offset"),
	}

	entries, err := reader.ReadNew()
	if err != nil {
		t.Fatalf("ReadNew: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Deity != "ra" {
		t.Errorf("entry 0 deity = %q, want ra", entries[0].Deity)
	}
	if entries[2].Deity != "thoth" {
		t.Errorf("entry 2 deity = %q, want thoth", entries[2].Deity)
	}
}

func TestHashChain(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "chain.jsonl")

	ledger, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for i := 0; i < 5; i++ {
		_ = ledger.Append("test", TypeText, "", map[string]string{"n": string(rune('0' + i))})
	}

	valid, errs := Verify(path)
	if len(errs) != 0 {
		t.Fatalf("Verify found %d errors: %v", len(errs), errs)
	}
	if valid != 5 {
		t.Fatalf("valid = %d, want 5", valid)
	}
}

func TestReaderOffset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "offset.jsonl")

	ledger, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Write 3 entries
	for i := 0; i < 3; i++ {
		_ = ledger.Append("test", TypeText, "", map[string]string{"n": string(rune('0' + i))})
	}

	reader := &Reader{
		stelePath:  path,
		offsetPath: filepath.Join(dir, "reader.offset"),
	}

	// First read: get all 3
	entries, _ := reader.ReadNew()
	if len(entries) != 3 {
		t.Fatalf("first read: expected 3, got %d", len(entries))
	}

	// Second read: nothing new
	entries, _ = reader.ReadNew()
	if len(entries) != 0 {
		t.Fatalf("second read: expected 0, got %d", len(entries))
	}

	// Write 2 more
	_ = ledger.Append("test", TypeText, "", map[string]string{"n": "3"})
	_ = ledger.Append("test", TypeText, "", map[string]string{"n": "4"})

	// Third read: only the 2 new ones
	entries, _ = reader.ReadNew()
	if len(entries) != 2 {
		t.Fatalf("third read: expected 2, got %d", len(entries))
	}
	if entries[0].Data["n"] != "3" {
		t.Errorf("entry[0].n = %q, want 3", entries[0].Data["n"])
	}
}

func TestConcurrentAppend(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "concurrent.jsonl")

	ledger, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// 4 goroutines each appending 10 entries
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				_ = ledger.Append("worker", TypeText, "", map[string]string{
					"gid": string(rune('0' + gid)),
					"seq": string(rune('0' + i)),
				})
			}
		}(g)
	}
	wg.Wait()

	// All 40 entries should be valid
	valid, errs := Verify(path)
	if len(errs) != 0 {
		t.Fatalf("concurrent Verify found %d errors: %v", len(errs), errs)
	}
	if valid != 40 {
		t.Fatalf("valid = %d, want 40", valid)
	}
}

func TestVerify_EmptyFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "empty.jsonl")

	valid, errs := Verify(path)
	// File doesn't exist — should report error
	if len(errs) == 0 {
		t.Fatal("expected error for nonexistent file")
	}
	if valid != 0 {
		t.Fatalf("valid = %d, want 0", valid)
	}
}

func TestOpen_ContinuesChain(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "continue.jsonl")

	// Write 3 entries, close, reopen, write 2 more
	l1, _ := Open(path)
	_ = l1.Append("a", TypeText, "", nil)
	_ = l1.Append("b", TypeText, "", nil)
	_ = l1.Append("c", TypeText, "", nil)

	l2, _ := Open(path)
	_ = l2.Append("d", TypeText, "", nil)
	_ = l2.Append("e", TypeText, "", nil)

	// Chain should be unbroken across both ledger instances
	valid, errs := Verify(path)
	if len(errs) != 0 {
		t.Fatalf("chain broke across reopen: %v", errs)
	}
	if valid != 5 {
		t.Fatalf("valid = %d, want 5", valid)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()
	p := DefaultPath()
	if p == "" {
		t.Fatal("DefaultPath returned empty")
	}
}
