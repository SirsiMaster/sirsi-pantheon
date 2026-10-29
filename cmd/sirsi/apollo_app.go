package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/spf13/cobra"
)

type apolloAppDependencies struct {
	home func() (string, error)
	chat func(context.Context, string, apollo.InstalledAppRequest) (apollo.InstalledAppResult, error)
}

var apolloAppMu sync.RWMutex
var apolloAppDeps = apolloAppDependencies{home: os.UserHomeDir, chat: apollo.ChatInstalledApp}

func getApolloAppDependencies() apolloAppDependencies {
	apolloAppMu.RLock()
	defer apolloAppMu.RUnlock()
	return apolloAppDeps
}

func setApolloAppDependencies(deps apolloAppDependencies) {
	apolloAppMu.Lock()
	defer apolloAppMu.Unlock()
	apolloAppDeps = deps
}

func runInstalledApolloAppChat(cmd *cobra.Command, messages []apollo.ChatMessage) error {
	if !cmd.Flags().Changed("precision") || !cmd.Flags().Changed("mode") {
		return fmt.Errorf("installed Qwen requires explicit --precision %s and --mode plain|flash|mlx-certified", apollo.InstalledAppPrecision)
	}
	// Native conversation IDs do not apply to the app's OpenAI text request.
	// Refuse rather than silently imply authenticated native-session binding.
	if apolloRequestID != "" || apolloSessionID != "" {
		return fmt.Errorf("native --request-id/--session-id are not supported by the installed Apollo app text route")
	}
	deps := getApolloAppDependencies()
	home, err := deps.home()
	if err != nil {
		return fmt.Errorf("locate installed Apollo app: %w", err)
	}
	result, err := deps.chat(cmd.Context(), home, apollo.InstalledAppRequest{
		Model: apolloModelID, Precision: apolloPrecision, Mode: apolloMode,
		Messages: messages, MaxOutputTokens: apolloMaxOutput,
	})
	if err != nil {
		return err
	}
	if JsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Text)
	return err
}
