package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mercury"
	"github.com/spf13/cobra"
)

func newMercuryCmd() *cobra.Command {
	return newMercuryCmdWithRead(func(ctx context.Context) mercury.Read {
		return mercury.NewConfigured().Read(ctx)
	})
}

func newMercuryCmdWithRead(read func(context.Context) mercury.Read) *cobra.Command {
	return &cobra.Command{
		Use:   "mercury",
		Short: "Read Mercury interconnect activity without controlling a run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := read(cmd.Context())
			if JsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			return writeMercuryHuman(cmd, result)
		},
	}
}

func writeMercuryHuman(cmd *cobra.Command, result mercury.Read) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Mercury: %s\n%s\n", result.State, result.Message); err != nil {
		return err
	}
	if result.Headline == nil {
		return nil
	}
	h := result.Headline
	for _, cell := range []struct {
		name  string
		value mercury.Cell
	}{{"Wire", h.Wire}, {"Compressed", h.Compressed}, {"Gain", h.Gain}} {
		value := strings.TrimSpace(cell.value.Number + " " + cell.value.Unit)
		if cell.value.Note != "" {
			value += " · " + cell.value.Note
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", cell.name, value); err != nil {
			return err
		}
	}
	if h.Context != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", h.Context); err != nil {
			return err
		}
	}
	if h.RunStatus != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", h.RunStatus); err != nil {
			return err
		}
	}
	return nil
}

func init() { rootCmd.AddCommand(newMercuryCmd()) }
