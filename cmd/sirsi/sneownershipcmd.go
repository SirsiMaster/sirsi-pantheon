package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const canonicalSNESupervisorLabel = "ai.sirsi.sne.supervisor"

type sneOwnershipRecord struct {
	Label      string `json:"label"`
	Plist      string `json:"plist"`
	Executable string `json:"executable,omitempty"`
	Canonical  bool   `json:"canonical"`
	Legacy     bool   `json:"legacy"`
	Loaded     bool   `json:"loaded"`
}

type sneOwnershipReport struct {
	Schema         string               `json:"schema"`
	State          string               `json:"state"`
	CanonicalCount int                  `json:"canonical_count"`
	LegacyCount    int                  `json:"legacy_count"`
	LoadedCount    int                  `json:"loaded_count"`
	Records        []sneOwnershipRecord `json:"records"`
	Recovery       string               `json:"recovery,omitempty"`
}

type sneOwnershipRepairReceipt struct {
	Schema      string            `json:"schema"`
	State       string            `json:"state"`
	CreatedAt   time.Time         `json:"created_at"`
	ReceiptDir  string            `json:"receipt_dir"`
	Canonical   string            `json:"canonical_label"`
	Retired     []string          `json:"retired_labels"`
	PlistSHA256 map[string]string `json:"plist_sha256"`
}

var (
	sneOwnershipHome    = os.UserHomeDir
	sneOwnershipExtract = func(path, key string) (string, error) {
		output, err := exec.Command("/usr/bin/plutil", "-extract", key, "raw", "-o", "-", path).Output()
		return strings.TrimSpace(string(output)), err
	}
	sneOwnershipLoaded = func(label string) bool {
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
		return exec.Command("/bin/launchctl", "print", target).Run() == nil
	}
	sneOwnershipLaunchctl = func(args ...string) error {
		return exec.Command("/bin/launchctl", args...).Run()
	}
	sneOwnershipNow = func() time.Time { return time.Now().UTC() }
)

func isSNEOwnershipLabel(label string) (canonical, legacy bool) {
	return label == canonicalSNESupervisorLabel, strings.HasPrefix(label, "ai.sirsi.pantheon-sne-")
}

func inspectSNEOwnership() (sneOwnershipReport, error) {
	report := sneOwnershipReport{Schema: "pantheon.sne-ownership.v1", State: "not-installed", Records: []sneOwnershipRecord{}}
	home, err := sneOwnershipHome()
	if err != nil {
		return report, fmt.Errorf("resolve user home: %w", err)
	}
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		if os.IsNotExist(err) {
			report.Recovery = "Install and start SNE through Pantheon."
			return report, nil
		}
		return report, fmt.Errorf("read LaunchAgents: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".plist" {
			continue
		}
		plist := filepath.Join(agentDir, entry.Name())
		label, extractErr := sneOwnershipExtract(plist, "Label")
		if extractErr != nil {
			continue
		}
		canonical, legacy := isSNEOwnershipLabel(label)
		if !canonical && !legacy {
			continue
		}
		executable, _ := sneOwnershipExtract(plist, "ProgramArguments.0")
		record := sneOwnershipRecord{
			Label: label, Plist: plist, Executable: executable,
			Canonical: canonical, Legacy: legacy, Loaded: sneOwnershipLoaded(label),
		}
		report.Records = append(report.Records, record)
		if canonical {
			report.CanonicalCount++
		}
		if legacy {
			report.LegacyCount++
		}
		if record.Loaded {
			report.LoadedCount++
		}
	}
	sort.Slice(report.Records, func(i, j int) bool { return report.Records[i].Label < report.Records[j].Label })
	switch {
	case report.CanonicalCount == 1 && report.LegacyCount == 0:
		report.State = "canonical"
	case report.CanonicalCount == 0 && report.LegacyCount == 0:
		report.State = "not-installed"
		report.Recovery = "Install and start SNE through Pantheon."
	default:
		report.State = "ownership-drift"
		report.Recovery = "Use Pantheon's transactional SNE repair to retain the canonical supervisor and retire legacy launch agents before starting a model."
	}
	return report, nil
}

