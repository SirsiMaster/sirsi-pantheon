package updater

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MinimumMaatUpdateVersion reserves the first official release line containing
// the atomic scheduler client. Earlier releases, including 0.24.114, cannot
// write reservations to the fenced canonical service. Publishing this line
// still requires the paired source/artifact qualification; a version is not
// itself a signature or proof of future artifact behavior.
const MinimumMaatUpdateVersion = "0.24.115"

var ErrIncompatibleUpdate = errors.New("update would install an incompatible or non-newer client")

var updateVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)
var rollbackVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ValidateUpdateVersion runs before download/open on both unified install
// aliases. It never treats a bare/dev/malformed version as update authority.
func ValidateUpdateVersion(current, candidate string) error {
	current = strings.TrimPrefix(current, "v")
	candidate = strings.TrimPrefix(candidate, "v")
	if !updateVersionPattern.MatchString(current) || !updateVersionPattern.MatchString(candidate) ||
		compareVersions(candidate, MinimumMaatUpdateVersion) < 0 || compareVersions(candidate, current) <= 0 {
		return fmt.Errorf("%w: current=%q candidate=%q; require a strictly newer release >=%s; installed product preserved", ErrIncompatibleUpdate, current, candidate, MinimumMaatUpdateVersion)
	}
	return nil
}

// ValidateRollbackVersion accepts an explicitly selected commercial release
// older than the exact installed version. Recovery-reader/schema support is
// proved from the exact signed target helper during package inspection.
func ValidateRollbackVersion(current, candidate string) error {
	current = strings.TrimPrefix(current, "v")
	candidate = strings.TrimPrefix(candidate, "v")
	currentParts, currentOK := parseRollbackVersion(current)
	candidateParts, candidateOK := parseRollbackVersion(candidate)
	minimumParts, minimumOK := parseRollbackVersion(MinimumMaatUpdateVersion)
	if !currentOK || !candidateOK || !minimumOK || compareRollbackParts(candidateParts, minimumParts) < 0 || compareRollbackParts(candidateParts, currentParts) >= 0 {
		return fmt.Errorf("%w: current=%q candidate=%q; require an explicitly selected older supported release >=%s; the signed target recovery reader is checked separately", ErrIncompatibleUpdate, current, candidate, MinimumMaatUpdateVersion)
	}
	return nil
}

func parseRollbackVersion(raw string) ([3]uint64, bool) {
	var parts [3]uint64
	if !rollbackVersionPattern.MatchString(raw) {
		return parts, false
	}
	for i, component := range strings.Split(raw, ".") {
		value, err := strconv.ParseUint(component, 10, 64)
		if err != nil {
			return parts, false
		}
		parts[i] = value
	}
	return parts, true
}

func compareRollbackParts(a, b [3]uint64) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
