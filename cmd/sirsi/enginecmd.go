package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/spf13/cobra"
)

var engineVariantUsage = "Backend variant: mlx-raw|mlx-patched|omlx-public|sne-plain|sne-mtp; product names: " +
	engine.RouteDisplayName(engine.KindSNE, engine.VariantSNEPlain) + "=sne-plain, " +
	engine.RouteDisplayName(engine.KindSNE, engine.VariantSNEMTP) + "=sne-mtp"

const defaultEnginePromptMaxTokens = 256

var (
	enginePromptEngine        string
	enginePromptVariant       string
	enginePromptAllowFallback bool
	enginePromptStream        bool
	enginePromptModel         string
	enginePromptSystem        string
	enginePromptText          string
	enginePromptMaxTokens     int
)

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Inspect and use the configured engine route",
}

var engineStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the configured engine selection as JSON",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		controller, err := buildDashboardEngineSelection()
		if err != nil {
			return err
		}
		if controller == nil {
			return errors.New("engine status: no configured engine connectors")
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(controller.Snapshot())
	},
}

var enginePromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Run one prompt through the explicit engine route",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		opts := enginePromptOptions{
			Engine:        enginePromptEngine,
			Variant:       enginePromptVariant,
			AllowFallback: enginePromptAllowFallback,
			Stream:        enginePromptStream,
			Model:         enginePromptModel,
			System:        enginePromptSystem,
			Prompt:        enginePromptText,
			MaxTokens:     enginePromptMaxTokens,
		}
		if err := opts.validate(); err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		builder := func() (*engine.SelectionController, error) {
			return buildDashboardEngineSelectionForPrompt(opts)
		}
		if opts.Stream {
			return executeEnginePromptStream(ctx, opts, cmd.OutOrStdout(), builder, streamEnginePrompt)
		}
		return executeEnginePrompt(ctx, opts, cmd.OutOrStdout(), builder, completeEnginePrompt)
	},
}

type enginePromptOptions struct {
	Engine        string
	Variant       string
	AllowFallback bool
	Stream        bool
	Model         string
	System        string
	Prompt        string
	MaxTokens     int
}

type enginePromptBuilder func() (*engine.SelectionController, error)
type enginePromptExecutor func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error)
type enginePromptStreamExecutor func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.PromptStream, error)

type enginePromptOutput struct {
	Completion engine.Completion `json:"completion"`
	Receipt    engine.Receipt    `json:"receipt"`
}

type enginePromptStreamOutput struct {
	Type    string                `json:"type"`
	Session *engine.Session       `json:"session,omitempty"`
	Route   *engine.RouteDecision `json:"route,omitempty"`
	Event   *engine.Event         `json:"event,omitempty"`
}

func (o enginePromptOptions) validate() error {
	if strings.TrimSpace(o.Prompt) == "" {
		return errors.New("engine prompt: --prompt is required")
	}
	if o.MaxTokens <= 0 {
		return errors.New("engine prompt: --max-tokens must be positive")
	}
	if strings.TrimSpace(o.Engine) == "" {
		return errors.New("engine prompt: --engine is required; choose mlx, omlx, or sne explicitly")
	}
	selectedEngine, err := parseEngineKind(o.Engine)
	if err != nil {
		return err
	}
	if strings.TrimSpace(o.Variant) == "" {
		return errors.New("engine prompt: --variant is required; choose a variant for the selected engine")
	}
	variant, err := engine.ParseVariant(o.Variant)
	if err != nil {
		return fmt.Errorf("engine prompt: --variant: %w", err)
	}
	if err := variant.ValidateForEngine(selectedEngine); err != nil {
		return fmt.Errorf("engine prompt: --variant: %w", err)
	}
	return nil
}

