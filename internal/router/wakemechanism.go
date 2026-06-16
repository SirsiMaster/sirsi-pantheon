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
)
