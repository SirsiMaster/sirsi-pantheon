//go:build !darwin

package main

import (
	"errors"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

func reconcilePendingMaatAppUpdate() (bool, error) { return false, nil }

func installAppReleaseWithMaat(*updater.Release) error {
	return errors.New("Ma'at whole-app updates require the native macOS Installer owner")
}
