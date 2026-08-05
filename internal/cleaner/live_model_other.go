//go:build !darwin

package cleaner

// defaultLoadedJobs reports no launchd jobs on platforms without launchd.
//
// This is a genuine empty set, not a swallowed error: there is no launchd here,
// so there is no locally-served model substrate to protect and nothing unknown
// about it. UnknownSubstrate() is therefore empty and deletion proceeds under
// the remaining protections — which is correct, and is why this must be a build
// boundary rather than a caught exec error.
func defaultLoadedJobs() map[string]JobArgs { return nil }
