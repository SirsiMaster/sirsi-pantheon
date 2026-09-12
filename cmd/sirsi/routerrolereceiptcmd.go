package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/rolereceipt"
	"github.com/spf13/cobra"
)

const maxRoleReceiptBytes = 64 << 10

var (
	routerRoleReceiptRole   string
	routerRoleReceiptHost   string
	routerRoleReceiptIssuer string
	routerRoleReceiptKeyID  string
	routerRoleReceiptScope  []string
	routerRoleReceiptNow    string
)

var routerRoleReceiptCmd = &cobra.Command{
	Use:   "role-receipt",
	Short: "Inspect local host-neutral role receipts without contacting a control plane",
}
var routerRoleReceiptValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate one role receipt from standard input",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readRoleReceipt(cmd.InOrStdin())
		if err != nil {
			return err
		}
		constraints, err := roleReceiptConstraints(
			routerRoleReceiptRole,
			routerRoleReceiptHost,
			routerRoleReceiptIssuer,
			routerRoleReceiptKeyID,
			routerRoleReceiptScope,
			routerRoleReceiptNow,
		)
		if err != nil {
			return err
		}
		receipt, err := rolereceipt.Parse(body)
		if err != nil {
			return err
		}
		if err := receipt.ValidateFor(constraints); err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(roleReceiptValidationOutput{Receipt: receipt})
	},
}

type roleReceiptValidationOutput struct {
	Receipt rolereceipt.Receipt
}

func (o roleReceiptValidationOutput) MarshalJSON() ([]byte, error) {
	type output struct {
		Schema                  string                  `json:"schema"`
		ReceiptID               string                  `json:"receipt_id"`
		Role                    rolereceipt.Role        `json:"role"`
		HostProfile             rolereceipt.HostProfile `json:"host_profile"`
		Scope                   []string                `json:"scope"`
		IssuedAt                time.Time               `json:"issued_at"`
		ExpiresAt               time.Time               `json:"expires_at"`
		Issuer                  string                  `json:"issuer"`
		KeyID                   string                  `json:"key_id"`
		PolicyVersion           string                  `json:"policy_version"`
		SignatureAuthentication string                  `json:"signature_authentication"`
	}
	return json.Marshal(output{
		Schema:                  o.Receipt.Schema,
		ReceiptID:               o.Receipt.ReceiptID,
		Role:                    o.Receipt.Role,
		HostProfile:             o.Receipt.HostProfile,
		Scope:                   rolereceipt.CanonicalScope(o.Receipt.Scope),
		IssuedAt:                o.Receipt.IssuedAt,
		ExpiresAt:               o.Receipt.ExpiresAt,
		Issuer:                  o.Receipt.Issuer,
		KeyID:                   o.Receipt.KeyID,
		PolicyVersion:           o.Receipt.PolicyVersion,
		SignatureAuthentication: "not_performed_trust_root_required",
	})
}

func init() {
	routerRoleReceiptValidateCmd.Flags().StringVar(&routerRoleReceiptRole, "required-role", "", "Require this exact role")
	routerRoleReceiptValidateCmd.Flags().StringVar(&routerRoleReceiptHost, "host-id", "", "Require this exact host profile ID")
	routerRoleReceiptValidateCmd.Flags().StringVar(&routerRoleReceiptIssuer, "issuer", "", "Require this exact issuer")
	routerRoleReceiptValidateCmd.Flags().StringVar(&routerRoleReceiptKeyID, "key-id", "", "Require this exact issuer key ID")
	routerRoleReceiptValidateCmd.Flags().StringSliceVar(&routerRoleReceiptScope, "require-scope", nil, "Require an exact scope entry; may be repeated")
	routerRoleReceiptValidateCmd.Flags().StringVar(&routerRoleReceiptNow, "now", "", "RFC3339 validation time; defaults to current UTC time")
	routerRoleReceiptCmd.AddCommand(routerRoleReceiptValidateCmd)
	routerCmd.AddCommand(routerRoleReceiptCmd)
}

func readRoleReceipt(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxRoleReceiptBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read role receipt: %w", err)
	}
	if len(body) > maxRoleReceiptBytes {
		return nil, fmt.Errorf("role receipt exceeds %d-byte limit", maxRoleReceiptBytes)
	}
	return body, nil
}

func roleReceiptConstraints(role, host, issuer, keyID string, scope []string, now string) (rolereceipt.Constraints, error) {
	constraints := rolereceipt.Constraints{
		Role:          rolereceipt.Role(strings.TrimSpace(role)),
		HostID:        strings.TrimSpace(host),
		Issuer:        strings.TrimSpace(issuer),
		KeyID:         strings.TrimSpace(keyID),
		RequiredScope: append([]string(nil), scope...),
	}
	if strings.TrimSpace(now) == "" {
		return constraints, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		return rolereceipt.Constraints{}, fmt.Errorf("parse --now: %w", err)
	}
	constraints.Now = parsed
	return constraints, nil
}
