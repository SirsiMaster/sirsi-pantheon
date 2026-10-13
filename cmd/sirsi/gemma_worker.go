package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

const gemmaWorkerAgent = "gemma-pantheon"

var (
	gemmaWorkerOnce    bool
	gemmaWorkerVersion bool
)

// gemmaWorkerStore is deliberately the token-fenced subset of the router
// ledger.  The worker never scans an inbox or closes an unclaimed item.
type gemmaWorkerStore interface {
	ClaimNext(string, time.Duration) (*routerstore.Lease, error)
	StartWork(string, string) error
	Get(string) (routerstore.Item, error)
	AckItem(string) error
	VerifyLease(string, string) (bool, error)
	RenewLease(string, string, time.Duration) error
	Fail(string, string, string, string) error
}

type gemmaWorkerReply interface {
	Reply(*routerstore.Lease, routerstore.Item, string) error
}

type nativeGemmaWorker struct {
	store     gemmaWorkerStore
	reply     gemmaWorkerReply
	generate  func(context.Context, string) (string, error)
	leaseTTL  time.Duration
	heartbeat func() error
}

func (w nativeGemmaWorker) runOnce(ctx context.Context) (bool, error) {
	lease, err := w.store.ClaimNext(gemmaWorkerAgent, w.leaseTTL)
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
	answer, err := w.generate(genCtx, gemmaWorkerPrompt(item.Instructions))
	close(done)
	<-maintained
	select {
	case leaseErr := <-lost:
		return true, fmt.Errorf("Gemma lease for %s lost during inference: %w", lease.ItemID, leaseErr)
	default:
	}
	if err != nil {
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
	if _, err := r.facade.Send(gemmaWorkerAgent, item.From, title, "review", body); err != nil {
		return fmt.Errorf("send reply: %w", err)
	}
	// Send precedes completion: if delivery fails, the original stays open; if
	// completion loses its fence, a resend is deduplicated and never becomes a
	// false completion.
	return r.facade.CompleteItem(gemmaWorkerAgent, lease.ItemID, lease.Token,
		"Gemma draft delivered to "+item.From+"; response is the authoritative result.")
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
	base, model := gemmaSelectedLocalEndpoint(home)
	if base == "" || !gemmaServerPing(base) {
		return "", fmt.Errorf("selected native SNE inference endpoint is unavailable")
	}
	return gemmaWarmCompleteContext(ctx, base, model, prompt, gemmaMaxTokens)
}

var gemmaWorkerPreflight = func() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home for selected local inference: %w", err)
	}
	base, _ := gemmaSelectedLocalEndpoint(home)
	if base == "" || !gemmaServerPing(base) {
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
	gemmaWorkerCmd.Flags().BoolVar(&gemmaWorkerOnce, "once", false, "claim and process at most one item")
	gemmaWorkerCmd.Flags().BoolVar(&gemmaWorkerVersion, "version", false, "print worker version without registration or inference")
	gemmaCmd.AddCommand(gemmaWorkerCmd)
}

func runGemmaWorker(cmd *cobra.Command, _ []string) error {
	if gemmaWorkerVersion {
		fmt.Fprintln(cmd.OutOrStdout(), "sirsi-gemma-worker native")
		return nil
	}
	if err := os.Setenv("SIRSI_AGENT_ID", gemmaWorkerAgent); err != nil {
		return fmt.Errorf("set Gemma worker identity: %w", err)
	}
	if err := os.Setenv("SIRSI_ROUTER_AGENT", gemmaWorkerAgent); err != nil {
		return fmt.Errorf("set Gemma router identity: %w", err)
	}
	if err := gemmaWorkerPreflight(); err != nil {
		return err
	}
	unlock, err := gemmaWorkerTryLock("gemma-pantheon-worker")
	if err != nil {
		return err
	}
	defer unlock()
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return err
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		return err
	}
	defer f.Close()
	routerRoot := repoRoot + "/.agents/idea-router"
	t, err := router.RegisterThread(routerRoot, &router.Thread{AgentID: gemmaWorkerAgent, Surface: "worker", Repo: repoRoot, PID: os.Getpid(), ConsumerCapable: true, WakeMechanism: "none"})
	if err != nil {
		return fmt.Errorf("register Gemma worker thread: %w", err)
	}
	if err := os.Setenv("SIRSI_THREAD_ID", t.ThreadID); err != nil {
		return fmt.Errorf("export registered thread id: %w", err)
	}
	defer func() {
		if _, err := router.CloseThread(routerRoot, t.ThreadID); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Gemma worker thread close failed: %v\n", err)
		}
	}()
	w := nativeGemmaWorker{store: f.Store(), reply: facadeGemmaReply{facade: f}, generate: gemmaWorkerGenerate, leaseTTL: 15 * time.Minute,
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
