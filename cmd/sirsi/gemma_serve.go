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
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

// `sirsi gemma serve` is the Pantheon inference broker: it loads the local model
// ONCE onto the GPU and keeps it resident, serving CONCURRENT requests (mlx_lm.server
// with --decode/--prompt-concurrency). This is the unlock for using the whole
// machine — every consumer (the `sirsi gemma` CLI, the router gemma worker, MCP)
// hits one warm model instead of cold-reloading ~12 GB per call on a single thread.
// macOS truth: MLX runs on the GPU (not the ANE); the 40-core GPU is the right home.

const gemmaServerDefaultPort = 8765

// gemmaPromptCacheSlots is how many DISTINCT prompt-prefix KV caches the broker
// retains (mlx_lm.server --prompt-cache-size). Pantheon's router alternates
// between a handful of recurring prefix families (reconcile / build / worker /
// NL query), so 8 lets each stay warm across requests; total memory is still
// bounded by --prompt-cache-bytes, which the slots share. (oMLX lesson, 2026-07-21.)
const gemmaPromptCacheSlots = 8

var (
	gemmaServeStop        bool
	gemmaServeStatusFlag  bool
	gemmaServeForeground  bool
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
	gemmaServeCmd.Flags().BoolVar(&gemmaServeForeground, "foreground", false, "Run the broker as this process (for launchd — KeepAlive revives it after crashes and reboots)")
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
const gemmaCapWrapper = `import os, socket, sys, time, traceback, runpy
import mlx.core as mx
cap = int(sys.argv[1])
for fn in ("set_memory_limit", "set_wired_limit"):
    try: getattr(mx, fn)(cap)
    except Exception as e: print("WARN: mx.%s(%d): %s" % (fn, cap, e), file=sys.stderr)
try: mx.set_cache_limit(cap // 4)   # cap the buffer-cache pool (the thing that ballooned on 06-18)
except Exception as e: print("WARN: mx.set_cache_limit:", e, file=sys.stderr)
argv = sys.argv[2:]

# Wait-for-port-free (broker deaths #3+#4, 2026-07-23): a respawn that binds
# while the dying predecessor still holds the port got Errno 48 INSIDE
# mlx_lm.server, and the failed process wedged as a zombie — model loaded
# (12.9 GB), main thread dead, port unheld. Probe-bind until the port frees
# (the only competitor is our own dying predecessor, which never re-binds, so
# the probe→release→server-bind window is benign); give up after 30s with a
# distinct exit code so launchd KeepAlive retries on its ThrottleInterval.
host, port = "127.0.0.1", 8080
for i, a in enumerate(argv):
    if a == "--host" and i + 1 < len(argv): host = argv[i + 1]
    if a == "--port" and i + 1 < len(argv): port = int(argv[i + 1])
deadline = time.time() + 30
while True:
    probe = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        probe.bind((host, port))
        probe.close()
        break
    except OSError:
        probe.close()
        if time.time() > deadline:
            print("FATAL: port %d still held after 30s — exiting for launchd retry" % port, file=sys.stderr)
            os._exit(48)
        time.sleep(0.5)

# Last-gasp + no-zombie guarantee: ANY exit path ends in os._exit so
# non-daemon worker threads can never keep a dead server resident holding the
# model (the death-#3 wedge class), and every crash leaves an attributable
# traceback in the launchd log (4 silent deaths on 2026-07-23 had none).
sys.argv = ["mlx_lm.server"] + argv
code = 0
try:
    runpy.run_module("mlx_lm.server", run_name="__main__")
except SystemExit as e:
    code = e.code if isinstance(e.code, int) else (0 if e.code is None else 1)
except BaseException:
    traceback.print_exc()
    code = 1
os._exit(code)
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
		if gemmaServeForeground {
			// launchd respawned us while a broker is still serving (e.g. a
			// stray manually-started one). Exit non-zero; ThrottleInterval
			// spaces the retries until the stray broker is gone.
			return fmt.Errorf("a Gemma broker is already warm at %s — not starting a second one", base)
		}
		fmt.Printf("Gemma broker already warm at %s.\n", base)
		return nil
	}
	// When the reboot-durable LaunchAgent owns the broker (ADR-045), start it
	// through launchd so there is exactly ONE supervisor of the process —
	// KeepAlive then revives it after any crash and at every boot.
	if !gemmaServeForeground && setup.GemmaBrokerInstalled() {
		fmt.Println("gemma broker starting via launchd (reboot-durable) — loading onto the GPU…")
		_ = exec.Command("launchctl", "kickstart", fmt.Sprintf("gui/%d/%s", os.Getuid(), setup.GemmaBrokerLabel)).Run()
		return gemmaAwaitWarm(home)
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
	// BINDING REFUSE GATE: refuse-rather-than-OOM. nc.Fits requires 2×model +
	// reserve — that headroom existed because an UNCAPPED broker could balloon a
	// full model's worth of working memory (the 06-18/07-14 Jetsam). This broker
	// now launches with hard runtime caps (mx.set_cache_limit on the buffer pool +
	// --prompt-cache-bytes on the KV cache, #215), so it CANNOT grow past
	// model + capped-caches. The 2× is therefore obsolete: require 1×model + the
	// dynamic reserve, which keeps a capable model (gemma-4-12B) runnable without
	// the balloon risk the 2× guarded against. The runtime cap is the real floor.
	need := modelBytes + nc.DynamicReserve()
	if nc.FreeRAM < need {
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

	// LAYER 3 — cap the PROMPT/KV cache too. The wrapper's mx.set_cache_limit caps
	// the buffer-cache pool (the 06-18 balloon), but the 07-14 SIGABRT+Jetsam was a
	// DIFFERENT balloon: the mlx_lm.server prompt/KV cache growing unbounded with
	// long prompts. --prompt-cache-bytes bounds it. Node-derived (capBytes/4),
	// consistent with the buffer-pool cap, so it scales with the box and stays well
	// under the balloon line. This makes the P0 fix durable, not a hand-patched runtime.
	promptCacheBytes := capBytes / 4
	args := []string{wrapper, strconv.FormatInt(capBytes, 10),
		"--model", model,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(gemmaServePort),
		"--decode-concurrency", strconv.Itoa(gemmaServeConcurrency),
		"--prompt-concurrency", strconv.Itoa(gemmaServeConcurrency),
		// Retain MULTIPLE distinct prompt prefixes, not just the most-recent one.
		// Pantheon's workload is the "coding-agent shifting-prefix" pattern (oMLX,
		// 2026-07-21): the router alternates between reconcile / build / worker /
		// NL-query prompts that each carry a large STABLE prefix (canon, source).
		// Stock mlx_lm.server holds ~one prefix and evicts it on every shift →
		// full KV recompute → the slow TTFT we measure on the liveness probe.
		// Holding N prefixes lets each request type hit a warm cache. Memory stays
		// bounded by --prompt-cache-bytes (the N caches SHARE that byte budget,
		// LRU-evicted) — so this is a pure latency win with no added RAM risk.
		"--prompt-cache-size", strconv.Itoa(gemmaPromptCacheSlots),
		"--prompt-cache-bytes", strconv.FormatInt(promptCacheBytes, 10),
	}

	if gemmaServeForeground {
		// launchd mode (ADR-045): exec the capped server IN PLACE so the pid
		// launchd supervises IS the serving process — KeepAlive revives it after
		// any crash and at every boot, with zero cloud involvement. exec keeps
		// our pid, so the pid file written here stays truthful for `--stop`,
		// the load-bearing guard (ADR-040), and Hapi governance (ADR-031-A).
		pid := os.Getpid()
		_ = os.WriteFile(gemmaPidPath(home), []byte(strconv.Itoa(pid)), 0o644)
		_ = os.WriteFile(gemmaPortPath(home), []byte(strconv.Itoa(gemmaServePort)), 0o644)
		_ = guard.HapiRegisterGoverned(pid, "gemma-broker")
		return syscall.Exec(pyBin, append([]string{pyBin}, args...), os.Environ())
	}

	logf, err := os.Create(gemmaServerLogPath(home))
	if err != nil {
		return fmt.Errorf("opening server log: %w", err)
	}
	c := exec.Command(pyBin, args...)
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
	return gemmaAwaitWarm(home)
}

// gemmaAwaitWarm polls the broker until it answers or ~90s passes.
func gemmaAwaitWarm(home string) error {
	base := fmt.Sprintf("http://127.0.0.1:%d", gemmaServePort)
	for i := 0; i < 45; i++ {
		time.Sleep(2 * time.Second)
		if gemmaServerPing(base) {
			fmt.Printf("✓ Gemma is WARM at %s. `sirsi gemma \"...\"` now answers instantly.\n", base)
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
	if setup.GemmaBrokerInstalled() {
		fmt.Printf("Note: the reboot-durable agent will bring it back within ~30s. To keep it off: launchctl bootout gui/%d/%s\n",
			os.Getuid(), setup.GemmaBrokerLabel)
	}
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
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("warm broker returned no choices")
	}
	msg := out.Choices[0].Message
	text := msg.Content
	if strings.TrimSpace(text) == "" {
		if out.Choices[0].FinishReason == "length" {
			return "", fmt.Errorf("warm broker returned only reasoning before the token limit; raise --max-tokens")
		}
		text = msg.Reasoning
	}
	return gemmaClean(text), nil
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
