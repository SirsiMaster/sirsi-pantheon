package guard

import (
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// processDisplayName turns an implementation-runtime name into the service the
// operator actually owns. The process census reads `comm`, so MLX's sanctioned
// Gemma broker otherwise appears as the frightening and context-free "Python".
// Identity is proved from argv with the same broker signature used by the
// duplicate-broker safety check; an unrelated Python process stays Python.
func processDisplayName(p platform.Platform, proc ProcessInfo) string {
	if !strings.EqualFold(proc.Name, "python") && !strings.EqualFold(proc.Name, "python3") {
		return proc.Name
	}
	out, err := p.Command("ps", "-p", strconv.Itoa(proc.PID), "-o", "args=")
	if err == nil && brokerCommand.Match(out) {
		return "Gemma local AI broker"
	}
	return proc.Name
}
