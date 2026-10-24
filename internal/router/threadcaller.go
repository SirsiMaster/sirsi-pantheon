package router

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// CallerThreadLifecycle retains registration's backend selection. In durable
// mode it binds calls to the same explicit audience as the caller's tools;
// legacy mode retains JSON compatibility, not RPC isolation. It adds no lifecycle policy: the
// bounded read, CAS save, heartbeat guards and close guards are shared above.
// The returned functions own each resolved handle, not credentials or a cache.
func CallerThreadLifecycle(agent, threadID string) (
	heartbeat func(string, string, HeartbeatUpdate) (*Thread, error),
	closeThread func(string, string) (*Thread, error),
) {
	// Match registration's existing backend, without forcing a JSON session to
	// cut over. Captured at binding so this connection's lifecycle is consistent.
	legacy := !routercfg.StoreWake()
	withStore := func(run func(routerstore.Store) (*Thread, error)) (*Thread, error) {
		store, err := routerstore.ResolveForCaller(agent, threadID)
		if err != nil {
			return nil, err
		}
		defer func() { _ = store.Close() }()
		return run(store)
	}
	registry := func(store routerstore.Store) (func(string, string) (*ThreadRegistry, error), func(string, *ThreadRegistry) error) {
		return func(root, id string) (*ThreadRegistry, error) { return loadThreadRegistryOneStore(store, id) },
			func(_ string, reg *ThreadRegistry) error { return saveThreadRegistryStore(reg, store) }
	}
	heartbeat = func(root, id string, update HeartbeatUpdate) (*Thread, error) {
		if id != threadID {
			return nil, fmt.Errorf("lifecycle target differs from bound caller")
		}
		if legacy {
			return heartbeatWithRegistry(root, id, update,
				func(root, _ string) (*ThreadRegistry, error) { return loadLegacyThreadRegistry(root) }, saveLegacyThreadRegistry)
		}
		return withStore(func(store routerstore.Store) (*Thread, error) {
			load, save := registry(store)
			return heartbeatWithRegistry(root, id, update, load, save)
		})
	}
	closeThread = func(root, id string) (*Thread, error) {
		if id != threadID {
			return nil, fmt.Errorf("lifecycle target differs from bound caller")
		}
		if legacy {
			return closeThreadWithRegistry(root, id,
				func(root, _ string) (*ThreadRegistry, error) { return loadLegacyThreadRegistry(root) }, saveLegacyThreadRegistry)
		}
		return withStore(func(store routerstore.Store) (*Thread, error) {
			load, save := registry(store)
			return closeThreadWithRegistry(root, id, load, save)
		})
	}
	return heartbeat, closeThread
}
