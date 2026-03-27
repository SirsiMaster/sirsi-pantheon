package guard

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
)

// AntiGravity is the self-monitoring deity. It ensures Pantheon doesn't
// become the very technical debt it was designed to purge.
type AntiGravity struct {
	MaxMemoryMB int // Threshold for warning (e.g. 1536 for 1.5GB)
}

// CheckSelf inspects the current process for resource leaks or loops.
func (ag *AntiGravity) CheckSelf() error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	pid := os.Getpid()
	// Get memory/CPU via ps (RSS in KB)
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "rss").Output()
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil
	}

	rssKB, err := strconv.Atoi(strings.TrimSpace(lines[1]))
	if err != nil {
		return nil
	}

	rssMB := rssKB / 1024
	logging.Debug("AntiGravity: Current memory footprint", "pid", pid, "rss_mb", rssMB)

	if rssMB > ag.MaxMemoryMB {
		logging.Warn("𓂀 AntiGravity: Pantheon memory pressure critical!", "rss_mb", rssMB, "limit_mb", ag.MaxMemoryMB)

		// Attempt to free memory
		debug.FreeOSMemory()

		// If still too high after GC, we might be leaking
		if rssMB > ag.MaxMemoryMB+512 {
			logging.Error("𓂀 AntiGravity: PANIC — memory leak detected. Invoking self-judgment.", "rss_mb", rssMB)
		}
	}

	return nil
}
