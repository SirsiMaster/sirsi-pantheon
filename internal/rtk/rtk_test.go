package rtk

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no ansi", "hello world", "hello world"},
		{"color codes", "\x1b[31mred\x1b[0m", "red"},
		{"bold", "\x1b[1mbold\x1b[22m text", "bold text"},
		{"osc title", "\x1b]0;Window Title\x07rest", "rest"},
		{"charset select", "\x1b(Btext", "text"},
		{"mixed", "\x1b[32m✓\x1b[0m passed \x1b[31m✗\x1b[0m failed", "✓ passed ✗ failed"},
		{"multiline", "\x1b[1mline1\x1b[0m\n\x1b[2mline2\x1b[0m", "line1\nline2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripANSI(tt.in)
			if got != tt.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDedupRing(t *testing.T) {
	t.Parallel()
	r := newDedupRing(3)

	if r.seen("a") {
		t.Error("first 'a' should not be seen")
	}
	if !r.seen("a") {
		t.Error("second 'a' should be seen")
	}
	if r.seen("b") {
		t.Error("first 'b' should not be seen")
	}
	if r.seen("c") {
		t.Error("first 'c' should not be seen")
	}
	// Ring is full (a, b, c). Adding 'd' evicts 'a'.
	if r.seen("d") {
		t.Error("first 'd' should not be seen")
	}
	// 'a' was evicted, so it should not be seen.
	if r.seen("a") {
		t.Error("'a' should have been evicted from ring")
	}
}

func TestFilter_Apply(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		cfg            FilterConfig
		input          string
		wantContains   string
		wantNotContain string
		wantTruncated  bool
		wantDups       int
	}{
		{
			name:         "strip ansi",
			cfg:          FilterConfig{StripANSI: true},
			input:        "\x1b[31merror\x1b[0m: something failed",
			wantContains: "error: something failed",
		},
		{
			name:     "dedup lines",
			cfg:      FilterConfig{Dedup: true, DedupWindow: 32},
			input:    "line1\nline1\nline1\nline2",
			wantDups: 2,
		},
		{
			name:         "collapse blanks",
			cfg:          FilterConfig{CollapseBlank: true},
			input:        "a\n\n\n\nb",
			wantContains: "a\n\nb",
		},
		{
			name:          "truncate with tail",
			cfg:           FilterConfig{MaxLines: 5, TailLines: 2},
			input:         "1\n2\n3\n4\n5\n6\n7\n8\n9\n10",
			wantTruncated: true,
			wantContains:  "10",
		},
		{
			name:          "truncate by bytes",
			cfg:           FilterConfig{MaxBytes: 10},
			input:         "hello world, this is a long string",
			wantTruncated: true,
		},
		{
			name:         "default config full pipeline",
			cfg:          DefaultConfig(),
			input:        "\x1b[32m✓\x1b[0m test passed\n\x1b[32m✓\x1b[0m test passed\n\n\n\nresult: ok",
			wantContains: "result: ok",
			wantDups:     1,
		},
		{
			name:  "empty input",
			cfg:   DefaultConfig(),
			input: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := New(tt.cfg)
			result := f.Apply(tt.input)

			if tt.wantContains != "" && !strings.Contains(result.Output, tt.wantContains) {
				t.Errorf("output %q should contain %q", result.Output, tt.wantContains)
			}
			if tt.wantNotContain != "" && strings.Contains(result.Output, tt.wantNotContain) {
				t.Errorf("output %q should not contain %q", result.Output, tt.wantNotContain)
			}
			if result.Truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", result.Truncated, tt.wantTruncated)
			}
			if tt.wantDups > 0 && result.DupsCollapsed != tt.wantDups {
				t.Errorf("dupsCollapsed = %d, want %d", result.DupsCollapsed, tt.wantDups)
			}
			if result.OriginalBytes != len(tt.input) {
				t.Errorf("originalBytes = %d, want %d", result.OriginalBytes, len(tt.input))
			}
		})
	}
}

func TestFilter_Ratio(t *testing.T) {
	t.Parallel()
	// A highly repetitive input should achieve significant reduction.
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "repeated line"
	}
	input := strings.Join(lines, "\n")

	f := New(DefaultConfig())
	result := f.Apply(input)

	if result.Ratio >= 0.5 {
		t.Errorf("ratio = %.2f, expected < 0.5 for highly repetitive input", result.Ratio)
	}
}
