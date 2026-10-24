package seshat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func claudeTranscriptFixture(session string) string {
	return strings.Join([]string{
		`{"type":"user","sessionId":"` + session + `","message":{"role":"user","content":"Question"}}`,
		`{"type":"assistant","sessionId":"` + session + `","message":{"role":"assistant","content":"Answer"}}`,
	}, "\n")
}

func TestClaudeArchiveReaderRetainsFullBodyAndStableNativeIdentity(t *testing.T) {
	root := t.TempDir()
	projects := filepath.Join(root, "projects", "fixture")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projects, "session.jsonl")
	transcript := strings.Join([]string{
		`{"type":"user","sessionId":"session-native-1","timestamp":"2026-10-24T10:00:00Z","message":{"role":"user","content":"What is the archive boundary?"}}`,
		`{"type":"assistant","sessionId":"session-native-1","timestamp":"2026-10-24T10:00:01Z","message":{"role":"assistant","content":"The private archive retains full versioned evidence."}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := &ClaudeAdapter{ClaudeDir: root}
	first, err := adapter.IngestArchive(time.Time{})
	if err != nil || len(first) != 1 {
		t.Fatalf("archive read = %d records, %v", len(first), err)
	}
	if first[0].NativeItemID != "session-native-1" || first[0].InstanceID == "" || !strings.Contains(first[0].Content, "private archive retains full versioned evidence") {
		t.Fatalf("source-native identity/full content not retained: %+v", first[0])
	}
	second, err := adapter.IngestArchive(time.Time{})
	if err != nil || len(second) != 1 || first[0].InstanceID != second[0].InstanceID || first[0].NativeItemID != second[0].NativeItemID {
		t.Fatalf("repeat source identity changed: first=%+v second=%+v err=%v", first, second, err)
	}
}

func TestClaudeArchiveReaderBoundsActualGrowthAndKeepsSiblingsPartial(t *testing.T) {
	root := t.TempDir()
	projects := filepath.Join(root, "projects")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(projects, "a.jsonl")
	secondPath := filepath.Join(projects, "b.jsonl")
	if err := os.WriteFile(firstPath, []byte(claudeTranscriptFixture("first")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte(claudeTranscriptFixture("second")), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := &ClaudeAdapter{ClaudeDir: root}
	firstBytes, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(len(firstBytes) + 2)
	items, err := adapter.ingestArchiveContext(context.Background(), time.Time{}, 1024, limit, 10)
	if err == nil || len(items) != 1 || items[0].NativeItemID != "first" {
		t.Fatalf("aggregate source bound did not preserve only the complete sibling: %d records, %v", len(items), err)
	}

	growing := filepath.Join(projects, "growing.jsonl")
	initial := []byte(claudeTranscriptFixture("growing"))
	if err := os.WriteFile(growing, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	_, read, err := adapter.parseArchiveTranscriptWithHook(context.Background(), growing, "instance", int64(len(initial)+4), func() error {
		file, err := os.OpenFile(growing, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.Write([]byte(strings.Repeat("x", 32)))
		return err
	})
	if err == nil || read <= int64(len(initial)) {
		t.Fatalf("growth after the earlier size observation was accepted: bytes=%d err=%v", read, err)
	}
}

func TestClaudeArchiveReaderStopsCanceledTraversal(t *testing.T) {
	root := t.TempDir()
	projects := filepath.Join(root, "projects")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projects, "session.jsonl"), []byte(claudeTranscriptFixture("cancel")), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	items, err := (&ClaudeAdapter{ClaudeDir: root}).IngestArchiveContext(ctx, time.Time{})
	if err == nil || len(items) != 0 {
		t.Fatalf("canceled traversal continued reading source: %d records, %v", len(items), err)
	}
}

func TestClaudeArchiveReaderRetainsReadableTranscriptOnMalformedSiblingLine(t *testing.T) {
	root := t.TempDir()
	projects := filepath.Join(root, "projects")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projects, "partial.jsonl")
	transcript := strings.Join([]string{
		`{"type":"user","sessionId":"partial-session","message":{"role":"user","content":"Keep this readable turn."}}`,
		`{malformed source line`,
		`{"type":"assistant","sessionId":"partial-session","message":{"role":"assistant","content":"Keep this readable answer."}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := (&ClaudeAdapter{ClaudeDir: root}).IngestArchive(time.Time{})
	if err == nil || len(items) != 1 {
		t.Fatalf("partial read = %d records, %v", len(items), err)
	}
	if !strings.Contains(items[0].Content, "Keep this readable answer") || items[0].NativeItemID != "partial-session" {
		t.Fatalf("readable transcript was lost on partial error: %+v", items[0])
	}
}
