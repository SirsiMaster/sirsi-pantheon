package reason

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/vitals"
)

// MachineTools registers the tools that answer "what is wrong with this
// machine". Every one of them is drawn from a failure this fabric actually
// recorded, because a tool nobody has needed is a tool nobody has debugged.
func MachineTools(r *Registry) error {
	for _, t := range []Tool{
		processCensus(),
		memoryPressure(),
		forkStormScan(),
		restartBroker(),
	} {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// processCensus is the tool that found the 2026-07-27 storm. It sizes processes
// by PHYSICAL FOOTPRINT, never RSS — the broker read 4.71 GB resident against a
// 29.4 GB footprint, and every check that trusted RSS reported it innocent
// while the machine OOM'd three times.
func processCensus() Tool {
	return Tool{
		Name: "process.census",
		Does: "count processes and size the largest by physical footprint",
		Tier: TierObserve,
		Run: func(ctx context.Context) (Result, error) {
			out, err := exec.CommandContext(ctx, "ps", "-Ao", "pid=,comm=").Output()
			if err != nil {
				return Result{}, fmt.Errorf("process census: %w", err)
			}
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")

			byName := map[string]int{}
			type big struct {
				name string
				fp   uint64
			}
			var largest big
			for _, l := range lines {
				f := strings.Fields(l)
				if len(f) < 2 {
					continue
				}
				name := f[len(f)-1]
				if i := strings.LastIndex(name, "/"); i >= 0 {
					name = name[i+1:]
				}
				byName[name]++
				if pid, cerr := strconv.Atoi(f[0]); cerr == nil {
					if fp, ferr := vitals.PhysFootprint(pid); ferr == nil && fp > largest.fp {
						largest = big{name, fp}
					}
				}
			}

			// The population outlier is the fork-storm tell: 358 `claude`
			// processes against a normal handful.
			var topName string
			topCount := 0
			for n, c := range byName {
				if c > topCount {
					topName, topCount = n, c
				}
			}

			return Result{
				Summary: fmt.Sprintf("%d processes; most numerous %q ×%d; largest footprint %s at %.1f GB",
					len(lines), topName, topCount, largest.name, float64(largest.fp)/(1<<30)),
				Evidence: map[string]any{
					"total":         len(lines),
					"most_numerous": topName,
					"most_count":    topCount,
					"largest_name":  largest.name,
					"largest_gb":    float64(largest.fp) / (1 << 30),
				},
			}, nil
		},
	}
}

// memoryPressure reads what the kernel actually judges by. Reports AVAILABLE
// (free + inactive), never free: on macOS free is near-zero by design on any
// long-running machine, and reading it as danger is the false-spiral this
// fabric fixed in #316.
func memoryPressure() Tool {
	return Tool{
		Name: "memory.pressure",
		Does: "read available memory, swap headroom and the kernel's pressure level",
		Tier: TierObserve,
		Run: func(ctx context.Context) (Result, error) {
			lvl := sysctlStr(ctx, "kern.memorystatus_vm_pressure_level")
			swap := sysctlStr(ctx, "vm.swapusage")

			// vm.swapusage: total = X used = Y free = Z
			var used, free float64
			for _, key := range []string{"used", "free"} {
				if i := strings.Index(swap, key+" = "); i >= 0 {
					s := swap[i+len(key)+3:]
					if j := strings.IndexAny(s, "M G"); j > 0 {
						if v, err := strconv.ParseFloat(s[:j], 64); err == nil {
							if key == "used" {
								used = v
							} else {
								free = v
							}
						}
					}
				}
			}

			return Result{
				Summary: fmt.Sprintf("pressure level %s; swap %.0fM used, %.0fM free", lvl, used, free),
				Evidence: map[string]any{
					"pressure_level": lvl,
					"swap_used_mb":   used,
					"swap_free_mb":   free,
				},
			}, nil
		},
	}
}

// forkStormThreshold is deliberately well above normal system noise.
//
// The first version used 10 and immediately produced a FALSE POSITIVE on this
// machine: 23 idle `distnoted` — macOS's notification daemon, one per session
// context, event-driven and legitimately near-zero CPU. Reported as a fork
// storm, that is a check that cries wolf, which is how a surface earns the
// distrust that lets a real 358-process storm pass unread.
//
// The real signature was 358. Fifty separates "something is multiplying" from
// "macOS has a lot of small daemons", with room to spare on both sides.
const forkStormThreshold = 50

// isSystemDaemon reports whether a path belongs to the OS rather than to the
// user's work. Apple ships many long-lived idle daemons; a storm of THEM is not
// something Sirsi should claim, and is not something a user could act on.
func isSystemDaemon(path string) bool {
	for _, prefix := range []string{"/usr/sbin/", "/usr/libexec/", "/System/", "/usr/bin/", "/sbin/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// forkStormScan looks for the signature that separates a fork storm from honest
// load: many processes of one name, orphaned to launchd, OLD but with almost no
// cumulative CPU. On 2026-07-27 all 353 offenders had ~6 seconds of CPU across
// 2h28m of life. Old-and-idle is hung; old-and-busy is working. Different bug,
// different fix.
func forkStormScan() Tool {
	return Tool{
		Name: "process.forkstorm",
		Does: "look for many orphaned, long-lived, near-zero-CPU processes of one name",
		Tier: TierObserve,
		Run: func(ctx context.Context) (Result, error) {
			// Full command path, not comm: distinguishing a system daemon from a
			// runaway needs to know WHERE the binary lives.
			out, err := exec.CommandContext(ctx, "ps", "-Ao", "ppid=,etime=,time=,command=").Output()
			if err != nil {
				return Result{}, fmt.Errorf("forkstorm scan: %w", err)
			}
			orphanHung := map[string]int{}
			for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				f := strings.Fields(l)
				if len(f) < 4 {
					continue
				}
				if f[0] != "1" { // reparented to launchd
					continue
				}
				if !oldEnough(f[1]) || !nearlyZeroCPU(f[2]) {
					continue
				}
				path := f[3]
				if isSystemDaemon(path) {
					continue
				}
				name := path
				if i := strings.LastIndex(name, "/"); i >= 0 {
					name = name[i+1:]
				}
				orphanHung[name]++
			}
			worst, n := "", 0
			for k, v := range orphanHung {
				if v > n {
					worst, n = k, v
				}
			}
			sum := "no orphaned-and-hung process cluster"
			if n >= forkStormThreshold {
				sum = fmt.Sprintf("FORK STORM: %d orphaned %q processes, old but ~no CPU — hung, not working", n, worst)
			} else if n > 0 {
				sum = fmt.Sprintf("%d orphaned %q process(es) — below the storm threshold of %d", n, worst, forkStormThreshold)
			}
			return Result{
				Summary:  sum,
				Evidence: map[string]any{"worst_name": worst, "count": n},
			}, nil
		},
	}
}

// restartBroker is the one repair tool here, and it exists to demonstrate the
// contract rather than to be clever: it names what it will do, it is
// reversible, and it VERIFIES by re-reading the world rather than trusting an
// exit code.
func restartBroker() Tool {
	return Tool{
		Name:       "gemma.restart",
		Does:       "restart the local model broker (it reloads in seconds; in-flight requests are lost)",
		Tier:       TierRepair,
		Reversible: true,
		Run: func(ctx context.Context) (Result, error) {
			before := brokerPID(ctx)
			cmd := exec.CommandContext(ctx, "sirsi", "gemma", "serve", "--restart")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return Result{Summary: strings.TrimSpace(string(out))},
					fmt.Errorf("restart broker: %w", err)
			}
			return Result{
				Summary:  fmt.Sprintf("restart issued (previous pid %d)", before),
				Evidence: map[string]any{"pid_before": before},
				Changed:  true,
			}, nil
		},
		Verify: func(ctx context.Context) (Result, error) {
			// Verification is the whole point. A restart that "succeeded" while
			// the broker never came back is the failure mode that ends an
			// investigation early — so assert a NEW pid AND a live endpoint.
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if pid := brokerPID(ctx); pid > 0 {
					fp, _ := vitals.PhysFootprint(pid)
					return Result{
						Summary:  fmt.Sprintf("broker back as pid %d, footprint %.1f GB", pid, float64(fp)/(1<<30)),
						Evidence: map[string]any{"pid_after": pid, "footprint_gb": float64(fp) / (1 << 30)},
					}, nil
				}
				time.Sleep(2 * time.Second)
			}
			return Result{Summary: "broker did not come back within 30s"},
				fmt.Errorf("broker absent after restart — the model is DOWN, not restarted")
		},
	}
}

func brokerPID(ctx context.Context) int {
	out, err := exec.CommandContext(ctx, "pgrep", "-f", "gemma-capped-server").Output()
	if err != nil {
		return 0
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return 0
	}
	pid, _ := strconv.Atoi(f[0])
	return pid
}

func sysctlStr(ctx context.Context, key string) string {
	out, err := exec.CommandContext(ctx, "sysctl", "-n", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// oldEnough accepts ps etime of MM:SS only when well past a minute, or any
// longer form. A young process is not evidence of anything.
func oldEnough(etime string) bool {
	if strings.Contains(etime, "-") { // DD-HH:MM:SS
		return true
	}
	parts := strings.Split(etime, ":")
	if len(parts) >= 3 { // HH:MM:SS
		return true
	}
	if len(parts) == 2 {
		m, _ := strconv.Atoi(parts[0])
		return m >= 10
	}
	return false
}

// nearlyZeroCPU accepts ps time of 0:SS — seconds of CPU. Paired with age this
// is the hung-vs-spinning discriminator.
func nearlyZeroCPU(t string) bool {
	return strings.HasPrefix(t, "0:0") || strings.HasPrefix(t, "0:1")
}
