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
	for _, op := range []string{"inventory", "status", "preflight", "ready", "coordinate", "dispatch", "return", "reconcile", "observe", "abort-before-native"} {
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
	defer func() { _ = store.Close() }()
	auth, err := routerstore.DesktopSleepAuthority(store, desktopCustodyServiceOrigin)
	if err != nil {
		return err
	}
	role := "custodian"
	if op == "observe" {
		role = "observer"
	}
	if op == "coordinate" {
		role = "coordinator"
	}
	if cmd.Flags().Lookup("role") != nil {
		role, _ = cmd.Flags().GetString("role")
	}
	state, err := auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "status", ID: id, Role: role})
	if errors.Is(err, routerstore.ErrNotFound) && op == "dispatch" {
		// Normal supervised startup publishes public binding facts, then waits
		// within its REAL job budget for management to create the policy. This
		// cannot issue policy or execute power, and does not retry a consume.
		budget, budgetErr := dc.NativeExecutionBudget(cmd.Context())
		if budgetErr != nil {
			return budgetErr
		}
		i, identityErr := dc.NativeIdentity(cmd.Context(), "m5")
		if identityErr != nil {
			return identityErr
		}
		inv, inventoryErr := dc.NativeInventory(cmd.Context())
		if inventoryErr != nil {
			return inventoryErr
		}
		b, _ := json.Marshal(budget)
		if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			State, RequestID              string
			Identity                      dc.HostIdentity
			InventorySHA256, BudgetSHA256 string
			Budget                        dc.ExecutionBudget
		}{"awaiting-canonical-management", id, i, inv, dc.EvidenceDigest(b), budget}); encodeErr != nil {
			return encodeErr
		}
		deadline, _ := time.Parse(time.RFC3339Nano, budget.ExecutionDeadline)
		waiting, cancel := context.WithDeadline(cmd.Context(), deadline)
		defer cancel()
		for errors.Is(err, routerstore.ErrNotFound) && time.Now().Before(deadline) {
			select {
			case <-waiting.Done():
				return waiting.Err()
			case <-time.After(time.Second):
			}
			state, err = auth.DesktopSleep(waiting, dc.CustodyCall{Operation: "status", ID: id, Role: role})
		}
	}
	if err != nil {
		return err
	}
	if op != "status" && op != "return" && op != "reconcile" && op != "abort-before-native" {
		bounded, cancel := context.WithDeadline(cmd.Context(), state.Delegation.Admission.NotAfter)
		defer cancel()
		cmd.SetContext(bounded)
	}
	if op == "status" {
		var events []dc.SleepEvent
		if role == "custodian" {
			events, err = localDesktopSleepEvents(id)
			if err != nil {
				return err
			}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Canonical   dc.CustodyState
			LocalEvents []dc.SleepEvent
		}{state, events})
	}
	if op == "observe" {
		result, err := dc.ObserveNativeReturn(cmd.Context(), auth, id)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	if op == "coordinate" {
		for time.Now().Before(state.Delegation.Admission.NotAfter) {
			if !state.RevokedAt.IsZero() {
				return errors.New("sleep custody: coordinator delegation revoked")
			}
			if !state.ConsumedAt.IsZero() {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(state)
			}
			b, _ := json.Marshal(struct {
				ID          string
				At          time.Time
				Participant dc.Participant
			}{id, time.Now().UTC(), state.Delegation.Coordinator})
			if _, err := auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "ready", ID: id, Role: role, QuietDigest: dc.EvidenceDigest(b)}); err != nil {
				return err
			}
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(time.Second):
			}
			state, err = auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "status", ID: id, Role: role})
			if err != nil {
				return err
			}
		}
		return errors.New("sleep custody: finite coordinator window ended without consume")
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
	var inv string
	if op != "abort-before-native" {
		inv, err = dc.NativeInventory(cmd.Context())
		if err != nil {
			return err
		}
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
		var budget *dc.ExecutionBudget
		if role == "custodian" {
			actual, err := dc.NativeExecutionBudget(cmd.Context())
			if err != nil {
				return err
			}
			budget = &actual
		}
		result, err := auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "ready", ID: id, Role: role, Identity: i, InventorySHA256: inv, QuietDigest: dc.EvidenceDigest(b), Budget: budget})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	controller := &dc.OnlineController{Authority: auth, ID: id, Identity: func(ctx context.Context) (dc.HostIdentity, error) {
		return dc.NativeIdentity(ctx, state.Delegation.Admission.Host)
	}, Inventory: dc.NativeInventory, Budget: dc.NativeExecutionBudget, ReturnProof: func(ctx context.Context, _ dc.SleepAdmission, _ dc.WakeReceipt) (dc.ReturnEvidence, error) {
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
	defer func() { _ = closeJournal() }()
	service := dc.SleepService{Verifier: controller, Custodian: controller, Power: power, Journal: journal, Now: time.Now}
	switch op {
	case "abort-before-native":
		err := service.AbortBeforeNative(cmd.Context(), state.Delegation.Request)
		if errors.Is(err, dc.ErrSleepAbortedBeforeNative) {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"state": "pre-native-aborted", "request_id": id})
		}
		return err
	case "dispatch":
		err := service.Dispatch(cmd.Context(), state.Delegation.Request)
		reconcile := false
		if !errors.Is(err, dc.ErrReturnVerificationPending) {
			if !errors.Is(err, dc.ErrSleepUnknown) {
				return err
			}
			events, readErr := journal.ReadSleep(id)
			if readErr != nil {
				return readErr
			}
			invoked, readback := false, false
			for _, e := range events {
				invoked = invoked || e.Phase == dc.SleepInvoked
				readback = readback || e.Phase == dc.WakeReadBack
			}
			if !invoked || !readback {
				return err
			}
			reconcile = true
		}
		// Keep this actual controller/session alive across sleep. Only status
		// reads repeat; authorization and native invocation never repeat.
		for time.Now().Before(state.Delegation.Admission.NotAfter) {
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(time.Second):
			}
			fresh, readErr := auth.DesktopSleep(cmd.Context(), dc.CustodyCall{Operation: "status", ID: id, Role: "custodian"})
			if readErr != nil {
				continue
			}
			if fresh.Observation != nil {
				if reconcile {
					return service.ReconcileReturn(cmd.Context(), state.Delegation.Request)
				}
				return service.VerifyReturn(cmd.Context(), state.Delegation.Request)
			}
		}
		return fmt.Errorf("%w: original request retained; inspect status and reconcile without redispatch", dc.ErrReturnVerificationPending)
	case "return":
		return service.VerifyReturn(cmd.Context(), state.Delegation.Request)
	case "reconcile":
		return service.ReconcileReturn(cmd.Context(), state.Delegation.Request)
	}
	return errors.New("sleep custody: unsupported controller operation")
}

func localDesktopSleepEvents(id string) ([]dc.SleepEvent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".sirsi", "desktop-custody")
	if _, err = os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	j, closeJournal, err := dc.OpenNativeJournal(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeJournal() }()
	return j.ReadSleep(id)
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

func revokeManagedDesktopSleep(store routerstore.Store, raw string) error {
	if raw == "" {
		return nil
	}
	var request struct{ ID, Reason string }
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("managed sleep revocation: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("managed sleep revocation: trailing input")
	}
	backend, ok := store.(*routerstore.SQLiteStore)
	if !ok {
		return errors.New("managed sleep revocation: concrete service backend required")
	}
	return backend.RevokeDesktopSleep(request.ID, request.Reason)
}
