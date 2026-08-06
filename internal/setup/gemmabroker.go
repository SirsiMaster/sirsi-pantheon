package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ── Reboot-durable Gemma broker (ADR-045, owner directive 2026-07-23) ────────
//
// "local-llm-sovereignty": the local model must survive reboots and crashes on
// its own — never depend on a cloud agent to revive it. The old launcher
// (`ai.sirsi.gemma`, KeepAlive=false, one-shot `sirsi gemma serve`) started the
// broker at boot and forgot it; a nohup-started broker died with the terminal.
//
// The durable shape: launchd owns SNE directly. SNE is the native Go inference
// server; no Python runtime exists in the request or supervision path.

// GemmaBrokerLabel is the LaunchAgent label for the reboot-durable broker.
const GemmaBrokerLabel = "ai.sirsi.gemma-broker"

// legacyGemmaLauncherLabel is the retired one-shot launcher (KeepAlive=false —
// boot-only, nothing revived a dead broker; ADR-031-C provenance).
const legacyGemmaLauncherLabel = "ai.sirsi.gemma"

// gemmaBrokerPlistContent renders the KeepAlive LaunchAgent. HF_HUB_OFFLINE
// keeps model load off the network (boot may run before the network is up; the
// model is always served from the local cache). Output goes to the broker's
// own log so `sirsi gemma serve` (background path) never truncates it.
func gemmaBrokerPlistContent(_ string, home string) string {
	logPath := filepath.Join(home, ".sirsi", "sne-server.log")
	sneBin := filepath.Join(home, ".local", "bin", "sirsi-inference")
	modelDir := filepath.Join(home, ".cache", "huggingface", "hub", "models--mlx-community--gemma-4-12B-it-8bit", "snapshots", "200bb6db075e137a4deb08838865ac4ddb86292e")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
		<string>%s</string>
		<string>--profile</string>
		<string>interactive</string>
		<string>127.0.0.1:8477</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>30</integer>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, GemmaBrokerLabel, sneBin, modelDir, logPath, logPath)
}

// GemmaBrokerInstalled reports whether the reboot-durable broker LaunchAgent
// is in place. macOS only.
func GemmaBrokerInstalled() bool {
	return runtime.GOOS == "darwin" && fileExists(launchAgentPath(GemmaBrokerLabel))
}

// InstallGemmaBroker writes the reboot-durable broker LaunchAgent, retires the
// legacy one-shot `ai.sirsi.gemma` launcher, and loads the new job so the
// local model survives crashes and reboots on its own. Skips cleanly on
// machines without the MLX runtime. macOS only; idempotent.
func InstallGemmaBroker() InstallResult {
	res := InstallResult{Surface: SurfaceGemmaBroker}
	if runtime.GOOS != "darwin" {
		res.Status, res.Message = StatusSkipped, "local model broker is macOS only"
		return res
	}
	home, _ := os.UserHomeDir()
	if !fileExists(filepath.Join(home, ".local", "bin", "sirsi-inference")) {
		res.Status, res.Message = StatusSkipped, "skipped — native SNE binary not installed"
		return res
	}
	modelDir := filepath.Join(home, ".cache", "huggingface", "hub", "models--mlx-community--gemma-4-12B-it-8bit", "snapshots", "200bb6db075e137a4deb08838865ac4ddb86292e")
	if !fileExists(filepath.Join(modelDir, "model.safetensors.index.json")) {
		res.Status, res.Message = StatusSkipped, "skipped — SNE model snapshot not installed"
		return res
	}
	bin := BinaryPath()
	if bin == "" || bin == "sirsi" {
		res.Status, res.Message = StatusFailed, "sirsi binary path could not be resolved"
		return res
	}

	// Retire the legacy one-shot launcher — with the durable agent in place it
	// would only race the broker's port at boot.
	legacyRetired := false
	if legacy := launchAgentPath(legacyGemmaLauncherLabel); fileExists(legacy) {
		_ = runLaunchctl("unload", legacy)
		if err := os.Remove(legacy); err != nil {
			res.Status = StatusFailed
			res.Message = fmt.Sprintf("legacy %s launcher could not be removed: %v", legacyGemmaLauncherLabel, err)
			return res
		}
		legacyRetired = true
	}

	path := launchAgentPath(GemmaBrokerLabel)
	content := gemmaBrokerPlistContent(bin, home)
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		res.Status, res.Message = StatusOK, "local model broker already reboot-durable (no change)"
		return res
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		res.Status, res.Message = StatusFailed, err.Error()
		return res
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		res.Status, res.Message = StatusFailed, err.Error()
		return res
	}
	_ = os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte("8477\n"), 0o644)
	_ = runLaunchctl("unload", path) // fails when not loaded — fine
	if err := runLaunchctl("load", path); err != nil {
		res.Status, res.Message = StatusFailed, "wrote LaunchAgent but launchctl load failed: "+err.Error()
		return res
	}
	res.Status = StatusOK
	res.Message = "local model broker is now reboot-durable (launchd keeps it alive)"
	if legacyRetired {
		res.Message += " — retired the old one-shot launcher"
	}
	return res
}
