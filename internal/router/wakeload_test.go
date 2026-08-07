package router

import "testing"

// The whole point of this helper is that it can say NO. A liveness probe that
// cannot fail is not a probe — that is the defect this file exists to fix, where
// "installed" meant "a file exists" and could never report an unarmed lane.
func TestWakeAgentLoadedIsFalseForAnAbsentLabel(t *testing.T) {
	if WakeAgentLoaded("definitely-not-a-real-agent-9f3a1c") {
		t.Fatal("reported an unregistered agent as loaded — the check cannot distinguish armed from unarmed")
	}
}

// Bootstrapping a path launchd cannot read must report NOT loaded with the
// reason, never a bare success. Silent failure here is what put 18 lanes in the
// "installed but never loaded" state.
func TestLoadWakeAgentReportsFailureForABogusPlist(t *testing.T) {
	loaded, detail := LoadWakeAgent("definitely-not-a-real-agent-9f3a1c", "/nonexistent/nope.plist")
	if loaded {
		t.Fatal("claimed loaded for a plist that does not exist")
	}
	if detail == "" {
		t.Error("failure carried no detail — the caller cannot tell WHY it is unarmed, which is the difference between retry and escalate")
	}
}

func TestWakeLabelMatchesTheInstalledNaming(t *testing.T) {
	if got := wakeLabel("codex-mail"); got != "ai.sirsi.router.wake.codex-mail" {
		t.Errorf("label = %q — must match the plist naming or the check queries a label that never exists and always reports unarmed", got)
	}
}
