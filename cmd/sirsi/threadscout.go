package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var threadScoutLimit int

var threadScoutCmd = &cobra.Command{
	Use:   "scout",
	Short: "Scout visible OS processes into Pantheon's process registry",
	Long: `Scout the local computer's visible process table into the router ledger.

This is read-only process awareness. It records PID, PPID, user, command,
memory/CPU, and a coarse role (agent, ide, terminal, system, process) in
.agents/idea-router/processes.json. It does not kill, renice, steer, or claim
router work for arbitrary processes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		reg, err := runThreadScout(routerRoot)
		if err != nil {
			return err
		}
		return renderScout(reg, threadScoutLimit)
	},
}

// runThreadScout is the native process-ledger pass shared by the interactive
// command and registry police. It records visible processes only; it does not
// kill, renice, steer, or otherwise act on them.
func runThreadScout(routerRoot string) (*router.ProcessRegistry, error) {
	host, _ := os.Hostname()
	prev, err := router.LoadProcessRegistry(routerRoot)
	if err != nil {
		return nil, err
	}
	visible, err := enumerateVisibleProcesses()
	if err != nil {
		return nil, err
	}
	reg := router.ReconcileProcessRegistry(prev, visible, host, time.Now().UTC())
	if err := router.SaveProcessRegistry(routerRoot, reg); err != nil {
		return nil, err
	}
	return reg, nil
}

func enumerateVisibleProcesses() ([]router.ProcessRecord, error) {
	out, err := exec.Command("ps", "-axo", "pid,ppid,rss,vsz,%cpu,user,comm").Output()
	if err != nil {
		return nil, fmt.Errorf("ps process scout: %w", err)
	}
	var procs []router.ProcessRecord
	for i, line := range strings.Split(string(out), "\n") {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		rss, _ := strconv.ParseInt(fields[2], 10, 64)
		vsz, _ := strconv.ParseInt(fields[3], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[4], 64)
		user := fields[5]
		comm := strings.Join(fields[6:], " ")
		name := comm
		if idx := strings.LastIndex(comm, "/"); idx >= 0 {
			name = comm[idx+1:]
		}
		procs = append(procs, router.ProcessRecord{
			PID:        pid,
			PPID:       ppid,
			Name:       name,
			Command:    comm,
			User:       user,
			RSS:        rss * 1024,
			VSZ:        vsz * 1024,
			CPUPercent: cpu,
			Role:       router.ClassifyProcessRole(name, comm),
		})
	}
	return procs, nil
}

type scoutReport struct {
	Host      string                     `json:"host"`
	Visible   int                        `json:"visible"`
	Gone      int                        `json:"gone"`
	ByRole    map[router.ProcessRole]int `json:"by_role"`
	Processes []*router.ProcessRecord    `json:"processes,omitempty"`
}

func renderScout(reg *router.ProcessRegistry, limit int) error {
	rows := reg.SortedProcessRecords()
	rep := scoutReport{Host: reg.Host, ByRole: map[router.ProcessRole]int{}}
	for _, p := range rows {
		if p.Status == "gone" {
			rep.Gone++
			continue
		}
		rep.Visible++
		rep.ByRole[p.Role]++
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	rep.Processes = rows

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	output.Header("CTR — Process Scout")
	fmt.Println()
	fmt.Printf("  host=%s  visible=%d  gone=%d\n", rep.Host, rep.Visible, rep.Gone)
	roles := make([]string, 0, len(rep.ByRole))
	for role := range rep.ByRole {
		roles = append(roles, string(role))
	}
	sort.Strings(roles)
	for _, role := range roles {
		fmt.Printf("  %-9s %d\n", role+":", rep.ByRole[router.ProcessRole(role)])
	}
	if limit == 0 {
		return nil
	}
	fmt.Println()
	for _, p := range rows {
		if p.Status != "visible" {
			continue
		}
		fmt.Printf("  %-8s pid=%-6d ppid=%-6d rss=%-8s %s\n",
			p.Role, p.PID, p.PPID, formatBytesLocal(p.RSS), p.Name)
	}
	return nil
}

func formatBytesLocal(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1fGB", float64(bytes)/gb)
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/mb)
	case bytes >= kb:
		return fmt.Sprintf("%.1fKB", float64(bytes)/kb)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func init() {
	threadScoutCmd.Flags().IntVar(&threadScoutLimit, "limit", 25, "Number of process rows to print in human output; 0 prints only summary")
	threadCmd.AddCommand(threadScoutCmd)
}
