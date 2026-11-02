//go:build !darwin

package maat

import "fmt"

func VerifyCurrentInstalledLifecycleAuthority() (InstalledEngineAuthority, error) {
	return InstalledEngineAuthority{}, fmt.Errorf("installed lifecycle acceptance requires the verified canonical macOS package")
}
