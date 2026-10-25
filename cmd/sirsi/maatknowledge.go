package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var maatKnowledgeCmd = &cobra.Command{
	Use:   "knowledge [text]",
	Short: "Inspect Ma'at's local evidence and knowledge library",
	Long: `Inspect the local knowledge that Ma'at can connect to evidence-backed decisions.

This is a read-only projection. It does not ingest, export, rescore, or change
the archive. Search is local and bounded; use detail/history for retained
versions and provenance. The legacy 'seshat' commands remain compatibility
aliases while Ma'at is the sole archive and operator surface.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate Ma'at knowledge home: %w", err)
		}
		queryText, _ := cmd.Flags().GetString("query")
		if queryText == "" {
			queryText = strings.Join(args, " ")
		}
		classification, _ := cmd.Flags().GetString("classification")
		resolution, _ := cmd.Flags().GetString("resolution")
		source, _ := cmd.Flags().GetString("source")
		limit, _ := cmd.Flags().GetInt("limit")
		cursor, _ := cmd.Flags().GetString("cursor")
		includeHistory, _ := cmd.Flags().GetBool("history")
		withheldOnly, _ := cmd.Flags().GetBool("withheld")
		view, err := knowledge.LoadPageContext(cmd.Context(), home, knowledge.SearchQuery{
			Text: queryText, Classification: classification, Resolution: knowledge.ResolutionState(resolution),
			Source: source, Limit: limit, Cursor: cursor, IncludeHistory: includeHistory, WithheldOnly: withheldOnly,
		})
		if err != nil {
			return err
		}

		if JsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(view)
		}
		if view.Total == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Ma'at's private archive has no matching records. Review source coverage or confirm a local refresh.")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Ma'at knowledge — %d item(s)\n", view.Total)
		for _, run := range view.Coverage.Sources {
			if run.State != knowledge.ImportComplete && run.State != knowledge.ImportNoNewItems {
				fmt.Fprintf(cmd.OutOrStdout(), "Source %s: %s (%s)\n", run.Source, run.State, run.ErrorCode)
			}
		}
		for _, item := range view.Items {
			fmt.Fprintf(cmd.OutOrStdout(), "\n%s [%s · %s · r%d]\n%s\n", item.Title, item.Classification, item.Resolution, item.Revision, item.Summary)
			if item.WithheldReason != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Withheld reason: %s\n", item.WithheldReason)
			}
			if item.Excerpt != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", item.Excerpt)
			}
		}
		if view.NextCursor != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Next page: sirsi maat knowledge --cursor %s\n", view.NextCursor)
		}
		return nil
	},
}

var maatKnowledgeDetailCmd = &cobra.Command{
	Use:   "detail <id>",
	Short: "Inspect one retained Ma'at knowledge record",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		archive, err := knowledge.Open(home)
		if err != nil {
			return err
		}
		defer archive.Close()
		revision, _ := cmd.Flags().GetInt("revision")
		record, err := archive.Detail(cmd.Context(), args[0], revision)
		if err != nil {
			return err
		}
		if JsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(record)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n%s\n", record.Title, record.Summary, record.Content)
		for _, origin := range record.Origins {
			fmt.Fprintf(cmd.OutOrStdout(), "Origin %s · %s · %s\n", origin.Source, origin.InstanceID, origin.Reference)
		}
		return nil
	},
}

var maatKnowledgeHistoryCmd = &cobra.Command{
	Use:   "history <id>",
	Short: "Review immutable versions of one Ma'at knowledge record",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		archive, err := knowledge.Open(home)
		if err != nil {
			return err
		}
		defer archive.Close()
		limit, _ := cmd.Flags().GetInt("limit")
		cursor, _ := cmd.Flags().GetString("cursor")
		history, err := archive.HistoryPage(cmd.Context(), args[0], limit, cursor)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(history)
	},
}

var maatKnowledgeRebuildIndexCmd = &cobra.Command{
	Use:   "rebuild-index",
	Short: "Rebuild Ma'at's private search index from retained safe versions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		archive, err := knowledge.Open(home)
		if err != nil {
			return err
		}
		defer archive.Close()
		if err := archive.RebuildIndex(cmd.Context()); err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"status": "rebuilt", "scope": "safe-retained-versions"})
	},
}

var maatKnowledgeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Inspect per-source Ma'at archive coverage",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		archive, err := knowledge.Open(home)
		if err != nil {
			return err
		}
		defer archive.Close()
		coverage, err := archive.Coverage(cmd.Context(), 50)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(coverage)
	},
}

var maatKnowledgeResolveCmd = &cobra.Command{
	Use:   "resolve <id>",
	Short: "Record an explicit owner review of a retained knowledge item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		stateText, _ := cmd.Flags().GetString("state")
		revision, _ := cmd.Flags().GetInt("revision")
		digest, _ := cmd.Flags().GetString("digest")
		stdinNote, _ := cmd.Flags().GetBool("note-stdin")
		if !stdinNote {
			return errors.New("knowledge owner note must be supplied through --note-stdin")
		}
		var note string
		if stdinNote {
			data, readErr := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 4001))
			if readErr != nil || len(data) > 4000 {
				return errors.New("knowledge owner note could not be read within its bound")
			}
			note = string(data)
		}
		archive, err := knowledge.Open(home)
		if err != nil {
			return err
		}
		defer archive.Close()
		annotationID, err := archive.RecordOwnerAnnotation(cmd.Context(), args[0], revision, digest, knowledge.ResolutionState(stateText), note)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"record_id": args[0], "annotation_id": annotationID, "state": stateText,
			"casebook_projection": "pending", "next_action": "sirsi maat knowledge status",
		})
	},
}

var maatKnowledgeReconcileCmd = &cobra.Command{
	Use:   "reconcile-projections",
	Short: "Retry pending owner-review projections into Ma'at Casebook",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		archive, err := knowledge.Open(home)
		if err != nil {
			return err
		}
		defer archive.Close()
		pending, err := archive.PendingProjectionAnnotations(cmd.Context(), 50)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(maatKnowledgeProjectionResult(0, 0))
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return errors.New("Ma'at Casebook journal is unavailable; projection debt remains pending")
		}
		once, ok := journal.(interface {
			AppendOnce(maat.Decision) (bool, error)
		})
		if !ok {
			return errors.New("Ma'at Casebook journal does not support idempotent projection; debt remains pending")
		}
		projected := 0
		for _, annotation := range pending {
			decision := maat.Decision{
				Time: annotation.CreatedAt.UTC().Format(time.RFC3339Nano),
				Kind: "knowledge owner review", Requester: "maat", Resource: annotation.RecordID,
				Assessed: "retained knowledge archive item", Determination: string(annotation.State),
				Why: annotation.Text, Evidence: fmt.Sprintf("maat-knowledge-annotation:%d", annotation.ID),
			}
			if _, err := once.AppendOnce(decision); err != nil {
				return errors.New("Ma'at Casebook projection failed; archive debt remains pending for retry")
			}
			if err := archive.MarkProjected(cmd.Context(), annotation.ID); err != nil {
				return errors.New("Ma'at Casebook receipt could not be acknowledged; retry will not duplicate the decision")
			}
			projected++
		}
		coverage, err := archive.Coverage(cmd.Context(), 50)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(maatKnowledgeProjectionResult(projected, coverage.PendingProjectionDebt))
	},
}

func maatKnowledgeProjectionResult(projected, remaining int) *output.CommandResult {
	status := "ok"
	summary := "Ma'at reconciled pending owner decisions into Casebook."
	var errs []string
	if remaining > 0 {
		status = "partial"
		summary = "Ma'at projected available owner decisions; Casebook projection debt remains."
		errs = []string{"projection_debt_remains"}
	} else if projected == 0 {
		summary = "No pending Ma'at owner decisions required Casebook reconciliation."
	}
	return &output.CommandResult{
		Command:    "sirsi maat knowledge reconcile-projections",
		BriefTitle: "Ma'at Casebook projection",
		Summary:    summary,
		Status:     status,
		Errors:     errs,
		Evidence: []output.Evidence{
			{Label: "Projected", Value: strconv.Itoa(projected)},
			{Label: "Remaining", Value: strconv.Itoa(remaining)},
		},
	}
}

// maatKnowledgeRefreshCmd is the explicit local source-ingestion path for the
// canonical Ma'at archive. Read-only projections never invoke it implicitly.
var maatKnowledgeRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh Ma'at's local knowledge from configured sources",
	Long: `Refresh Ma'at's private local archive from explicitly enabled local readers.

This first archive slice enables only the local Claude transcript reader.
Online sources are never contacted by this command. It does not export
knowledge, open a browser, authorize work, or make a remote decision.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		started := time.Now()
		source, _ := cmd.Flags().GetString("source")
		sinceText, _ := cmd.Flags().GetString("since")
		profile, _ := cmd.Flags().GetString("profile")
		allProfiles, _ := cmd.Flags().GetBool("all-profiles")
		if profile != "" || allProfiles {
			result := knowledge.RefreshResult{State: knowledge.ImportUnavailable, Runs: []knowledge.ImportRun{}, ErrorCodes: []string{"profile_reader_not_enabled"}}
			maatKnowledgeRefreshReceipt(time.Since(started), result).Render()
			return errors.New("requested local knowledge profile reader is unavailable")
		}
		since, err := parseMaatKnowledgeSince(sinceText)
		if err != nil {
			return err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		result, refreshErr := knowledge.RefreshLocal(cmd.Context(), home, source, since, "")
		maatKnowledgeRefreshReceipt(time.Since(started), result).Render()
		if refreshErr != nil {
			return errors.New("Ma'at local knowledge refresh failed; inspect the typed source status")
		}
		if result.State != knowledge.ImportComplete && result.State != knowledge.ImportNoNewItems {
			return errors.New("Ma'at local knowledge refresh was incomplete; inspect the typed source status")
		}
		return nil
	},
}

