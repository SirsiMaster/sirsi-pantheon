package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reason"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

const gemmaWorkerAgent = "gemma-pantheon"

var (
	gemmaWorkerOnce           bool
	gemmaWorkerVersion        bool
	gemmaWorkerProfile        string
	gemmaWorkerMaxTokens      int
	gemmaWorkerTokenFile      string
	gemmaWorkerApolloToolTurn bool
	gemmaWorkerAllowedTools   []string
	gemmaWorkerItemID         string
)

// gemmaWorkerStore is deliberately the token-fenced subset of the router
// ledger.  The worker never scans an inbox or closes an unclaimed item.
type gemmaWorkerStore interface {
	ClaimItem(string, string, time.Duration) (*routerstore.Lease, error)
	ClaimNext(string, time.Duration) (*routerstore.Lease, error)
	StartWork(string, string) error
	Get(string) (routerstore.Item, error)
	AckItem(string) error
	VerifyLease(string, string) (bool, error)
	RenewLease(string, string, time.Duration) error
	Fail(string, string, string, string) error
	Block(string, string, string) error
}

type gemmaWorkerReply interface {
	Reply(*routerstore.Lease, routerstore.Item, string) error
}

type nativeGemmaWorker struct {
	store     gemmaWorkerStore
	reply     gemmaWorkerReply
	generate  func(context.Context, string) (string, error)
	leaseTTL  time.Duration
	itemID    string
	heartbeat func() error
}

func (w nativeGemmaWorker) runOnce(ctx context.Context) (bool, error) {
	var (
		lease *routerstore.Lease
		err   error
	)
	if strings.TrimSpace(w.itemID) != "" {
		lease, err = w.store.ClaimItem(gemmaWorkerAgent, w.itemID, w.leaseTTL)
	} else {
		lease, err = w.store.ClaimNext(gemmaWorkerAgent, w.leaseTTL)
	}
	if errors.Is(err, routerstore.ErrNoWork) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim Gemma work: %w", err)
	}
	if err := w.store.StartWork(lease.ItemID, lease.Token); err != nil {
		return true, w.retry(lease, "start claimed Gemma work", "", err)
	}
	item, err := w.store.Get(lease.ItemID)
	if err != nil {
		return true, w.retry(lease, "read claimed item", "", err)
	}
	if err := w.store.AckItem(lease.ItemID); err != nil {
		return true, w.retry(lease, "acknowledge fully-read item", "", err)
	}
	genCtx, cancel := context.WithTimeout(ctx, w.leaseTTL-(w.leaseTTL/3))
	defer cancel()
	lost := make(chan error, 1)
	done := make(chan struct{})
	maintained := make(chan struct{})
	go func() {
		defer close(maintained)
		w.maintainLease(genCtx, done, cancel, lease, lost)
	}()
	answer, err := gemmaGenerateWhileBusy(genCtx, w.generate, gemmaWorkerPrompt(item.Instructions))
	close(done)
	<-maintained
	select {
	case leaseErr := <-lost:
		return true, fmt.Errorf("Gemma lease for %s lost during inference: %w", lease.ItemID, leaseErr)
	default:
	}
	if err != nil {
		var broker *gemmaInferenceError
		if errors.As(err, &broker) && broker.Permanent {
			reason := "GEMMA BLOCKED: " + broker.Error()
			if blockErr := w.store.Block(lease.ItemID, lease.Token, reason); blockErr != nil {
				return true, fmt.Errorf("record fenced inference block: %w", blockErr)
			}
			return true, broker
		}
		return true, w.retry(lease, "native inference unavailable", "", err)
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return true, w.retry(lease, "native inference returned an empty draft", "", nil)
	}
	if gemmaWorkerLooksTruncated(answer) {
		return true, w.retry(lease, "native inference draft is truncated", answer, nil)
	}
	// This check is deliberately immediately before publication. CompleteItem
	// fences again, but a superseded worker must not even send a fresh reply.
	ok, err := w.store.VerifyLease(lease.ItemID, lease.Token)
	if err != nil {
		return true, fmt.Errorf("verify Gemma lease before reply %s: %w", lease.ItemID, err)
	}
	if !ok {
		return true, fmt.Errorf("Gemma lease for %s was lost before reply publication", lease.ItemID)
	}
	if err := w.reply.Reply(lease, item, answer); err != nil {
		if errors.Is(err, routerstore.ErrWorkerAnswers) {
			// An answering-role refusal is not an inference failure. Park the
			// same original under its fence so another worker cannot regenerate
			// the preserved draft on every retry. Only the lead may reconcile it.
			blocked := fmt.Errorf("GEMMA BLOCKED: reply publication authority refused: %w\n\nDRAFT PRESERVED FOR LEAD REVIEW:\n%s", err, answer)
			if blockErr := w.store.Block(lease.ItemID, lease.Token, blocked.Error()); blockErr != nil {
				return true, errors.Join(blocked, fmt.Errorf("record fenced Gemma authority block: %w", blockErr))
			}
			return true, blocked
		}
		return true, w.retry(lease, "reply publication failed", answer, err)
	}
	return true, nil
}

