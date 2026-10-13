package main

import (
	"context"
	"errors"
	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"net/http"
	"time"
)

type gemmaInferenceError = provider.InferenceError

func gemmaBrokerError(resp *http.Response, bearer string) error {
	return provider.BrokerError(resp, bearer)
}
func gemmaBrokerBearer(path string) (string, error) { return provider.BrokerBearer(path) }

// BUSY retries keep the same request, lease and renewal goroutine. No new claim
// or process kill is performed, and cancellation bounds the retry interval.
func gemmaGenerateWhileBusy(ctx context.Context, generate func(context.Context, string) (string, error), prompt string) (string, error) {
	for {
		answer, err := generate(ctx, prompt)
		var broker *gemmaInferenceError
		if !errors.As(err, &broker) || !broker.Busy {
			return answer, err
		}
		t := time.NewTimer(broker.RetryAfter)
		select {
		case <-ctx.Done():
			t.Stop()
			return "", broker
		case <-t.C:
		}
	}
}
