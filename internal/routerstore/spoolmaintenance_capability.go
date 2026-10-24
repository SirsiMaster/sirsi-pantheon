package routerstore

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
)

//go:embed spool.go spooladmission_unix.go spoolmaintenance_darwin.go
var rootMaintenanceImplementation embed.FS

type RootMaintenanceContract struct {
	Schema         int    `json:"schema"`
	Profile        string `json:"profile"`
	Implementation string `json:"implementation_sha256"`
}

// Bind the actual startup/publication source, not a capability label alone.
func RootMaintenanceCapability() RootMaintenanceContract {
	h := sha256.New()
	for _, path := range []string{"spool.go", "spooladmission_unix.go", "spoolmaintenance_darwin.go"} {
		b, _ := rootMaintenanceImplementation.ReadFile(path)
		_, _ = h.Write([]byte(path + "\x00"))
		_, _ = h.Write(b)
	}
	_, _ = h.Write([]byte(servicedrain.MaintenanceImplementationSHA256()))
	return RootMaintenanceContract{1, "root-relay-legacy-maintenance-v1", hex.EncodeToString(h.Sum(nil))}
}
