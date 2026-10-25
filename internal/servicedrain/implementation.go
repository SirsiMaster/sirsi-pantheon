package servicedrain

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
)

//go:embed maintenance.go maintenance_darwin.go implementation.go custody.go
var maintenanceImplementation embed.FS

func MaintenanceImplementationSHA256() string {
	h := sha256.New()
	for _, name := range []string{"maintenance.go", "maintenance_darwin.go", "implementation.go", "custody.go"} {
		b, _ := maintenanceImplementation.ReadFile(name)
		_, _ = h.Write([]byte(name + "\x00"))
		_, _ = h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}
