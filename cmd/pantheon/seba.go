package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

var (
	sebaOutputPath string
	sebaNoBrowser  bool
)

var sebaCmd = &cobra.Command{
	Use:   "seba",
	Short: "𓇼 Visualize your infrastructure as an interactive network graph",
	Long: `𓇼 Seba — The Star Map

Named after the ancient Egyptian word for "star" and "gateway."
Seba maps the constellation of your infrastructure.

Generate an interactive visual map of your workstation's infrastructure:
assets, caches, ghost apps, running processes, containers, and more.

Opens a self-contained HTML file in your browser with a WebGL-powered
force-directed graph. Hover nodes for details, scroll to zoom, drag to pan.

  pantheon seba                    Generate map and open in browser
  pantheon seba --output map.html  Save to a specific file
  pantheon seba --json             Export graph data as JSON

Uses Sigma.js + Graphology (MIT licensed) for visualization.
No external services. Everything runs locally.`,
	Run: runSeba,
}

func init() {
	sebaCmd.Flags().StringVarP(&sebaOutputPath, "output", "o", "", "Output HTML file path (default: ~/.config/pantheon/seba.html)")
	sebaCmd.Flags().BoolVar(&sebaNoBrowser, "no-open", false, "Don't auto-open the map in the browser")
}

func runSeba(cmd *cobra.Command, args []string) {
	output.Header("𓇼 Seba — Building Infrastructure Graph")
	fmt.Println()

	graph := seba.NewGraph()

	// 1. Add the workstation as the central node
	hostname, _ := os.Hostname()
	graph.AddNode("workstation", fmt.Sprintf("🖥 %s", hostname), seba.NodeDevice)

	// 2. Scan for caches/artifacts and add as nodes
	output.Info("Scanning file system...")
	engine := jackal.NewEngine()
	allRules := rules.AllRules()
	for _, r := range allRules {
		engine.Register(r)
	}

	scanResult, _ := engine.Scan(context.Background(), jackal.ScanOptions{})
	if scanResult != nil {
		for _, f := range scanResult.Findings {
			nodeID := fmt.Sprintf("cache_%s", f.RuleName)
			label := fmt.Sprintf("%s (%s)", f.Description, jackal.FormatSize(f.SizeBytes))
			graph.AddNode(nodeID, label, seba.NodeCache)
			graph.AddEdge("workstation", nodeID, "has cache")
		}
		output.Info(fmt.Sprintf("  Found %d cache/artifact nodes", len(scanResult.Findings)))
	}

	// 3. Add running processes as nodes
	output.Info("Scanning processes...")
	auditResult, err := guard.Audit()
	if err == nil {
		for _, g := range auditResult.Groups {
			if g.TotalRSS < 10*1024*1024 {
				continue
			}
			nodeID := fmt.Sprintf("proc_%s", g.Name)
			label := fmt.Sprintf("⚡ %s (%d procs, %s)", g.Name, g.TotalCount, guard.FormatBytes(g.TotalRSS))
			graph.AddNode(nodeID, label, seba.NodeProcess)
			graph.AddEdge("workstation", nodeID, "running")
		}

		for _, o := range auditResult.Orphans {
			nodeID := fmt.Sprintf("orphan_%d", o.PID)
			label := fmt.Sprintf("👻 %s (PID %d, %s)", o.Name, o.PID, guard.FormatBytes(o.RSS))
			graph.AddNode(nodeID, label, seba.NodeGhost)
			parentGroup := fmt.Sprintf("proc_%s", o.Group)
			graph.AddEdge(parentGroup, nodeID, "orphan")
		}
		output.Info(fmt.Sprintf("  Found %d process groups, %d orphans", len(auditResult.Groups), len(auditResult.Orphans)))
	}

	// 4. Detect Docker
	if dockerRunning() {
		graph.AddNode("docker", "🐳 Docker Desktop", seba.NodeService)
		graph.AddEdge("workstation", "docker", "runs")

		containers := getDockerContainers()
		for _, c := range containers {
			graph.AddNode(c.id, fmt.Sprintf("📦 %s", c.name), seba.NodeContainer)
			graph.AddEdge("docker", c.id, "container")
		}
		if len(containers) > 0 {
			output.Info(fmt.Sprintf("  Found %d Docker containers", len(containers)))
		}
	}

	// 5. Detect volumes
	volumes := getMountedVolumes()
	for _, v := range volumes {
		graph.AddNode(v.id, fmt.Sprintf("💾 %s", v.name), seba.NodeVolume)
		graph.AddEdge("workstation", v.id, "mounted")
	}
	if len(volumes) > 0 {
		output.Info(fmt.Sprintf("  Found %d mounted volumes", len(volumes)))
	}

	// 6. Network interfaces
	if ifaces := getNetworkInterfaces(); len(ifaces) > 0 {
		graph.AddNode("network", "🌐 Network", seba.NodeNetwork)
		graph.AddEdge("workstation", "network", "connected")
		for _, iface := range ifaces {
			graph.AddNode(iface.id, fmt.Sprintf("🔗 %s", iface.name), seba.NodeNetwork)
			graph.AddEdge("network", iface.id, "interface")
		}
		output.Info(fmt.Sprintf("  Found %d network interfaces", len(ifaces)))
	}

	fmt.Println()

	// Output
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(graph)
		return
	}

	// Generate HTML
	outPath := sebaOutputPath
	if outPath == "" {
		home, _ := os.UserHomeDir()
		outPath = filepath.Join(home, ".config", "pantheon", "seba.html")
	}

	if err := graph.RenderHTML(outPath); err != nil {
		output.Error(fmt.Sprintf("Failed to generate map: %v", err))
		os.Exit(1)
	}

	output.Info(fmt.Sprintf("✅ Infrastructure map generated: %s", outPath))
	output.Info(fmt.Sprintf("   Nodes: %d | Edges: %d", len(graph.Nodes), len(graph.Edges)))
	fmt.Println()

	if !sebaNoBrowser {
		openInBrowser(outPath)
	}
}

