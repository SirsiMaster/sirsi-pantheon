package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/guard"
	"github.com/SirsiMaster/sirsi-anubis/internal/jackal"
	"github.com/SirsiMaster/sirsi-anubis/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-anubis/internal/ka"
)

var bookVerbose bool

var bookCmd = &cobra.Command{
	Use:    "book-of-the-dead",
	Short:  "𓁿 The complete system autopsy — for initiates only",
	Hidden: true, // Easter egg — not in --help
	Long: `𓁿 The Book of the Dead — System Autopsy

"I have come forth by day. I have weighed the heart.
I have measured the fields. I know the names of the gods."

A deep, comprehensive report of your workstation's full state:
disk, RAM, GPU, processes, ghosts, caches, network, and more.

This is the ritual of the dead — a complete accounting.`,
	Run: runBookOfTheDead,
}

func init() {
	bookCmd.Flags().BoolVarP(&bookVerbose, "verbose", "v", false, "Extended detail for each section")
}

func runBookOfTheDead(cmd *cobra.Command, args []string) {
	start := time.Now()

	printPapyrus("╔══════════════════════════════════════════════════════════════╗")
	printPapyrus("║                                                              ║")
	printPapyrus("║          𓁿  THE BOOK OF THE DEAD  𓁿                          ║")
	printPapyrus("║                                                              ║")
	printPapyrus("║     \"I have come forth by day. I have weighed the heart.\"    ║")
	printPapyrus("║                                                              ║")
	printPapyrus("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ═══════════════════════════════════════
	// CHAPTER I — THE VESSEL (Hardware)
	// ═══════════════════════════════════════
	printChapter("I", "THE VESSEL", "Hardware & Platform")

	hostname, _ := os.Hostname()
	printField("Host", hostname)
	printField("OS", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	printField("CPUs", fmt.Sprintf("%d cores", runtime.NumCPU()))
	printField("Go Runtime", runtime.Version())

	// Chip info (macOS)
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			printField("Processor", strings.TrimSpace(string(out)))
		}
		if out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Chipset Model:") {
					printField("GPU", strings.TrimPrefix(line, "Chipset Model: "))
				}
				if strings.HasPrefix(line, "Metal Support:") || strings.HasPrefix(line, "Metal Family:") {
					printField("Metal", strings.TrimPrefix(strings.TrimPrefix(line, "Metal Support: "), "Metal Family: "))
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════
	// CHAPTER II — THE BREATH (Memory)
	// ═══════════════════════════════════════
	printChapter("II", "THE BREATH", "Memory & RAM Pressure")

	auditResult, err := guard.Audit()
	if err == nil {
		printField("Total RAM", guard.FormatBytes(auditResult.TotalRAM))
		printField("Used", guard.FormatBytes(auditResult.UsedRAM))
		printField("Free", guard.FormatBytes(auditResult.FreeRAM))

		usedPct := float64(auditResult.UsedRAM) / float64(auditResult.TotalRAM) * 100
		if usedPct > 85 {
			printField("⚠ Pressure", fmt.Sprintf("%.0f%% — CRITICAL", usedPct))
		} else if usedPct > 70 {
			printField("⚡ Pressure", fmt.Sprintf("%.0f%% — elevated", usedPct))
		} else {
			printField("✅ Pressure", fmt.Sprintf("%.0f%% — healthy", usedPct))
		}

		if bookVerbose {
			fmt.Println()
			printSubheader("Process Groups")
			for _, g := range auditResult.Groups {
				if g.TotalRSS < 5*1024*1024 {
					continue
				}
				fmt.Printf("    %-16s  %3d procs  %s\n", g.Name, g.TotalCount, guard.FormatBytes(g.TotalRSS))
			}
		}

		if auditResult.TotalOrphans > 0 {
			fmt.Println()
			printField("👻 Orphans", fmt.Sprintf("%d processes using %s",
				auditResult.TotalOrphans, guard.FormatBytes(auditResult.OrphanRSS)))
			for _, o := range auditResult.Orphans {
				fmt.Printf("    PID %-6d  %-25s  %s  [%s]\n", o.PID, truncate(o.Name, 25), guard.FormatBytes(o.RSS), o.Group)
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════
	// CHAPTER III — THE FIELDS (Disk & Storage)
	// ═══════════════════════════════════════
	printChapter("III", "THE FIELDS", "Disk & Storage")

	if out, err := exec.Command("df", "-h", "/").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "/dev/") {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					printField("Boot Volume", fields[0])
					printField("Total", fields[1])
					printField("Used", fmt.Sprintf("%s (%s)", fields[2], fields[4]))
					printField("Free", fields[3])
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════
	// CHAPTER IV — THE WEIGHING (Scan Results)
	// ═══════════════════════════════════════
	printChapter("IV", "THE WEIGHING", "Infrastructure Waste")

	engine := jackal.NewEngine()
	for _, r := range rules.AllRules() {
		engine.Register(r)
	}
	scanResult, _ := engine.Scan(context.Background(), jackal.ScanOptions{})
	if scanResult != nil {
		printField("Rules Matched", fmt.Sprintf("%d", scanResult.RulesWithFindings))
		printField("Findings", fmt.Sprintf("%d", len(scanResult.Findings)))
		printField("Total Waste", jackal.FormatSize(scanResult.TotalSize))

		if bookVerbose {
			fmt.Println()
			printSubheader("Top Findings")
			limit := 15
			if len(scanResult.Findings) < limit {
				limit = len(scanResult.Findings)
			}
			for _, f := range scanResult.Findings[:limit] {
				fmt.Printf("    %-35s  %s\n", truncate(f.Description, 35), jackal.FormatSize(f.SizeBytes))
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════
	// CHAPTER V — THE SPIRITS (Ghost Apps)
	// ═══════════════════════════════════════
	printChapter("V", "THE SPIRITS", "Ghost Applications")

	scanner := ka.NewScanner()
	ghosts, _ := scanner.Scan(false)
	printField("Ghost Apps", fmt.Sprintf("%d detected", len(ghosts)))
	if bookVerbose && len(ghosts) > 0 {
		limit := 10
		if len(ghosts) < limit {
			limit = len(ghosts)
		}
		for _, g := range ghosts[:limit] {
			fmt.Printf("    👻 %s (%s)\n", g.AppName, g.BundleID)
		}
		if len(ghosts) > limit {
			fmt.Printf("    ... and %d more\n", len(ghosts)-limit)
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════
	// CHAPTER VI — THE GATES (Network)
	// ═══════════════════════════════════════
	printChapter("VI", "THE GATES", "Network & Connectivity")

	if out, err := exec.Command("ifconfig", "-l").Output(); err == nil {
		ifaces := strings.Fields(strings.TrimSpace(string(out)))
		active := 0
		for _, name := range ifaces {
			if name == "lo0" || strings.HasPrefix(name, "utun") {
				continue
			}
			active++
		}
		printField("Interfaces", fmt.Sprintf("%d active (of %d total)", active, len(ifaces)))
	}

	// External IP (if available quickly)
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("ipconfig", "getifaddr", "en0").Output(); err == nil {
			printField("Local IP", strings.TrimSpace(string(out)))
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════
	// CHAPTER VII — THE JUDGMENT
	// ═══════════════════════════════════════
	printChapter("VII", "THE JUDGMENT", "Verdict")

	elapsed := time.Since(start)
	fmt.Println()

	verdicts := 0
	if scanResult != nil && scanResult.TotalSize > 10*1024*1024*1024 {
		printVerdict("⚖️", "HEAVY — infrastructure waste exceeds 10 GB")
		verdicts++
	}
	if auditResult != nil && auditResult.TotalOrphans > 3 {
		printVerdict("👻", fmt.Sprintf("HAUNTED — %d orphan processes consuming %s",
			auditResult.TotalOrphans, guard.FormatBytes(auditResult.OrphanRSS)))
		verdicts++
	}
	if len(ghosts) > 10 {
		printVerdict("𓂓", fmt.Sprintf("RESTLESS — %d ghost apps detected", len(ghosts)))
		verdicts++
	}
	if verdicts == 0 {
		printVerdict("𓂀", "PURE — the heart is lighter than the feather of Ma'at")
	}

	fmt.Println()
	printPapyrus("────────────────────────────────────────────────────────")
	printPapyrus(fmt.Sprintf("  Autopsy completed in %s", elapsed.Round(time.Millisecond)))
	printPapyrus("  To perform this ritual across 100+ nodes,")
	printPapyrus("  connect to the Sirsi Altar: https://sirsi.dev")
	printPapyrus("────────────────────────────────────────────────────────")
	fmt.Println()
}

func printPapyrus(text string) {
	// Gold on dark — uses output module
	fmt.Printf("  %s\n", text)
}

func printChapter(num, title, subtitle string) {
	fmt.Printf("  ═══ Chapter %s: %s ═══\n", num, title)
	fmt.Printf("  %s\n\n", subtitle)
}

func printField(label, value string) {
	fmt.Printf("    %-18s  %s\n", label+":", value)
}

func printSubheader(text string) {
	fmt.Printf("    ── %s ──\n", text)
}

func printVerdict(icon, text string) {
	fmt.Printf("    %s  %s\n", icon, text)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
