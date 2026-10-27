//go:build darwin

package setup

import (
	"errors"
	"os/user"
	"testing"

	"golang.org/x/sys/unix"
)

func maintenanceIdentityFixture(account *user.User, group *user.Group, ids []string, userErr, groupErr, membershipErr error) (func(string) (*user.User, error), func(string) (*user.Group, error), func(*user.User) ([]string, error)) {
	lookupUser := func(name string) (*user.User, error) {
		if name != maintenanceRelayAccountName {
			return nil, errors.New("unexpected account lookup")
		}
		return account, userErr
	}
	lookupGroup := func(name string) (*user.Group, error) {
		if name != maintenanceRelayGroupName {
			return nil, errors.New("unexpected group lookup")
		}
		return group, groupErr
	}
	groupIDs := func(*user.User) ([]string, error) { return ids, membershipErr }
	return lookupUser, lookupGroup, groupIDs
}

func TestMaintenanceRelayIdentityUsesNamedGroupMembership(t *testing.T) {
	account := &user.User{Username: maintenanceRelayAccountName, Uid: "543", Gid: "20"}
	group := &user.Group{Name: maintenanceRelayGroupName, Gid: "501"}
	lookupUser, lookupGroup, groupIDs := maintenanceIdentityFixture(account, group, []string{"20", "501"}, nil, nil, nil)
	uid, gid, err := maintenanceRelayIdentityWith(lookupUser, lookupGroup, groupIDs)
	if err != nil || uid != 543 || gid != 501 {
		t.Fatalf("relay identity = (%d,%d), %v; want service UID and named group GID despite differing primary GID", uid, gid, err)
	}

	spool := unix.Stat_t{Uid: uint32(uid), Gid: uint32(gid), Mode: unix.S_IFDIR | 0770}
	if !maintenanceSpoolIdentityAllowed(spool, uid, gid, 0, 0) {
		t.Fatal("spool with the explicitly named member group was refused")
	}
	spool.Gid = 20 // The same account's primary group is not the spool trust group.
	if maintenanceSpoolIdentityAllowed(spool, uid, gid, 0, 0) {
		t.Fatal("spool with the account's primary GID was accepted in place of the named group")
	}
}

func TestMaintenanceRelayIdentityRefusesLookupMembershipAndUntrustedGroups(t *testing.T) {
	lookupFailure := errors.New("lookup unavailable")
	membershipFailure := errors.New("membership unavailable")
	tests := []struct {
		name          string
		account       *user.User
		group         *user.Group
		ids           []string
		userErr       error
		groupErr      error
		membershipErr error
	}{
		{name: "user lookup", userErr: lookupFailure},
		{name: "group lookup", account: &user.User{Username: maintenanceRelayAccountName, Uid: "543", Gid: "20"}, groupErr: lookupFailure},
		{name: "membership lookup", account: &user.User{Username: maintenanceRelayAccountName, Uid: "543", Gid: "20"}, group: &user.Group{Name: maintenanceRelayGroupName, Gid: "501"}, membershipErr: membershipFailure},
		{name: "nonmember arbitrary supplementary group", account: &user.User{Username: maintenanceRelayAccountName, Uid: "543", Gid: "20"}, group: &user.Group{Name: maintenanceRelayGroupName, Gid: "501"}, ids: []string{"20", "999"}},
		{name: "noncanonical membership ID", account: &user.User{Username: maintenanceRelayAccountName, Uid: "543", Gid: "20"}, group: &user.Group{Name: maintenanceRelayGroupName, Gid: "501"}, ids: []string{"20", "0501"}},
		{name: "invalid primary ID", account: &user.User{Username: maintenanceRelayAccountName, Uid: "543", Gid: "+20"}, group: &user.Group{Name: maintenanceRelayGroupName, Gid: "501"}, ids: []string{"501"}},
		{name: "invalid named group ID", account: &user.User{Username: maintenanceRelayAccountName, Uid: "543", Gid: "20"}, group: &user.Group{Name: maintenanceRelayGroupName, Gid: "501x"}, ids: []string{"501"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookupUser, lookupGroup, groupIDs := maintenanceIdentityFixture(test.account, test.group, test.ids, test.userErr, test.groupErr, test.membershipErr)
			if uid, gid, err := maintenanceRelayIdentityWith(lookupUser, lookupGroup, groupIDs); err == nil || uid != 0 || gid != 0 {
				t.Fatalf("relay identity = (%d,%d), %v; want refusal", uid, gid, err)
			}
		})
	}
}
