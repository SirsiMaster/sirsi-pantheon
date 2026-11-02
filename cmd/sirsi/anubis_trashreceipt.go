package main

import (
	"encoding/json"
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/trashreceipt"
	"github.com/spf13/cobra"
)

func newTrashReceiptCommand(apply bool) *cobra.Command {
	name := "preview"
	if apply {
		name = "apply"
	}
	c := &cobra.Command{Use: name, Args: cobra.NoArgs, Short: "Review-bound typed Empty Trash " + name}
	if apply {
		c.Flags().Bool("confirm", false, "confirm permanent deletion of exactly the stdin review")
	}
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		var result trashreceipt.Report
		if apply {
			confirmed, _ := cmd.Flags().GetBool("confirm")
			if !confirmed {
				result = trashreceipt.Failure(fmt.Errorf("--confirm is required"))
			} else {
				m, err := trashreceipt.Decode(cmd.InOrStdin())
				if err != nil {
					result = trashreceipt.Failure(err)
				} else {
					result = trashreceipt.Apply(m, true)
				}
			}
		} else {
			result = trashreceipt.Preview()
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
			return err
		}
		if (result.Status != "ready" && result.Status != "empty" && result.Status != "completed") || len(result.Errors) > 0 {
			return fmt.Errorf("Empty Trash %s: %s", name, result.Status)
		}
		return nil
	}
	return c
}

func init() {
	anubisEmptyTrashCmd.AddCommand(newTrashReceiptCommand(false), newTrashReceiptCommand(true), newTrashBrowseCommand(), newTrashSelectionCommand(), newTrashHistoryCommand())
}

func newTrashHistoryCommand() *cobra.Command {
	c := &cobra.Command{Use: "history", Args: cobra.NoArgs, Short: "Read bounded durable Empty Trash history"}
	c.Flags().Int("limit", 20, "maximum history entries in this page (capped at 20)")
	c.Flags().String("cursor", "", "continue a bounded history page")
	c.Flags().String("operation-id", "", "show the exact reviewed inventory and recorded result")
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		operationID, _ := cmd.Flags().GetString("operation-id")
		cursor, _ := cmd.Flags().GetString("cursor")
		limit, _ := cmd.Flags().GetInt("limit")
		if operationID != "" && (cmd.Flags().Changed("cursor") || cmd.Flags().Changed("limit")) {
			return fmt.Errorf("--operation-id cannot be combined with --cursor or --limit")
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		if operationID != "" {
			return enc.Encode(trashreceipt.ReadHistoryDetail(operationID))
		}
		return enc.Encode(trashreceipt.ReadHistoryPage(limit, cursor))
	}
	return c
}

func newTrashBrowseCommand() *cobra.Command {
	return &cobra.Command{Use: "browse", Hidden: true, Args: cobra.NoArgs, Short: "Unsupported stateless browse; use the native retained read-only session", RunE: func(cmd *cobra.Command, _ []string) error {
		q, err := trashreceipt.DecodeBrowse(cmd.InOrStdin())
		var page trashreceipt.Page
		if err != nil {
			page = trashreceipt.BrowseFailure(err)
		} else {
			page = trashreceipt.Browse(q)
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(page); err != nil {
			return err
		}
		if page.Status != "page" {
			return fmt.Errorf("Trash browse failed")
		}
		return nil
	}}
}

func newTrashSelectionCommand() *cobra.Command {
	return &cobra.Command{Use: "select", Args: cobra.NoArgs, Short: "Read-only full review of a bounded selected Trash batch", RunE: func(cmd *cobra.Command, _ []string) error {
		q, err := trashreceipt.DecodeSelection(cmd.InOrStdin())
		var result trashreceipt.Report
		if err != nil {
			result = trashreceipt.Failure(err)
		} else {
			result = trashreceipt.Select(q)
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
			return err
		}
		if result.Status != "ready" {
			return fmt.Errorf("Choose a smaller batch or open a folder to select descendants")
		}
		return nil
	}}
}
