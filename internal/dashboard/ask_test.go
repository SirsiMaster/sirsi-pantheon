package dashboard

import "testing"

// The engine returns its chat-template scaffolding inline. Rendering that to an
// operator looks exactly like a broken surface, and the scratch channel must
// never reach the screen as if it were the answer.
func TestCleanCompletion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// Verbatim from gemma-4-12b-it-8bit, 2026-08-05.
			name: "channel protocol with scratch channel",
			in:   "<|channel>thought\n<channel|>The top consumer of memory is sne-server-macos-arm64, which is using 34.9 GB.<turn|>",
			want: "The top consumer of memory is sne-server-macos-arm64, which is using 34.9 GB.",
		},
		{
			name: "turn markers only",
			in:   "<start_of_turn>Disk is healthy at 1%.<end_of_turn>",
			want: "Disk is healthy at 1%.",
		},
		{
			name: "plain answer untouched",
			in:   "  Swap is in use but pressure is normal.  ",
			want: "Swap is in use but pressure is normal.",
		},
		{
			// Must not eat ordinary angle brackets — a real answer can name a
			// command like `kill <target>`.
			name: "preserves prose angle brackets it should not own",
			in:   "Run kill <TARGET> to clear it.",
			want: "Run kill <TARGET> to clear it.",
		},
		{
			name: "scratch channel alone yields nothing",
			in:   "<|channel>thought<turn|>",
			want: "thought",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanCompletion(tt.in); got != tt.want {
				t.Errorf("cleanCompletion(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}
