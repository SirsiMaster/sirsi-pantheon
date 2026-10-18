package updater

import (
	"errors"
	"fmt"
	"regexp"
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
