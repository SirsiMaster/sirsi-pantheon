package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knownfail"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	appversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

var (
	kfSignature, kfCause, kfTitle     string
	kfFixedIn, kfText, kfRef, kfGuard string
)

var maatKnownFailuresCmd = &cobra.Command{
	Use:   "known-failures",
	Short: "Recurring failures: register the problem, record the fix and its regression guard, recognize it next time",
	Long: `The closed loop for failures that keep coming back. A problem is registered once
(signature + cause); the fix is recorded with the release it shipped in and a regression
test that must exist; from then on the router recognizes the failure by its signature and
says what the fix is instead of waiting for someone to diagnose it again. The catalog is a
Stack Lab component of the Ra/Horus fabric recipe and changes go through a PR.`,
}

var maatKnownFailuresListCmd = &cobra.Command{
	Use: "list", Short: "List registered failures", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := knownfail.Load()
		if err != nil {
			return err
		}
		for _, e := range c.Entries {
			fmt.Printf("%-34s %-8s fixed_in=%-8s guard=%s\n", e.ID, e.Status, e.Fix.FixedIn, e.Guard.Ref)
		}
		return nil
	},
}

var maatKnownFailuresMatchCmd = &cobra.Command{
	Use: "match <failure text>", Short: "Recognize a failure message", Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		text := ""
		for _, a := range args {
			text += a + " "
		}
		ms := knownfail.Match(text)
		if len(ms) == 0 {
			fmt.Println("no known failure matches; if it is new and recurring, register it: sirsi maat known-failures register")
			return nil
		}
		for _, e := range ms {
			fmt.Println(e.Summary(appversion.Version))
		}
		return nil
	},
}

func kfCatalogPath() (repo, path string, err error) {
	repo, err = router.FindRepoRoot()
	if err != nil {
		return "", "", err
	}
	return repo, filepath.Join(repo, "internal", "maat", "knownfail", "catalog.json"), nil
}

var maatKnownFailuresRegisterCmd = &cobra.Command{
	Use: "register <id>", Short: "Register a new recurring problem (open, no fix claimed)", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, path, err := kfCatalogPath()
		if err != nil {
			return err
		}
		if err := knownfail.Register(path, knownfail.Entry{ID: args[0], Title: kfTitle, Signature: kfSignature, Cause: kfCause}); err != nil {
			return err
		}
		fmt.Printf("registered %q as open in %s: commit it through a PR; resolve it with `known-failures resolve` once the fix and its regression test exist\n", args[0], path)
		return nil
	},
}

var maatKnownFailuresResolveCmd = &cobra.Command{
	Use: "resolve <id>", Short: "Record the fix; refused unless the regression guard is a real test", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, path, err := kfCatalogPath()
		if err != nil {
			return err
		}
		if err := knownfail.Resolve(path, repo, args[0], knownfail.Fix{Text: kfText, FixedIn: kfFixedIn, Ref: kfRef}, kfGuard); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "resolved %q (fixed in %s, guard %s)\n", args[0], kfFixedIn, kfGuard)
		return nil
	},
}

func init() {
	maatKnownFailuresRegisterCmd.Flags().StringVar(&kfSignature, "signature", "", "case-insensitive regexp over the failure text (required)")
	maatKnownFailuresRegisterCmd.Flags().StringVar(&kfCause, "cause", "", "what causes it (required)")
	maatKnownFailuresRegisterCmd.Flags().StringVar(&kfTitle, "title", "", "short title")
	maatKnownFailuresResolveCmd.Flags().StringVar(&kfFixedIn, "fixed-in", "", "first release containing the fix, e.g. 0.24.69")
	maatKnownFailuresResolveCmd.Flags().StringVar(&kfText, "text", "", "what an operator does / what the fix is")
	maatKnownFailuresResolveCmd.Flags().StringVar(&kfRef, "ref", "", "PR or commit")
	maatKnownFailuresResolveCmd.Flags().StringVar(&kfGuard, "guard", "", "name of the regression test (must exist)")
	maatKnownFailuresCmd.AddCommand(maatKnownFailuresListCmd, maatKnownFailuresMatchCmd, maatKnownFailuresRegisterCmd, maatKnownFailuresResolveCmd)
	maatCmd.AddCommand(maatKnownFailuresCmd)
}