// Helper types and functions

type containerInfo struct {
	id   string
	name string
}

type volumeInfo struct {
	id   string
	name string
}

type ifaceInfo struct {
	id   string
	name string
}

func dockerRunning() bool {
	_, err := exec.Command("docker", "info").Output()
	return err == nil
}

func getDockerContainers() []containerInfo {
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}").Output()
	if err != nil {
		return nil
	}
	var containers []containerInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			containers = append(containers, containerInfo{
				id:   "container_" + parts[0],
				name: parts[1],
			})
		}
	}
	return containers
}

func getMountedVolumes() []volumeInfo {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil
	}
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		return nil
	}
	var volumes []volumeInfo
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "/dev/") && !strings.Contains(line, "/System/Volumes") {
			fields := strings.Fields(line)
			if len(fields) >= 6 {
				mountPoint := fields[len(fields)-1]
				volumes = append(volumes, volumeInfo{
					id:   "vol_" + strings.ReplaceAll(mountPoint, "/", "_"),
					name: fmt.Sprintf("%s (%s used)", mountPoint, fields[2]),
				})
			}
		}
	}
	return volumes
}

func getNetworkInterfaces() []ifaceInfo {
	out, err := exec.Command("ifconfig", "-l").Output()
	if err != nil {
		return nil
	}
	var ifaces []ifaceInfo
	for _, name := range strings.Fields(strings.TrimSpace(string(out))) {
		if name == "lo0" || strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "awdl") {
			continue
		}
		ifaces = append(ifaces, ifaceInfo{
			id:   "iface_" + name,
			name: name,
		})
	}
	return ifaces
}

func openInBrowser(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		return
	}
	_ = cmd.Start()
}
