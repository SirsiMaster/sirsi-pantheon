package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/output"
	"github.com/SirsiMaster/sirsi-anubis/internal/scales"
)

var (
	scalesPolicyFile string
)

var scalesCmd = &cobra.Command{
	Use:   "scales",
	Short: "⚖️ Enforce infrastructure hygiene policies",
	Long: `⚖️ Scales — The Judgment of Ma'at

Evaluate your infrastructure against defined policies.
Policies define thresholds for waste, ghost apps, and findings.

  anubis scales enforce           Run default policies against current state
  anubis scales enforce -f pol.yaml  Run custom policy file
  anubis scales validate -f pol.yaml Validate a policy file
  anubis scales verdicts          Show last enforcement results

Policies can define:
  - Maximum waste thresholds (e.g., fail if > 20 GB)
  - Ghost app limits (e.g., warn if > 50 ghosts)
  - Finding count limits (e.g., warn if > 100 findings)
  - Notification targets (Slack, Teams, webhook)

To create fleet-wide policies, upgrade to Eye of Horus or Ra.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var scalesEnforceCmd = &cobra.Command{
	Use:   "enforce",
	Short: "Run policy enforcement against current state",
	Run:   runScalesEnforce,
}

var scalesValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a policy YAML file",
	Run:   runScalesValidate,
}

var scalesVerdictsCmd = &cobra.Command{
	Use:   "verdicts",
	Short: "Show enforcement verdicts",
	Run:   runScalesVerdicts,
}

func init() {
	scalesCmd.AddCommand(scalesEnforceCmd)
	scalesCmd.AddCommand(scalesValidateCmd)
	scalesCmd.AddCommand(scalesVerdictsCmd)

	scalesEnforceCmd.Flags().StringVarP(&scalesPolicyFile, "file", "f", "", "Policy YAML file (default: built-in workstation policy)")
	scalesValidateCmd.Flags().StringVarP(&scalesPolicyFile, "file", "f", "", "Policy YAML file to validate")
}

func runScalesEnforce(cmd *cobra.Command, args []string) {
	start := time.Now()

	if !quietMode {
		output.Header("⚖️ Scales — Policy Enforcement")
		fmt.Println()
	}

	// Load policies
	var pf *scales.PolicyFile
	var err error

	if scalesPolicyFile != "" {
		pf, err = scales.LoadPolicyFile(scalesPolicyFile)
		if err != nil {
			output.Error("Failed to load policy file: %v", err)
			os.Exit(1)
		}
		if !quietMode {
			output.Info("Loaded policy file: %s", scalesPolicyFile)
		}
	} else {
		pf = scales.DefaultPolicy()
		if !quietMode {
			output.Info("Using default workstation hygiene policy")
		}
	}

	if !quietMode {
		output.Info("Collecting metrics...")
		fmt.Println()
	}

	// Enforce each policy
	var allResults []*scales.EnforceResult

	for _, policy := range pf.Policies {
		result, err := scales.Enforce(policy)
		if err != nil {
			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]string{"error": err.Error()})
				return
			}
			output.Error("Enforcement failed: %v", err)
			os.Exit(1)
		}
		allResults = append(allResults, result)
	}

	// JSON output
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(allResults)
		return
	}

	// Terminal output
	overallPass := true
	for _, result := range allResults {
		output.Header(fmt.Sprintf("Policy: %s", result.PolicyName))
		fmt.Println()

		for _, v := range result.Verdicts {
			fmt.Printf("  %s\n", scales.FormatVerdict(v))
			if !v.Passed && v.Remediation != "" {
				output.Dim("     Fix: %s", v.Remediation)
			}
		}
		fmt.Println()

		// Summary for this policy
		output.Info("Verdicts: %d passed, %d warnings, %d failures",
			result.Passes, result.Warnings, result.Failures)

		if !result.OverallPass {
			overallPass = false
			output.Error("Policy FAILED: %s", result.PolicyName)
		} else if result.Warnings > 0 {
			output.Warn("Policy passed with warnings: %s", result.PolicyName)
		} else {
			output.Success("Policy passed: %s", result.PolicyName)
		}
		fmt.Println()
	}

	elapsed := time.Since(start)
	output.Dim("  Evaluated in %s", elapsed.Round(time.Millisecond))

	if !overallPass {
		fmt.Println()
		output.Dim("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		output.Dim("𓂀 To enforce policies across 100+ nodes, upgrade to")
		output.Dim("  Eye of Horus (subnet) or Ra (enterprise).")
		output.Dim("  https://github.com/SirsiMaster/sirsi-anubis")
		output.Dim("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		os.Exit(1)
	}
}

func runScalesValidate(cmd *cobra.Command, args []string) {
	if scalesPolicyFile == "" {
		output.Error("Policy file required: anubis scales validate -f policy.yaml")
		os.Exit(1)
	}

	output.Header("⚖️ Scales — Policy Validation")
	fmt.Println()

	errs := scales.ValidatePolicy(scalesPolicyFile)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if len(errs) == 0 {
			_ = enc.Encode(map[string]interface{}{"valid": true, "errors": []string{}})
		} else {
			_ = enc.Encode(map[string]interface{}{"valid": false, "errors": errs})
		}
		return
	}

	if len(errs) == 0 {
		output.Success("Policy file is valid: %s", scalesPolicyFile)
		return
	}

	output.Error("Policy validation failed: %d errors", len(errs))
	fmt.Println()
	for _, e := range errs {
		location := ""
		if e.PolicyName != "" {
			location += fmt.Sprintf(" policy=%q", e.PolicyName)
		}
		if e.RuleID != "" {
			location += fmt.Sprintf(" rule=%q", e.RuleID)
		}
		if e.Field != "" {
			location += fmt.Sprintf(" field=%q", e.Field)
		}
		output.Warn(" %s:%s", e.Message, location)
	}
	os.Exit(1)
}

func runScalesVerdicts(cmd *cobra.Command, args []string) {
	// For now, verdicts runs enforce with the default policy
	// In the future, this will read cached results
	output.Header("⚖️ Scales — Current Verdicts")
	fmt.Println()
	output.Info("Running policy enforcement to generate verdicts...")
	fmt.Println()

	runScalesEnforce(cmd, args)
}
