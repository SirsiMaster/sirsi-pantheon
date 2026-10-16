### Fixed
- CompassPoint's two existing backstops use separate worktrees rather than the dirty attended checkout. Sandboxed workers can retain their own router session cache with `SIRSI_ROUTER_SESSION_DIR`, without write access to other lanes' session caches. Backend and agent/thread/runtime authentication are unchanged.
- Native service actors no longer generate misleading missing-LLM-worker alerts. Their actual service health remains separate, and unknown/model identities still escalate.