func (w nativeGemmaWorker) maintainLease(ctx context.Context, done <-chan struct{}, cancel context.CancelFunc, lease *routerstore.Lease, lost chan<- error) {
	interval := w.leaseTTL / 4
	if interval <= 0 {
		interval = time.Minute
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
			if w.heartbeat != nil {
				if err := w.heartbeat(); err != nil {
					cancel()
					select {
					case lost <- fmt.Errorf("worker heartbeat: %w", err):
					default:
					}
					return
				}
			}
			if err := w.store.RenewLease(lease.ItemID, lease.Token, w.leaseTTL); err != nil {
				cancel()
				select {
				case lost <- err:
				default:
				}
				return
			}
		}
	}
}

func (w nativeGemmaWorker) retry(lease *routerstore.Lease, stage, draft string, cause error) error {
	message := "GEMMA RETRYABLE FAILURE: " + stage
	if cause != nil {
		message += ": " + cause.Error()
	}
	if draft != "" {
		message += "\n\nDRAFT PRESERVED FOR RETRY:\n" + draft
	}
	if err := w.store.Fail(lease.ItemID, lease.Token, message, "gemma_worker"); err != nil {
		return fmt.Errorf("%s; retain unresolved item failed: %w", message, err)
	}
	return errors.New(message)
}

func gemmaWorkerPrompt(instructions string) string {
	return "You are Gemma, Pantheon's native local drafting worker. Produce a concrete text draft only; do not claim to run tools, publish changes, or issue a binding verdict.\n\n" + strings.TrimSpace(instructions)
}

func gemmaWorkerLooksTruncated(s string) bool {
	return strings.Count(s, "```")%2 != 0 || strings.HasSuffix(strings.TrimSpace(s), "<|channel")
}

type facadeGemmaReply struct{ facade *dispatch.Facade }

func (r facadeGemmaReply) Reply(lease *routerstore.Lease, item routerstore.Item, draft string) error {
	if strings.TrimSpace(item.From) == "" {
		return fmt.Errorf("claimed item %s has no sender", item.ID)
	}
	title := "NONBINDING Gemma draft for " + item.ID + ": " + strings.TrimSpace(item.Title)
	body := gemmaWorkerReplyEnvelope(item.ID, lease.Token, draft)
	sent, err := r.facade.Send(gemmaWorkerAgent, item.From, title, "review", body)
	if err != nil {
		return fmt.Errorf("send reply: %w", err)
	}
	// Send precedes completion: if delivery fails, the original stays open; if
	// completion loses its fence, a resend is deduplicated and never becomes a
	// false completion.
	published, err := r.facade.Store().Get(sent.ID)
	if err != nil {
		return fmt.Errorf("read back reply before completion: %w", err)
	}
	finger := sharedGemmaReplyFingerprint(published, item.From, item.ID)
	if finger == "" {
		return fmt.Errorf("published reply correlation mismatch")
	}
	ref, e := json.Marshal(gemmaSharedCompletionRef{Kind: "nonbinding_draft", OriginalItemID: item.ID, ReplyItemID: sent.ID, LeaseCorrelation: finger})
	if e != nil {
		return e
	}
	return r.facade.CompleteItem(gemmaWorkerAgent, lease.ItemID, lease.Token, string(ref))
}

func gemmaWorkerReplyEnvelope(itemID, leaseToken, draft string) string {
	// The live lease token is bearer authority; correlation is one-way only.
	fingerprint := sha256.Sum256([]byte(leaseToken))
	return fmt.Sprintf("NONBINDING DRAFT — original item %s; lease correlation sha256:%x. This reply is not a completion claim.\n\n%s", itemID, fingerprint[:8], draft)
}