func parseEnginePromptOptions(args []string) (enginePromptOptions, error) {
	opts := enginePromptOptions{MaxTokens: defaultEnginePromptMaxTokens}
	fs := flag.NewFlagSet("sirsi engine prompt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Engine, "engine", "", "Engine: mlx|omlx|sne")
	fs.StringVar(&opts.Variant, "variant", "", engineVariantUsage)
	fs.BoolVar(&opts.AllowFallback, "allow-fallback", false, "Allow explicit fallback if the preferred engine is unavailable")
	fs.BoolVar(&opts.Stream, "stream", false, "Stream newline-delimited JSON session and event records")
	fs.StringVar(&opts.Model, "model", "", "Expected admitted model")
	fs.StringVar(&opts.System, "system", "", "System prompt")
	fs.StringVar(&opts.Prompt, "prompt", "", "User prompt")
	fs.IntVar(&opts.MaxTokens, "max-tokens", defaultEnginePromptMaxTokens, "Positive maximum output tokens")
	if err := fs.Parse(args); err != nil {
		return enginePromptOptions{}, err
	}
	if fs.NArg() != 0 {
		return enginePromptOptions{}, errors.New("engine prompt: positional arguments are not accepted")
	}
	if err := opts.validate(); err != nil {
		return enginePromptOptions{}, err
	}
	return opts, nil
}

func parseEngineKind(value string) (engine.Kind, error) {
	switch engine.Kind(strings.ToLower(strings.TrimSpace(value))) {
	case engine.KindMLX:
		return engine.KindMLX, nil
	case engine.KindOMLX:
		return engine.KindOMLX, nil
	case engine.KindSNE:
		return engine.KindSNE, nil
	default:
		return "", fmt.Errorf("engine prompt: --engine must be one of mlx, omlx, or sne")
	}
}

func executeEnginePrompt(ctx context.Context, opts enginePromptOptions, stdout io.Writer, build enginePromptBuilder, execute enginePromptExecutor) error {
	if err := opts.validate(); err != nil {
		return err
	}
	if build == nil || execute == nil {
		return errors.New("engine prompt: execution seams are required")
	}
	controller, err := selectEnginePromptController(opts, build)
	if err != nil {
		return err
	}
	completion, receipt, err := execute(ctx, controller, engine.PromptRequest{
		Model: opts.Model, System: opts.System, Prompt: opts.Prompt, MaxTokens: opts.MaxTokens,
	})
	if err != nil {
		return err
	}
	if !hasRouteDataBoundary(receipt.Route) {
		return errors.New("engine prompt: completion receipt is missing route boundary provenance")
	}
	if completion.Model != receipt.Identity.ModelID {
		return fmt.Errorf("engine prompt: completion model %q does not match receipt identity %q", completion.Model, receipt.Identity.ModelID)
	}
	session := engine.Session{ID: receipt.SessionID, Identity: receipt.Identity, CreatedAt: receipt.StartedAt}
	if err := receipt.Validate(session); err != nil {
		return fmt.Errorf("engine prompt: completion receipt is invalid: %w", err)
	}
	if err := validatePromptRoutePolicy(controller, *receipt.Route); err != nil {
		return err
	}
	completionSum := sha256.Sum256([]byte(completion.Text))
	if got := hex.EncodeToString(completionSum[:]); receipt.CompletionSHA256 != got {
		return fmt.Errorf("engine prompt: completion text digest does not match its receipt")
	}
	return json.NewEncoder(stdout).Encode(enginePromptOutput{Completion: completion, Receipt: receipt})
}

func hasRouteDataBoundary(route *engine.RouteDecision) bool {
	if route == nil {
		return false
	}
	switch route.DataBoundary {
	case engine.DataBoundaryOnDevice, engine.DataBoundaryRemote, engine.DataBoundaryNotDisclosed:
		return true
	default:
		return false
	}
}

func validatePromptRoutePolicy(controller *engine.SelectionController, route engine.RouteDecision) error {
	if controller == nil {
		return errors.New("engine prompt: selected route controller is missing")
	}
	policy := controller.Policy()
	if route.Requested != policy.Preferred || route.RequestedVariant != policy.PreferredVariant {
		return fmt.Errorf("engine prompt: receipt route request %s/%s differs from selected policy %s/%s", route.Requested, route.RequestedVariant, policy.Preferred, policy.PreferredVariant)
	}
	if route.Fallback && !policy.AllowFallback {
		return errors.New("engine prompt: route reports fallback although this invocation did not allow it")
	}
	return nil
}

func selectEnginePromptController(opts enginePromptOptions, build enginePromptBuilder) (*engine.SelectionController, error) {
	controller, err := build()
	if err != nil {
		return nil, err
	}
	if controller == nil {
		return nil, errors.New("engine prompt: no configured engine connectors")
	}
	policy := controller.Policy()
	policy.Preferred, err = parseEngineKind(opts.Engine)
	if err != nil {
		return nil, err
	}
	policy.PreferredVariant, err = engine.ParseVariant(opts.Variant)
	if err != nil {
		return nil, err
	}
	// Fallback is deliberately false unless this invocation supplies the flag.
	// The environment may configure the dashboard, but it cannot silently widen
	// this one-shot CLI request.
	policy.AllowFallback = opts.AllowFallback
	if _, err := controller.Select(policy); err != nil {
		return nil, err
	}
	return controller, nil
}

func executeEnginePromptStream(ctx context.Context, opts enginePromptOptions, stdout io.Writer, build enginePromptBuilder, execute enginePromptStreamExecutor) error {
	if err := opts.validate(); err != nil {
		return err
	}
	if build == nil || execute == nil {
		return errors.New("engine prompt: stream execution seams are required")
	}
	controller, err := selectEnginePromptController(opts, build)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := execute(ctx, controller, engine.PromptRequest{
		Model: opts.Model, System: opts.System, Prompt: opts.Prompt, MaxTokens: opts.MaxTokens,
	})
	if err != nil {
		return err
	}
	if stream.Events == nil {
		return errors.New("engine prompt: stream returned no event channel")
	}
	if !hasRouteDataBoundary(&stream.Route) {
		return errors.New("engine prompt: stream session is missing route boundary provenance")
	}
	if err := stream.Session.Validate(); err != nil {
		return fmt.Errorf("engine prompt: stream session is invalid: %w", err)
	}
	if err := stream.Route.Validate(stream.Session.Identity); err != nil {
		return fmt.Errorf("engine prompt: stream route is invalid: %w", err)
	}
	if err := validatePromptRoutePolicy(controller, stream.Route); err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	session, route := stream.Session, stream.Route
	if err := encoder.Encode(enginePromptStreamOutput{Type: "session", Session: &session, Route: &route}); err != nil {
		return fmt.Errorf("engine prompt: write stream session: %w", err)
	}
	terminal := false
	var terminalEvent *engine.Event
	var previousSequence uint64
	var forwarded strings.Builder
	for event := range stream.Events {
		if terminal {
			return errors.New("engine prompt: stream emitted an event after its terminal record")
		}
		if err := event.Validate(previousSequence); err != nil {
			return fmt.Errorf("engine prompt: invalid stream event: %w", err)
		}
		if event.SessionID != stream.Session.ID {
			return fmt.Errorf("engine prompt: stream event session %q differs from admitted session %q", event.SessionID, stream.Session.ID)
		}
		if event.Receipt != nil {
			if !hasRouteDataBoundary(event.Receipt.Route) || *event.Receipt.Route != stream.Route {
				return errors.New("engine prompt: stream receipt route differs from admitted session route")
			}
			if err := event.Receipt.Validate(stream.Session); err != nil {
				return fmt.Errorf("engine prompt: invalid stream receipt: %w", err)
			}
		}
		if event.Kind == engine.EventDelta {
			forwarded.WriteString(event.Text)
		}
		if event.Kind == engine.EventCompleted {
			if event.Model != stream.Session.Identity.ModelID {
				return fmt.Errorf("engine prompt: completed stream model %q differs from admitted identity %q", event.Model, stream.Session.Identity.ModelID)
			}
			if event.Text != forwarded.String() {
				return errors.New("engine prompt: completed stream text differs from forwarded deltas")
			}
			completionSum := sha256.Sum256([]byte(event.Text))
			if event.Receipt.CompletionSHA256 != hex.EncodeToString(completionSum[:]) {
				return errors.New("engine prompt: completed stream text digest differs from its receipt")
			}
		}
		if event.Kind == engine.EventError && event.ErrorCode == "cancelled" {
			completionSum := sha256.Sum256([]byte(forwarded.String()))
			if event.Receipt.CompletionSHA256 != hex.EncodeToString(completionSum[:]) {
				return errors.New("engine prompt: cancelled stream digest differs from forwarded text")
			}
		}
		if event.Kind == engine.EventCompleted || event.Kind == engine.EventError {
			terminal = true
			terminalCopy := event
			if event.Receipt != nil {
				receiptCopy := *event.Receipt
				if event.Receipt.Route != nil {
					routeCopy := *event.Receipt.Route
					receiptCopy.Route = &routeCopy
				}
				terminalCopy.Receipt = &receiptCopy
			}
			terminalEvent = &terminalCopy
			previousSequence = event.Sequence
			continue
		}
		if err := encoder.Encode(enginePromptStreamOutput{Type: "event", Event: &event}); err != nil {
			return fmt.Errorf("engine prompt: write stream event: %w", err)
		}
		previousSequence = event.Sequence
	}
	if !terminal || terminalEvent == nil {
		return errors.New("engine prompt: stream closed without a terminal event")
	}
	if err := encoder.Encode(enginePromptStreamOutput{Type: "event", Event: terminalEvent}); err != nil {
		return fmt.Errorf("engine prompt: write stream terminal event: %w", err)
	}
	if terminalEvent.Kind == engine.EventError {
		return fmt.Errorf("engine prompt: stream ended with %s: %s", terminalEvent.ErrorCode, terminalEvent.Error)
	}
	return nil
}

func completeEnginePrompt(ctx context.Context, controller *engine.SelectionController, request engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	return controller.CompletePrompt(ctx, request)
}

func streamEnginePrompt(ctx context.Context, controller *engine.SelectionController, request engine.PromptRequest) (engine.PromptStream, error) {
	return controller.StreamPrompt(ctx, request)
}

func init() {
	enginePromptCmd.Flags().StringVar(&enginePromptEngine, "engine", "", "Engine: mlx|omlx|sne")
	enginePromptCmd.Flags().StringVar(&enginePromptVariant, "variant", "", engineVariantUsage)
	enginePromptCmd.Flags().BoolVar(&enginePromptAllowFallback, "allow-fallback", false, "Allow explicit fallback if the preferred engine is unavailable")
	enginePromptCmd.Flags().BoolVar(&enginePromptStream, "stream", false, "Stream newline-delimited JSON session and event records")
	enginePromptCmd.Flags().StringVar(&enginePromptModel, "model", "", "Expected admitted model")
	enginePromptCmd.Flags().StringVar(&enginePromptSystem, "system", "", "System prompt")
	enginePromptCmd.Flags().StringVar(&enginePromptText, "prompt", "", "User prompt")
	enginePromptCmd.Flags().IntVar(&enginePromptMaxTokens, "max-tokens", defaultEnginePromptMaxTokens, "Positive maximum output tokens")
	engineCmd.AddCommand(engineStatusCmd, enginePromptCmd)
	rootCmd.AddCommand(engineCmd)
}
