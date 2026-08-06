package router

// Wake mechanism identifiers — how a registered agent surface is woken when it
// has pending work. Read by the pull-model paths (node-status, register,
// supervisor) to classify each surface as wakeable/blocked. The push-model wake
// adapters that consumed these (Executor.wake / wakeAPI / wakeMCP) were retired
// with the daemon/executor/runner cluster per A26/A27 (router is pull-model; no
// daemon verbs); only the identifiers remain as the surface-classification enum.
const (
	WakeCLISpawn        = "cli-spawn"
	WakeAPICall         = "api-call"
	WakeMCPNotification = "mcp-notification"
	// WakeLaunchAgent is the worker/headless pull-loop wake: a per-agent
	// LaunchAgent that polls items/ and heartbeats. It registers as a
	// pull-loop watcher (armed by heartbeat freshness), NOT a loop-monitor —
	// the wake pass must never widen the #79/#80 pgrep gate to it.
	WakeLaunchAgent    = "launchagent"
	WakeSessionMessage = "session-message"
	WakeRoutine        = "routine"
	WakeOwnerSurface   = "owner-surface"
	// WakeNone explicitly opts an agent out of waking. Distinct from a missing
	// mechanism ("" — a legacy command agent that simply has no wake intent):
	// "none" is a deliberate "do not wake me", "" is "no contract declared".
	WakeNone = "none"
)
