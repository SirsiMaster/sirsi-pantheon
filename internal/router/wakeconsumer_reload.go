package router

import "fmt"

// loadWakeConsumer uses the same canonical registry resolver as startup. A
// missing/invalid replacement withdraws capability instead of retaining stale
// executable authority. Call only before first dispatch or after child join.
func loadWakeConsumer(root, agent, thread string) (*ResolvedConsumer, bool, error) {
	reg, err := LoadRegistry(root)
	if err != nil {
		return nil, false, err
	}
	cfg, err := reg.Lookup(agent)
	if err != nil {
		return nil, false, err
	}
	rc, why := ResolveConsumer(*cfg, root)
	if rc == nil {
		// The lead's answer grant is registry policy independent of whether an
		// executable consumer can currently be resolved. Capability stays off.
		return nil, cfg.WorkerAnswers, fmt.Errorf("%s", why)
	}
	bindConsumerThread(rc, thread)
	return rc, cfg.WorkerAnswers, nil
}
