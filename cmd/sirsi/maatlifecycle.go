package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/lifecycleevidence"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

type lifecycleJournal interface {
	ObserveLifecycle(context.Context, string, string, string, string, bool, maat.LifecycleResolver) (maat.LifecycleObservation, error)
}
type lifecycleAdoptionJournal interface {
	AdoptLifecycleReview(context.Context, string, string, string, string, string, string, string, bool, maat.LifecycleResolver) (maat.LifecycleAdoptionObservation, error)
}

type lazyLifecycleRouterReader struct {
	once  sync.Once
	store routerstore.Store
	err   error
}

func (r *lazyLifecycleRouterReader) Get(id string) (routerstore.Item, error) {
	r.once.Do(func() { r.store, r.err = routerstore.Resolve() })
	if r.err != nil {
		return routerstore.Item{}, r.err
	}
	return r.store.Get(id)
}

func (r *lazyLifecycleRouterReader) Close() error {
	if r.store == nil {
		return nil
	}
	return r.store.Close()
}

func newMaatLifecycleCmdWith(journal func() (lifecycleJournal, error), evidence func() (maat.LifecycleResolver, func(), error)) *cobra.Command {
	var id, repo, head, digest, originID, reviewSHA, originSHA, adoptionSHA string
	var confirm, preview, ownerAdopt bool
	cmd := &cobra.Command{Use: "lifecycle", Short: "Preview or retain a source lifecycle observation; unknown acceptance stays unqualified", Args: cobra.NoArgs}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if id == "" || repo == "" || head == "" {
			return fmt.Errorf("--receipt-id and independently selected --expect-repo/--expect-head are required; public source imports remain unqualified")
		}
		if preview && confirm {
			return fmt.Errorf("preview and confirm are separate intents")
		}
		if confirm && digest == "" && !ownerAdopt {
			return fmt.Errorf("preview first, then provide --expect-receipt-sha256 with --confirm")
		}
		resolver, close, err := evidence()
		if err != nil {
			return err
		}
		defer close()
		j, err := journal()
		if err != nil {
			return err
		}
		if ownerAdopt {
			if preview == confirm {
				return fmt.Errorf("owner adoption requires exactly one of --preview or --confirm")
			}
			if originID == "" {
				return fmt.Errorf("--origin-receipt-id must select the exact originating evaluation")
			}
			if confirm && (reviewSHA == "" || originSHA == "" || adoptionSHA == "") {
				return fmt.Errorf("owner adoption confirm requires previewed --expect-review-sha256, --expect-origin-sha256 and --expect-adoption-sha256")
			}
			adopter, ok := j.(lifecycleAdoptionJournal)
			if !ok {
				return fmt.Errorf("journal does not support exact owner adoption")
			}
			view, err := adopter.AdoptLifecycleReview(cmd.Context(), id, originID, repo, head, reviewSHA, originSHA, adoptionSHA, confirm, resolver)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(view)
		}
		view, err := j.ObserveLifecycle(cmd.Context(), id, repo, head, digest, confirm, resolver)
		if err != nil {
			return err
		}
		// One typed result for native/CLI. --json is explicit native compatibility.
		return json.NewEncoder(cmd.OutOrStdout()).Encode(view)
	}
	cmd.Flags().StringVar(&id, "receipt-id", "", "exact existing canonical review/decision ID")
	cmd.Flags().StringVar(&head, "expect-head", "", "independently selected full source head")
	cmd.Flags().StringVar(&repo, "expect-repo", "", "independently selected canonical repository")
	cmd.Flags().StringVar(&digest, "expect-receipt-sha256", "", "deny changed receipt bytes before recording")
	cmd.Flags().BoolVar(&ownerAdopt, "owner-adopt-review", false, "preview/confirm explicit owner adoption of the exact independent review and originating evaluation")
	cmd.Flags().StringVar(&originID, "origin-receipt-id", "", "exact canonical originating evaluation receipt ID for owner adoption")
	cmd.Flags().StringVar(&reviewSHA, "expect-review-sha256", "", "deny changed independent review bytes before owner adoption")
	cmd.Flags().StringVar(&originSHA, "expect-origin-sha256", "", "deny changed originating evaluation bytes before owner adoption")
	cmd.Flags().StringVar(&adoptionSHA, "expect-adoption-sha256", "", "deny changed owner-adoption envelope before recording")
	cmd.Flags().BoolVar(&preview, "preview", false, "read-only complete-history projection; never qualifies or authorizes")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "retain this exact observation once, not a qualification grant")
	cmd.Flags().Bool("json", false, "typed JSON result (this command always returns JSON)")
	return cmd
}

func init() {
	maatCmd.AddCommand(newMaatLifecycleCmdWith(func() (lifecycleJournal, error) { return maat.NewDefaultDecisionJournal() }, func() (maat.LifecycleResolver, func(), error) {
		reader := &lazyLifecycleRouterReader{}
		resolver := lifecycleevidence.Router{Reader: reader}
		if installed, verifyErr := lifecycleevidence.NewInstalledRouter(reader); verifyErr == nil {
			resolver = installed
		}
		return resolver, func() { _ = reader.Close() }, nil
	}))
}
