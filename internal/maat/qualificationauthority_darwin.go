//go:build darwin

package maat

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/packagedengine"
)

// VerifyCurrentInstalledLifecycleAuthority returns an opaque token only for
// the canonical installed engine after its existing Developer-ID check.
func VerifyCurrentInstalledLifecycleAuthority() (InstalledEngineAuthority, error) {
	engine, err := packagedengine.Current()
	if err != nil {
		return InstalledEngineAuthority{}, err
	}
	if err := packagedengine.Verify(engine); err != nil {
		return InstalledEngineAuthority{}, err
	}
	return InstalledEngineAuthority{enginePath: engine}, nil
}
