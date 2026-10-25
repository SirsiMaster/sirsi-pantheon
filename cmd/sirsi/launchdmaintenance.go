package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
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
	AbortLegacyMaintenance(context.Context, string, string) (setup.MaintenanceStatus, error)
	PreviewLegacyRestore(context.Context, string, string, string) (setup.MaintenanceRestorePlan, error)
	ConfirmLegacyRestore(context.Context, setup.MaintenanceRestorePlan) (setup.MaintenanceStatus, error)
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
		request, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(request, 45*time.Second)
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
		if result.ID != "" {
			err = errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(result))
		}
		return err
	}}
	recover := &cobra.Command{Use: "recover", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !confirm || transaction == "" || digest == "" {
			return fmt.Errorf("legacy maintenance: exact --transaction, --expected-plan-sha256 and --confirm-maintenance required; no blind retirement/replay")
		}
		request, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(request, 45*time.Second)
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
	abort := &cobra.Command{Use: "abort", Args: cobra.NoArgs, Short: "Abort only a verified pre-retirement preparation; never revive a retired service", RunE: func(cmd *cobra.Command, _ []string) error {
		if !confirm || transaction == "" || digest == "" {
			return errors.New("legacy maintenance: exact abort transaction, plan and confirmation required")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		result, err := controller.AbortLegacyMaintenance(ctx, transaction, digest)
		if result.ID != "" {
			err = errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(result))
		}
		return err
	}}
	abort.Flags().StringVar(&transaction, "transaction", "", "Exact retained transaction")
	abort.Flags().StringVar(&digest, "expected-plan-sha256", "", "Exact retained plan digest")
	abort.Flags().BoolVar(&confirm, "confirm-maintenance", false, "Restore only this verified pre-retirement admission")
	var artifact, restoreSHA string
	restorePreview := &cobra.Command{Use: "restore-preview", Args: cobra.NoArgs, Short: "Inspect exact approved replacement restoration; never execute the artifact", RunE: func(cmd *cobra.Command, _ []string) error {
		if transaction == "" || digest == "" || artifact == "" {
			return errors.New("legacy maintenance: transaction, original plan and existing approved artifact required")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		plan, err := controller.PreviewLegacyRestore(ctx, transaction, digest, artifact)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
	}}
	restorePreview.Flags().StringVar(&transaction, "transaction", "", "Exact pending transaction")
	restorePreview.Flags().StringVar(&digest, "expected-plan-sha256", "", "Original immutable maintenance digest")
	restorePreview.Flags().StringVar(&artifact, "artifact", "", "Existing signed artifact matching the originally approved replacement")
	restoreConfirm := &cobra.Command{Use: "restore-confirm", Args: cobra.NoArgs, Short: "Restore only reviewed replacement bytes from bounded captured plan stdin", RunE: func(cmd *cobra.Command, _ []string) error {
		if !confirm || restoreSHA == "" {
			return errors.New("legacy maintenance: explicit restoration confirmation and expected restore digest required")
		}
		request, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(request, 45*time.Second)
		defer cancel()
		reader := cmd.InOrStdin()
		if file, ok := reader.(*os.File); ok {
			info, err := file.Stat()
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				if info.Size() > 8192 {
					return errors.New("legacy maintenance: restore plan input exceeds bound")
				}
			} else {
				deadline, _ := ctx.Deadline()
				if err := file.SetReadDeadline(deadline); err != nil {
					return errors.New("legacy maintenance: bounded captured restore-plan input required")
				}
				defer func() { _ = file.SetReadDeadline(time.Time{}) }()
			}
		}
		raw, err := io.ReadAll(io.LimitReader(reader, 8193))
		if err != nil {
			return err
		}
		plan, err := setup.DecodeMaintenanceRestorePlan(raw)
		if err != nil {
			return err
		}
		if plan.SHA256 != restoreSHA {
			return errors.New("legacy maintenance: captured restoration plan differs from consent")
		}
		result, err := controller.ConfirmLegacyRestore(ctx, plan)
		if result.ID != "" {
			err = errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(result))
		}
		return err
	}}
	restoreConfirm.Flags().StringVar(&restoreSHA, "expected-restore-sha256", "", "Exact reviewed restoration digest")
	restoreConfirm.Flags().BoolVar(&confirm, "confirm-maintenance", false, "Restore only the captured approved replacement, retaining backup/stage debt")
	c.AddCommand(preview, apply, status, recover, abort, restorePreview, restoreConfirm)
	return c
}

func init() {
	launchdCmd.AddCommand(newLegacyMaintenanceCmd(setup.ServiceController{Backend: setup.NativeServiceBackend(), Authorize: authorizeLegacyMaintenance}))
}

func authorizeLegacyMaintenance(ctx context.Context, label string) error {
	if label != "ai.sirsi.router.relay" {
		return errors.New("legacy maintenance: exact root relay authority required")
	}
	return authorizeLifecycleWithReader(ctx, label, func(ctx context.Context, repo, actor, tid string) (routerstore.ThreadRecord, error) {
		var record routerstore.ThreadRecord
		err := setup.WithRootRelayMaintenanceAuthority(ctx, func(endpoint, token, pin string) error {
			client, err := routerstore.NewMemoryOnlyRemoteStoreForCaller(endpoint, token, actor, tid, pin)
			if err != nil {
				return err
			}
			defer client.Close()
			record, err = client.GetThreadRecordContext(ctx, tid)
			return err
		})
		return record, err
	})
}
