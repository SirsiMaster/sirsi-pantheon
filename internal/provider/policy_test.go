package provider

import (
	"errors"
	"testing"
)

func TestDecidePolicy(t *testing.T) {
	full := LaneCaps{ContextTokens: 128000, Streaming: true, Deterministic: true, JSONMode: true}
	local := LaneState{Availability: Available, Caps: LaneCaps{ContextTokens: 8192, Streaming: true, Deterministic: true, JSONMode: true}}
	remote := LaneState{Availability: Available, Caps: full, Provider: "openai", Model: "frontier"}
	tests := []struct {
		name    string
		req     PolicyRequest
		state   PolicyState
		want    Decision
		wantErr bool
	}{
		{"local-only stays local", PolicyRequest{Task: TaskJudgment, Privacy: PrivacyLocalOnly}, PolicyState{Local: local, Remote: remote}, Decision{Lane: LaneLocal, Reason: "local-only privacy requires the local lane"}, false},
		{"local-only capability mismatch fails closed", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyLocalOnly, Needs: CapabilityNeeds{ContextTokens: 9000}}, PolicyState{Local: local, Remote: remote}, Decision{}, true},
		{"local-only offline is explicit", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyLocalOnly}, PolicyState{Local: LaneState{Availability: Offline}, Remote: remote}, Decision{}, true},
		{"judgment uses remote with local fallback", PolicyRequest{Task: TaskJudgment, Privacy: PrivacyShareable}, PolicyState{Local: local, Remote: remote}, Decision{Lane: LaneRemote, Fallback: LaneLocal, Reason: "shareable judgment uses the best qualified remote lane"}, false},
		{"judgment fails down on budget", PolicyRequest{Task: TaskJudgment, Privacy: PrivacyShareable}, PolicyState{Local: local, Remote: LaneState{Availability: Budgeted, Caps: full}}, Decision{Lane: LaneLocal, Reason: "local lane satisfies the request capabilities"}, false},
		{"generation defaults local", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyShareable}, PolicyState{Local: local, Remote: remote}, Decision{Lane: LaneLocal, Fallback: LaneRemote, Reason: "local lane satisfies the request capabilities"}, false},
		{"extraction exceeds local context", PolicyRequest{Task: TaskExtraction, Privacy: PrivacyShareable, Needs: CapabilityNeeds{ContextTokens: 9000}}, PolicyState{Local: local, Remote: remote}, Decision{Lane: LaneRemote, Reason: "request capabilities exceed the local envelope"}, false},
		{"offline remote is not qualified", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyShareable, Needs: CapabilityNeeds{ContextTokens: 9000}}, PolicyState{Local: local, Remote: LaneState{Availability: Offline, Caps: full}}, Decision{}, true},
		{"explicit remote override", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyShareable, Override: LaneRemote}, PolicyState{Local: local, Remote: remote}, Decision{Lane: LaneRemote, Reason: "explicit per-request override", Override: true}, false},
		{"explicit hybrid override", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyShareable, Override: LaneHybrid}, PolicyState{Local: local, Remote: remote}, Decision{Lane: LaneHybrid, Fallback: LaneRemote, Reason: "explicit hybrid override", Override: true}, false},
		{"hybrid override requires both lanes", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyShareable, Override: LaneHybrid}, PolicyState{Local: local, Remote: LaneState{Availability: Offline, Caps: full}}, Decision{}, true},
		{"remote override cannot violate privacy", PolicyRequest{Task: TaskGeneration, Privacy: PrivacyLocalOnly, Override: LaneRemote}, PolicyState{Local: local, Remote: remote}, Decision{}, true},
		{"required JSON mode is qualified", PolicyRequest{Task: TaskExtraction, Privacy: PrivacyShareable, Needs: CapabilityNeeds{JSONMode: true}}, PolicyState{Local: LaneState{Availability: Available, Caps: LaneCaps{ContextTokens: 8192}}, Remote: remote}, Decision{Lane: LaneRemote, Reason: "request capabilities exceed the local envelope"}, false},
		{"gemini 2 is below floor", PolicyRequest{Task: TaskJudgment, Privacy: PrivacyShareable}, PolicyState{Local: local, Remote: LaneState{Availability: Available, Caps: full, Provider: "gemini", Model: "gemini-2.5-pro"}}, Decision{Lane: LaneLocal, Reason: "local lane satisfies the request capabilities"}, false},
		{"gemini 3 meets floor", PolicyRequest{Task: TaskJudgment, Privacy: PrivacyShareable}, PolicyState{Local: local, Remote: LaneState{Availability: Available, Caps: full, Provider: "gemini", Model: "gemini-3-pro"}}, Decision{Lane: LaneRemote, Fallback: LaneLocal, Reason: "shareable judgment uses the best qualified remote lane"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decide(tt.req, tt.state)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Decide() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrNoQualifiedLane) {
				t.Fatalf("Decide() error = %v, want ErrNoQualifiedLane", err)
			}
			if got != tt.want {
				t.Fatalf("Decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecideRejectsInvalidInputs(t *testing.T) {
	tests := []PolicyRequest{
		{Task: "unknown", Privacy: PrivacyShareable},
		{Task: TaskGeneration, Privacy: "unknown"},
		{Task: TaskGeneration, Privacy: PrivacyShareable, Needs: CapabilityNeeds{ContextTokens: -1}},
	}
	for _, req := range tests {
		if _, err := Decide(req, PolicyState{}); err == nil {
			t.Fatalf("Decide(%+v) accepted invalid input", req)
		}
	}
}
