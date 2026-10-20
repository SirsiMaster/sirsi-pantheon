package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	dc "github.com/SirsiMaster/sirsi-pantheon/internal/desktopcustody"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// Set by the normal release to its EXISTING canonical service HTTPS origin.
// Empty fails closed. No environment/flag can select an alternative issuer.
var desktopCustodyServiceOrigin = "https://sirsi-router-6kdf4or4qq-uc.a.run.app"

var desktopSleepCmd = &cobra.Command{Use: "desktop-sleep", Short: "Governed finite sleep custody (no raw power commands)"}

func init() {
	for _, op := range []string{"inventory", "status", "preflight", "ready", "dispatch", "return", "reconcile", "observe"} {
		op := op
		cmd := &cobra.Command{Use: op + " <request-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return runDesktopSleep(cmd, op, args[0]) }}
		if op == "inventory" {
			cmd.Use = "inventory <machine-name>"
		}
		if op == "ready" || op == "status" {
			cmd.Flags().String("role", "custodian", "delegated coordinator, custodian or observer role (not authority)")
		}
		desktopSleepCmd.AddCommand(cmd)
	}
	rootCmd.AddCommand(desktopSleepCmd)
}

func runDesktopSleep(cmd *cobra.Command, op, id string) error {
	if op == "inventory" {
		i, err := dc.NativeIdentity(cmd.Context(), id)
		if err != nil {
			return err
		}
		inv, err := dc.NativeInventory(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Identity        dc.HostIdentity
			InventorySHA256 string
		}{i, inv})
	}
	store, err := routerstore.Resolve()
	if err != nil {
		return err
	}
	defer store.Close()
	auth, err := routerstore.DesktopSleepAuthority(store, desktopCustodyServiceOrigin)
	if err != nil {
		return err
	}
	role := "custodian"
	if op == "observe" {
		role = "observer"
	}
	if cmd.Flags().Lookup("role") != nil {
		role, _ = cmd.Flags().GetString("role")
	}
	state, err := auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "status", ID: id, Role: role})
	if err != nil {
		return err
	}
	if op == "status" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(state)
	}
	if op == "observe" {
		result, err := dc.ObserveNativeReturn(cmd.Context(), auth, id)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	if op == "ready" && role == "coordinator" {
		b, _ := json.Marshal(struct {
			ID          string
			At          time.Time
			Participant dc.Participant
		}{id, time.Now().UTC(), state.Delegation.Coordinator})
		result, err := auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "ready", ID: id, Role: role, QuietDigest: dc.EvidenceDigest(b)})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	nativeHost := state.Delegation.Admission.Host
	if role == "observer" {
		nativeHost = state.Delegation.ObserverMachineLabel
	}
	i, err := dc.NativeIdentity(cmd.Context(), nativeHost)
	if err != nil {
		return err
	}
	inv, err := dc.NativeInventory(cmd.Context())
	if err != nil {
		return err
	}
	if op == "inventory" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Identity        dc.HostIdentity
			InventorySHA256 string
		}{i, inv})
	}
	if op == "ready" {
		b, _ := json.Marshal(struct {
			Identity        dc.HostIdentity
			InventorySHA256 string
		}{i, inv})
		result, err := auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "ready", ID: id, Role: role, Identity: i, InventorySHA256: inv, QuietDigest: dc.EvidenceDigest(b)})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	controller := &dc.OnlineController{Authority: auth, ID: id, Identity: func(ctx context.Context) (dc.HostIdentity, error) {
		return dc.NativeIdentity(ctx, state.Delegation.Admission.Host)
	}, Inventory: dc.NativeInventory, ReturnProof: func(ctx context.Context, _ dc.SleepAdmission, _ dc.WakeReceipt) (dc.ReturnEvidence, error) {
		fresh, err := auth.DesktopSleep(ctx, dc.CustodyCall{Operation: "status", ID: id, Role: "custodian"})
		if err != nil {
			return dc.ReturnEvidence{}, err
		}
		return dc.NativeReturn(ctx, fresh)
	}}
	if op == "preflight" {
		a, err := controller.VerifySleep(cmd.Context(), state.Delegation.Request)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(a)
	}
	// No native command is constructed until canonical inspection succeeded.
	power, err := dc.NewDarwinPower()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	journal, closeJournal, err := dc.OpenNativeJournal(filepath.Join(home, ".sirsi", "desktop-custody"))
	if err != nil {
		return err
	}
	defer closeJournal()
	service := dc.SleepService{Verifier: controller, Custodian: controller, Power: power, Journal: journal, Now: time.Now}
	switch op {
	case "dispatch":
		return service.Dispatch(cmd.Context(), state.Delegation.Request)
	case "return":
		return service.VerifyReturn(cmd.Context(), state.Delegation.Request)
	case "reconcile":
		return service.ReconcileReturn(cmd.Context(), state.Delegation.Request)
	}
	return errors.New("sleep custody: unsupported controller operation")
}

// This function is called ONLY at canonical service startup. The environment
// is revision management configuration, not a lane's environment or RPC body.
// Reapplying an exact revision cannot revive consumed or revoked authority.
func provisionManagedDesktopSleep(store routerstore.Store, raw string) error {
	if raw == "" {
		return nil
	}
	var d dc.Delegation
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return fmt.Errorf("managed sleep policy: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("managed sleep policy: trailing input")
	}
	backend, ok := store.(*routerstore.SQLiteStore)
	if !ok {
		return errors.New("managed sleep policy: concrete service backend required")
	}
	return backend.CreateDesktopSleep(d)
}