func writeOwnershipReceipt(path string, receipt sneOwnershipRepairReceipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func repairLegacySNEOwnership() (sneOwnershipRepairReceipt, error) {
	report, err := inspectSNEOwnership()
	if err != nil {
		return sneOwnershipRepairReceipt{}, err
	}
	if report.CanonicalCount != 1 {
		return sneOwnershipRepairReceipt{}, fmt.Errorf("repair requires exactly one canonical SNE supervisor; found %d", report.CanonicalCount)
	}
	legacy := make([]sneOwnershipRecord, 0, report.LegacyCount)
	for _, record := range report.Records {
		if record.Legacy {
			legacy = append(legacy, record)
		}
	}
	if len(legacy) == 0 {
		return sneOwnershipRepairReceipt{}, fmt.Errorf("no legacy SNE ownership drift was found")
	}

	home, err := sneOwnershipHome()
	if err != nil {
		return sneOwnershipRepairReceipt{}, fmt.Errorf("resolve user home: %w", err)
	}
	now := sneOwnershipNow()
	receiptDir := filepath.Join(home, "Library", "Application Support", "Sirsi", "Pantheon", "recovery-receipts", now.Format("20060102T150405Z")+"-sne-ownership-repair")
	if err := os.MkdirAll(receiptDir, 0o700); err != nil {
		return sneOwnershipRepairReceipt{}, fmt.Errorf("create recovery receipt: %w", err)
	}
	receipt := sneOwnershipRepairReceipt{
		Schema: "pantheon.sne-ownership-repair.v1", State: "staged", CreatedAt: now,
		ReceiptDir: receiptDir, Canonical: canonicalSNESupervisorLabel,
		Retired: []string{}, PlistSHA256: map[string]string{},
	}

	for _, record := range legacy {
		info, statErr := os.Lstat(record.Plist)
		if statErr != nil || !info.Mode().IsRegular() {
			return receipt, fmt.Errorf("legacy plist is missing or unsafe: %s", record.Plist)
		}
		data, readErr := os.ReadFile(record.Plist)
		if readErr != nil {
			return receipt, fmt.Errorf("read legacy plist %s: %w", record.Label, readErr)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(data))
		receipt.PlistSHA256[record.Label] = digest
		backup := filepath.Join(receiptDir, record.Label+".plist.backup")
		if writeErr := os.WriteFile(backup, data, 0o600); writeErr != nil {
			return receipt, fmt.Errorf("stage legacy plist %s: %w", record.Label, writeErr)
		}
	}
	if err := writeOwnershipReceipt(filepath.Join(receiptDir, "receipt.staged.json"), receipt); err != nil {
		return receipt, fmt.Errorf("write staged receipt: %w", err)
	}

	domain := fmt.Sprintf("gui/%d", os.Getuid())
	moved := make([]sneOwnershipRecord, 0, len(legacy))
	rollback := func() {
		for index := len(moved) - 1; index >= 0; index-- {
			record := moved[index]
			retired := filepath.Join(receiptDir, record.Label+".plist.retired")
			_ = os.Rename(retired, record.Plist)
			_ = sneOwnershipLaunchctl("enable", domain+"/"+record.Label)
			_ = sneOwnershipLaunchctl("bootstrap", domain, record.Plist)
		}
	}
	for _, record := range legacy {
		_ = sneOwnershipLaunchctl("bootout", domain+"/"+record.Label)
		if err := sneOwnershipLaunchctl("disable", domain+"/"+record.Label); err != nil {
			_ = sneOwnershipLaunchctl("enable", domain+"/"+record.Label)
			_ = sneOwnershipLaunchctl("bootstrap", domain, record.Plist)
			rollback()
			return receipt, fmt.Errorf("disable legacy SNE owner %s: %w", record.Label, err)
		}
		retired := filepath.Join(receiptDir, record.Label+".plist.retired")
		if err := os.Rename(record.Plist, retired); err != nil {
			_ = sneOwnershipLaunchctl("enable", domain+"/"+record.Label)
			rollback()
			return receipt, fmt.Errorf("retire legacy SNE owner %s: %w", record.Label, err)
		}
		moved = append(moved, record)
		receipt.Retired = append(receipt.Retired, record.Label)
	}

	after, err := inspectSNEOwnership()
	if err != nil || after.State != "canonical" || after.CanonicalCount != 1 || after.LegacyCount != 0 {
		rollback()
		return receipt, fmt.Errorf("post-repair ownership verification failed")
	}
	receipt.State = "accepted"
	if err := writeOwnershipReceipt(filepath.Join(receiptDir, "receipt.accepted.json"), receipt); err != nil {
		rollback()
		return receipt, fmt.Errorf("write accepted receipt: %w", err)
	}
	return receipt, nil
}

func newSNEOwnershipCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "ownership",
		Short: "Audit canonical and legacy SNE launchd ownership",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := inspectSNEOwnership()
			if err != nil {
				return err
			}
			if JsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "SNE ownership: %s\n", report.State)
			for _, record := range report.Records {
				kind := "legacy"
				if record.Canonical {
					kind = "canonical"
				}
				loaded := "stopped"
				if record.Loaded {
					loaded = "loaded"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-9s %-7s %s -> %s\n", kind, loaded, record.Label, record.Executable)
			}
			if report.Recovery != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Recovery:", report.Recovery)
			}
			if report.State == "ownership-drift" {
				return fmt.Errorf("SNE launchd ownership drift detected")
			}
			return nil
		},
	}
	command.AddCommand(newSNEOwnershipRepairCommand())
	return command
}

func newSNEOwnershipRepairCommand() *cobra.Command {
	var confirmed bool
	command := &cobra.Command{
		Use:   "repair",
		Short: "Transactionally retire legacy SNE launchd ownership",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirmed {
				return fmt.Errorf("ownership repair not confirmed: inspect `sirsi sne ownership`, then pass --confirm")
			}
			receipt, err := repairLegacySNEOwnership()
			if err != nil {
				return err
			}
			if JsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(receipt)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "SNE ownership repair: %s\nReceipt: %s\nRetired: %s\n", receipt.State, receipt.ReceiptDir, strings.Join(receipt.Retired, ", "))
			return nil
		},
	}
	command.Flags().BoolVar(&confirmed, "confirm", false, "confirm retirement of backed-up legacy SNE launch agents")
	return command
}

func init() {
	sneCmd.AddCommand(newSNEOwnershipCommand())
}
