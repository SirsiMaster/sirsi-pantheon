//go:build darwin && !cgo

package routerstore

// Without the supported libSystem bridge, absence of an extended ACL cannot
// be established. Refuse observation/publication; never weaken the ACL policy.
func relayObservationNoACL(fd int) bool { return false }