var gemmaWorkerGenerate = func(ctx context.Context, prompt string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home for selected local inference: %w", err)
	}
	base, model := gemmaWorkerEndpoint(home)
	if base == "" || !gemmaWorkerPing(base) {
		return "", fmt.Errorf("selected native SNE inference endpoint is unavailable")
	}
	return gemmaWarmCompleteProfile(ctx, base, model, prompt, gemmaWorkerMaxTokens, gemmaWorkerTokenFile, provider.ProfileRequestTimeoutSeconds(gemmaWorkerSelectedProfile))
}

var gemmaWorkerPreflight = func() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home for selected local inference: %w", err)
	}
	if _, err := gemmaBrokerBearer(gemmaWorkerTokenFile); err != nil {
		return err
	}
	base, _ := gemmaWorkerEndpoint(home)
	if base == "" || !gemmaWorkerPing(base) {
		return fmt.Errorf("selected native SNE inference endpoint is unavailable; worker will not claim work")
	}
	return nil
}

var gemmaWorkerTryLock = platform.TryLock

var gemmaWorkerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Run the native fenced Gemma router worker",
	Args:  cobra.NoArgs,
	RunE:  runGemmaWorker,
}

func init() {
	gemmaWorkerCmd.Flags().StringVar(&gemmaWorkerInferenceProfilePath, "inference-profile", "", "Explicit selected installed broker JSON: endpoint, full model, existing access_token_file; mandatory for M1")
	gemmaWorkerCmd.Flags().StringVar(&gemmaWorkerProfile, "profile", "m5", "Worker output profile: m1=256 or m5=1024 tokens; endpoint/model identity stays selected")
	gemmaWorkerCmd.Flags().IntVar(&gemmaWorkerMaxTokens, "max-tokens", 0, "Explicit worker output budget (0 uses profile)")
	gemmaWorkerCmd.Flags().StringVar(&gemmaWorkerTokenFile, "access-token-file", "", "Existing selected private broker capability file (0600); empty preserves unauthenticated M5 profile")
	gemmaWorkerCmd.Flags().BoolVar(&gemmaWorkerApolloToolTurn, "apollo-tool-turn", false, "Use one bounded Apollo 2.1 read-only tool turn; requires the exact selected Apollo profile")
	gemmaWorkerCmd.Flags().StringSliceVar(&gemmaWorkerAllowedTools, "allow-tool", nil, "Exact observe-tier reason tool(s) offered when --apollo-tool-turn is set")
	gemmaWorkerCmd.Flags().StringVar(&gemmaWorkerItemID, "item", "", "Exact router item ID to claim; requires --once and never falls back to another inbox item")
	gemmaWorkerCmd.Flags().BoolVar(&gemmaWorkerOnce, "once", false, "claim and process at most one item")
	gemmaWorkerCmd.Flags().BoolVar(&gemmaWorkerVersion, "version", false, "print worker version without registration or inference")
	gemmaCmd.AddCommand(gemmaWorkerCmd)
}

// Per-invocation effects keep startup-order fixtures off live providers and
// process locks without adding mutable package-level test hooks.
type gemmaWorkerRuntime struct {
	preflight func() error
	generate  func(context.Context, string) (string, error)
	tryLock   func(string) (func(), error)
}

func runGemmaWorker(cmd *cobra.Command, _ []string) error {
	return runGemmaWorkerWith(cmd, gemmaWorkerRuntime{
		preflight: gemmaWorkerPreflight,
		generate:  gemmaWorkerGenerate,
		tryLock:   gemmaWorkerTryLock,
	})
}

