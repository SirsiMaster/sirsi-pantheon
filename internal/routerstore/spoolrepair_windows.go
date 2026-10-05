//go:build windows

package routerstore

import "fmt"

// RepairSpoolOutbox is intentionally unavailable on Windows until the relay
// has an equivalent handle-relative ACL repair primitive. Returning an explicit
// error is safer than widening permissions through a pathname API.
func RepairSpoolOutbox(spoolRoot, agent string) (SpoolOutboxRepair, error) {
	return SpoolOutboxRepair{}, fmt.Errorf("outbox repair: unavailable on Windows without a handle-relative ACL repair primitive")
}
