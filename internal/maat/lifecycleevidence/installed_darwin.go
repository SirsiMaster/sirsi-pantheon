//go:build darwin

package lifecycleevidence

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

// NewInstalledRouter requires the canonical packaged engine path and its
// existing Developer-ID requirement before the embedded inventory is used.
func NewInstalledRouter(reader ItemReader) (Router, error) {
	if reader == nil {
		return Router{}, fmt.Errorf("canonical lifecycle reader required")
	}
	authority, err := maat.VerifyCurrentInstalledLifecycleAuthority()
	if err != nil {
		return Router{}, err
	}
	return Router{Reader: reader, authority: authority}, nil
}
