package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// Per-command bindings, not mutable package-wide authority. The production
// constructor resolves only the existing configured canonical service/relay.
func newTaskResumeCommand(resolve func() (routerstore.Store, error)) *cobra.Command {
	var apply bool
	var version, evidence, digest, reason string
	var ttl time.Duration
	cmd := &cobra.Command{
		Use: "resume-claim <agent> <task-id>", Args: cobra.ExactArgs(2),
		Short: "Inspect or explicitly resume one exhausted original with evidence, preserving history",
		Long:  "Inspect by default. --apply issues ONE fresh normal lease using exact row-version CAS and cause-fixed evidence. Never resets attempts or marks DONE; use ordinary fenced task complete with the returned lease. Requires a compatible configured canonical service and this lane's fresh registered thread.",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := routerstore.TaskResumeRequest{Version: 1, Action: "inspect", Agent: args[0], TaskID: args[1]}
			if apply {
				if ttl%time.Second != 0 {
					return fmt.Errorf("--ttl must be whole seconds")
				}
				req.Action, req.ExpectedVersion, req.EvidenceRef, req.EvidenceSHA256, req.Reason, req.TTLSeconds = "claim", version, evidence, digest, reason, int64(ttl/time.Second)
			} else if version != "" || evidence != "" || digest != "" || reason != "" || cmd.Flags().Changed("ttl") {
				return fmt.Errorf("claim inputs require explicit --apply; inspection is read-only")
			}
			if err := routerstore.ValidateTaskResumeRequest(req); err != nil {
				return err
			}
			s, err := resolve()
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()
			if _, ok := s.(*routerstore.RemoteStore); !ok {
				return fmt.Errorf("task resume requires the configured canonical service/relay, not a local store")
			}
			out, err := s.ResumeTaskClaim(cmd.Context(), req)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Issue one audited recovery lease (default: inspect only)")
	cmd.Flags().StringVar(&version, "expected-version", "", "Exact opaque row version from inspection")
	cmd.Flags().StringVar(&evidence, "evidence-ref", "", "Cause-fixed proof reference (actor attestation, not service verification)")
	cmd.Flags().StringVar(&digest, "evidence-sha256", "", "SHA256 of the referenced proof")
	cmd.Flags().StringVar(&reason, "reason", "", "Specific corrected cause")
	cmd.Flags().DurationVar(&ttl, "ttl", 10*time.Minute, "Fresh normal lease duration, at most 30m")
	return cmd
}

func init() { routerTaskCmd.AddCommand(newTaskResumeCommand(routerstore.Resolve)) }
