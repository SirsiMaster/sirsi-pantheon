package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// `sirsi gemma serve` is the Pantheon inference broker: it loads the local model
// ONCE onto the GPU and keeps it resident, serving CONCURRENT requests (mlx_lm.server
// with --decode/--prompt-concurrency). This is the unlock for using the whole
// machine — every consumer (the `sirsi gemma` CLI, the router gemma worker, MCP)
// hits one warm model instead of cold-reloading ~12 GB per call on a single thread.
// macOS truth: MLX runs on the GPU (not the ANE); the 40-core GPU is the right home.

const gemmaServerDefaultPort = 8765

var (
	gemmaServeStop        bool
	gemmaServeStatusFlag  bool
	gemmaServePort        int
	gemmaServeConcurrency int
)

var gemmaServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Keep Gemma WARM — load the model once on the GPU, serve concurrent requests (the Pantheon broker)",
	Long: `Loads the local model ONCE onto the GPU and keeps it resident, so every
` + "`sirsi gemma`" + ` call (and the router gemma worker) answers instantly with no
reload, and multiple requests decode concurrently on the GPU.

  sirsi gemma serve            # start the warm broker (stays up in the background)
  sirsi gemma serve --status   # is it warm?
  sirsi gemma serve --stop     # stop it

This is the "use the whole machine" path: one resident model, concurrent inference.`,
	RunE: runGemmaServe,
}

func init() {
	gemmaServeCmd.Flags().BoolVar(&gemmaServeStop, "stop", false, "Stop the warm Gemma server")
	gemmaServeCmd.Flags().BoolVar(&gemmaServeStatusFlag, "status", false, "Show whether the warm server is running")
	gemmaServeCmd.Flags().IntVar(&gemmaServePort, "port", gemmaServerDefaultPort, "Local port for the server")
	gemmaServeCmd.Flags().IntVar(&gemmaServeConcurrency, "concurrency", 1, "Max concurrent decodes (RAM-gated — each slot grows ~a full model in memory; capped to fit)")
	gemmaCmd.AddCommand(gemmaServeCmd)
}

func gemmaPidPath(home string) string       { return filepath.Join(home, ".sirsi/gemma-server.pid") }
func gemmaPortPath(home string) string      { return filepath.Join(home, ".sirsi/gemma-server.port") }
func gemmaServerLogPath(home string) string { return filepath.Join(home, ".sirsi/gemma-server.log") }

func runGemmaServe(cmd *cobra.Command, args []string) error {
	home, _ := os.UserHomeDir()
	switch {
	case gemmaServeStop:
		return gemmaServerStop(home)
	case gemmaServeStatusFlag:
		if base := gemmaServerBase(home); base != "" {
			fmt.Printf("Gemma broker: WARM at %s — `sirsi gemma \"...\"` answers instantly.\n", base)
		} else {
			fmt.Println("Gemma broker: cold (not running). `sirsi gemma` reloads the model per call. Start it: sirsi gemma serve")
		}
		return nil
	default:
		return gemmaServerStart(home)
	}
}

func gemmaServerStart(home string) error {
	if base := gemmaServerBase(home); base != "" {
		fmt.Printf("Gemma broker already warm at %s.\n", base)
		return nil
	}
	mlxsrv := filepath.Join(home, ".venvs/mlx/bin/mlx_lm.server")
	if _, err := os.Stat(mlxsrv); err != nil {
		return fmt.Errorf("mlx_lm.server not found (%s) — install the MLX runtime first", mlxsrv)
	}
	model := gemmaResolveModel(home)

	// ── RAM SAFETY (Hapi / A1) — the broker must NEVER OOM the machine ──────────
	// A resident model + N concurrent decode slots must fit in physical RAM with
	// headroom for the OS, foreground apps, and the other agents (Claude/Codex).
	// Empirically a concurrent slot grows ~a full model's worth of working memory
	// (concurrency 4 ballooned a 12 GB model to ~64 GB on a 48 GB box → Jetsam).
	// Refuse rather than crash; cap concurrency to what fits.
	modelBytes := gemmaEstimateModelBytes(home, model)
	freeBytes := gemmaFreeRAMBytes()
	safe, note := gemmaSafeConcurrency(gemmaServeConcurrency, modelBytes, freeBytes)
	if safe == 0 {
		return fmt.Errorf("Pantheon refuses to start the broker: %s. Free up memory first — the broker will not OOM your machine", note)
	}
	if note != "" {
		fmt.Printf("⚠ %s\n", note)
	}
	gemmaServeConcurrency = safe

	_ = os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755)
	logf, err := os.Create(gemmaServerLogPath(home))
	if err != nil {
		return fmt.Errorf("opening server log: %w", err)
	}

	c := exec.Command(mlxsrv,
		"--model", model,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(gemmaServePort),
		"--decode-concurrency", strconv.Itoa(gemmaServeConcurrency),
		"--prompt-concurrency", strconv.Itoa(gemmaServeConcurrency),
	)
	c.Stdout = logf
	c.Stderr = logf
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group → survives this CLI exiting
	if err := c.Start(); err != nil {
		return fmt.Errorf("starting mlx_lm.server: %w", err)
	}
	pid := c.Process.Pid
	_ = os.WriteFile(gemmaPidPath(home), []byte(strconv.Itoa(pid)), 0o644)
	_ = os.WriteFile(gemmaPortPath(home), []byte(strconv.Itoa(gemmaServePort)), 0o644)
	_ = c.Process.Release() // detach — do not wait/reap

	fmt.Printf("gemma broker starting (pid %d, port %d, model %s, concurrency %d) — loading onto the GPU…\n",
		pid, gemmaServePort, gemmaShortModel(model), gemmaServeConcurrency)
	base := fmt.Sprintf("http://127.0.0.1:%d", gemmaServePort)
	for i := 0; i < 45; i++ {
		time.Sleep(2 * time.Second)
		if gemmaServerPing(base) {
			fmt.Printf("✓ Gemma is WARM at %s. `sirsi gemma \"...\"` now answers instantly — no reload, %d concurrent.\n", base, gemmaServeConcurrency)
			return nil
		}
	}
	return fmt.Errorf("server didn't become healthy in ~90s — check %s", gemmaServerLogPath(home))
}

