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
	SetLoadAvgFn(func() (float64, bool) { return .1, true })
	defer SetLoadAvgFn(nil)
	hermeticDispatchDir(t)
	log := filepath.Join(t.TempDir(), "calls")
	gate := filepath.Join(t.TempDir(), "release")
	script := filepath.Join(t.TempDir(), "consumer.sh")
	body := "#!/bin/sh\necho \"$* thread=$SIRSI_THREAD_ID\" >> " + log + "\nif [ \"${2%% *}\" = old ]; then while [ ! -f " + gate + " ]; do sleep 0.02; done; fi\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{ID: "reload-agent", Type: "worker", Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}, Prompt: "old {{thread}}"}}
	root := wakeTestRoot(t, cfg)
	sendItem(t, root, cfg.ID, "real fixture request")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunWakeLoop(ctx, root, cfg.ID, 20*time.Millisecond) }()
	select {
	case err := <-done:
		t.Fatalf("controller startup: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0600); cancel() })
	first := mustAwaitConsumerLines(t, log, 1)[0]
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
		time.Sleep(180 * time.Millisecond)
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if lines := consumerLines(t, log); len(lines) != 1 {
			t.Fatalf("invalid replacement dispatched: %v", lines)
		}
		reg, err := LoadThreadRegistry(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, thr := range reg.Threads {
			if thr.AgentID == cfg.ID && (thr.ConsumerCapable || thr.AnswerRole) {
				t.Fatal("invalid replacement retained capability/answer role")
			}
		}
		return
	}
	lines := mustAwaitConsumerLines(t, log, 2)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
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
	if err == nil || rc != nil || role {
		t.Fatal("invalid config retained authority")
	}
	if err := SaveRegistry(root, &Registry{Agents: map[string]AgentConfig{}}); err != nil {
		t.Fatal(err)
	}
	rc, role, err = loadWakeConsumer(root, cfg.ID, "actual-controller")
	if err == nil || rc != nil || role {
		t.Fatal("removed lane retained authority")
	}
	if !strings.Contains(strings.Join(old.Argv, " "), "work actual-controller") {
		t.Fatal("old child snapshot mutated")
	}
}
