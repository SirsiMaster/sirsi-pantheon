package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/spf13/cobra"
)

const defaultEnginePromptMaxTokens = 256

var (
	enginePromptEngine        string
	enginePromptVariant       string
	enginePromptAllowFallback bool
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
	Short: "Complete one prompt through the explicit engine route",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		opts := enginePromptOptions{
			Engine:        enginePromptEngine,
			Variant:       enginePromptVariant,
			AllowFallback: enginePromptAllowFallback,
			Model:         enginePromptModel,
			System:        enginePromptSystem,
			Prompt:        enginePromptText,
			MaxTokens:     enginePromptMaxTokens,
		}
		if err := opts.validate(); err != nil {
			return err
		}
		return executeEnginePrompt(cmd.Context(), opts, cmd.OutOrStdout(), buildDashboardEngineSelection, completeEnginePrompt)
	},
}

type enginePromptOptions struct {
	Engine        string
	Variant       string
	AllowFallback bool
	Model         string
	System        string
	Prompt        string
	MaxTokens     int
}

type enginePromptBuilder func() (*engine.SelectionController, error)
type enginePromptExecutor func(context.Context, *engine.SelectionController, engine.PromptRequest) (engine.Completion, engine.Receipt, error)

type enginePromptOutput struct {
	Completion engine.Completion `json:"completion"`
	Receipt    engine.Receipt    `json:"receipt"`
}

func (o enginePromptOptions) validate() error {
	if strings.TrimSpace(o.Prompt) == "" {
		return errors.New("engine prompt: --prompt is required")
	}
	if o.MaxTokens <= 0 {
		return errors.New("engine prompt: --max-tokens must be positive")
	}
	if o.Engine != "" {
		if _, err := parseEngineKind(o.Engine); err != nil {
			return err
		}
	}
	if o.Variant != "" {
		if _, err := engine.ParseVariant(o.Variant); err != nil {
			return fmt.Errorf("engine prompt: --variant: %w", err)
		}
	}
	return nil
}

func parseEnginePromptOptions(args []string) (enginePromptOptions, error) {
	opts := enginePromptOptions{MaxTokens: defaultEnginePromptMaxTokens}
	fs := flag.NewFlagSet("sirsi engine prompt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Engine, "engine", "", "Engine: mlx|omlx|sne")
	fs.StringVar(&opts.Variant, "variant", "", "Backend variant: mlx-raw|mlx-patched|omlx-public|sne-plain|sne-mtp")
	fs.BoolVar(&opts.AllowFallback, "allow-fallback", false, "Allow explicit fallback if the preferred engine is unavailable")
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
	controller, err := build()
	if err != nil {
		return err
	}
	if controller == nil {
		return errors.New("engine prompt: no configured engine connectors")
	}
	policy := controller.Policy()
	if opts.Engine != "" {
		policy.Preferred, err = parseEngineKind(opts.Engine)
		if err != nil {
			return err
		}
	} else {
		policy.Preferred = controller.Snapshot().Preferred
	}
	if opts.Variant != "" {
		policy.PreferredVariant, err = engine.ParseVariant(opts.Variant)
		if err != nil {
			return err
		}
	}
	// Fallback is deliberately false unless this invocation supplies the flag.
	// The environment may configure the dashboard, but it cannot silently widen
	// this one-shot CLI request.
	policy.AllowFallback = opts.AllowFallback
	if _, err := controller.Select(policy); err != nil {
		return err
	}
	completion, receipt, err := execute(ctx, controller, engine.PromptRequest{
		Model: opts.Model, System: opts.System, Prompt: opts.Prompt, MaxTokens: opts.MaxTokens,
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(enginePromptOutput{Completion: completion, Receipt: receipt})
}

func completeEnginePrompt(ctx context.Context, controller *engine.SelectionController, request engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	return controller.CompletePrompt(ctx, request)
}

func init() {
	enginePromptCmd.Flags().StringVar(&enginePromptEngine, "engine", "", "Engine: mlx|omlx|sne")
	enginePromptCmd.Flags().StringVar(&enginePromptVariant, "variant", "", "Backend variant: mlx-raw|mlx-patched|omlx-public|sne-plain|sne-mtp")
	enginePromptCmd.Flags().BoolVar(&enginePromptAllowFallback, "allow-fallback", false, "Allow explicit fallback if the preferred engine is unavailable")
	enginePromptCmd.Flags().StringVar(&enginePromptModel, "model", "", "Expected admitted model")
	enginePromptCmd.Flags().StringVar(&enginePromptSystem, "system", "", "System prompt")
	enginePromptCmd.Flags().StringVar(&enginePromptText, "prompt", "", "User prompt")
	enginePromptCmd.Flags().IntVar(&enginePromptMaxTokens, "max-tokens", defaultEnginePromptMaxTokens, "Positive maximum output tokens")
	engineCmd.AddCommand(engineStatusCmd, enginePromptCmd)
	rootCmd.AddCommand(engineCmd)
}
