package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
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

// gemmaColdMaxKVSize caps the cold-path mlx_lm.generate KV cache (in tokens) so
// no bare `sirsi gemma` call can balloon its Python unbounded and trigger a
// jetsam (2026-07-21: a separate unbounded Python hit 28.88 GB). Generous for
// Tier-0 work; the model rotates the cache past this cap rather than growing.
const gemmaColdMaxKVSize = 32768

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
	prompt = renderGemmaPrompt(prompt)

	home, _ := os.UserHomeDir()
	model := gemmaResolveModel(home)

	// Prefer the WARM Pantheon broker if it's up: no model reload, instant answer,
	// concurrent with other requests on the GPU. Falls through to the cold path on
	// any error so a single prompt never fails just because the server hiccuped.
	// warmErr/warmUp are captured so the cold path can report the REAL cause when
	// the warm broker was up but declined — otherwise a warm-vs-cold routing gap
	// (broker holds model X, request asks for Y) surfaces as a bare cold RAM
	// refusal that hides why (claude-nexus flag, 2026-07-17).
	var warmErr error
	warmUp := false
	if base := gemmaServerBase(home); base != "" {
		warmUp = true
		fmt.Fprintf(os.Stderr, "gemma · %s · warm broker, thinking…\n", gemmaShortModel(model))
		ans, err := gemmaWarmComplete(base, model, prompt, gemmaMaxTokens)
		if err == nil && ans != "" {
			fmt.Println(ans)
			return nil
		}
		if err != nil {
			warmErr = err
		} else {
			warmErr = fmt.Errorf("warm broker returned an empty answer")
		}
	}

	// Cold path: shell mlx_lm.generate (reloads the model each call). Nudge the user
	// toward `sirsi gemma serve` for instant, concurrent answers.
	mlx := filepath.Join(home, ".venvs/mlx/bin/mlx_lm.generate")
	if _, err := os.Stat(mlx); err != nil {
		return fmt.Errorf("Gemma's local runtime isn't installed (%s missing) — run the MLX/Gemma setup first", mlx)
	}

	// LAYER 4 (ADR-031-A) — the cold path is the 06-18 OOM culprit: 4 concurrent
	// `sirsi gemma` calls each forked a full ~12 GB model load (5 at once → Jetsam).
	// (1) RAM-gate it like the broker; (2) serialize it machine-wide with a file
	// lock so concurrent callers can NEVER stack N full model loads.
	_ = os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755)
	// ADR-031-B: the cold path now refuses through the SAME NodeCapacity self-model
	// as the warm broker — one node-derived budget, not a separate gemmaSafeConcurrency
	// with its own constants. Fits requires 2×model + DynamicReserve (resident model +
	// one model of working memory + OS/agents/margin), so the cold path keeps the
	// #63 2×model conservatism, now node-proportional and cross-agent-aware.
	modelBytes := gemmaEstimateModelBytes(home, model)
	if nc := guard.SampleNodeCapacity(); !nc.Fits(modelBytes) {
		// The warm broker was up but declined THIS model, and now the cold path
		// can't fit it either (the broker is holding RAM). Report the warm cause
		// so a warm-vs-cold routing gap is diagnosable, not a bare cold refusal.
		if warmUp && warmErr != nil {
			return fmt.Errorf("the warm broker is up but did not serve %s (%v), and a cold load won't fit (~%dGB model + ~%dGB reserve > %dGB free). The warm broker likely holds a DIFFERENT model resident — restart it on this model with `sirsi gemma serve --port <p>` (its default reads gemma-model.conf = %s), or free memory. Refusing rather than OOM the machine",
				gemmaShortModel(model), warmErr, modelBytes/(1<<30), nc.DynamicReserve()/(1<<30), nc.FreeRAM/(1<<30), gemmaShortModel(model))
		}
		return fmt.Errorf("not enough RAM to load Gemma cold (~%dGB model + ~%dGB dynamic reserve > %dGB free) — start the warm broker (`sirsi gemma serve`) or free memory. Refusing rather than OOM the machine",
			modelBytes/(1<<30), nc.DynamicReserve()/(1<<30), nc.FreeRAM/(1<<30))
	}
	if lf, lerr := os.OpenFile(filepath.Join(home, ".sirsi/gemma-cold.lock"), os.O_CREATE|os.O_RDWR, 0o644); lerr == nil {
		defer lf.Close()
		if syscall.Flock(int(lf.Fd()), syscall.LOCK_EX) == nil { // blocks until we hold it
			defer func() { _ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) }()
		}
	}

	fmt.Fprintf(os.Stderr, "gemma · %s · cold (reloading — run `sirsi gemma serve` to keep it warm)…\n", gemmaShortModel(model))
	// Bound the KV cache (--max-kv-size) so a pathologically long prompt can NEVER
	// balloon the cold-path Python unbounded — the 2026-07-21 jetsam was a separate,
	// unbounded Python that hit 28.88 GB → forced OS jetsam. The warm broker bounds
	// its cache via --prompt-cache-bytes (#215); this is the generate-path analog on
	// the ONLY non-serve mlx spawn a bare `sirsi gemma` call makes. 32K tokens is
	// generous for Tier-0 tasks while hard-capping the balloon (mlx uses a rotating
	// cache past the cap). Refs A32/ADR-040; claude-home P0 2026-07-21.
	out, err := exec.Command(mlx, "--model", model,
		"--max-tokens", fmt.Sprint(gemmaMaxTokens),
		"--max-kv-size", fmt.Sprint(gemmaColdMaxKVSize),
		"--prompt", prompt).Output()
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
