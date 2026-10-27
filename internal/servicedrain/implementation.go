package servicedrain

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
)

//go:embed maintenance.go maintenance_darwin.go maintenance_identity_darwin.go implementation.go custody.go
var maintenanceImplementation embed.FS

func MaintenanceImplementationSHA256() string {
	return maintenanceImplementationHash(maintenanceImplementation)
}

func maintenanceImplementationHash(files fs.FS) string {
	h := sha256.New()
	for _, name := range []string{"maintenance.go", "maintenance_darwin.go", "maintenance_identity_darwin.go", "implementation.go", "custody.go"} {
		b, _ := fs.ReadFile(files, name)
		_, _ = h.Write([]byte(name + "\x00"))
		_, _ = h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}
