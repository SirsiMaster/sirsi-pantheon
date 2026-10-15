package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/spf13/cobra"
)

var (
	apolloEngine  string
	apolloMachine string
	apolloCores   int
	apolloMemory  int64
	apolloSwap    int64
	apolloEstates string

	// Cluster commands consume the negotiated Apollo v1 service contract. The
	// endpoint must be HTTPS; credentials live in the installed transport, not
	// in flags, config output, or a request body.
	apolloClusterEndpoint string
	apolloRequestID       string
	apolloIdempotencyKey  string
	apolloSessionID       string
	apolloPlanSHA256      string
	apolloModelID         string
	apolloCheckpointSHA   string
	apolloTokenizerSHA    string
	apolloTemplateSHA     string
	apolloMode            string
	apolloPrecision       string
	apolloInputTokens     string
	apolloMaxOutput       uint32
	apolloJobID           string
	apolloEventAfter      uint64
	apolloJobReceipt      string
)

var apolloCmd = &cobra.Command{
	Use:   "apollo",
	Short: "Plan Apollo inference against a selected measured machine",
	Long: `Apollo is Pantheon's inference planning surface. It lists configured SNE
routes and detected chip estates for each typed machine-capacity receipt, then
validates a resource plan without starting a model or reserving device memory.
SNE remains the execution authority.`,
}

var apolloCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Show selectable Apollo machines, engines, and chip estates",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate home directory: %w", err)
		}
		catalog, err := apollo.Collect(home)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(catalog)
	},
}

var apolloPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Validate an Apollo run plan without starting inference",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate home directory: %w", err)
		}
		catalog, err := apollo.Collect(home)
		if err != nil {
			return err
		}
		estates := splitApolloEstates(apolloEstates)
		plan, err := apollo.BuildPlanForMachine(catalog, apolloMachine, apolloEngine, apolloCores, apolloMemory*1024*1024*1024, apolloSwap*1024*1024*1024, estates)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(plan)
	},
}

var apolloTelemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Read the latest SNE-owned Apollo session telemetry",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate home directory: %w", err)
		}
		read, err := apollo.ReadTelemetry(home)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(read)
	},
}

var apolloCapabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Read authenticated Apollo cluster capabilities",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := configuredApolloClusterClient()
		if err != nil {
			return err
		}
		capabilities, err := client.Capabilities(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(capabilities)
	},
}