func maatKnowledgeRefreshResult(elapsed time.Duration) *output.CommandResult {
	return &output.CommandResult{
		Command:    "sirsi maat knowledge refresh",
		BriefTitle: "Ma'at knowledge refresh",
		Summary:    "Ma'at refreshed its private local archive. Review retained items before using them in a decision.",
		Status:     "ok",
		Duration:   elapsed,
		NextActions: []output.NextAction{{
			Label:       "Open Ma'at knowledge",
			Command:     "sirsi maat knowledge --json",
			Description: "Inspect the retained, sensitivity-filtered knowledge projection.",
		}},
	}
}

func maatKnowledgeRefreshReceipt(elapsed time.Duration, refresh knowledge.RefreshResult) *output.CommandResult {
	status := "ok"
	summary := "Ma'at refreshed its private local knowledge archive."
	var errs []string
	if refresh.State != knowledge.ImportComplete && refresh.State != knowledge.ImportNoNewItems {
		status = string(refresh.State)
		if refresh.State == knowledge.ImportFailed || refresh.State == knowledge.ImportUnavailable {
			summary = "Ma'at could not confirm a complete local archive refresh. Inspect the typed source status before retrying."
		} else {
			summary = "Ma'at retained available knowledge, but one or more source or migration checks need review."
		}
		errs = append(errs, refresh.ErrorCodes...)
		for _, run := range refresh.Runs {
			if run.ErrorCode != "" {
				errs = append(errs, run.Source+": "+run.ErrorCode)
			}
		}
	}
	return &output.CommandResult{
		Command: "sirsi maat knowledge refresh", BriefTitle: "Ma'at knowledge refresh",
		Summary: summary, Status: status, Duration: elapsed, Errors: errs,
		NextActions: []output.NextAction{{Label: "Review Ma'at knowledge", Command: "sirsi maat knowledge status", Description: "Inspect per-source coverage, then retry only the source that needs attention."}},
	}
}

