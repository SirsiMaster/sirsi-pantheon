package main

import (
	"encoding/json"
	"errors"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/spf13/cobra"
)

// Local prevention is a closed default-store operation, never an arbitrary
// --store/status/signature importer, producer gate, executable or permission.
func newMaatLocalFailureReadCmd() *cobra.Command {
	var key string
	var jsonOutput bool
	cmd := &cobra.Command{Use: "failure", Short: "Inspect a verified local managed-watch occurrence and retained history", Args: cobra.NoArgs}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		s, err := maat.OpenDefaultStore()
		if err != nil {
			return err
		}
		defer s.Close()
		view, err := s.ReadLocalPreventionDisposition(key)
		if err != nil {
			return err
		}
		return renderMaatLocalMemory(cmd, view, jsonOutput)
	}
	cmd.Flags().StringVar(&key, "incident-key", "", "exact captured stable local incident key")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	_ = cmd.MarkFlagRequired("incident-key")
	return cmd
}

func newMaatLocalDispositionCmd() *cobra.Command {
	parent := &cobra.Command{Use: "disposition", Short: "Explicit local owner conclusion; never a repair or health claim"}
	var key, head, body, caseEvidence, effect, conclusion string
	var previewJSON bool
	preview := &cobra.Command{Use: "preview", Short: "Retain the exact data-only owner-disposition plan", Args: cobra.NoArgs}
	preview.RunE = func(cmd *cobra.Command, _ []string) error {
		s, err := maat.OpenDefaultStore()
		if err != nil {
			return err
		}
		defer s.Close()
		plan, err := s.PreviewLocalPreventionDisposition(key, head, body, caseEvidence, effect, conclusion)
		if err != nil {
			return err
		}
		if err := renderMaatLocalMemory(cmd, plan, previewJSON); err != nil {
			return err
		}
		if !plan.CanCommit {
			return errors.New("local prevention lift is blocked; inspect retained action/debt and reconcile without replay")
		}
		return nil
	}
	preview.Flags().StringVar(&key, "incident-key", "", "exact captured incident key")
	preview.Flags().StringVar(&head, "occurrence-key", "", "exact captured current occurrence")
	preview.Flags().StringVar(&body, "body-sha256", "", "exact captured occurrence body SHA256")
	preview.Flags().StringVar(&caseEvidence, "case-evidence", "", "exact captured retained Casebook preflight reference")
	preview.Flags().StringVar(&effect, "effect", "", "accept-only or retire-local-prevention; never grants future action")
	preview.Flags().StringVar(&conclusion, "conclusion", "", "owner conclusion, plain bounded data; not a command")
	preview.Flags().BoolVar(&previewJSON, "json", false, "JSON output")
	for _, flag := range []string{"incident-key", "occurrence-key", "body-sha256", "case-evidence", "effect", "conclusion"} {
		_ = preview.MarkFlagRequired(flag)
	}
	var planSHA string
	var confirm, commitJSON bool
	commit := &cobra.Command{Use: "commit", Short: "Confirm only the exact retained local owner-disposition plan", Args: cobra.NoArgs}
	commit.RunE = func(cmd *cobra.Command, _ []string) error {
		if !confirm {
			return errors.New("local owner disposition requires the exact plan SHA and explicit --confirm; no change was made")
		}
		s, err := maat.OpenDefaultStore()
		if err != nil {
			return err
		}
		defer s.Close()
		receipt, err := s.CommitLocalPreventionDisposition(planSHA, planSHA)
		return renderMaatLocalDispositionResult(cmd, planSHA, receipt, err, commitJSON)
	}
	commit.Flags().StringVar(&planSHA, "plan-sha256", "", "exact reviewed plan SHA256")
	commit.Flags().BoolVar(&confirm, "confirm", false, "confirm only the exact named plan/effect; no repair")
	commit.Flags().BoolVar(&commitJSON, "json", false, "JSON output")
	_ = commit.MarkFlagRequired("plan-sha256")
	var reconcileSHA string
	var reconcileJSON bool
	reconcile := &cobra.Command{Use: "reconcile", Short: "Complete only an existing retained consent intent, without installer replay", Args: cobra.NoArgs}
	reconcile.RunE = func(cmd *cobra.Command, _ []string) error {
		s, err := maat.OpenDefaultStore()
		if err != nil {
			return err
		}
		defer s.Close()
		receipt, err := s.ReconcileLocalPreventionDisposition(reconcileSHA)
		return renderMaatLocalDispositionResult(cmd, reconcileSHA, receipt, err, reconcileJSON)
	}
	reconcile.Flags().StringVar(&reconcileSHA, "plan-sha256", "", "exact already-retained consent plan SHA256")
	reconcile.Flags().BoolVar(&reconcileJSON, "json", false, "JSON output")
	_ = reconcile.MarkFlagRequired("plan-sha256")
	var pendingKey string
	var pendingJSON bool
	pending := &cobra.Command{Use: "pending", Short: "Inspect retained local consent recovery references; grants no authority", Args: cobra.NoArgs}
	pending.RunE = func(cmd *cobra.Command, _ []string) error {
		s, err := maat.OpenDefaultStore()
		if err != nil {
			return err
		}
		defer s.Close()
		offers, err := s.InspectLocalPreventionRecovery(pendingKey)
		if err != nil {
			return err
		}
		return renderMaatLocalMemory(cmd, offers, pendingJSON)
	}
	pending.Flags().StringVar(&pendingKey, "incident-key", "", "exact captured local incident key")
	pending.Flags().BoolVar(&pendingJSON, "json", false, "JSON output")
	_ = pending.MarkFlagRequired("incident-key")
	parent.AddCommand(preview, commit, reconcile, pending)
	return parent
}

func renderMaatLocalMemory(cmd *cobra.Command, value any, jsonOutput bool) error {
	if jsonOutput || JsonOutput || maatJSON {
		return emitJSON(value)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	cmd.Println(string(data))
	return nil
}
func renderMaatLocalDispositionResult(cmd *cobra.Command, sha string, receipt maat.LocalPreventionDispositionReceipt, actionErr error, jsonOutput bool) error {
	if actionErr == nil {
		return renderMaatLocalMemory(cmd, receipt, jsonOutput)
	}
	if jsonOutput || JsonOutput || maatJSON {
		if err := emitJSON(struct {
			Schema           string `json:"schema"`
			PlanSHA256       string `json:"plan_sha256"`
			Status           string `json:"status"`
			Recovery         string `json:"recovery"`
			ActionAuthorized bool   `json:"action_authorized"`
		}{"sirsi.maat.local-prevention-disposition-error.v1", sha, "incomplete", "requires-exact-existing-retained-intent", false}); err != nil {
			return err
		}
	}
	return actionErr
}

func init() {
	maatFailureMemoryCmd.AddCommand(newMaatLocalFailureReadCmd(), newMaatLocalDispositionCmd())
}
