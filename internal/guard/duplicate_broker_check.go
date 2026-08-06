package guard

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/vitals"
)

// checkDuplicateModelBrokers flags MORE THAN ONE local model broker running at
// once. Two brokers means two full copies of a multi-GB model resident at the
// same time, and the second one is usually invisible: nothing routes to it, so
// it sits idle while its pages get swapped out and quietly consume the swap file.
//
// Found live 2026-07-30. Two brokers were serving the same gemma-4-12B model:
//
//	pid 10970  gemma-capped-server.py 22320611328 … --port 8765   (canonical, capped)
//	pid  4201  ./gemma serve …/gemma-4-12B-it-8bit :8477          (orphan, UNCAPPED)
//
// The orphan was reparented to launchd, owned by no launchd unit, named by
// neither the pidfile nor the port file, idle at 0.0% CPU with 12s of CPU across
// 8h40m — and holding 14.9 GB of physical footprint against 0.1 GB RSS, i.e. its
// entire working set had been pushed into swap. Swap sat at 93% used (17.6 GB of
// 18.4 GB). Killing it released ~15 GB and macOS shrank the swap file from 18,432
// MB to 3,072 MB.
//
// WHY THIS CHECK IS SHAPED LIKE THIS. `sirsi diagnose` already SAW both processes
// — it reported "gemma at 14.9 GB, Python at 14.1 GB" — and reported them as two
// unrelated memory hogs. The data was present and the relationship was not, so a
// reader saw two big processes rather than one component duplicated. Every other
// guard keys on the KNOWN pidfile or port file, which is exactly why none of them
// could see a broker they did not start.
//
// So this check DISCOVERS brokers by what they are doing — serving a model — and
// never by the path Sirsi happens to know about (PANTHEON_RULES.md A29: discover,
// never enumerate; a guard that only inspects the thing it launched shares the
// blind spot of the bug it is meant to catch).
func checkDuplicateModelBrokers(p platform.Platform, report *DoctorReport) {
	finding := DiagnosticFinding{Check: "Duplicate Model Brokers"}

	brokers, err := FindModelBrokers(p)
	if err != nil {
		finding.Severity = SeverityOK
		finding.Message = "process list unavailable — duplicate-broker check skipped"
		report.Findings = append(report.Findings, finding)
		return
	}
	if len(brokers) <= 1 {
		finding.Severity = SeverityOK
		if len(brokers) == 1 {
			finding.Message = fmt.Sprintf("one local model broker (pid %d) — no duplication", brokers[0].PID)
		} else {
			finding.Message = "no local model broker running"
		}
		report.Findings = append(report.Findings, finding)
		return
	}

	// Rank worst-first so the message names the biggest offender.
	sort.Slice(brokers, func(i, j int) bool { return brokers[i].Worst > brokers[j].Worst })

	var total int64
	var parts []string
	for _, b := range brokers {
		total += b.Worst
		parts = append(parts, fmt.Sprintf("pid %d %.1f GB%s", b.PID, fpGB(b.Worst), b.Note))
	}

	finding.Severity = SeverityCritical
	finding.Message = fmt.Sprintf(
		"%d local model brokers running at once, %.1f GB combined — a second broker serves nothing and its pages end up in swap",
		len(brokers), fpGB(total))
	finding.Detail = strings.Join(parts, " | ") +
		" — only the pidfile's broker is routed to; the others are orphans"
	report.Findings = append(report.Findings, finding)
}

// BrokerProc is one discovered model broker, exported so the reap-orphans lever
// uses the SAME discovery the check alarms on. Two definitions would drift, and
// a lever that disagrees with its check is worse than no lever.
type BrokerProc struct {
	PID   int
	Worst int64
	Note  string
}

