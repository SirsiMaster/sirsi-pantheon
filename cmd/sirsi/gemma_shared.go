package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/signal"

	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const sharedGemmaPromptLimit = 8192
const sharedGemmaPrivacy = "shared canonical router: prompt leaves this host; Gemma may execute on another host"

type sharedGemmaStore interface {
	GetContext(context.Context, string) (routerstore.Item, error)
}
type sharedGemmaResult struct {
	State          string `json:"state"`
	OriginalID     string `json:"original_id"`
	OriginalStatus string `json:"original_status,omitempty"`
	ReplyID        string `json:"reply_id,omitempty"`
	Draft          string `json:"draft,omitempty"`
	Detail         string `json:"detail,omitempty"`
	Privacy        string `json:"privacy"`
}
type sharedGemmaProducer struct {
	store     sharedGemmaStore
	actor     string
	validate  func(context.Context) error
	send      func(context.Context, string) (string, error)
	poll      time.Duration
	submitted func(string)
}
type gemmaSharedCompletionRef struct {
	Kind             string `json:"kind"`
	OriginalItemID   string `json:"original_item_id"`
	ReplyItemID      string `json:"reply_item_id"`
	LeaseCorrelation string `json:"lease_correlation"`
}

func sharedGemmaReplyFingerprint(r routerstore.Item, actor, id string) string {
	if r.ID == "" || r.From != gemmaWorkerAgent || r.To != actor || !strings.HasPrefix(r.Title, "NONBINDING Gemma draft for "+id+": ") {
		return ""
	}
	line := strings.SplitN(r.Instructions, "\n", 2)[0]
	prefix := "NONBINDING DRAFT — original item " + id + "; lease correlation sha256:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(line, prefix)
	finger, tail, ok := strings.Cut(rest, ". ")
	if !ok || len(finger) != 16 || tail != "This reply is not a completion claim." {
		return ""
	}
	if _, e := hex.DecodeString(finger); e != nil {
		return ""
	}
	return finger
}
func sharedGemmaMatchesReply(r routerstore.Item, actor, id string) bool {
	return sharedGemmaReplyFingerprint(r, actor, id) != ""
}
func (p sharedGemmaProducer) run(ctx context.Context, prompt, resume string, wait time.Duration) (sharedGemmaResult, error) {
	out := sharedGemmaResult{State: "pending", OriginalID: resume, Privacy: sharedGemmaPrivacy}
	if wait < 0 || wait > 15*time.Minute {
		return out, fmt.Errorf("wait must be 0..15m")
	}
	if resume != "" && prompt != "" {
		return out, fmt.Errorf("resume accepts only an original ID, no new prompt")
	}
	if resume == "" && (len(prompt) > sharedGemmaPromptLimit || strings.TrimSpace(prompt) == "" || !utf8.ValidString(prompt)) {
		return out, fmt.Errorf("prompt must be nonempty UTF-8, at most 8192 bytes; it is never truncated")
	}
	if p.validate == nil || p.actor == "" {
		return out, fmt.Errorf("registered current actor/thread required")
	}
	admission, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if e := p.validate(admission); e != nil {
		return out, e
	}
	if resume == "" {
		id, e := p.send(admission, prompt)
		out.OriginalID = id
		if id != "" && p.submitted != nil {
			p.submitted(id)
		}
		if e != nil {
			return out, e
		}
		if id == "" {
			return out, fmt.Errorf("canonical send returned no original ID")
		}
	}
	if ctx.Err() != nil {
		out.Detail = "canceled; original remains durable"
		return out, nil
	}
	if resume == "" && wait == 0 {
		return out, nil
	}
	deadline := time.Now().Add(wait)
	for {
		remaining := time.Until(deadline)
		budget := 30 * time.Second
		if wait == 0 {
			remaining = budget
		}
		if remaining <= 0 {
			out.Detail = "wait expired; resume the same original"
			return out, nil
		}
		if remaining < budget {
			budget = remaining
		}
		call, c := context.WithTimeout(ctx, budget)
		original, e := p.store.GetContext(call, out.OriginalID)
		if e != nil {
			c()
			if ctx.Err() != nil || (wait > 0 && time.Now().After(deadline)) {
				out.Detail = "canceled or timed out; original remains durable"
				return out, nil
			}
			return out, e
		}
		if original.ID != out.OriginalID || original.From != p.actor || original.To != gemmaWorkerAgent {
			c()
			return out, fmt.Errorf("original is not this actor's Gemma request")
		}
		out.OriginalStatus = original.Status
		out.Detail = original.Result
		if original.Status == "blocked" || original.Status == "dead_letter" {
			c()
			out.State = original.Status
			return out, nil
		}
		if original.Status == "completed" || original.Status == "closed" {
			// Prefer the worker's exact authoritative result reference. Old workers
			// are supported by a unique read-only reply in this actor's own inbox.
			var ref gemmaSharedCompletionRef
			if json.Unmarshal([]byte(original.Result), &ref) == nil && ref.Kind == "nonbinding_draft" && ref.OriginalItemID == original.ID && ref.ReplyItemID != "" {
				r, re := p.store.GetContext(call, ref.ReplyItemID)
				if re != nil {
					c()
					return out, re
				}
				finger := sharedGemmaReplyFingerprint(r, p.actor, original.ID)
				if r.ID == ref.ReplyItemID && finger != "" && finger == ref.LeaseCorrelation {
					c()
					out.State = "completed"
					out.ReplyID = r.ID
					out.Draft = r.Instructions
					out.Detail = "exact original completion and correlated reply read back"
					return out, nil
				}
			}
			out.Detail = "correlation unavailable or mismatched; pending/manual retrieval, no inbox scan"
		}
		c()
		if wait == 0 {
			return out, nil
		}
		pause := p.poll
		if pause <= 0 {
			pause = 2 * time.Second
		}
		if until := time.Until(deadline); until < pause {
			pause = until
		}
		if pause <= 0 {
			out.Detail = "wait expired; resume the same original"
			return out, nil
		}
		t := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			t.Stop()
			out.Detail = "canceled; original remains durable"
			return out, nil
		case <-t.C:
		}
	}
}
func sharedGemmaActorThread(actor, tid, machineID string, records []routerstore.ThreadRecord, ownsPID func(int) bool) error {
	for _, r := range records {
		if r.ThreadID == tid {
			var t router.Thread
			if e := json.Unmarshal(r.Payload, &t); e != nil {
				return e
			}
			if r.Agent != actor || t.AgentID != actor || r.Status != "active" || t.Status != router.ThreadStatusActive || machineID == "" || t.MachineID != machineID || !ownsPID(t.PID) {
				return fmt.Errorf("actor/thread is not the current live runtime")
			}
			return nil
		}
	}
	return fmt.Errorf("current registered thread not found")
}
func sharedGemmaAncestor(pid int) bool {
	for current, hops := os.Getpid(), 0; current > 1 && hops < maxAnchorAncestryHops; hops++ {
		if current == pid {
			return true
		}
		p, e := lookupAnchorProcess(current)
		if e != nil || p.parentPID == current {
			return false
		}
		current = p.parentPID
	}
	return false
}
func sharedGemmaRemoteConfigured() error {
	if _, ok := os.LookupEnv("SIRSI_ROUTER_DB"); ok {
		return fmt.Errorf("shared Gemma refuses local DB configuration")
	}
	u := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_URL"))
	if u == "" || (!strings.HasPrefix(u, "https://") && routerstore.SpoolDir(u) == "") || !routercfg.StoreWake() {
		return fmt.Errorf("shared Gemma requires the existing canonical HTTPS service or its configured relay; no local fallback")
	}
	return nil
}
func newGemmaSharedCmd() *cobra.Command {
	var resume string
	var wait time.Duration
	cmd := &cobra.Command{Use: "shared [prompt]", Short: "Send one draft request through the canonical router (prompt leaves this host)", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if e := sharedGemmaRemoteConfigured(); e != nil {
			return e
		}
		prompt := strings.Join(args, " ")
		if resume == "" {
			in := cmd.InOrStdin()
			read := true
			if f, ok := in.(*os.File); ok {
				if st, e := f.Stat(); e == nil && st.Mode()&os.ModeCharDevice != 0 {
					read = false
				}
			}
			if read {
				b, e := io.ReadAll(io.LimitReader(in, sharedGemmaPromptLimit+1))
				if e != nil {
					return e
				}
				if len(b) > 0 {
					if prompt != "" {
						prompt += "\n\n"
					}
					prompt += string(b)
				}
			}
		}
		repo, e := router.FindRepoRoot()
		if e != nil {
			return e
		}

		actor := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID"))
		if actor == "" {
			actor = router.ReadSessionAgentMarker(router.CurrentSessionID())
		}
		tid := strings.TrimSpace(os.Getenv("SIRSI_THREAD_ID"))
		if actor == "" || tid == "" {
			return fmt.Errorf("current SIRSI_AGENT_ID/session actor and registered SIRSI_THREAD_ID required; no --from override")
		}
		if a := os.Getenv("SIRSI_ROUTER_AGENT"); a != "" && a != actor {
			return fmt.Errorf("router actor conflicts with current actor")
		}
		f, e := dispatch.Open(repo)
		if e != nil {
			return e
		}
		defer f.Close()
		rs, ok := f.Store().(*routerstore.RemoteStore)
		if !ok {
			return fmt.Errorf("shared producer requires canonical RemoteStore")
		}
		p := sharedGemmaProducer{store: rs, actor: actor, validate: func(ctx context.Context) error {
			if e := f.ValidateAgent("sender", actor); e != nil {
				return e
			}
			if e := f.ValidateAgent("recipient", gemmaWorkerAgent); e != nil {
				return e
			}
			records, e := rs.ListThreadsContext(ctx)
			if e != nil {
				return e
			}
			return sharedGemmaActorThread(actor, tid, router.MachineID(), records, sharedGemmaAncestor)
		},
			send: func(ctx context.Context, prompt string) (string, error) {
				nonce := make([]byte, 16)
				if _, e := rand.Read(nonce); e != nil {
					return "", e
				}
				body := "NONBINDING shared Gemma request\nRequester: " + actor + "\nRequester thread: " + tid + "\nPrivacy: " + sharedGemmaPrivacy + "\n\nFull prompt:\n" + prompt
				r, e := f.SendContext(ctx, actor, gemmaWorkerAgent, "Gemma shared "+hex.EncodeToString(nonce), "proposal", body)
				return r.ID, e
			},
			submitted: func(id string) {
				fmt.Fprintln(cmd.ErrOrStderr(), sharedGemmaPrivacy+"; original "+id+"; resume with --resume "+id)
			}}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		out, e := p.run(ctx, prompt, resume, wait)
		if JsonOutput {
			json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s original=%s reply=%s\n", out.State, out.OriginalID, out.ReplyID)
			if out.Draft != "" {
				fmt.Fprintln(cmd.OutOrStdout(), out.Draft)
			} else if out.Detail != "" {
				fmt.Fprintln(cmd.OutOrStdout(), out.Detail)
			}
		}
		return e
	}}
	cmd.Flags().StringVar(&resume, "resume", "", "Read/wait for this exact owned original; never resubmits")
	cmd.Flags().DurationVar(&wait, "wait", 15*time.Minute, "Finite result wait, 0..15m; timeout/cancel leaves the original pending")
	return cmd
}
func init() { gemmaCmd.AddCommand(newGemmaSharedCmd()) }
