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

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
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
	gemmaServeCmd.Flags().IntVar(&gemmaServeConcurrency, "concurrency", 0, "Concurrent decodes; 0 = auto-derive from this node (ADR-031-B), >0 requests a specific count capped to what the node can hold")
	gemmaCmd.AddCommand(gemmaServeCmd)
}

func gemmaPidPath(home string) string       { return filepath.Join(home, ".sirsi/gemma-server.pid") }
func gemmaPortPath(home string) string      { return filepath.Join(home, ".sirsi/gemma-server.port") }
func gemmaServerLogPath(home string) string { return filepath.Join(home, ".sirsi/gemma-server.log") }
func gemmaCapWrapperPath(home string) string {
	return filepath.Join(home, ".sirsi/gemma-capped-server.py")
}

// gemmaCapWrapper is the LAYER-2 hard runtime cap (ADR-031-A invariant): it sets
// MLX's memory ceilings BEFORE the model loads, so MLX evicts/errors at the cap
// instead of growing into Jetsam — the guarantee that makes "never OOM the host"
// true even when the pre-launch estimate is wrong or the KV cache grows mid-decode.
// argv: <cap_bytes> <mlx_lm.server args...>
const gemmaCapWrapper = `import sys, runpy
import mlx.core as mx
cap = int(sys.argv[1])
for fn in ("set_memory_limit", "set_wired_limit"):
    try: getattr(mx, fn)(cap)
    except Exception as e: print("WARN: mx.%s(%d): %s" % (fn, cap, e), file=sys.stderr)
try: mx.set_cache_limit(cap // 4)   # cap the buffer-cache pool (the thing that ballooned on 06-18)
except Exception as e: print("WARN: mx.set_cache_limit:", e, file=sys.stderr)
sys.argv = ["mlx_lm.server"] + sys.argv[2:]
runpy.run_module("mlx_lm.server", run_name="__main__")
`

// gemmaWriteCapWrapper writes the launcher next to the other broker state.
func gemmaWriteCapWrapper(home string) (string, error) {
	p := gemmaCapWrapperPath(home)
	if err := os.WriteFile(p, []byte(gemmaCapWrapper), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

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
	// ADR-031-B: the broker's numbers now derive from the MEASURED node, not 48GB
	// constants. NodeCapacity.DynamicReserve accounts for the OS + live Claude/Codex
	// RSS + a margin that scales with the box (cross-agent protection is the point).
	nc := guard.SampleNodeCapacity()
	// BINDING REFUSE GATE (claude-home): MaxConcurrency floors at 1, so it never
	// refuses on its own — gate on Fits FIRST and refuse-rather-than-OOM when one
	// model won't fit within the dynamic reserve.
	if !nc.Fits(modelBytes) {
		return fmt.Errorf("Pantheon refuses to start the broker: a ~%d GB model + ~%d GB dynamic reserve (OS + live agents + margin) > %d GB free. Free up memory first — the broker will not OOM your machine",
			modelBytes/(1<<30), nc.DynamicReserve()/(1<<30), nc.FreeRAM/(1<<30))
	}
	// Node-derived concurrency (mapping #1). Default flag 0 = auto (use the node max);
	// a user may request FEWER with --concurrency N, never more than the node can hold.
	safe := nc.MaxConcurrency(modelBytes)
	if gemmaServeConcurrency > 0 && gemmaServeConcurrency < safe {
		safe = gemmaServeConcurrency
	}
	if safe < nc.MaxConcurrency(modelBytes) {
		fmt.Printf("⚠ concurrency %d (node could hold %d for a ~%d GB model)\n", safe, nc.MaxConcurrency(modelBytes), modelBytes/(1<<30))
	} else if safe > 1 {
		fmt.Printf("✓ node-derived concurrency %d for a ~%d GB model (this box can hold it)\n", safe, modelBytes/(1<<30))
	}
	gemmaServeConcurrency = safe

	_ = os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755)
	logf, err := os.Create(gemmaServerLogPath(home))
	if err != nil {
		return fmt.Errorf("opening server log: %w", err)
	}

	// LAYER 2 — HARD RUNTIME CAP. MLX may use up to (free − headroom); beyond that
	// it evicts/errors instead of taking the host down. The pre-launch gate already
	// guaranteed free ≥ 2×model + headroom, so the cap always holds model + working
	// memory. Launch the server THROUGH the cap wrapper (sets the limit before load).
	// Cap = node free − dynamic reserve (mapping #3), floored at one model. MLX
	// evicts/errors at this ceiling instead of growing into Jetsam; the Fits gate
	// above already guaranteed free ≥ model + reserve, so the cap holds the model.
	capBytes := nc.DynamicCap(modelBytes)
	wrapper, werr := gemmaWriteCapWrapper(home)
	if werr != nil {
		return fmt.Errorf("writing the memory-cap launcher: %w", werr)
	}
	pyBin := filepath.Join(home, ".venvs/mlx/bin/python")

	c := exec.Command(pyBin, wrapper, strconv.FormatInt(capBytes, 10),
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
	// Layer 4 (ADR-031-A): register the broker as Hapi-governed compute. This is
	// the broker CONSENTING to be stopped — if it ever balloons toward Jetsam
	// despite the hard MLX cap, Hapi suspends (reversibly) then kills THIS pid
	// before the kernel takes the whole host down. The thing that wasn't true
	// on 2026-06-18 is now true at the source.
	_ = guard.HapiRegisterGoverned(pid, "gemma-broker")
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
		_ = guard.HapiUnregisterGoverned(pid)   // no longer Hapi's to govern
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

// gemmaAgentReserveBytes sums the RSS of live AI-agent processes (Claude / Codex)
// so the broker reserves what they're using instead of counting it as available.
func gemmaAgentReserveBytes() int64 {
	out, err := exec.Command("ps", "-axo", "rss,comm").Output()
	if err != nil {
		return 0
	}
	var total int64
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		name := strings.ToLower(strings.Join(f[1:], " "))
		if strings.Contains(name, "claude") || strings.Contains(name, "codex") {
			if kb, e := strconv.ParseInt(f[0], 10, 64); e == nil {
				total += kb * 1024
			}
		}
	}
	return total
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
	// Serial budget is 2×model (resident weights + ~one model of working memory),
	// not 1× — under-budgeting serial is part of how the 06-18 estimate was too loose.
	if modelBytes*2+headroom > freeBytes {
		return 0, fmt.Sprintf("even serial needs ~%d GB (2× the %d GB model) + %d GB headroom > %d GB free", (modelBytes*2)/gb, modelBytes/gb, headroom/gb, freeBytes/gb)
	}
	if requested < 1 {
		requested = 1
	}
	safe := 1 // serial (2×model + headroom) just cleared the gate, so 1 is safe
	for n := requested; n >= 1; n-- {
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
