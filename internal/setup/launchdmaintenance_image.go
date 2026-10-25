package setup

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
)

type maintenanceStaticCDHashReader func(string) ([]byte, error)
type maintenanceLoadedCDHashReader func(int) ([]byte, error)

func maintenanceLoadedImageIdentity(pid int, kernelStart func(int) (string, error), loadedHash maintenanceLoadedCDHashReader) (MaintenanceImageIdentity, error) {
	var identity MaintenanceImageIdentity
	if pid <= 1 || kernelStart == nil || loadedHash == nil {
		return identity, errors.New("legacy maintenance: exact loaded-image readers and PID required")
	}
	before, err := kernelStart(pid)
	if err != nil {
		return identity, fmt.Errorf("legacy maintenance: selected PID start identity unavailable: %w", err)
	}
	if before == "" {
		return identity, errors.New("legacy maintenance: selected PID start identity unavailable")
	}
	hash, err := loadedHash(pid)
	if err != nil {
		return identity, fmt.Errorf("legacy maintenance: kernel loaded-image CDHash unavailable: %w", err)
	}
	if len(hash) != 20 {
		return identity, errors.New("legacy maintenance: kernel loaded-image CDHash has invalid length")
	}
	after, err := kernelStart(pid)
	if err != nil || after == "" || after != before {
		return identity, errors.New("legacy maintenance: selected PID changed during loaded-image observation")
	}
	return MaintenanceImageIdentity{PID: pid, CDHash: hex.EncodeToString(hash), KernelStart: before}, nil
}

// maintenanceImageCDHash binds a retained, strictly validated image to the
// kernel-reported CDHash for pid. pid zero requests the retained image hash
// alone for artifact selection; it never asserts a loaded-image identity.
func maintenanceImageCDHash(path string, pid int, staticHash maintenanceStaticCDHashReader, loadedHash maintenanceLoadedCDHashReader) (string, error) {
	if path == "" || pid < 0 || pid == 1 || staticHash == nil {
		return "", errors.New("legacy maintenance: invalid retained image identity request")
	}
	candidate, err := staticHash(path)
	if err != nil {
		return "", fmt.Errorf("legacy maintenance: retained signed image unavailable: %w", err)
	}
	if len(candidate) != 20 {
		return "", errors.New("legacy maintenance: retained signed image has invalid CDHash length")
	}
	if pid == 0 {
		return hex.EncodeToString(candidate), nil
	}
	if loadedHash == nil {
		return "", errors.New("legacy maintenance: kernel loaded-image CDHash reader unavailable")
	}
	loaded, err := loadedHash(pid)
	if err != nil {
		return "", fmt.Errorf("legacy maintenance: kernel loaded-image CDHash unavailable: %w", err)
	}
	if len(loaded) != 20 || subtle.ConstantTimeCompare(candidate, loaded) != 1 {
		return "", errors.New("legacy maintenance: retained signed image does not match kernel-loaded CDHash")
	}
	return hex.EncodeToString(candidate), nil
}
