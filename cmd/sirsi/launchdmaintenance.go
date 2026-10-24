package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

// This canonical app-engine entry is root-only in its native backend. It never
// requests elevation or turns a GUI confirmation into new OS authority.
type legacyMaintenanceController interface {
	PreviewLegacyMaintenance(context.Context, string) (setup.MaintenancePlan, error)
	ConfirmLegacyMaintenance(context.Context, setup.MaintenancePlan) (setup.MaintenanceStatus, error)
	LegacyMaintenanceStatus(context.Context) (setup.MaintenanceStatus, error)
	RecoverLegacyMaintenance(context.Context, string, string) (setup.MaintenanceStatus, error)
}

func newLegacyMaintenanceCmd(controller legacyMaintenanceController) *cobra.Command {
	c := &cobra.Command{Use: "legacy-maintenance", Short: "Explicit root-relay interruption with retained transport custody, never a legacy drain certificate"}
	var legacy, digest, transaction string
	var confirm bool
	preview := &cobra.Command{Use: "preview", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if legacy == "" {
			return errors.New("legacy maintenance: explicitly selected recoverable signed legacy payload required")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		plan, err := controller.PreviewLegacyMaintenance(ctx, legacy)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
	}}
	preview.Flags().StringVar(&legacy, "legacy-payload", "", "Existing root-owned signed payload matching the actual legacy loaded image")
	apply := &cobra.Command{Use: "confirm", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !confirm || legacy == "" || digest == "" {
			return errors.New("legacy maintenance: --confirm-maintenance, --legacy-payload and exact --expected-plan-sha256 required; not --drained")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
		defer cancel()
		plan, err := controller.PreviewLegacyMaintenance(ctx, legacy)
		if err != nil {
			return err
		}
		if plan.SHA256 != digest {
			return errors.New("legacy maintenance: reviewed preview changed; no lifecycle mutation")
		}
		result, err := controller.ConfirmLegacyMaintenance(ctx, plan)
		if result.ID != "" {
			err = errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(result))
		}
		return err
	}}
	apply.Flags().StringVar(&legacy, "legacy-payload", "", "Exact payload selector from the immutable preview")
	apply.Flags().StringVar(&digest, "expected-plan-sha256", "", "Exact complete preview digest")
	apply.Flags().BoolVar(&confirm, "confirm-maintenance", false, "Acknowledge selected legacy interruption and unknown old remote outcomes, not drained work")
	status := &cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()
		result, err := controller.LegacyMaintenanceStatus(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}}
	recover := &cobra.Command{Use: "recover", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !confirm || transaction == "" || digest == "" {
			return fmt.Errorf("legacy maintenance: exact --transaction, --expected-plan-sha256 and --confirm-maintenance required; no blind retirement/replay")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
		defer cancel()
		result, err := controller.RecoverLegacyMaintenance(ctx, transaction, digest)
		if result.ID != "" {
			err = errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(result))
		}
		return err
	}}
	recover.Flags().StringVar(&transaction, "transaction", "", "Retained original maintenance transaction ID")
	recover.Flags().StringVar(&digest, "expected-plan-sha256", "", "Retained complete preview digest")
	recover.Flags().BoolVar(&confirm, "confirm-maintenance", false, "Resume only this recorded root-relay transaction")
	c.AddCommand(preview, apply, status, recover)
	return c
}

func init() {
	launchdCmd.AddCommand(newLegacyMaintenanceCmd(setup.ServiceController{Backend: setup.NativeServiceBackend(), Authorize: authorizeLifecycle}))
}
