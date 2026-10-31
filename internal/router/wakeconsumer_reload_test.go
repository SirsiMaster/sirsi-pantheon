package router

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWakeConsumerReloadSameControllerAfterJoin(t *testing.T) {
	testWakeConsumerReloadAfterJoin(t, false)
}
func TestWakeConsumerReloadInvalidController(t *testing.T) { testWakeConsumerReloadAfterJoin(t, true) }
func testWakeConsumerReloadAfterJoin(t *testing.T, invalid bool) {
	oldLoad := getLoadAvgFn()
	SetLoadAvgFn(func() (float64, bool) { return .1, true })
	t.Cleanup(func() { SetLoadAvgFn(oldLoad) })
	hermeticDispatchDir(t)
	fixtureDir := t.TempDir()
	log := filepath.Join(fixtureDir, "calls")
	gate := filepath.Join(fixtureDir, "release")
	finish := filepath.Join(fixtureDir, "finish-child")
	childReleased := filepath.Join(fixtureDir, "child-released")
	childExited := filepath.Join(fixtureDir, "child-exited")
	script := filepath.Join(t.TempDir(), "consumer.sh")
	quote := func(path string) string { return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'" }
	body := "#!/bin/sh\nprintf '%s thread=%s\\n' \"$*\" \"$SIRSI_THREAD_ID\" >> " + quote(log) + "\n"
	body += "if [ \"${2%% *}\" = old ]; then while [ ! -f " + quote(gate) + " ]; do sleep 0.01; done;\n"
	if invalid {
		body += ": > " + quote(childReleased) + "\nwhile [ ! -f " + quote(finish) + " ]; do sleep 0.01; done\n"
	}
	body += "fi\n"
	if invalid {
		body += ": > " + quote(childExited) + "\n"
	}
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{ID: "reload-agent", Type: "worker", WorkerAnswers: true, Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}, Prompt: "old {{thread}}"}}
	root := wakeTestRoot(t, cfg)
	sendItem(t, root, cfg.ID, "real fixture request")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	loopJoined := false
	waitForFile := func(path string, timeout time.Duration) bool {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(path); err == nil {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		_, err := os.Stat(path)
		return err == nil
	}
	t.Cleanup(func() {
		_ = os.WriteFile(gate, nil, 0600)
		if invalid {
			_ = os.WriteFile(finish, nil, 0600)
			if !waitForFile(childExited, 3*time.Second) {
				t.Errorf("inert consumer child did not exit during cleanup")
			}
		}
		cancel()
		if !loopJoined {
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("wake loop cleanup: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Errorf("wake loop did not join during cleanup")
			}
		}
	})
	go func() { done <- RunWakeLoop(ctx, root, cfg.ID, 20*time.Millisecond) }()
	select {
	case err := <-done:
		loopJoined = true
		t.Fatalf("controller startup: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	first := mustAwaitConsumerLines(t, log, 1)[0]
	controllerParts := strings.SplitN(first, "thread=", 2)
	if len(controllerParts) != 2 || controllerParts[1] == "" {
		t.Fatalf("initial consumer did not record its controller thread: %q", first)
	}
	controllerID := controllerParts[1]
	cfg.Consumer.Prompt = "new {{thread}}"
	if invalid {
		cfg.Consumer = ConsumerConfig{}
	}
	if err := SaveRegistry(root, &Registry{Agents: map[string]AgentConfig{cfg.ID: cfg}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	if lines := consumerLines(t, log); len(lines) != 1 {
		t.Fatalf("concurrent dispatch: %v", lines)
	}
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if invalid {
		if !waitForFile(childReleased, 3*time.Second) {
			t.Fatal("old child did not reach the controlled post-release hold")
		}
		if _, err := os.Stat(childExited); err == nil {
			t.Fatal("negative control requires the old child to remain unjoined")
		} else if !os.IsNotExist(err) {
			t.Fatalf("check delayed-child exit marker: %v", err)
		}
		// This gate holds the inert child past the old test's 180 ms sleep.
		// At that point the existing false/true oracle would fail because the
		// loop correctly has not reloaded while its old child is still running.
		time.Sleep(350 * time.Millisecond)
		reg, err := LoadThreadRegistry(root)
		if err != nil {
			t.Fatal(err)
		}
		stale := reg.Threads[controllerID]
		if stale == nil || stale.AgentID != cfg.ID || !stale.ConsumerCapable || !stale.AnswerRole {
			t.Fatalf("delayed-child negative control did not preserve old same-controller grants: %#v", stale)
		}
		if err := os.WriteFile(finish, nil, 0600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			reg, err = LoadThreadRegistry(root)
			if err != nil {
				t.Fatalf("read controller registry while awaiting joined reload: %v", err)
			}
			thr := reg.Threads[controllerID]
			if thr != nil && thr.AgentID == cfg.ID && !thr.ConsumerCapable && thr.AnswerRole {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("joined reload did not publish invalid replacement as false/true on controller %q: %#v", controllerID, thr)
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !waitForFile(childExited, 3*time.Second) {
			t.Fatal("consumer child did not exit before capability reload")
		}
		cancel()
		select {
		case err := <-done:
			loopJoined = true
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("wake loop did not stop after capability reload")
		}
		if lines := consumerLines(t, log); len(lines) != 1 {
			t.Fatalf("invalid replacement dispatched: %v", lines)
		}
		reg, err = LoadThreadRegistry(root)
		if err != nil {
			t.Fatal(err)
		}
		thr := reg.Threads[controllerID]
		if thr == nil || thr.AgentID != cfg.ID || thr.ConsumerCapable || !thr.AnswerRole {
			t.Fatalf("invalid replacement capability/answer grant on controller %q = %#v, want false/true", controllerID, thr)
		}
		return
	}
	lines := mustAwaitConsumerLines(t, log, 2)
	cancel()
	select {
	case err := <-done:
		loopJoined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("wake loop did not stop after valid replacement dispatch")
	}
	if !strings.Contains(first, "old ") || !strings.Contains(lines[1], "new ") {
		t.Fatalf("prompts: %v", lines)
	}
	a := strings.Split(first, "thread=")
	b := strings.Split(lines[1], "thread=")
	if len(a) != 2 || len(b) != 2 || a[1] == "" || a[1] != b[1] {
		t.Fatalf("controller changed: %v", lines)
	}
}

func TestWakeConsumerReloadRejectsRemovedAndInvalid(t *testing.T) {
	script := recordingConsumer(t, filepath.Join(t.TempDir(), "calls"), 0)
	cfg := AgentConfig{ID: "reload-invalid", Type: "worker", WorkerAnswers: true, Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}, Prompt: "work {{thread}}"}}
	root := wakeTestRoot(t, cfg)
	old, role, err := loadWakeConsumer(root, cfg.ID, "actual-controller")
	if err != nil || old == nil || !role {
		t.Fatalf("initial %v", err)
	}
	if !strings.Contains(strings.Join(old.Argv, " "), "actual-controller") {
		t.Fatal("unbound thread")
	}
	cfg.Consumer = ConsumerConfig{}
	if err := SaveRegistry(root, &Registry{Agents: map[string]AgentConfig{cfg.ID: cfg}}); err != nil {
		t.Fatal(err)
	}
	rc, role, err := loadWakeConsumer(root, cfg.ID, "actual-controller")
	if err == nil || rc != nil || !role {
		t.Fatal("missing consumer changed the lead's answer grant or retained capability")
	}
	if err := SaveRegistry(root, &Registry{Agents: map[string]AgentConfig{}}); err != nil {
		t.Fatal(err)
	}
	rc, role, err = loadWakeConsumer(root, cfg.ID, "actual-controller")
	if err == nil || rc != nil || role {
		t.Fatal("removed lane retained authority or answer grant")
	}
	if !strings.Contains(strings.Join(old.Argv, " "), "work actual-controller") {
		t.Fatal("old child snapshot mutated")
	}
}