func runGemmaWorkerWith(cmd *cobra.Command, runtime gemmaWorkerRuntime) error {
	if gemmaWorkerVersion {
		fmt.Fprintln(cmd.OutOrStdout(), "sirsi-gemma-worker native")
		return nil
	}
	budget, err := gemmaWorkerBudget(gemmaWorkerProfile, gemmaWorkerMaxTokens)
	if err != nil {
		return err
	}
	gemmaWorkerMaxTokens = budget
	gemmaWorkerItemID = strings.TrimSpace(gemmaWorkerItemID)
	if gemmaWorkerItemID != "" && !gemmaWorkerOnce {
		return fmt.Errorf("--item requires --once")
	}
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return err
	}
	routerRoot := repoRoot + "/.agents/idea-router"
	// Ordinary drafts also finish via CompleteItem. Both modes must consume
	// the existing lead declaration before provider preflight or any claim;
	// selecting a worker mode must never create an answering grant.
	answerRole, err := gemmaWorkerAnswerRoleGranted(routerRoot)
	if err != nil {
		return err
	}
	if !answerRole {
		return fmt.Errorf("Gemma worker refuses to claim work: %w", routerstore.ErrWorkerAnswers)
	}
	if gemmaWorkerInferenceProfilePath == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		gemmaWorkerInferenceProfilePath = provider.SelectedLocalProfilePath(home, provider.LoadConf(home))
	}
	selected, err := readGemmaWorkerProfile(gemmaWorkerInferenceProfilePath, gemmaWorkerProfile)
	if err != nil {
		return err
	}
	gemmaWorkerSelectedProfile = selected
	if selected != nil {
		if selected.OutputTokens < gemmaWorkerMaxTokens {
			gemmaWorkerMaxTokens = selected.OutputTokens
		}
		if gemmaWorkerProfile == "m1" && selected.ContextTokens != 1024 {
			return fmt.Errorf("M1 candidate requires verified 1024-token context profile")
		}
	}
	if selected != nil {
		if gemmaWorkerTokenFile != "" && gemmaWorkerTokenFile != selected.AccessTokenFile {
			return fmt.Errorf("broker capability flag conflicts with selected profile")
		}
		gemmaWorkerTokenFile = selected.AccessTokenFile
	}
	generate := runtime.generate
	if gemmaWorkerApolloToolTurn {
		if gemmaWorkerItemID == "" {
			return fmt.Errorf("--apollo-tool-turn requires --item to fence the bounded tool turn to one router item")
		}
		if selected == nil || selected.ID != apollo21ProfileID || !provider.ProfileSupportsTools(selected) {
			return fmt.Errorf("--apollo-tool-turn requires explicit %s profile with tool_calls: true", apollo21ProfileID)
		}
		registry := reason.NewRegistry()
		if err := reason.MachineTools(registry); err != nil {
			return fmt.Errorf("register bounded Apollo tools: %w", err)
		}
		if err := gemmaWorkerObserveAllowlist(registry, gemmaWorkerAllowedTools); err != nil {
			return err
		}
		generate = gemmaWorkerApolloTurnGenerator(selected, registry, gemmaWorkerAllowedTools, gemmaWorkerMaxTokens)
	}
	if err := os.Setenv("SIRSI_AGENT_ID", gemmaWorkerAgent); err != nil {
		return fmt.Errorf("set Gemma worker identity: %w", err)
	}
	if err := os.Setenv("SIRSI_ROUTER_AGENT", gemmaWorkerAgent); err != nil {
		return fmt.Errorf("set Gemma router identity: %w", err)
	}
	if err := runtime.preflight(); err != nil {
		return err
	}
	unlock, err := runtime.tryLock("gemma-pantheon-worker")
	if err != nil {
		return err
	}
	defer unlock()
	t, f, err := gemmaWorkerOpenFacade(repoRoot, routerRoot, answerRole)
	if err != nil {
		return err
	}
	defer func() {
		if _, err := router.CloseThread(routerRoot, t.ThreadID); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Gemma worker thread close failed: %v\n", err)
		}
	}()
	defer f.Close()
	w := nativeGemmaWorker{store: f.Store(), reply: facadeGemmaReply{facade: f}, generate: generate, leaseTTL: 15 * time.Minute, itemID: gemmaWorkerItemID,
		heartbeat: func() error {
			_, err := router.Heartbeat(routerRoot, t.ThreadID, router.HeartbeatUpdate{Status: router.ThreadStatusActive})
			return err
		},
	}
	for {
		if _, err := router.Heartbeat(routerRoot, t.ThreadID, router.HeartbeatUpdate{Status: router.ThreadStatusActive}); err != nil {
			return fmt.Errorf("Gemma worker heartbeat: %w", err)
		}
		worked, err := w.runOnce(cmd.Context())
		if err != nil {
			var broker *gemmaInferenceError
			if !gemmaWorkerOnce && errors.As(err, &broker) {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				continue
			}
			return err
		}
		if gemmaWorkerOnce {
			return nil
		}
		if !worked {
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(30 * time.Second):
			}
		}
	}
}

