//go:build darwin

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

// This is the sole local-app adapter. Existing legacy-maintenance and modern
// service commands retain their registered actor/thread/remote authorization.
func newLegacyAppMaintenanceCmd(factory func(context.Context, setup.LegacyAppRequest) (legacyMaintenanceController, error)) *cobra.Command {
	return &cobra.Command{Use: "legacy-maintenance-app <bounded-request>", Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := setup.DecodeLegacyAppRequest(args[0])
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 90*time.Second)
			defer cancel()
			reply := setup.LegacyAppReply{Schema: 1, Operation: request.Operation}
			controller, err := factory(ctx, request)
			if err == nil {
				selected, selectErr := request.Arguments()
				err = selectErr
				if err == nil {
					inner := newLegacyMaintenanceCmd(controller)
					inner.SilenceUsage = true
					inner.SilenceErrors = true
					var output legacyAppOutput
					inner.SetContext(ctx)
					inner.SetArgs(selected)
					inner.SetIn(strings.NewReader(request.RestoreJSON))
					inner.SetOut(&output)
					inner.SetErr(io.Discard)
					err = inner.Execute()
					if output.Len() > 0 {
						if json.Valid(output.Bytes()) {
							reply.BodyBase64 = base64.StdEncoding.EncodeToString(output.Bytes())
						} else {
							err = errors.Join(err, errors.New("maintenance app: controller reply unverified; read status"))
						}
					}
				}
			}
			reply.OK = err == nil
			if err != nil {
				reply.Error = string([]rune(err.Error())[:min(len([]rune(err.Error())), 1200)])
			}
			// Operation errors remain framed, including structured retained debt.
			// A transport error is never relabeled a permission denial or rollback.
			return json.NewEncoder(cmd.OutOrStdout()).Encode(reply)
		}}
}

type legacyAppOutput struct{ data bytes.Buffer }

func (o *legacyAppOutput) Write(p []byte) (int, error) {
	if o.data.Len()+len(p) > 32*1024 {
		return 0, errors.New("maintenance app: reply exceeds bound")
	}
	return o.data.Write(p)
}
func (o *legacyAppOutput) Len() int      { return o.data.Len() }
func (o *legacyAppOutput) Bytes() []byte { return o.data.Bytes() }

func init() {
	launchdCmd.AddCommand(newLegacyAppMaintenanceCmd(func(ctx context.Context, r setup.LegacyAppRequest) (legacyMaintenanceController, error) {
		if err := setup.AuthorizeLegacyAppMaintenance(ctx, r.EngineSHA256); err != nil {
			return nil, err
		}
		return setup.ServiceController{Backend: setup.NativeServiceBackend(), Authorize: func(ctx context.Context, label string) error {
			if label != "ai.sirsi.router.relay" {
				return errors.New("maintenance app: unsupported service")
			}
			return setup.AuthorizeLegacyAppMaintenance(ctx, r.EngineSHA256)
		}}, nil
	}))
}
