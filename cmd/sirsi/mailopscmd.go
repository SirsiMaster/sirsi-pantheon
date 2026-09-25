package main

// mailopscmd.go — `sirsi mail`: mailbox hygiene (poison-message detection,
// archive-only cleanup, sender census). Governed by 𓆄 Ma'at under the
// Anubis hygiene domain (docs/DEITY_REGISTRY.md). Default DRY-RUN, like
// every mutating Pantheon command. Never deletes, never empties trash,
// never sends mail (internal/mailops.Client has no such methods).
// See docs/design-notes/MAILOPS_DESIGN.md.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mailops"
)

var (
	mailApply       bool
	mailAccountsCfg string
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Mailbox hygiene: poison-message detection, archive-only cleanup, sender census",
}

var mailPoisonCmd = &cobra.Command{
	Use:   "poison <label>",
	Short: "Find (and optionally archive) inbox messages that hang Outlook's IMAP sync (default: dry-run)",
	Long: `Detects inbox messages whose multipart body has a zero-length text/plain
part — the shape that hangs Outlook's IMAP sync loop for ~5 minutes per
occurrence. Default is dry-run (report only). --apply archives the matched
messages (removes the INBOX label). Never deletes, never empties trash.

  sirsi mail poison sirsi              # dry-run: list candidates
  sirsi mail poison sirsi --apply      # archive them, write a signed receipt`,
	Args: cobra.ExactArgs(1),
	RunE: runMailPoison,
}

var mailSendersCmd = &cobra.Command{
	Use:   "senders <label> [N]",
	Short: "Top inbox senders over the last year, flagging List-Unsubscribe",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runMailSenders,
}

var mailAuthCmd = &cobra.Command{
	Use:   "auth <label>",
	Short: "One-time interactive OAuth consent for a mailbox; writes the refresh token",
	Long: `Opens the browser for Google's consent screen and writes the refresh
token to ~/.sirsi/mailops/token-<label>.json (mode 600). Requires a GCP
OAuth Desktop client credentials file (--client-secret, default:
~/.sirsi/mailops/client.json). Register the account afterward in
accounts.yaml (see configs/mailops.yaml).`,
	Args: cobra.ExactArgs(1),
	RunE: runMailAuth,
}

var mailClientSecret string

func init() {
	f := mailPoisonCmd.Flags()
	f.BoolVar(&mailApply, "apply", false, "archive matched messages (default is dry-run)")
	mailCmd.PersistentFlags().StringVar(&mailAccountsCfg, "accounts", "", "path to accounts.yaml (default: ~/.sirsi/mailops/accounts.yaml)")
	mailAuthCmd.Flags().StringVar(&mailClientSecret, "client-secret", "", "path to the GCP OAuth Desktop client.json (default: ~/.sirsi/mailops/client.json)")
	mailCmd.AddCommand(mailPoisonCmd, mailSendersCmd, mailAuthCmd)
	rootCmd.AddCommand(mailCmd)
}

func runMailAuth(cmd *cobra.Command, args []string) error {
	label := args[0]
	secret := mailClientSecret
	if secret == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		secret = home + "/.sirsi/mailops/client.json"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	tokenPath := fmt.Sprintf("%s/.sirsi/mailops/token-%s.json", home, label)
	ctx := context.Background()
	if err := mailops.Authorize(ctx, secret, tokenPath, mailops.RunLocalConsentFlow(ctx)); err != nil {
		return fmt.Errorf("mail auth: %w", err)
	}
	fmt.Printf("ok %s: token written to %s\n", label, tokenPath)
	return nil
}

func mailClientForLabel(label string) (mailops.Client, mailops.Account, error) {
	path := mailAccountsCfg
	if path == "" {
		p, err := mailops.DefaultAccountsPath()
		if err != nil {
			return nil, mailops.Account{}, err
		}
		path = p
	}
	accounts, err := mailops.LoadAccounts(path)
	if err != nil {
		return nil, mailops.Account{}, err
	}
	acct, err := mailops.Find(accounts, label)
	if err != nil {
		return nil, mailops.Account{}, err
	}
	c, err := mailops.NewClient(context.Background(), acct.TokenPath)
	return c, acct, err
}

func runMailPoison(cmd *cobra.Command, args []string) error {
	label := args[0]
	c, _, err := mailClientForLabel(label)
	if err != nil {
		return fmt.Errorf("mail poison: %w", err)
	}
	var log *mailops.ReceiptLog
	if mailApply {
		log, err = mailops.NewReceiptLog(label)
		if err != nil {
			return fmt.Errorf("mail poison: %w", err)
		}
	}
	res, err := mailops.PoisonScan(c, mailApply, log)
	if err != nil {
		return fmt.Errorf("mail poison: %w", err)
	}
	if JsonOutput {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	for _, m := range res.Poisoned {
		fmt.Printf("%-31s | %-40s | %s\n", m.Date, m.From, m.Subject)
	}
	fmt.Printf("%d of %d inbox messages have an empty text part\n", len(res.Poisoned), res.ScannedTotal)
	if !mailApply {
		if len(res.Poisoned) > 0 {
			fmt.Println("(dry-run — nothing archived. --apply to archive.)")
		}
		return nil
	}
	if len(res.Archived) > 0 {
		fmt.Printf("archived %d — receipt: %s\n", len(res.Archived), log.Path())
	}
	return nil
}

func runMailSenders(cmd *cobra.Command, args []string) error {
	label := args[0]
	n := 40
	if len(args) == 2 {
		if _, err := fmt.Sscanf(args[1], "%d", &n); err != nil {
			return fmt.Errorf("mail senders: invalid N %q", args[1])
		}
	}
	c, _, err := mailClientForLabel(label)
	if err != nil {
		return fmt.Errorf("mail senders: %w", err)
	}
	out, err := mailops.SenderCensus(c, n)
	if err != nil {
		return fmt.Errorf("mail senders: %w", err)
	}
	if JsonOutput {
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	for _, s := range out {
		flag := "     "
		if s.ListUnsubscribe {
			flag = "LIST "
		}
		fmt.Printf("%5d %s%s\n", s.Count, flag, s.From)
	}
	return nil
}
