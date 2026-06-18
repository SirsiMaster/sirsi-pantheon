package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// `sirsi gemma` is the human-facing way to talk to Gemma — the local MLX model
// that runs on this Mac at zero API tokens. Until now Gemma was only reachable by
// agents (the router `to: gemma` inbox + the sirsi-gemma MCP tools); a person had
// no one-line way in. This is a thin, synchronous wrapper over the SAME
// `mlx_lm.generate` path the gemma-worker uses, so the CLI and the daemon speak to
// the identical model. Single-shot text reasoning: no tools, no binding verdicts.

var (
	gemmaMaxTokens int
	gemmaUseMax    bool
	gemmaModelFlag string
	gemmaTask      string
)

var gemmaCmd = &cobra.Command{
	Use:   "gemma [prompt]",
	Short: "Talk to Gemma — the local on-device model (zero API tokens, nothing leaves this Mac)",
	Long: `Gemma is a local MLX model running on THIS Mac — no network, no API tokens,
nothing leaves the machine. Give it a prompt and it answers.

  sirsi gemma "summarize this: <paste text>"
  sirsi gemma --task draft "a release note for the menubar Eye fix"
  git log -20 --oneline | sirsi gemma --task summarize "what shipped?"
  sirsi gemma --max "decompose building a feature for X into ordered steps"

Good at: classify | summarize | draft | analyze | plan | extract.
NOT for: tool use (no git/files/web) or binding verdicts — it reads your text and
writes text back. For anything that must ACT or sign off, use a real agent.`,
	Args: cobra.ArbitraryArgs,
	RunE: runGemma,
}

func init() {
	gemmaCmd.Flags().IntVar(&gemmaMaxTokens, "max-tokens", 1024, "Maximum tokens to generate")
	gemmaCmd.Flags().BoolVar(&gemmaUseMax, "max", false, "Use the largest local model (needs ample free RAM)")
	gemmaCmd.Flags().StringVar(&gemmaModelFlag, "model", "", "Override the model id (default: the resolved local model)")
	gemmaCmd.Flags().StringVar(&gemmaTask, "task", "", "Tune the prompt: classify|summarize|draft|analyze|plan|extract")
}

func runGemma(cmd *cobra.Command, args []string) error {
	prompt := strings.TrimSpace(strings.Join(args, " "))

	// Fold in piped stdin so `cat notes.txt | sirsi gemma --task summarize` works.
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		if b, err := io.ReadAll(os.Stdin); err == nil {
			if piped := strings.TrimSpace(string(b)); piped != "" {
				if prompt == "" {
					prompt = piped
				} else {
					prompt = prompt + "\n\n" + piped
				}
			}
		}
	}
	if prompt == "" {
		return fmt.Errorf("give Gemma a prompt, e.g.  sirsi gemma \"summarize this: ...\"")
	}
	if t := strings.ToLower(strings.TrimSpace(gemmaTask)); t != "" {
		prompt = "TASK: " + t + "\n\n" + prompt
	}

	home, _ := os.UserHomeDir()
	mlx := filepath.Join(home, ".venvs/mlx/bin/mlx_lm.generate")
	if _, err := os.Stat(mlx); err != nil {
		return fmt.Errorf("Gemma's local runtime isn't installed (%s missing) — run the MLX/Gemma setup first", mlx)
	}
	model := gemmaResolveModel(home)

	fmt.Fprintf(os.Stderr, "gemma · %s · local, thinking…\n", gemmaShortModel(model))
	out, err := exec.Command(mlx, "--model", model, "--max-tokens", fmt.Sprint(gemmaMaxTokens), "--prompt", prompt).Output()
	if err != nil {
		return fmt.Errorf("gemma generation failed: %w (first run downloads the model — that can take a while)", err)
	}
	cleaned := gemmaClean(string(out))
	if cleaned == "" {
		return fmt.Errorf("gemma returned no text (model may still be downloading; try again)")
	}
	fmt.Println(cleaned)
	return nil
}

// gemmaResolveModel mirrors the worker: flag > env > ~/.sirsi/gemma-model[-max].conf > fallback.
func gemmaResolveModel(home string) string {
	if gemmaModelFlag != "" {
		return gemmaModelFlag
	}
	if m := os.Getenv("GEMMA_MODEL"); m != "" {
		return m
	}
	conf := ".sirsi/gemma-model.conf"
	if gemmaUseMax {
		conf = ".sirsi/gemma-model-max.conf"
	}
	if b, err := os.ReadFile(filepath.Join(home, conf)); err == nil {
		if m := strings.TrimSpace(string(b)); m != "" {
			return m
		}
	}
	return "mlx-community/gemma-2-27b-it-bf16-4bit"
}

func gemmaShortModel(m string) string {
	if i := strings.LastIndex(m, "/"); i >= 0 {
		return m[i+1:]
	}
	return m
}

var gemmaCtrlTok = regexp.MustCompile(`<\|?[a-zA-Z_]+\|?>`)

// gemmaClean turns raw mlx_lm.generate output into just the answer:
//  1. drop the trailing stats footer (the `====` banner + Prompt/Generation lines);
//  2. this model is a REASONING model — it emits "<|channel>thought …reasoning…
//     <channel|>FINAL ANSWER". Keep only what's after the final `<channel|>` so the
//     user sees the answer, not the chain-of-thought;
//  3. strip any leftover control/template tokens and stat lines.
func gemmaClean(raw string) string {
	s := raw
	if i := strings.LastIndex(s, "=========="); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "<channel|>"); i >= 0 {
		s = s[i+len("<channel|>"):] // text after the last channel-close = the final answer
	}
	s = gemmaCtrlTok.ReplaceAllString(s, "")
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		t := strings.TrimSpace(ln)
		if t != "" && strings.Trim(t, "=") == "" {
			continue // a ===== separator
		}
		if strings.HasPrefix(t, "Prompt:") || strings.HasPrefix(t, "Generation:") ||
			strings.HasPrefix(t, "Peak memory:") || strings.HasPrefix(t, "Fetching") {
			continue
		}
		if t == "" && len(keep) == 0 {
			continue // leading blanks
		}
		keep = append(keep, ln)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}
