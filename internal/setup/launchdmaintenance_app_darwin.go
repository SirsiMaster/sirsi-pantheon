//go:build darwin

package setup

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

// Used only by the closed OS-authorized app entry. It does not replace the
// registered-agent authorizer or interpret a caller's claimed identity as power.
func AuthorizeLegacyAppMaintenance(ctx context.Context, expectedHash string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return authorizeLegacyAppAt(ctx, os.Getuid(), os.Geteuid(), executable, expectedHash,
		func() (string, error) {
			if err := legacyAppProtectedParents(); err != nil {
				return "", err
			}
			_, hash, err := serviceArtifactPolicy(LegacyAppEngine, false, func(fd int) error {
				var st unix.Stat_t
				if err := unix.Fstat(fd, &st); err != nil {
					return err
				}
				if st.Nlink != 1 || !routerstore.MaintenanceMetadataNoACL(fd) {
					return errors.New("maintenance app: protected single-link engine without ACL required")
				}
				return rootRelayArtifactPolicy(false, st)
			})
			return hash, err
		}, func(ctx context.Context) error {
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_, err := rootRelayRead(bounded, nil, "/usr/bin/codesign", "--verify", "--strict", "-R", horusSigningRequirement, LegacyAppEngine)
			return err
		})
}

func legacyAppProtectedParents() error {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	for _, part := range []string{"usr", "local", "libexec", "sirsi-pantheon"} {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if err != nil {
			return err
		}
		fd = next
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 || !routerstore.MaintenanceMetadataNoACL(fd) {
			_ = unix.Close(fd)
			return errors.New("maintenance app: protected root-owned engine ancestry required")
		}
	}
	return unix.Close(fd)
}

func authorizeLegacyAppAt(ctx context.Context, uid, euid int, executable, expected string, artifact func() (string, error), signature func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uid != 0 || euid != 0 || executable != LegacyAppEngine || !appHex(expected, 32) {
		return errors.New("maintenance app: OS-authorized root and exact protected installed engine required")
	}
	before, err := artifact()
	if err != nil || before != expected {
		return errors.New("maintenance app: installed engine differs from consent; install the normal signed PKG and review again")
	}
	if err := signature(ctx); err != nil {
		return errors.New("maintenance app: installed engine signature unverified; no action")
	}
	after, err := artifact()
	if err != nil || after != before {
		return errors.New("maintenance app: installed engine changed during qualification")
	}
	return ctx.Err()
}
