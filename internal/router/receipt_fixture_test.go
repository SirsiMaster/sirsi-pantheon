package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// receiptFixture lets this package's tests complete tasks with
// pr:SirsiMaster/fixture#N receipts; they test leases, not receipts.
type receiptFixture struct{}

func (receiptFixture) Verify(_ context.Context, kind, value string) error {
	if kind == "pr" && strings.HasPrefix(value, "SirsiMaster/fixture#") {
		return nil
	}
	return fmt.Errorf("%w: fixture verifier", routerstore.ErrReceiptUnverifiable)
}

func init() { routerstore.SetReceiptVerifier(receiptFixture{}) }
