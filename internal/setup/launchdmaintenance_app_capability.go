package setup

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type LegacyAppCapability struct {
	Schema         int    `json:"schema"`
	Protocol       string `json:"protocol"`
	Release        string `json:"release"`
	Implementation string `json:"implementation"`
	Native         bool   `json:"native"`
}

//go:embed launchdmaintenance_app.go launchdmaintenance_app_darwin.go launchdmaintenance.go launchdmaintenance_restore.go launchdmaintenance_app_admission.go launchdmaintenance_appstate_darwin.go launchdmaintenance_darwin.go launchdmaintenance_journal_darwin.go launchdmaintenance_prepare_darwin.go launchdmaintenance_spool_darwin.go launchdmaintenance_restore_darwin.go
var legacyAppSource embed.FS

func LegacyAppImplementation() string {
	h := sha256.New()
	for _, name := range []string{"launchdmaintenance_app.go", "launchdmaintenance_app_darwin.go", "launchdmaintenance.go", "launchdmaintenance_restore.go", "launchdmaintenance_app_admission.go", "launchdmaintenance_appstate_darwin.go", "launchdmaintenance_darwin.go", "launchdmaintenance_journal_darwin.go", "launchdmaintenance_prepare_darwin.go", "launchdmaintenance_spool_darwin.go", "launchdmaintenance_restore_darwin.go"} {
		data, _ := legacyAppSource.ReadFile(name)
		_, _ = h.Write([]byte(name + "\x00"))
		_, _ = h.Write(data)
	}
	_, _ = h.Write([]byte(routerstore.RootMaintenanceCapability().Implementation))
	return hex.EncodeToString(h.Sum(nil))
}