// FindModelBrokers discovers every running model broker, however it was started.
//
// It reads ARGV (`ps -axo pid,args`), NOT the process census's Command field.
// That field comes from `ps -axo …,comm`, which holds only the executable name —
// "Python" for the capped broker and "gemma" for the orphan — so neither is
// distinguishable from anything else by name alone. A broker is identifiable
// only by what it was told to run, which lives in argv.
//
// This was caught by comparing the check against the live machine rather than
// against its own tests: the unit fixtures supplied full argv strings the real
// census never produces, so they passed while the check found nothing.
func FindModelBrokers(p platform.Platform) ([]BrokerProc, error) {
	out, err := p.Command("ps", "-axo", "pid=,args=")
	if err != nil {
		return nil, err
	}
	var brokers []BrokerProc
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp <= 0 {
			continue
		}
		pid, perr := strconv.Atoi(line[:sp])
		if perr != nil {
			continue
		}
		argv := strings.TrimSpace(line[sp+1:])
		if !brokerCommand.MatchString(argv) {
			continue
		}
		worst := brokerFootprint(pid)
		// A broker holding a resident model is always GB-SCALE. The floor is a
		// gigabyte, not merely nonzero, for two observed reasons (claude-home,
		// review of #403): a zero footprint is a transient that matched mid-scan
		// (pid 75492 at 0.0 GB against one real broker), and a tens-of-MB match
		// is a CLI wrapper — `sirsi gemma serve --stop`, the sanctioned graceful
		// stop and the process most likely to be running DURING an incident,
		// matches the argv pattern but never holds a model. Counting either
		// produces a phantom duplicate, and an alarm that cries wolf gets the
		// real alarm ignored.
		if worst < brokerFootprintFloor {
			continue
		}
		brokers = append(brokers, BrokerProc{PID: pid, Worst: worst, Note: capNote(argv)})
	}
	return brokers, nil
}

// brokerFootprintFloor is the smallest footprint that can be a resident model.
// A 4-bit 3B model is still >1.5 GB resident; no CLI wrapper reaches 1 GB.
const brokerFootprintFloor = int64(1) << 30

// brokerCommand matches a process that is SERVING a model, however it was
// started. Deliberately not anchored to gemma-capped-server.py or to the
// configured port: the orphan matched neither.
// The list is broad ON PURPOSE and must keep growing: this check exists because
// a guard that only inspects the thing it launched shares the blind spot of the
// bug it catches — and this pattern had that same gap one level up. Observed
// live 2026-08-03: `omlx-server` held a 23.3 GB peak footprint against 18 MB RSS
// (its whole working set in swap, 90% of the swap file) and matched NOTHING
// here, so the duplicate-broker check reported a clean machine while an unmanaged
// model server ate the swap. An enumeration of names is the weakest part of this
// file; anything that serves a model locally belongs in it.
var brokerCommand = regexp.MustCompile(`(?i)(sirsi-infer(?:ence)?\s+serve|gemma-capped-server|gemma\s+serve|omlx[-_]?server|mlx[-_]?server|mlx_lm\.server|llama[-_]server|ollama\s+serve|vllm\.entrypoints|text-generation-launcher)`)

// capNote marks a broker that carries no memory ceiling — the more dangerous of
// a duplicated pair, because nothing bounds it.
func capNote(argv string) string {
	if strings.Contains(argv, "--prompt-cache-bytes") || hasCapArg(argv) {
		return ""
	}
	return " (uncapped)"
}

// hasCapArg reports whether the command carries a byte-count cap argument — the
// capped broker is invoked as `gemma-capped-server.py <bytes> …`.
func hasCapArg(cmd string) bool {
	for _, f := range strings.Fields(cmd) {
		if len(f) >= 10 && strings.IndexFunc(f, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			return true
		}
	}
	return false
}

// brokerFootprint returns max(footprint, peak) for pid, the number the kernel
// judges. RSS would call a fully swapped-out broker harmless — the orphan read
// 0.1 GB RSS against 14.9 GB footprint.
func brokerFootprint(pid int) int64 {
	fp, err := vitals.PhysFootprint(pid)
	if err != nil {
		return 0
	}
	worst := int64(fp)
	if peak, perr := vitals.PeakPhysFootprint(pid); perr == nil && int64(peak) > worst {
		worst = int64(peak)
	}
	return worst
}
