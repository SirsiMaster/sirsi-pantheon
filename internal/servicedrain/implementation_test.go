package servicedrain

import (
	"testing"
	"testing/fstest"
)

func TestMaintenanceImplementationHashBindsRelayIdentityPolicy(t *testing.T) {
	files := fstest.MapFS{
		"maintenance.go":                 {Data: []byte("maintenance")},
		"maintenance_darwin.go":          {Data: []byte("darwin")},
		"maintenance_identity_darwin.go": {Data: []byte("named-group-policy-v1")},
		"implementation.go":              {Data: []byte("hash")},
		"custody.go":                     {Data: []byte("custody")},
	}
	before := maintenanceImplementationHash(files)
	files["maintenance_identity_darwin.go"] = &fstest.MapFile{Data: []byte("named-group-policy-v2")}
	if after := maintenanceImplementationHash(files); after == before {
		t.Fatal("maintenance identity policy change did not change the capability fingerprint")
	}
}
