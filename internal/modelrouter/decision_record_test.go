package modelrouter

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrivateDecisionSinkIsPrivateRedactedAndBounded(t *testing.T) {
	home := t.TempDir()
	sink := NewPrivateDecisionSink(home)
	for i := 0; i < 200; i++ {
		record := DecisionRecord{
			At: time.Unix(int64(i), 0).UTC(), Task: TaskJudgment, Privacy: PrivacyShareable,
			Latency: LatencyInteractive, MinContextTokens: 8192, RequestedOverride: LaneRemote,
			InitialLane: LaneRemote, FinalLane: LaneLocal, Provider: "local-profile", Model: "fixture-model",
			Reason: "structured exhaustion fallback", Outcome: "fallback_completed", FailureCategory: "",
		}
		if err := sink.Record(record); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	dir := filepath.Join(home, ".sirsi")
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode/stat = %v, %v", info, err)
	}
	path := filepath.Join(dir, "model-router-decisions.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Size() > decisionJournalLimit {
		t.Fatalf("journal mode/size = %o/%d", info.Mode().Perm(), info.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		var got DecisionRecord
		if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
			t.Fatalf("invalid bounded journal line: %v", err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 || count > 200 {
		t.Fatalf("bounded records = %d", count)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"prompt", "answer", "Bearer", "sk-proj-", "error body", "Measured state"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("decision journal contains forbidden content %q", forbidden)
		}
	}
}

func TestPrivateDecisionSinkRefusesNonPrivateExistingTargets(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".sirsi")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NewPrivateDecisionSink(home).Record(DecisionRecord{Task: TaskGeneration, Privacy: PrivacyLocalOnly, Outcome: "selected"}); err == nil {
		t.Fatal("sink accepted a group/world-accessible decision directory")
	}
}

func TestPrivateDecisionSinkTruncatesBeforeExceedingJournalLimit(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".sirsi")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "model-router-decisions.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", decisionJournalLimit-8)), 0o600); err != nil {
		t.Fatal(err)
	}
	record := DecisionRecord{Task: TaskGeneration, Privacy: PrivacyLocalOnly, Outcome: "selected"}
	if err := NewPrivateDecisionSink(home).Record(record); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > decisionJournalLimit {
		t.Fatalf("journal exceeded its bound: %d > %d", info.Size(), decisionJournalLimit)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got DecisionRecord
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil || got.Outcome != "selected" {
		t.Fatalf("bounded journal did not retain the latest record: %v (%q)", err, data)
	}
}
