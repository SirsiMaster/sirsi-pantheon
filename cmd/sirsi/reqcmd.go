package main

// `sirsi req` — the canonical requirement registry (ADR-057 step 1).
//
// This is the referent for the third term of the runnable predicate ("unmet
// traced canon requirement") and the thing the completion gate traces to.
//
//	sirsi req add --title "..." --source ADR-057 --agent claude-home
//	sirsi req evidence REQ-001 --tests "CI 123" --production "verified ..."
//	sirsi req satisfy REQ-001        # refuses without all six references
//	sirsi req list --agent claude-home [--unmet]

import (
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

func newReqCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "req",
		Short: "Canonical requirement registry (ADR-057)",
		Long: "Requirements are the referent for \"unmet traced canon requirement\" in the\n" +
			"runnable predicate. `satisfy` refuses unless every ADR-057 §6 evidence\n" +
			"reference is present — a green build is one of six, never the whole.",
	}
	cmd.AddCommand(newReqAddCmd(), newReqEvidenceCmd(), newReqSatisfyCmd(), newReqWaiveCmd(), newReqAuditCmd(), newReqListCmd())
	return cmd
}

func newReqAddCmd() *cobra.Command {
	var title, source, sourceRef, agent string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a requirement (REQ-NNN allocated by the store)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			r, err := st.AddRequirement(title, source, sourceRef, agent)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s registered (%s)\n  %s\n", r.ID, r.Source, r.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "what the requirement demands (required)")
	cmd.Flags().StringVar(&source, "source", "", "canon source, e.g. ADR-057 or PRD (required)")
	cmd.Flags().StringVar(&sourceRef, "ref", "", "section within the source, e.g. 'step 1'")
	cmd.Flags().StringVar(&agent, "agent", "", "owning agent id (required)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("agent")
	return cmd
}

func newReqEvidenceCmd() *cobra.Command {
	var ev routerstore.Evidence
	cmd := &cobra.Command{
		Use:   "evidence <REQ-NNN>",
		Short: "Attach evidence references (additive — supplied fields overwrite, others persist)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.RecordEvidence(args[0], ev); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "evidence recorded for %s\n", strings.ToUpper(args[0]))
			return nil
		},
	}
	cmd.Flags().StringVar(&ev.Commit, "commit", "", "implementation commit or PR")
	cmd.Flags().StringVar(&ev.Tests, "tests", "", "test run reference")
	cmd.Flags().StringVar(&ev.Security, "security", "", "security control reference")
	cmd.Flags().StringVar(&ev.Design, "design", "", "design acceptance reference")
	cmd.Flags().StringVar(&ev.Deployment, "deployment", "", "deployment version")
	cmd.Flags().StringVar(&ev.Production, "production", "", "production verification reference")
	return cmd
}

func newReqSatisfyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "satisfy <REQ-NNN>",
		Short: "Mark satisfied — refuses unless all evidence references exist",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.Satisfy(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s satisfied\n", strings.ToUpper(args[0]))
			return nil
		},
	}
}

func newReqWaiveCmd() *cobra.Command {
	var reason, ownerDecision string
	cmd := &cobra.Command{
		Use:   "waive <REQ-NNN>",
		Short: "Record an explicit decision not to meet a requirement",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.Waive(args[0], reason, ownerDecision); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s waived: %s\n", strings.ToUpper(args[0]), reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why it will not be met (required)")
	cmd.Flags().StringVar(&ownerDecision, "owner-decision", "", "terminal router decision item sent by owner (required)")
	_ = cmd.MarkFlagRequired("reason")
	_ = cmd.MarkFlagRequired("owner-decision")
	return cmd
}

func newReqAuditCmd() *cobra.Command {
	var evidence string
	cmd := &cobra.Command{
		Use: "audit <agent>", Short: "Record a completed canon enumeration for one lane",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.MarkRequirementAudit(args[0], evidence); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "requirement audit recorded for %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&evidence, "evidence", "", "durable audit evidence reference (required)")
	_ = cmd.MarkFlagRequired("evidence")
	return cmd
}

func newReqListCmd() *cobra.Command {
	var agent string
	var unmetOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List requirements and what evidence each is still missing",
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()

			var reqs []routerstore.Requirement
			if unmetOnly {
				reqs, err = st.UnmetRequirements(agent)
			} else {
				reqs, err = st.ListRequirements(agent)
			}
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(reqs) == 0 {
				// "Zero unmet" and "no registry" are different facts and must
				// not render identically — an empty registry is what lets a
				// lane wrongly conclude it may park.
				fmt.Fprintln(w, "no requirements match — note this is NOT the same as 'canon fully implemented'; an empty registry means nothing is traced yet")
				return nil
			}
			for _, r := range reqs {
				fmt.Fprintf(w, "%-9s %-12s %-14s %s\n", r.ID, r.Status, r.Source, r.Title)
				if missing := r.Evidence.Missing(); len(missing) > 0 && r.Status != routerstore.ReqWaived {
					fmt.Fprintf(w, "          missing: %s\n", strings.Join(missing, ", "))
				}
			}
			fmt.Fprintf(w, "\n%d requirement(s)\n", len(reqs))
			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "filter to one owning agent")
	cmd.Flags().BoolVar(&unmetOnly, "unmet", false, "only requirements that still count toward the runnable predicate")
	return cmd
}
