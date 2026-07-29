package scarab

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Container represents a Docker container.
type Container struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Image        string `json:"image"`
	Status       string `json:"status"`
	Size         string `json:"size,omitempty"`
	Ports        string `json:"ports,omitempty"`
	Running      bool   `json:"running"`
	Health       string `json:"health,omitempty"` // healthy|unhealthy|starting|unknown|not-running
	HealthSource string `json:"health_source,omitempty"`
}

// ContainerAudit contains Docker/container scan results.
type ContainerAudit struct {
	Containers     []Container `json:"containers"`
	DanglingImages int         `json:"dangling_images"`
	StoppedCount   int         `json:"stopped_count"`
	RunningCount   int         `json:"running_count"`
	HealthyCount   int         `json:"healthy_count"`
	UnhealthyCount int         `json:"unhealthy_count"`
	UnknownHealth  int         `json:"unknown_health_count"`
	UnusedVolumes  int         `json:"unused_volumes"`
	// RetainedVolumes are dangling volumes that hold irreplaceable state and are
	// deliberately NOT counted as reclaimable. Surfaced separately so an operator
	// sees they exist and sees that they were protected on purpose.
	RetainedVolumes int  `json:"retained_volumes"`
	DockerRunning   bool `json:"docker_running"`
}

// AuditContainers scans the local Docker environment using the current platform.
func AuditContainers() (*ContainerAudit, error) {
	return AuditContainersWith(platform.Current())
}

// AuditContainersWith scans the local Docker environment using the provided platform (Rule A16).
// Docker PS, Images, and Volumes run concurrently on dedicated OS threads.
func AuditContainersWith(p platform.Platform) (*ContainerAudit, error) {
	audit := &ContainerAudit{}

	// Check Docker is running
	_, err := p.Command("docker", "info")
	if err != nil {
		return audit, nil // Docker not running — not an error
	}
	audit.DockerRunning = true

	// Run all Docker queries concurrently on dedicated threads.
	var psOut, imgOut, volOut []byte
	var psErr, imgErr, volErr error
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		psOut, psErr = p.Command("docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
	}()
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		imgOut, imgErr = p.Command("docker", "images", "-f", "dangling=true", "-q")
	}()
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		volOut, volErr = p.Command("docker", "volume", "ls", "-f", "dangling=true", "--format", "{{.Name}}\t{{.Labels}}")
	}()
	wg.Wait()

	// Process results
	if psErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(psOut)), "\n") {
			c := splitContainerLine(line)
			if c == nil {
				continue
			}
			if c.Running {
				audit.RunningCount++
				switch c.Health {
				case "healthy":
					audit.HealthyCount++
				case "unhealthy":
					audit.UnhealthyCount++
				default:
					audit.UnknownHealth++
				}
			} else {
				audit.StoppedCount++
			}
			audit.Containers = append(audit.Containers, *c)
		}
	}

	if imgErr == nil {
		audit.DanglingImages = countNonEmptyLines(strings.TrimSpace(string(imgOut)))
	}

	if volErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(volOut)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if isRetainedVolume(line) {
				audit.RetainedVolumes++
				continue
			}
			audit.UnusedVolumes++
		}
	}

	return audit, nil
}

// retainedVolumeMarkers name the container/compose projects whose volumes hold
// state that cannot be regenerated. The hedera-local consensus ledger lives in
// an ANONYMOUS docker volume inside Colima: once its container is removed the
// volume goes dangling and every reclamation surface would otherwise advertise
// it as free space. It is not free space — it is the sovereign node's ledger,
// and `hedera-local stop` already destroys it once (it runs `down -v`), which is
// why a 108 MB emergency snapshot exists at
// ~/.sirsi/hypergraph/ledger-backup/. Prevention, not a second snapshot.
//
// Matched against "<name>\t<labels>" so BOTH a named volume and an anonymous
// one carrying compose labels (com.docker.compose.project=hedera-local) are
// caught — an anonymous volume's hash name says nothing on its own.
var retainedVolumeMarkers = []string{"hedera", "network-node", "mirror-node"}

// isRetainedVolume reports whether a `docker volume ls` line describes state we
// must never offer for reclamation. Case-insensitive, because compose project
// labels are not case-normalized.
//
// This is fail-OPEN, and that is worth stating plainly rather than dressing up:
// a volume whose record is unrecognized or malformed stays RECLAIMABLE. Making
// it fail-closed would mean treating every unclassifiable volume as protected,
// which turns the reclaimable count into noise and trains operators to ignore
// it. The mitigation for the unknown-volume case is the enforcement boundary in
// configs/default_rules.yaml (no `--volumes` on the advertised prune), not this
// predicate.
func isRetainedVolume(line string) bool {
	l := strings.ToLower(line)
	for _, m := range retainedVolumeMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}

// splitContainerLine parses a single tab-delimited docker ps output line.
func splitContainerLine(line string) *Container {
	if line == "" {
		return nil
	}
	parts := strings.SplitN(line, "\t", 5)
	if len(parts) < 4 {
		return nil
	}
	c := &Container{
		ID:           parts[0],
		Name:         parts[1],
		Image:        parts[2],
		Status:       parts[3],
		Running:      strings.HasPrefix(parts[3], "Up"),
		Health:       containerHealth(parts[3]),
		HealthSource: "docker-status",
	}
	if len(parts) >= 5 {
		c.Ports = strings.TrimSpace(parts[4])
	}
	return c
}

func containerHealth(status string) string {
	lower := strings.ToLower(status)
	switch {
	case !strings.HasPrefix(status, "Up"):
		return "not-running"
	case strings.Contains(lower, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(lower, "(healthy)"):
		return "healthy"
	case strings.Contains(lower, "(health: starting)"):
		return "starting"
	default:
		return "unknown"
	}
}

// countNonEmptyLines counts non-blank lines in a string.
func countNonEmptyLines(s string) int {
	if s == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// FormatContainerStatus returns a styled status string.
func FormatContainerStatus(c Container) string {
	if !c.Running {
		return fmt.Sprintf("🔴 %s", c.Status)
	}
	switch c.Health {
	case "unhealthy":
		return fmt.Sprintf("🔴 %s", c.Status)
	case "healthy":
		return fmt.Sprintf("🟡 %s — docker reachable; workload correctness unverified", c.Status)
	case "starting":
		return fmt.Sprintf("🟡 %s", c.Status)
	default:
		return fmt.Sprintf("🟡 %s — running; no Docker healthcheck", c.Status)
	}
}
