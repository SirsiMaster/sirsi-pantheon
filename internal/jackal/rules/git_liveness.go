package rules

import "time"

// defaultNowUnix returns the current Unix time. Split into its own file so
// it doesn't conflict with the package-level `nowUnix` indirection used by
// git.go (which exists so tests can swap the clock without reaching into
// the time package). Production callers go through nowUnix(); tests rebind
// nowUnix to a fake.
func defaultNowUnix() int64 { return time.Now().Unix() }
