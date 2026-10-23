package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

// The bounded own-thread RPC is required. Old servers must be upgraded before
// this control path is enabled; a global thread-list fallback is not equivalent.
type lifecycleThreadReader interface {
	GetThreadRecord(string) (routerstore.ThreadRecord, error)
}

func lifecycleGrant(reg *router.Registry, actor, label string) error {
	cfg, ok := reg.Agents[actor]
	if ok {
		for _, allowed := range cfg.LifecycleServices {
			if allowed == label {
				return nil
			}
		}
	}
	return fmt.Errorf("service lifecycle: actor has no explicit grant for %s", label)
}

func authorizeLifecycle(ctx context.Context, label string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	actor, tid := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID")), strings.TrimSpace(os.Getenv("SIRSI_THREAD_ID"))
	if actor == "" || tid == "" {
		return fmt.Errorf("service lifecycle: current registered actor and thread required")
	}
	if other := os.Getenv("SIRSI_ROUTER_AGENT"); other != "" && other != actor {
		return fmt.Errorf("service lifecycle: conflicting router actor")
	}
	repo, err := router.FindRepoRoot()
	if err != nil {
		return err
	}
	reg, err := router.LoadRegistry(filepath.Join(repo, ".agents", "idea-router"))
	if err != nil {
		return err
	}
	if err = lifecycleGrant(reg, actor, label); err != nil {
		return err
	}
	f, err := dispatch.Open(repo)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, ok := f.Store().(*routerstore.RemoteStore); !ok {
		return fmt.Errorf("service lifecycle: canonical remote router required")
	}
	r, ok := f.Store().(lifecycleThreadReader)
	if !ok {
		return fmt.Errorf("service lifecycle: compatible bounded own-thread RPC required; no global-list fallback")
	}
	record, err := r.GetThreadRecord(tid)
	if err != nil {
		return err
	}
	return sharedGemmaActorThread(actor, tid, router.MachineID(), []routerstore.ThreadRecord{record}, func(t router.Thread) bool {
		return sharedGemmaRuntimeOwned(t, os.Getenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE"), sharedGemmaAncestor, resolveAnchorPID, router.PIDStartTimeOf)
	})
}

func newLaunchdServiceCmd() *cobra.Command {
	var apply, drained bool
	var pid int
	var started, plistHash, programHash string
	c := &cobra.Command{Use: "restart <label>", Short: "Inspect one explicitly supported service; restart only an exact approved instance", Args: cobra.ExactArgs(1)}
	c.Flags().BoolVar(&apply, "apply", false, "Apply the exact pinned plan; default is read-only")
	c.Flags().BoolVar(&drained, "drained", false, "Owning application has drained active work and retained its recovery plan")
	c.Flags().IntVar(&pid, "expected-pid", 0, "PID from the prior read-only plan")
	c.Flags().StringVar(&started, "expected-start", "", "Start identity from the prior read-only plan")
	c.Flags().StringVar(&plistHash, "expected-plist-sha256", "", "Plist digest from the prior read-only plan")
	c.Flags().StringVar(&programHash, "expected-program-sha256", "", "Program digest from the prior read-only plan")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
		defer cancel()
		if apply && (!drained || pid <= 1 || started == "" || plistHash == "" || programHash == "") {
			return fmt.Errorf("service lifecycle: --apply requires --drained and all four exact plan pins")
		}
		backend := setup.NativeServiceBackend()
		p, err := backend.Inspect(ctx, args[0])
		if err != nil {
			return err
		}
		if apply {
			p.PID, p.Started, p.PlistSHA256, p.ProgramSHA256 = pid, started, plistHash, programHash
			controller := setup.ServiceController{Backend: backend, Authorize: authorizeLifecycle}
			p, err = controller.Refresh(ctx, p)
			if err != nil {
				return err
			}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(p)
	}
	return c
}

func init() { launchdCmd.AddCommand(newLaunchdServiceCmd()) }
