### Fixed

- Memory-relief previews distinguish current system pressure from the largest process sample and clarify that cache flushing does not release active app/model memory or recommend stopping a workload.

Refs: `cmd/sirsi/relieve.go`, `cmd/sirsi/relieve_purge_test.go`, `docs/user-guides/relieve.md`
