package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// Own HOME, registry, provider selection and store before using any router API.
// Startup effects are per-call fakes; no provider, process lock or live service
// is contacted. Command globals are restored after the synchronous worker exits.
func gemmaGrantFixture(t *testing.T) (*routerstore.SQLiteStore, string, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "SIRSI_") || strings.HasPrefix(key, "GIT_") {
			t.Setenv(key, "")
		}
	}
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_LOCAL_PROFILE", "")
	t.Setenv("SIRSI_AGENT_ID", gemmaWorkerAgent)
	t.Setenv("SIRSI_ROUTER_AGENT", gemmaWorkerAgent)
	t.Setenv("SIRSI_THREAD_ID", "")
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))
	root := t.TempDir()
	t.Chdir(root)
	routerRoot := filepath.Join(root, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0700); err != nil {
		t.Fatal(err)
	}
	reg := &router.Registry{Agents: map[string]router.AgentConfig{
		gemmaWorkerAgent: {
			ID: gemmaWorkerAgent, Type: "gemma", Command: []string{"sirsi"},
			Cwd: root, Repo: root, Workstream: "pantheon",
			Wake: router.WakeConfig{Mechanism: "none"}, WorkerAnswers: true,
		},
		"codex-pantheon": {
			ID: "codex-pantheon", Type: "codex", Command: []string{"sirsi"},
			Cwd: root, Repo: root, Workstream: "pantheon",
			Wake: router.WakeConfig{Mechanism: "none"},
		},
	}}
	if err := router.SaveRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}
	store, err := routerstore.OpenPath(os.Getenv("SIRSI_ROUTER_DB"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	id, _, err := store.SendGuarded(routerstore.SendReq{
		From: "codex-pantheon", To: gemmaWorkerAgent, Type: "proposal",
		Title: "ordinary granted fixture", Instructions: "Return the fixture draft.",
	})
	if err != nil {
		t.Fatal(err)
	}

	once, version, toolTurn := gemmaWorkerOnce, gemmaWorkerVersion, gemmaWorkerApolloToolTurn
	profile, tokens, tokenFile := gemmaWorkerProfile, gemmaWorkerMaxTokens, gemmaWorkerTokenFile
	itemID, allowed := gemmaWorkerItemID, gemmaWorkerAllowedTools
	profilePath, selected := gemmaWorkerInferenceProfilePath, gemmaWorkerSelectedProfile
	t.Cleanup(func() {
		gemmaWorkerOnce, gemmaWorkerVersion, gemmaWorkerApolloToolTurn = once, version, toolTurn
		gemmaWorkerProfile, gemmaWorkerMaxTokens, gemmaWorkerTokenFile = profile, tokens, tokenFile
		gemmaWorkerItemID, gemmaWorkerAllowedTools = itemID, allowed
		gemmaWorkerInferenceProfilePath, gemmaWorkerSelectedProfile = profilePath, selected
	})
	gemmaWorkerOnce, gemmaWorkerVersion, gemmaWorkerApolloToolTurn = true, false, false
	gemmaWorkerProfile, gemmaWorkerMaxTokens, gemmaWorkerTokenFile = "m5", 0, ""
	gemmaWorkerItemID, gemmaWorkerAllowedTools = id, nil
	gemmaWorkerInferenceProfilePath, gemmaWorkerSelectedProfile = "", nil
	return store, routerRoot, id
}

func TestGemmaWorkerOrdinaryGrantStampedBeforeInference(t *testing.T) {
	store, routerRoot, id := gemmaGrantFixture(t)
	declaration, err := os.ReadFile(filepath.Join(routerRoot, "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	var preflight, generated, locks, unlocks int
	runtime := gemmaWorkerRuntime{
		preflight: func() error { preflight++; return nil },
		tryLock: func(string) (func(), error) {
			locks++
			return func() { unlocks++ }, nil
		},
		generate: func(context.Context, string) (string, error) {
			generated++
			if err := gemmaWorkerVerifyStampedAnswerRole(routerRoot, os.Getenv("SIRSI_THREAD_ID")); err != nil {
				return "", fmt.Errorf("ordinary worker role missing before inference: %w", err)
			}
			return "ordinary nonbinding fixture draft", nil
		},
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runGemmaWorkerWith(cmd, runtime); err != nil {
		t.Fatal(err)
	}
	if preflight != 1 || generated != 1 || locks != 1 || unlocks != 1 {
		t.Fatalf("effects: preflight=%d generated=%d locks=%d unlocks=%d", preflight, generated, locks, unlocks)
	}
	item, err := store.Get(id)
	if err != nil || item.Status != routerstore.StatusCompleted {
		t.Fatalf("ordinary completion: status=%q err=%v", item.Status, err)
	}
	threads, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads.Threads) != 1 {
		t.Fatalf("worker threads=%d, want exactly one", len(threads.Threads))
	}
	for _, thread := range threads.Threads {
		if thread.AgentID != gemmaWorkerAgent || thread.Surface != "worker" || !thread.AnswerRole || thread.Status != router.ThreadStatusClosed {
			t.Fatalf("ordinary worker identity/role/cleanup mismatch: %+v", thread)
		}
	}
	after, err := os.ReadFile(filepath.Join(routerRoot, "agents.json"))
	if err != nil || string(after) != string(declaration) {
		t.Fatalf("worker changed its lead declaration: %v", err)
	}
}

func TestGemmaWorkerGrantRefusalPrecedesProviderClaimAndInference(t *testing.T) {
	for _, mode := range []string{"ordinary", "tool-turn"} {
		for _, refusal := range []string{"absent-grant", "missing-agent", "malformed-registry"} {
			t.Run(mode+"/"+refusal, func(t *testing.T) {
				store, routerRoot, id := gemmaGrantFixture(t)
				gemmaWorkerApolloToolTurn = mode == "tool-turn"
				// A missing profile would fail if startup reached provider setup.
				gemmaWorkerInferenceProfilePath = filepath.Join(t.TempDir(), "must-not-read.json")
				registry := `{"agents":{"gemma-pantheon":{"id":"gemma-pantheon","type":"gemma","worker_answers":false}}}`
				want := "headless worker may not answer"
				switch refusal {
				case "missing-agent":
					registry, want = `{"agents":{}}`, "read Gemma worker registration"
				case "malformed-registry":
					registry, want = `{`, "read Gemma answering-role declaration"
				}
				if err := os.WriteFile(filepath.Join(routerRoot, "agents.json"), []byte(registry), 0600); err != nil {
					t.Fatal(err)
				}
				var effects int
				runtime := gemmaWorkerRuntime{
					preflight: func() error { effects++; return nil },
					generate:  func(context.Context, string) (string, error) { effects++; return "unexpected", nil },
					tryLock:   func(string) (func(), error) { effects++; return func() {}, nil },
				}
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				err := runGemmaWorkerWith(cmd, runtime)
				if err == nil || !strings.Contains(err.Error(), want) || effects != 0 {
					t.Fatalf("refusal=%v effects=%d, want %q before effects", err, effects, want)
				}
				item, err := store.Get(id)
				if err != nil || item.Status != routerstore.StatusOpen || item.AckedAt != "" {
					t.Fatalf("refused worker claimed work: item=%+v err=%v", item, err)
				}
				threads, err := store.ListThreads()
				if err != nil || len(threads) != 0 {
					t.Fatalf("refused worker registered a thread: count=%d err=%v", len(threads), err)
				}
			})
		}
	}
}

func TestGemmaWorkerAuthorityRefusalBlocksSameDraftWithoutReinference(t *testing.T) {
	store, _, id := gemmaGrantFixture(t)
	var generated int
	reply := &gemmaWorkerReplyFake{err: fmt.Errorf("completion refused: %w", routerstore.ErrWorkerAnswers)}
	w := nativeGemmaWorker{
		store: store, reply: reply, leaseTTL: time.Minute,
		generate: func(context.Context, string) (string, error) {
			generated++
			return "preserve this exact draft", nil
		},
	}
	if worked, err := w.runOnce(context.Background()); !worked || !errors.Is(err, routerstore.ErrWorkerAnswers) {
		t.Fatalf("authority refusal: worked=%v err=%v", worked, err)
	}
	item, err := store.Get(id)
	if err != nil || item.Status != routerstore.StatusBlocked || !strings.Contains(item.Result, "DRAFT PRESERVED FOR LEAD REVIEW:\npreserve this exact draft") {
		t.Fatalf("authority block lost original/draft: item=%+v err=%v", item, err)
	}
	if worked, err := w.runOnce(context.Background()); worked || err != nil {
		t.Fatalf("blocked draft remained claimable: worked=%v err=%v", worked, err)
	}
	if generated != 1 || reply.calls != 1 {
		t.Fatalf("authority refusal repeated work: inference=%d reply=%d", generated, reply.calls)
	}
}

type gemmaAuthorityBlockFailure struct {
	*gemmaWorkerStoreFake
	err error
}

func (s gemmaAuthorityBlockFailure) Block(_, _, reason string) error {
	s.blocked = reason
	return s.err
}

func TestGemmaWorkerAuthorityBlockFailureRetainsBothErrorsAndDraft(t *testing.T) {
	store, reply := claimedGemmaItem()
	reply.err = routerstore.ErrWorkerAnswers
	blockErr := errors.New("fixture lost block fence")
	w := nativeGemmaWorker{
		store: gemmaAuthorityBlockFailure{store, blockErr}, reply: reply, leaseTTL: time.Minute,
		generate: func(context.Context, string) (string, error) { return "retained draft", nil },
	}
	_, err := w.runOnce(context.Background())
	if !errors.Is(err, routerstore.ErrWorkerAnswers) || !errors.Is(err, blockErr) || !strings.Contains(err.Error(), "retained draft") {
		t.Fatalf("block failure lost evidence: %v", err)
	}
	if store.failed != "" || store.blocked == "" {
		t.Fatalf("authority refusal used retry instead of block: fail=%q block=%q", store.failed, store.blocked)
	}
}
