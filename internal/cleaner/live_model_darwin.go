//go:build darwin

package cleaner

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// launchd discovery is macOS-only. Keeping it in a build-tagged file rather
// than behind a runtime check means an unsupported platform CANNOT enter
// launchd failure semantics at all: on Linux there is no `launchctl`, so the
// fail-closed path would have read "exec: launchctl: not found" as unknown
// authority and blocked every deletion on a platform the cleaner supports.
// That regression was invisible to the tests because they replace the probe
// (codex-pantheon, PR #493).

func defaultLoadedJobs() map[string]JobArgs {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		// The whole listing failed. Report it against the canonical SNE label
		// so the fail-closed path engages rather than silently seeing no jobs.
		return map[string]JobArgs{canonicalSNELabel: {Err: "launchctl list failed: " + err.Error()}}
	}
	jobs := map[string]JobArgs{}
	uid := strconv.Itoa(os.Getuid())
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		label := fields[len(fields)-1]
		if !strings.HasPrefix(label, "ai.sirsi.") {
			continue
		}
		printed, err := exec.Command("launchctl", "print", "gui/"+uid+"/"+label).Output()
		if err != nil {
			jobs[label] = JobArgs{Err: "launchctl print failed: " + err.Error()}
			continue
		}
		args := parseLaunchctlArguments(string(printed))
		if len(args) == 0 {
			jobs[label] = JobArgs{Err: "launchctl print returned no parseable arguments block"}
			continue
		}
		jobs[label] = JobArgs{Args: args}
	}
	return jobs
}
