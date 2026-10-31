//go:build darwin

package servicedrain

import (
	"errors"
	"fmt"
	"os/user"
	"strconv"
)

const (
	maintenanceRelayAccountName = "_sirsipantheon"
	maintenanceRelayGroupName   = "_sirsipantheon"
)

// MaintenanceRelayIdentity resolves the service account and its explicitly
// enrolled group independently. The process primary/effective GID is not the
// group's authority for maintenance spool ownership.
func MaintenanceRelayIdentity() (int, int, error) {
	return maintenanceRelayIdentityWith(user.Lookup, user.LookupGroup, func(u *user.User) ([]string, error) {
		return u.GroupIds()
	})
}

func maintenanceRelayIdentityWith(lookupUser func(string) (*user.User, error), lookupGroup func(string) (*user.Group, error), groupIDs func(*user.User) ([]string, error)) (int, int, error) {
	u, err := lookupUser(maintenanceRelayAccountName)
	if err != nil {
		return 0, 0, fmt.Errorf("legacy maintenance: relay account lookup failed: %w", err)
	}
	if u == nil || u.Username != maintenanceRelayAccountName {
		return 0, 0, errors.New("legacy maintenance: existing relay service account unavailable")
	}
	uid, err := maintenanceIdentityID(u.Uid, false)
	if err != nil {
		return 0, 0, errors.New("legacy maintenance: relay service UID is invalid")
	}
	if _, err := maintenanceIdentityID(u.Gid, true); err != nil {
		return 0, 0, errors.New("legacy maintenance: relay primary GID is invalid")
	}
	g, err := lookupGroup(maintenanceRelayGroupName)
	if err != nil {
		return 0, 0, fmt.Errorf("legacy maintenance: named relay group lookup failed: %w", err)
	}
	if g == nil || g.Name != maintenanceRelayGroupName {
		return 0, 0, errors.New("legacy maintenance: named relay group unavailable")
	}
	gid, err := maintenanceIdentityID(g.Gid, false)
	if err != nil {
		return 0, 0, errors.New("legacy maintenance: named relay GID is invalid")
	}
	ids, err := groupIDs(u)
	if err != nil {
		return 0, 0, fmt.Errorf("legacy maintenance: relay group membership lookup failed: %w", err)
	}
	member := false
	for _, raw := range ids {
		id, err := maintenanceIdentityID(raw, true)
		if err != nil {
			return 0, 0, errors.New("legacy maintenance: relay group membership contains an invalid GID")
		}
		member = member || id == gid
	}
	if !member {
		return 0, 0, errors.New("legacy maintenance: relay account is not a member of the named relay group")
	}
	return uid, gid, nil
}

func maintenanceIdentityID(raw string, allowZero bool) (int, error) {
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || strconv.FormatUint(value, 10) != raw || (!allowZero && value == 0) {
		return 0, errors.New("invalid numeric identity")
	}
	return int(value), nil
}