var apolloChatCmd = &cobra.Command{
	Use:     "chat",
	Aliases: []string{"admit"},
	Short:   "Request authenticated, all-rank Apollo cluster inference",
	Long: `Submits a frozen, pre-tokenized Apollo job to the authenticated cluster
service. This command never starts a local model, uses no SSH or subprocess
transport, and refuses a launch unless the service returns live memory admissions
and execution fences for every rank. Plain and MTP modes remain distinct; the
client never falls back between them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := configuredApolloClusterClient()
		if err != nil {
			return err
		}
		tokens, err := parseApolloTokenIDs(apolloInputTokens)
		if err != nil {
			return err
		}
		job, err := client.CreateJob(cmd.Context(), apollo.JobRequest{
			Contract:         apollo.ClusterContractV1,
			RequestID:        apolloRequestID,
			IdempotencyKey:   apolloIdempotencyKey,
			SessionID:        apolloSessionID,
			PlanSHA256:       apolloPlanSHA256,
			ModelID:          apolloModelID,
			CheckpointSHA256: apolloCheckpointSHA,
			TokenizerSHA256:  apolloTokenizerSHA,
			TemplateSHA256:   apolloTemplateSHA,
			Mode:             apolloMode,
			Precision:        apolloPrecision,
			InputTokens:      tokens,
			MaxOutputTokens:  apolloMaxOutput,
		}, nil) // Disabled until the authenticated plan supplies its rank inventory.
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(job)
	},
}

var apolloStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Read the authenticated state of an Apollo cluster job",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := configuredApolloClusterClient()
		if err != nil {
			return err
		}
		admitted, err := readApolloJobReceipt(apolloJobReceipt, apolloJobID)
		if err != nil {
			return err
		}
		job, err := client.Status(cmd.Context(), admitted)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(job)
	},
}

var apolloCancelCmd = &cobra.Command{
	Use:   "cancel",
	Short: "Request bounded cancellation and rank release for an Apollo job",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := configuredApolloClusterClient()
		if err != nil {
			return err
		}
		admitted, err := readApolloJobReceipt(apolloJobReceipt, apolloJobID)
		if err != nil {
			return err
		}
		job, err := client.Cancel(cmd.Context(), admitted)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(job)
	},
}

var apolloEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Replay ordered Apollo cluster job events",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := configuredApolloClusterClient()
		if err != nil {
			return err
		}
		admitted, err := readApolloJobReceipt(apolloJobReceipt, apolloJobID)
		if err != nil {
			return err
		}
		return client.Events(cmd.Context(), admitted, apolloEventAfter, func(event apollo.Event) error {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(event)
		})
	},
}

func configuredApolloClusterClient() (*apollo.ClusterClient, error) {
	if strings.TrimSpace(apolloClusterEndpoint) == "" {
		return nil, fmt.Errorf("Apollo cluster endpoint is required; configure the authenticated HTTPS service with --endpoint")
	}
	return nil, fmt.Errorf("Apollo cluster commands are unavailable: enrolled authentication transport and authenticated frozen plan integration are not installed")
}

func readApolloJobReceipt(path, jobID string) (apollo.Job, error) {
	receipt, err := os.ReadFile(path)
	if err != nil {
		return apollo.Job{}, fmt.Errorf("read frozen Apollo job receipt: %w", err)
	}
	var admitted apollo.Job
	if err := json.Unmarshal(receipt, &admitted); err != nil {
		return apollo.Job{}, fmt.Errorf("decode frozen Apollo job receipt: %w", err)
	}
	if admitted.JobID != jobID {
		return apollo.Job{}, fmt.Errorf("Apollo job receipt differs from requested job")
	}
	return admitted, nil
}

func parseApolloTokenIDs(raw string) ([]uint32, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("input tokens are required")
	}
	tokens := make([]uint32, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("input tokens contain an empty value")
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("input token %q is not an unsigned 32-bit value", value)
		}
		tokens = append(tokens, uint32(parsed))
	}
	return tokens, nil
}

func splitApolloEstates(raw string) []string {
	var out []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func init() {
	apolloPlanCmd.Flags().StringVar(&apolloEngine, "engine", "", "Configured Apollo engine id")
	apolloPlanCmd.Flags().StringVar(&apolloMachine, "machine", "this-mac", "Machine id from the typed Apollo capacity catalog")
	apolloPlanCmd.Flags().IntVar(&apolloCores, "cores", 0, "Requested logical CPU cores")
	apolloPlanCmd.Flags().Int64Var(&apolloMemory, "memory-gib", 0, "Requested unified-memory envelope in GiB")
	apolloPlanCmd.Flags().Int64Var(&apolloSwap, "swap-gib", 0, "Requested swap ceiling in GiB (admission checks live pressure separately)")
	apolloPlanCmd.Flags().StringVar(&apolloEstates, "estates", "", "Comma-separated detected chip estate ids")

	for _, command := range []*cobra.Command{apolloCapabilitiesCmd, apolloChatCmd, apolloStatusCmd, apolloCancelCmd, apolloEventsCmd} {
		command.Flags().StringVar(&apolloClusterEndpoint, "endpoint", "", "Authenticated Apollo HTTPS service endpoint")
	}
	apolloChatCmd.Flags().StringVar(&apolloRequestID, "request-id", "", "Opaque request id bound to the authenticated session")
	apolloChatCmd.Flags().StringVar(&apolloIdempotencyKey, "idempotency-key", "", "Opaque per-session idempotency key")
	apolloChatCmd.Flags().StringVar(&apolloSessionID, "session-id", "", "Opaque authenticated conversation session id")
	apolloChatCmd.Flags().StringVar(&apolloPlanSHA256, "plan-sha256", "", "Frozen signed placement-plan SHA-256")
	apolloChatCmd.Flags().StringVar(&apolloModelID, "model", "", "Exact registered model id")
	apolloChatCmd.Flags().StringVar(&apolloCheckpointSHA, "checkpoint-sha256", "", "Exact checkpoint manifest SHA-256")
	apolloChatCmd.Flags().StringVar(&apolloTokenizerSHA, "tokenizer-sha256", "", "Exact tokenizer SHA-256")
	apolloChatCmd.Flags().StringVar(&apolloTemplateSHA, "template-sha256", "", "Exact chat template SHA-256")
	apolloChatCmd.Flags().StringVar(&apolloMode, "mode", "plain", "Inference mode: plain or mtp")
	apolloChatCmd.Flags().StringVar(&apolloPrecision, "precision", "Q8", "Exact admitted precision")
	apolloChatCmd.Flags().StringVar(&apolloInputTokens, "input-tokens", "", "Comma-separated pre-tokenized unsigned token ids")
	apolloChatCmd.Flags().Uint32Var(&apolloMaxOutput, "max-output-tokens", 0, "Maximum output token count")
	for _, command := range []*cobra.Command{apolloStatusCmd, apolloCancelCmd, apolloEventsCmd} {
		command.Flags().StringVar(&apolloJobID, "job-id", "", "Authoritative Apollo job id")
		command.Flags().StringVar(&apolloJobReceipt, "job-receipt", "", "Frozen authenticated admission receipt; never replace with a fresh status on reconnect")
	}
	apolloEventsCmd.Flags().Uint64Var(&apolloEventAfter, "after", 0, "Last persisted event sequence; replay starts after it")
	apolloCmd.AddCommand(apolloCatalogCmd, apolloPlanCmd, apolloTelemetryCmd, apolloCapabilitiesCmd, apolloChatCmd, apolloStatusCmd, apolloEventsCmd, apolloCancelCmd)
}