func parseMaatKnowledgeSince(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err == nil && days >= 0 && days <= 36500 {
			return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
		}
	}
	if duration, err := time.ParseDuration(value); err == nil && duration >= 0 {
		return time.Now().Add(-duration), nil
	}
	if date, err := time.Parse("2006-01-02", value); err == nil {
		return date, nil
	}
	return time.Time{}, errors.New("invalid --since value; use a nonnegative duration or YYYY-MM-DD")
}

func init() {
	maatKnowledgeCmd.Flags().String("classification", "", "Filter by deterministic source classification")
	maatKnowledgeCmd.Flags().String("resolution", "", "Filter by owner-resolution state")
	maatKnowledgeCmd.Flags().String("source", "", "Filter by source")
	maatKnowledgeCmd.Flags().Int("limit", 20, "Maximum results (1 through 100)")
	maatKnowledgeCmd.Flags().String("cursor", "", "Continue a stable search page")
	maatKnowledgeCmd.Flags().Bool("history", false, "Search all retained versions")
	maatKnowledgeCmd.Flags().Bool("withheld", false, "List only safe placeholders and reasons for withheld source items")
	maatKnowledgeCmd.Flags().String("query", "", "Full-text search in the local archive")
	maatKnowledgeDetailCmd.Flags().Int("revision", 0, "Specific immutable version; 0 means current")
	maatKnowledgeHistoryCmd.Flags().Int("limit", 20, "Maximum history versions per page")
	maatKnowledgeHistoryCmd.Flags().String("cursor", "", "Continue a stable history page")
	maatKnowledgeResolveCmd.Flags().String("state", "", "Owner conclusion: reviewed, accepted, or rejected")
	maatKnowledgeResolveCmd.Flags().Int("revision", 0, "Exact immutable revision shown during review")
	maatKnowledgeResolveCmd.Flags().String("digest", "", "Exact content digest shown during review")
	maatKnowledgeResolveCmd.Flags().Bool("note-stdin", false, "Read the owner annotation from standard input")
	maatKnowledgeCmd.AddCommand(maatKnowledgeDetailCmd, maatKnowledgeHistoryCmd, maatKnowledgeStatusCmd, maatKnowledgeRebuildIndexCmd, maatKnowledgeResolveCmd, maatKnowledgeReconcileCmd)
	maatKnowledgeRefreshCmd.Flags().String("source", "", "Specific local source adapter to refresh")
	maatKnowledgeRefreshCmd.Flags().String("since", "", "Refresh items since a duration or YYYY-MM-DD date")
	maatKnowledgeRefreshCmd.Flags().String("profile", "", "Chrome profile name or display name")
	maatKnowledgeRefreshCmd.Flags().Bool("all-profiles", false, "Refresh Chrome history from every local profile")
	maatKnowledgeCmd.AddCommand(maatKnowledgeRefreshCmd)
	maatCmd.AddCommand(maatKnowledgeCmd)
}
