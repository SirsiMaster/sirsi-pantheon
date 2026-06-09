package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// continueCmd surfaces the continuation prompt so you never have to hunt for it.
// Pantheon is the memory/resume tool — resuming should be one command, not a
// filesystem search.
var continueCmd = &cobra.Command{
	Use:     "continue",
	Aliases: []string{"resume"},
	Short:   "Show the continuation prompt — resume exactly where you left off",
	Long: `Print docs/CONTINUATION-PROMPT.md — the session-boundary marker (Rule A15)
that holds in-flight state, what shipped, what's held, and the next move.

No more hunting for where you left off:

  sirsi continue            Show the continuation prompt
  sirsi resume              (alias)
  sirsi continue --json     Machine-readable {path, content}`,
	RunE: runContinue,
}

func init() {
	rootCmd.AddCommand(continueCmd)
}

func runContinue(_ *cobra.Command, _ []string) error {
	root, err := router.FindRepoRoot()
	if err != nil {
		// Fall back to the current directory so `sirsi continue` still works in a
		// project that has the doc but no idea-router.
		root, _ = os.Getwd()
	}
	path := filepath.Join(root, "docs", "CONTINUATION-PROMPT.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("no continuation prompt at %s — write your session state there, or run `sirsi thoth sync` to refresh project memory first", path)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{"path": path, "content": string(data)})
	}

	output.Banner()
	output.Header("Continuation Prompt")
	fmt.Println()
	fmt.Print(string(data))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return nil
}
