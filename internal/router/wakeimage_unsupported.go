//go:build (!darwin && !linux) || (darwin && !cgo)

package router

import "fmt"

func nativeWakeImageIdentity() (wakeImageIdentity, error) {
	return wakeImageIdentity{}, fmt.Errorf("loaded wake executable identity unavailable on this build")
}
