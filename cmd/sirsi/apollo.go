package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	apolloModelID         string
	apolloMode            string
	apolloPrecision       string
	apolloMaxOutput       uint32
	apolloPrompt          string
	apolloReadStdin       bool
	apolloMachines        string
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
	Long: `Sends chat text to authenticated, service-owned Apollo preparation, then
submits only the resulting frozen preparation. This command never tokenizes or
starts a local model, uses no SSH or subprocess transport, and refuses a launch
unless the service returns live memory admissions and execution fences for every
rank. Plain and Flash are distinct; the client never falls back between them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := configuredApolloClusterClient()
		if err != nil {
			return err
		}
		messages, err := apolloChatMessages(cmd)
		if err != nil {
			return err
		}
		requestID, err := apolloOpaqueID(apolloRequestID)
		if err != nil {
			return err
		}
		sessionID, err := apolloOpaqueID(apolloSessionID)
		if err != nil {
			return err
		}
		idempotencyKey, err := apolloOpaqueID(apolloIdempotencyKey)
		if err != nil {
			return err
		}
		prepared, err := client.Prepare(cmd.Context(), apollo.PrepareRequest{
			Contract: apollo.ClusterContractV1, RequestID: requestID, SessionID: sessionID,
			ModelID: apolloModelID, Machines: splitApolloMachines(apolloMachines), Mode: apolloMode,
			Precision: apolloPrecision, Messages: messages, MaxOutputTokens: apolloMaxOutput,
		})
		if err != nil {
			return err
		}
		job, err := client.CreateJob(cmd.Context(), prepared, idempotencyKey)
		if err != nil {
			return err
		}
		return streamApolloChat(cmd, client, job)
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
		job, err := client.Status(cmd.Context(), admitted)
		if err != nil {
			return err
		}
		return client.Events(cmd.Context(), job, apolloEventAfter, func(event apollo.Event) error {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(event)
		})
	},
}

func streamApolloChat(cmd *cobra.Command, client *apollo.ClusterClient, job apollo.Job) error {
	completed := false
	failed := ""
	err := client.Events(cmd.Context(), job, 0, func(event apollo.Event) error {
		switch event.Type {
		case "completed":
			completed = true
		case "failed", "cancelled", "orphan_unknown":
			failed = event.Type
			if event.ErrorCode != "" {
				failed += ": " + event.ErrorCode
			}
		}
		if JsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(event)
		}
		if event.Type == "token" {
			_, err := fmt.Fprint(cmd.OutOrStdout(), event.Text)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if failed != "" || !completed {
		if failed == "" {
			failed = "completed event was not observed"
		}
		return fmt.Errorf("Apollo job %s did not complete successfully: %s", job.JobID, failed)
	}
	if !JsonOutput {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "\n[completed; all rank resources released]")
	}
	return err
}

func configuredApolloClusterClient() (*apollo.ClusterClient, error) {
	if strings.TrimSpace(apolloClusterEndpoint) == "" {
		return nil, fmt.Errorf("Apollo cluster endpoint is required; configure the authenticated HTTPS service with --endpoint")
	}
	return nil, fmt.Errorf("Apollo cluster commands are unavailable: enrolled authentication transport and verified endpoint/session binding are not installed")
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

func apolloChatMessages(cmd *cobra.Command) ([]apollo.ChatMessage, error) {
	if apolloReadStdin && strings.TrimSpace(apolloPrompt) != "" {
		return nil, fmt.Errorf("choose either --prompt or --stdin")
	}
	prompt := apolloPrompt
	if apolloReadStdin {
		content, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 32769))
		if err != nil {
			return nil, fmt.Errorf("read Apollo prompt: %w", err)
		}
		if len(content) > 32768 {
			return nil, fmt.Errorf("Apollo prompt exceeds the 32 KiB service limit")
		}
		prompt = string(content)
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("provide chat text with --prompt or --stdin")
	}
	return []apollo.ChatMessage{{Role: "user", Content: prompt}}, nil
}

func apolloOpaqueID(value string) (string, error) {
	if value = strings.TrimSpace(value); value != "" {
		return value, nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate Apollo request identity: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func splitApolloMachines(raw string) []string {
	machines := splitApolloEstates(raw)
	if len(machines) == 0 {
		return []string{"auto"}
	}
	return machines
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
	apolloChatCmd.Flags().StringVar(&apolloModelID, "model", "", "Exact registered model id")
	apolloChatCmd.Flags().StringVar(&apolloMode, "mode", "plain", "Inference mode: plain or flash")
	apolloChatCmd.Flags().StringVar(&apolloPrecision, "precision", "Q8", "Exact admitted precision")
	apolloChatCmd.Flags().Uint32Var(&apolloMaxOutput, "max-output-tokens", 0, "Maximum output token count")
	apolloChatCmd.Flags().StringVar(&apolloPrompt, "prompt", "", "One user chat message")
	apolloChatCmd.Flags().BoolVar(&apolloReadStdin, "stdin", false, "Read one user chat message from standard input")
	apolloChatCmd.Flags().StringVar(&apolloMachines, "machines", "auto", "Comma-separated service machine selection, or auto")
	for _, command := range []*cobra.Command{apolloStatusCmd, apolloCancelCmd, apolloEventsCmd} {
		command.Flags().StringVar(&apolloJobID, "job-id", "", "Authoritative Apollo job id")
		command.Flags().StringVar(&apolloJobReceipt, "job-receipt", "", "Original authenticated admission receipt for the frozen execution")
	}
	apolloEventsCmd.Flags().Uint64Var(&apolloEventAfter, "after", 0, "Last persisted event sequence; replay starts after it")
	apolloCmd.AddCommand(apolloCatalogCmd, apolloPlanCmd, apolloTelemetryCmd, apolloCapabilitiesCmd, apolloChatCmd, apolloStatusCmd, apolloEventsCmd, apolloCancelCmd)
}
