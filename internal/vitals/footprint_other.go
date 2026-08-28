//go:build !darwin || !cgo

package vitals

import "errors"

// errNoFootprint is returned on platforms without a phys_footprint equivalent.
// Callers degrade to RSS and must SAY SO rather than silently reporting a
// number that means something different.
var errNoFootprint = errors.New("phys_footprint unavailable on this platform")

func PhysFootprint(int) (uint64, error)     { return 0, errNoFootprint }
func PeakPhysFootprint(int) (uint64, error) { return 0, errNoFootprint }