func gemmaServerStop(home string) error {
	b, err := os.ReadFile(gemmaPidPath(home))
	if err != nil {
		fmt.Println("No warm Gemma broker recorded.")
		return nil
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGTERM) // the whole process group
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	_ = os.Remove(gemmaPidPath(home))
	_ = os.Remove(gemmaPortPath(home))
	fmt.Printf("Stopped Gemma broker (pid %d).\n", pid)
	return nil
}

// gemmaServerBase returns the warm broker's base URL if it's up, else "".
func gemmaServerBase(home string) string {
	b, err := os.ReadFile(gemmaPortPath(home))
	if err != nil {
		return ""
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || port == 0 {
		return ""
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if gemmaServerPing(base) {
		return base
	}
	return ""
}

func gemmaServerPing(base string) bool {
	cl := &http.Client{Timeout: 2 * time.Second}
	resp, err := cl.Get(base + "/v1/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// gemmaWarmComplete runs one chat completion against the warm broker (no reload).
func gemmaWarmComplete(base, model, prompt string, maxTokens int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTokens,
		"temperature": 0,
	})
	cl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := cl.Post(base+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("warm broker returned no choices")
	}
	return gemmaClean(out.Choices[0].Message.Content), nil
}

// gemmaFreeRAMBytes returns reclaimable RAM (free + inactive + speculative pages).
func gemmaFreeRAMBytes() int64 {
	pageSize := int64(16384)
	if out, err := exec.Command("sysctl", "-n", "hw.pagesize").Output(); err == nil {
		if n, e := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); e == nil && n > 0 {
			pageSize = n
		}
	}
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0
	}
	var pages int64
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "Pages free:") &&
			!strings.HasPrefix(line, "Pages inactive:") &&
			!strings.HasPrefix(line, "Pages speculative:") {
			continue
		}
		f := strings.Fields(line)
		v := strings.TrimSuffix(f[len(f)-1], ".")
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			pages += n
		}
	}
	return pages * pageSize
}

// gemmaEstimateModelBytes estimates the resident size of a model from its HF cache.
func gemmaEstimateModelBytes(home, model string) int64 {
	dir := filepath.Join(home, ".cache/huggingface/hub", "models--"+strings.ReplaceAll(model, "/", "--"))
	if out, err := exec.Command("du", "-sk", dir).Output(); err == nil {
		if f := strings.Fields(string(out)); len(f) > 0 {
			if kb, e := strconv.ParseInt(f[0], 10, 64); e == nil && kb > 0 {
				return kb * 1024
			}
		}
	}
	return 14 << 30 // conservative default ~14 GB
}

// gemmaSafeConcurrency returns the largest concurrency ≤ requested that fits RAM,
// or 0 (with a reason) if even the resident model + headroom won't fit. Each
// concurrent decode slot is budgeted at ~a full model of working memory because
// that is what was observed when concurrency 4 ballooned a 12 GB model to ~64 GB.
func gemmaSafeConcurrency(requested int, modelBytes, freeBytes int64) (int, string) {
	const gb = int64(1) << 30
	const headroom = 8 * gb // OS + foreground apps + the other agents (Claude/Codex)
	if modelBytes <= 0 {
		modelBytes = 14 * gb
	}
	if modelBytes+headroom > freeBytes {
		return 0, fmt.Sprintf("the ~%d GB model + %d GB headroom exceeds %d GB free", modelBytes/gb, headroom/gb, freeBytes/gb)
	}
	if requested < 1 {
		requested = 1
	}
	safe := 1 // model + headroom fits, so serial (1) is always safe here
	for n := requested; n >= 2; n-- {
		if modelBytes*int64(1+n)+headroom <= freeBytes {
			safe = n
			break
		}
	}
	note := ""
	if safe < requested {
		note = fmt.Sprintf("capped concurrency %d→%d to fit RAM (model ~%d GB, free ~%d GB, %d GB headroom) — the broker won't OOM the machine",
			requested, safe, modelBytes/gb, freeBytes/gb, headroom/gb)
	}
	return safe, note
}
