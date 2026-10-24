package modelrouter

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPrivateDecisionSinkUsesPrivateChildAndPreserves0755Parent(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(home, ".sirsi")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
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
	if info, err := os.Stat(parent); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("existing parent mode/stat = %v, %v; want unchanged 0755", info, err)
	}
	dir := filepath.Join(parent, "model-router")
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("private child mode/stat = %v, %v", info, err)
	}
	path := filepath.Join(dir, "decisions.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Size() > decisionJournalLimit {
		t.Fatalf("journal mode/size = %o/%d", info.Mode().Perm(), info.Size())
	}
	if lockInfo, err := os.Stat(filepath.Join(dir, "decisions.lock")); err != nil || lockInfo.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode/stat = %v, %v", lockInfo, err)
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

func TestPrivateDecisionSinkRejectsUnsafeParentsAndDirectoryLinks(t *testing.T) {
	t.Run("writable parent", func(t *testing.T) {
		home := t.TempDir()
		parent := filepath.Join(home, ".sirsi")
		if err := os.Mkdir(parent, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(parent, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := NewPrivateDecisionSink(home).Record(DecisionRecord{Outcome: "selected"}); err == nil {
			t.Fatal("sink accepted group/world-writable parent")
		}
	})
	t.Run("parent symlink", func(t *testing.T) {
		home := t.TempDir()
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(home, ".sirsi")); err != nil {
			t.Fatal(err)
		}
		if err := NewPrivateDecisionSink(home).Record(DecisionRecord{Outcome: "selected"}); err == nil {
			t.Fatal("sink followed .sirsi symlink")
		}
	})
	t.Run("private child symlink", func(t *testing.T) {
		home := t.TempDir()
		parent := filepath.Join(home, ".sirsi")
		if err := os.Mkdir(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(parent, "model-router")); err != nil {
			t.Fatal(err)
		}
		if err := NewPrivateDecisionSink(home).Record(DecisionRecord{Outcome: "selected"}); err == nil {
			t.Fatal("sink followed private-child symlink")
		}
	})
}

func TestPrivateDecisionSinkRejectsHardlinkedJournalBeforeAppendOrTruncate(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{name: "below cap append", size: 32},
		{name: "above cap truncate", size: decisionJournalLimit + 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			parent := filepath.Join(home, ".sirsi")
			dir := filepath.Join(parent, "model-router")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(dir, "decisions.jsonl")
			sentinel := filepath.Join(dir, "unrelated-owned-file")
			original := []byte(strings.Repeat("s", tc.size))
			if err := os.WriteFile(journal, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(journal, sentinel); err != nil {
				t.Fatal(err)
			}
			if err := NewPrivateDecisionSink(home).Record(DecisionRecord{Outcome: "selected"}); err == nil {
				t.Fatal("sink accepted multiply-linked journal")
			}
			got, err := os.ReadFile(sentinel)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(original) {
				t.Fatalf("unrelated hard-link target was mutated: size %d, want %d", len(got), len(original))
			}
		})
	}
}

func TestPrivateDecisionSinkRefusesJournalSymlink(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".sirsi", "model-router")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, filepath.Join(dir, "decisions.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := NewPrivateDecisionSink(home).Record(DecisionRecord{Outcome: "selected"}); err == nil {
		t.Fatal("sink followed journal symlink")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "unchanged" {
		t.Fatalf("symlink target changed: %q, %v", got, err)
	}
}

func TestPrivateDecisionSinkBoundsJournalWithIndependentLockDescriptors(t *testing.T) {
	home := t.TempDir()
	sinkInit := NewPrivateDecisionSink(home)
	if err := sinkInit.Record(DecisionRecord{Outcome: "seed"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".sirsi", "model-router", "decisions.jsonl")
	record := DecisionRecord{At: time.Unix(1, 0).UTC(), Task: TaskGeneration, Privacy: PrivacyLocalOnly, Outcome: "selected"}
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	line = append(line, '\n')
	if len(line) >= decisionJournalLimit {
		t.Fatalf("fixture record size is unsuitable: %d", len(line))
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", decisionJournalLimit-len(line))), 0o600); err != nil {
		t.Fatal(err)
	}

	firstAtCheck := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondAtCheck := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	first := NewPrivateDecisionSink(home)
	first.lockWait = 2 * time.Second
	first.afterSizeCheck = func(needsTruncate bool) {
		if needsTruncate {
			t.Error("first writer should see one record fitting exactly at the cap")
		}
		close(firstAtCheck)
		<-releaseFirst
	}
	second := NewPrivateDecisionSink(home)
	second.lockWait = 2 * time.Second
	second.afterSizeCheck = func(needsTruncate bool) {
		if !needsTruncate {
			t.Error("second writer must recheck after first append and observe cap overflow")
		}
		close(secondAtCheck)
	}

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	firstStarted, secondStarted := false, false
	firstJoined, secondJoined := false, false
	t.Cleanup(func() {
		// Always unblock the first writer before joining. This cleanup is
		// registered before either goroutine starts, so Fatal and timeout paths
		// cannot strand a writer at the size-check hook.
		release()
		join := func(name string, started, joined bool, done <-chan error) {
			if !started || joined {
				return
			}
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("cleanup: %s writer returned: %v", name, err)
				}
			case <-timer.C:
				t.Errorf("cleanup: timed out joining %s writer", name)
			}
		}
		join("first", firstStarted, firstJoined, firstDone)
		join("second", secondStarted, secondJoined, secondDone)
	})
	wait := func(name string, done <-chan error, joined *bool) error {
		t.Helper()
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case err := <-done:
			*joined = true
			return err
		case <-timer.C:
			t.Fatalf("timed out joining %s writer", name)
			return nil
		}
	}
	firstStarted = true
	go func() { firstDone <- first.Record(record) }()
	select {
	case <-firstAtCheck:
	case <-time.After(time.Second):
		t.Fatal("first writer did not reach its locked size check")
	}
	secondStarted = true
	go func() { secondDone <- second.Record(record) }()
	select {
	case <-secondAtCheck:
		t.Fatal("independent descriptor crossed held flock during size-check interleaving")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	if err := wait("first", firstDone, &firstJoined); err != nil {
		t.Fatalf("first writer: %v", err)
	}
	if err := wait("second", secondDone, &secondJoined); err != nil {
		t.Fatalf("second writer: %v", err)
	}
	select {
	case <-secondAtCheck:
	default:
		t.Fatal("second writer never passed lock after first released")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > decisionJournalLimit {
		t.Fatalf("interleaved independent writers exceeded journal cap: %d", info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var latest DecisionRecord
	if len(data) == 0 || json.Unmarshal(data[:len(data)-1], &latest) != nil || latest.Outcome != "selected" {
		t.Fatalf("bounded journal did not retain a complete latest record: %q", data)
	}
}

func TestAcquireDecisionLockTimesOutWhileIndependentDescriptorHoldsIt(t *testing.T) {
	home := t.TempDir()
	if err := NewPrivateDecisionSink(home).Record(DecisionRecord{Outcome: "seed"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".sirsi", "model-router")
	dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dirFD)
	firstFD, err := openDecisionFile(dirFD, "decisions.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(firstFD)
	secondFD, err := openDecisionFile(dirFD, "decisions.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(secondFD)
	if err := unix.Flock(firstFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(firstFD, unix.LOCK_UN)
	started := time.Now()
	err = acquireDecisionLock(secondFD, dirFD, "decisions.lock", 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "bounded wait") {
		t.Fatalf("contended lock error = %v, want bounded timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("bounded lock wait took too long: %s", elapsed)
	}
}
