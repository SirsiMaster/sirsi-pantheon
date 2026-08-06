package dashboard

import "testing"

// The engine returns its chat-template scaffolding inline. Rendering that to an
// operator looks exactly like a broken surface, and the scratch channel must
// never reach the screen as if it were the answer.
func TestCleanCompletion(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			// Verbatim from gemma-4-12b-it-8bit, 2026-08-05.
			name: "channel protocol with scratch channel",
			in:   "<|channel>thought\n<channel|>{\"findings\":[6]}<turn|>",
			want: `{"findings":[6]}`,
		},
		{
			name: "turn markers only",
			in:   "<start_of_turn>{\"findings\":[]}<end_of_turn>",
			want: `{"findings":[]}`,
		},
		{
			name: "plain output untouched",
			in:   "  {\"findings\":[1]}  ",
			want: `{"findings":[1]}`,
		},
		{
			// The earlier regex `<\|?[a-z_]+\|?>` ate these. The old
			// preservation test used uppercase <TARGET>, which never
			// exercised the pattern it claimed to cover.
			name: "preserves lowercase prose placeholders",
			in:   "Run kill <target> on <path> for <pid>.",
			want: "Run kill <target> on <path> for <pid>.",
		},
		{
			// Returning the scratch text would put the model's private
			// reasoning on screen as though it were the response.
			name:    "scratch-only channel is an error, not a value",
			in:      "<|channel>thought<turn|>",
			wantErr: true,
		},
		{
			name:    "unterminated channel is an error",
			in:      "<|channel>analysis I think the answer is",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanCompletion(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("cleanCompletion(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("cleanCompletion(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("cleanCompletion(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

// The integrity rule: anything factual-looking in the model's one free-text
// field must appear verbatim in the grounding, or the sentence is withheld.
// This is the check that would have caught "action.runner" for a report that
// says "actions.runner".
func TestUnverifiableTokens(t *testing.T) {
	grounding := `Workstation health score: 51/100 (status: degraded)
Diagnostic findings:
[0] [CRITICAL] launchd Disabled Override: 4 label(s) disabled — actions.runner.SirsiMaster-Assiduous.m5-sirsi
[1] [WARNING] Top Memory Consumers: sne-server-macos-arm64 at 32.7 GB (live)`

	tests := []struct {
		name    string
		summary string
		wantBad []string
	}{
		{
			name:    "faithful summary passes",
			summary: "Four launchd labels will not restart, and sne-server-macos-arm64 is holding 32.7 GB.",
		},
		{
			// The exact observed failure.
			name:    "paraphrased identifier is caught",
			summary: "The disabled label is action.runner.SirsiMaster-Assiduous.m5-sirsi.",
			wantBad: []string{"action.runner.SirsiMaster-Assiduous.m5-sirsi"},
		},
		{
			name:    "invented number is caught",
			summary: "Memory use reached 48.9 GB.",
			wantBad: []string{"48.9"},
		},
		{
			name:    "plain prose with no factual tokens passes",
			summary: "Some services will not come back after a restart.",
		},
		{
			// A number that IS in the grounding passes. The old test used only
			// this case to justify a small-integer exemption — but "4" is
			// present here, so it never exercised the exemption at all.
			name:    "integer present in the grounding passes",
			summary: "There are 4 problems worth your attention.",
		},
		{
			// The hole that exemption left: an invented small number was never
			// checked. A fabricated measurement is fabricated at any length.
			name:    "invented bare integers are caught",
			summary: "Memory hit 48 and swap hit 99.",
			wantBad: []string{"48", "99"},
		},
		{
			// Whole-token equality, not substring. This truncation is exactly
			// the class the gate exists to withhold — the exact value is
			// rendered in the selected finding directly beneath the summary.
			name:    "truncated identifier is caught even though it is a substring",
			summary: "The hog is sne-server-macos.",
			wantBad: []string{"sne-server-macos"},
		},
		{
			name:    "exact identifier passes",
			summary: "The hog is sne-server-macos-arm64.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unverifiableTokens(tt.summary, grounding)
			if len(got) != len(tt.wantBad) {
				t.Fatalf("unverifiableTokens = %v, want %v", got, tt.wantBad)
			}
			for i := range got {
				if got[i] != tt.wantBad[i] {
					t.Errorf("token %d = %q, want %q", i, got[i], tt.wantBad[i])
				}
			}
		})
	}
}
