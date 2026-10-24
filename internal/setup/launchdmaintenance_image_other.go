//go:build !darwin

package setup

import "errors"

func InspectRootMaintenanceImage(string, int) (MaintenanceImageIdentity, error) {
	return MaintenanceImageIdentity{}, errors.New("legacy maintenance: native Darwin image inspection required")
}