// gemmaWorkerOpenFacade registers this worker's own child session before it
// opens the write facade. RemoteStore captures SIRSI_THREAD_ID at construction;
// reversing this order binds every later claim, reply, and fenced completion to
// whichever parent session happened to launch the worker.
func gemmaWorkerOpenFacade(repoRoot, routerRoot string, answerRole bool) (*router.Thread, *dispatch.Facade, error) {
	t, err := router.RegisterThread(routerRoot, &router.Thread{
		// A worker must own a fresh session even when it is launched from an
		// attended process that shares its PID in a test or embedded host.
		ThreadID:        router.NewThreadID(),
		AgentID:         gemmaWorkerAgent,
		Surface:         "worker",
		Repo:            repoRoot,
		PID:             os.Getpid(),
		ConsumerCapable: true,
		WakeMechanism:   "none",
		AnswerRole:      answerRole,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("register Gemma worker thread: %w", err)
	}
	closeChild := func() {
		_, _ = router.CloseThread(routerRoot, t.ThreadID)
	}
	if err := os.Setenv("SIRSI_THREAD_ID", t.ThreadID); err != nil {
		closeChild()
		return nil, nil, fmt.Errorf("export registered thread id: %w", err)
	}
	if answerRole {
		if err := gemmaWorkerVerifyStampedAnswerRole(routerRoot, t.ThreadID); err != nil {
			closeChild()
			return nil, nil, err
		}
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		closeChild()
		return nil, nil, fmt.Errorf("open Gemma worker dispatch facade: %w", err)
	}
	return t, f, nil
}

// gemmaWorkerAnswerRoleGranted reads the lead-owned declaration; it never
// writes the registry or upgrades a worker by convention. The service keeps a
// second enforcement point at reply completion.
func gemmaWorkerAnswerRoleGranted(routerRoot string) (bool, error) {
	reg, err := router.LoadRegistry(routerRoot)
	if err != nil {
		return false, fmt.Errorf("read Gemma answering-role declaration: %w", err)
	}
	cfg, err := reg.Lookup(gemmaWorkerAgent)
	if err != nil {
		return false, fmt.Errorf("read Gemma worker registration: %w", err)
	}
	return cfg.WorkerAnswers, nil
}

// gemmaWorkerVerifyStampedAnswerRole proves that this exact registered worker,
// not merely a registry declaration, carries the lead's answering grant before
// it can claim any item, including an ordinary text-draft request.
func gemmaWorkerVerifyStampedAnswerRole(routerRoot, threadID string) error {
	threads, err := router.LoadThreadRegistry(routerRoot)
	if err != nil {
		return fmt.Errorf("read registered Gemma worker role: %w", err)
	}
	for _, thread := range threads.Threads {
		if thread != nil && thread.ThreadID == threadID {
			if thread.AgentID != gemmaWorkerAgent || thread.Surface != "worker" || !thread.AnswerRole {
				return fmt.Errorf("Gemma worker thread %s lacks the declared answering role", threadID)
			}
			return nil
		}
	}
	return fmt.Errorf("registered Gemma worker thread %s is absent from role readback", threadID)
}

func gemmaWorkerObserveAllowlist(registry *reason.Registry, names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("--apollo-tool-turn requires at least one --allow-tool")
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("--allow-tool cannot be empty")
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("--allow-tool %q is duplicated", name)
		}
		tool, ok := registry.Get(name)
		if !ok {
			return fmt.Errorf("--allow-tool %q is not a registered Pantheon tool", name)
		}
		if tool.Tier != reason.TierObserve {
			return fmt.Errorf("--apollo-tool-turn accepts observe tools only; %q is %s", name, tool.Tier)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func gemmaWorkerBudget(profile string, explicit int) (int, error) {
	limit := 0
	switch profile {
	case "m1":
		limit = 256
	case "m5":
		limit = 1024
	default:
		return 0, fmt.Errorf("unknown worker profile %q", profile)
	}
	if explicit == 0 {
		return limit, nil
	}
	if explicit < 1 || explicit > limit {
		return 0, fmt.Errorf("worker output budget must be 1..%d for %s", limit, profile)
	}
	return explicit, nil
}
