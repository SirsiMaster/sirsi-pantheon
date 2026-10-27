//go:build !darwin

package setup

import (
	"context"
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func WriteGemmaWorkerPauseAck(context.Context, string, string) error {
	return errors.New("Gemma lifecycle requires native macOS")
}
func NativeGemmaWorkerLifecycle(context.Context, string, string, string, string, *GemmaWorkerPlan, func(context.Context, string) error, func(context.Context, routerstore.LaneClaimBarrierRequest) (routerstore.LaneClaimBarrierResult, error)) (GemmaWorkerPlan, error) {
	return GemmaWorkerPlan{}, errors.New("Gemma lifecycle requires native macOS")
}
