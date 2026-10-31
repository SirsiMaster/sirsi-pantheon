//go:build !darwin

package main

import (
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

func reconcilePendingMaatAppUpdate() (bool, error) { return false, nil }

func installAppReleaseWithMaat(*updater.Release) error {
	return errors.New("Ma'at whole-app updates require the native macOS Installer owner")
}

func installAppRollbackWithMaat(*updater.Release) error {
	return errors.New("Ma'at whole-app rollback requires the native macOS Installer owner; no action occurred")
}

func appUpdateRecoveryCapability() (maat.AppUpdateRecoveryCapability, error) {
	return maat.AppUpdateRecoveryCapability{}, errors.New("whole-app recovery reader is unavailable outside the native macOS owner")
}

func reconcileMaatAppUpdateReport() (appUpdateReconciliationReport, error) {
	err := errors.New("Ma'at app-attempt readback requires the native macOS owner")
	return appUpdateReconciliationReport{Schema: appUpdateReconciliationSchema, State: "unsupported", Reason: err.Error()}, err
}
