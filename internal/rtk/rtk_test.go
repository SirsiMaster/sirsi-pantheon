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
		// Edge cases: nested sequences
		{"nested sequences", "\x1b[1m\x1b[31mred bold\x1b[0m\x1b[0m", "red bold"},
		// Incomplete-looking escape — regex matches \x1b[ + 0 digits + letter 'h', so 'h' is consumed
		{"incomplete escape", "\x1b[hello", "ello"},
		// Very long ANSI string
		{"long ansi", "\x1b[31m" + strings.Repeat("x", 10000) + "\x1b[0m", strings.Repeat("x", 10000)},
		// Only ANSI, no visible content
		{"only ansi", "\x1b[31m\x1b[0m\x1b[32m\x1b[0m", ""},
		// Multiple codes between content
		{"multi code", "\x1b[1m\x1b[4m\x1b[31mtext\x1b[0m", "text"},
		// Semicolons in sequence
		{"semicolons", "\x1b[38;5;196mcolored\x1b[0m", "colored"},
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

func TestDedupRing_Wraparound(t *testing.T) {
	t.Parallel()
	// Ring of size 2 — fill it, then overflow to test wraparound.
	r := newDedupRing(2)
	r.seen("x") // pos=0 -> [x, 0], pos=1
	r.seen("y") // pos=1 -> [x, y], pos=0 (wrapped)
	// Both should be recognized.
	if !r.seen("x") {
		t.Error("'x' should still be in ring")
	}
	if !r.seen("y") {
		t.Error("'y' should still be in ring")
	}
	// Now add 'z' — evicts 'x' (pos=0).
	r.seen("z") // pos wraps
	// 'z' should be seen, 'x' may be evicted (depends on exact position).
	if !r.seen("z") {
		t.Error("'z' should be in ring")
	}
}

func TestDedupRing_ZeroSize(t *testing.T) {
	t.Parallel()
	// Zero size should default to 32.
	r := newDedupRing(0)
	if r.size != 32 {
		t.Errorf("zero-size ring should default to 32, got %d", r.size)
	}
	r.seen("test")
	if !r.seen("test") {
		t.Error("should recognize duplicate even with defaulted size")
	}
}

func TestDedupRing_NegativeSize(t *testing.T) {
	t.Parallel()
	r := newDedupRing(-5)
	if r.size != 32 {
		t.Errorf("negative-size ring should default to 32, got %d", r.size)
	}
}

func TestDedupRing_SingleItemRing(t *testing.T) {
	t.Parallel()
	r := newDedupRing(1)
	r.seen("a")
	if !r.seen("a") {
		t.Error("single-item ring should recognize duplicate")
	}
	r.seen("b") // evicts 'a'
	if !r.seen("b") {
		t.Error("'b' should be in single-item ring")
	}
	// 'a' was evicted.
	if r.seen("a") {
		t.Error("'a' should have been evicted from single-item ring")
	}
}

func TestDedupRing_ExactCapacity(t *testing.T) {
	t.Parallel()
	size := 5
	r := newDedupRing(size)
	// Fill to exactly capacity.
	for i := 0; i < size; i++ {
		r.seen(string(rune('a' + i)))
	}
	// All should be recognized.
	for i := 0; i < size; i++ {
		if !r.seen(string(rune('a' + i))) {
			t.Errorf("item %c should be in ring at exact capacity", rune('a'+i))
		}
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
		// New test cases below
		{
			name:          "all options enabled",
			cfg:           FilterConfig{StripANSI: true, Dedup: true, DedupWindow: 8, MaxLines: 5, MaxBytes: 0, TailLines: 2, CollapseBlank: true},
			input:         "\x1b[31mline\x1b[0m\nline\nline\n\n\n\na\nb\nc\nd\ne\nf",
			wantContains:  "f",
			wantTruncated: true,
		},
		{
			name:         "all options disabled",
			cfg:          FilterConfig{},
			input:        "\x1b[31mred\x1b[0m\nred\n\n\n",
			wantContains: "\x1b[31mred\x1b[0m",
		},
		{
			name:          "only truncation",
			cfg:           FilterConfig{MaxLines: 3, TailLines: 0},
			input:         "a\nb\nc\nd\ne",
			wantTruncated: true,
		},
		{
			name:     "only dedup",
			cfg:      FilterConfig{Dedup: true, DedupWindow: 32},
			input:    "abc\nabc\nabc\ndef",
			wantDups: 2,
		},
		{
			name:          "MaxLines less than TailLines",
			cfg:           FilterConfig{MaxLines: 3, TailLines: 10},
			input:         "1\n2\n3\n4\n5\n6\n7\n8\n9\n10",
			wantTruncated: true,
		},
		{
			name:          "MaxLines equals TailLines",
			cfg:           FilterConfig{MaxLines: 3, TailLines: 3},
			input:         "1\n2\n3\n4\n5\n6\n7\n8\n9\n10",
			wantTruncated: true,
		},
		{
			name:          "TailLines zero with MaxLines",
			cfg:           FilterConfig{MaxLines: 3, TailLines: 0},
			input:         "1\n2\n3\n4\n5",
			wantTruncated: true,
		},
		{
			name:         "only whitespace input",
			cfg:          DefaultConfig(),
			input:        "   \t  \t  ",
			wantContains: "",
		},
		{
			name:  "only newlines input",
			cfg:   FilterConfig{CollapseBlank: true},
			input: "\n\n\n\n\n",
		},
		{
			name:         "crlf line endings",
			cfg:          FilterConfig{CollapseBlank: true},
			input:        "a\r\n\r\n\r\nb",
			wantContains: "a",
		},
		{
			name:          "MaxLines exact count no truncation",
			cfg:           FilterConfig{MaxLines: 3, TailLines: 1},
			input:         "a\nb\nc",
			wantTruncated: false,
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

func TestFilter_Ratio_NoChange(t *testing.T) {
	t.Parallel()
	// Input with no transformations applied should have ratio 1.0.
	f := New(FilterConfig{}) // all options disabled
	result := f.Apply("hello world")
	if result.Ratio != 1.0 {
		t.Errorf("ratio = %.4f, want 1.0 for no-change input", result.Ratio)
	}
}

func TestFilter_Ratio_EmptyOutput(t *testing.T) {
	t.Parallel()
	// Empty input returns ratio 1.0 (special case).
	f := New(DefaultConfig())
	result := f.Apply("")
	if result.Ratio != 1.0 {
		t.Errorf("ratio = %.4f, want 1.0 for empty input", result.Ratio)
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if !cfg.StripANSI {
		t.Error("DefaultConfig StripANSI should be true")
	}
	if !cfg.Dedup {
		t.Error("DefaultConfig Dedup should be true")
	}
	if cfg.DedupWindow != 32 {
		t.Errorf("DefaultConfig DedupWindow = %d, want 32", cfg.DedupWindow)
	}
	if cfg.MaxLines != 0 {
		t.Errorf("DefaultConfig MaxLines = %d, want 0", cfg.MaxLines)
	}
	if cfg.MaxBytes != DefaultMaxBytes {
		t.Errorf("DefaultConfig MaxBytes = %d, want %d (DefaultMaxBytes)", cfg.MaxBytes, DefaultMaxBytes)
	}
	if cfg.TailLines != 20 {
		t.Errorf("DefaultConfig TailLines = %d, want 20", cfg.TailLines)
	}
	if !cfg.CollapseBlank {
		t.Error("DefaultConfig CollapseBlank should be true")
	}
}

func TestTruncateWithTail_LinesEqualMaxLines(t *testing.T) {
	t.Parallel()
	// When called directly with len(lines) == maxLines and tailLines < maxLines:
	// headCount = 5 - 2 - 1 = 2, omitted = 5 - 2 - 2 = 1, so 1 line is omitted.
	lines := []string{"a", "b", "c", "d", "e"}
	result, removed := truncateWithTail(lines, 5, 2, 0)
	// head(2) + marker(1) + tail(2) = 5 lines, 1 omitted.
	if len(result) != 5 {
		t.Errorf("expected 5 lines, got %d", len(result))
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestTruncateWithTail_TailLinesZero(t *testing.T) {
	t.Parallel()
	// TailLines=0 means simple truncation.
	lines := []string{"a", "b", "c", "d", "e", "f", "g"}
	result, removed := truncateWithTail(lines, 3, 0, 0)
	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
	if removed != 4 {
		t.Errorf("expected 4 removed, got %d", removed)
	}
}

func TestTruncateWithTail_TailLinesGreaterOrEqualMaxLines(t *testing.T) {
	t.Parallel()
	// When tailLines >= maxLines, falls into no-tail branch.
	lines := []string{"a", "b", "c", "d", "e", "f", "g"}
	result, removed := truncateWithTail(lines, 3, 5, 0)
	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
	if removed != 4 {
		t.Errorf("expected 4 removed, got %d", removed)
	}
}

func TestTruncateWithTail_HeadCountMinimum(t *testing.T) {
	t.Parallel()
	// When headCount < 1 (maxLines - tailLines - 1 < 1), headCount is clamped to 1.
	// maxLines=3, tailLines=2 => headCount = 3-2-1 = 0 => clamped to 1.
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	result, _ := truncateWithTail(lines, 3, 2, 0)
	// Should have: head(1) + marker(1) + tail(2) = 4 lines.
	if len(result) < 3 {
		t.Errorf("expected at least 3 lines, got %d", len(result))
	}
	// Last two lines should be the tail.
	if result[len(result)-1] != "j" {
		t.Errorf("last line should be 'j', got %q", result[len(result)-1])
	}
	if result[len(result)-2] != "i" {
		t.Errorf("second-to-last should be 'i', got %q", result[len(result)-2])
	}
}

func TestLargeInput(t *testing.T) {
	t.Parallel()
	// Generate 10000+ lines of mixed repetitive content.
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		if i%10 == 0 {
			sb.WriteString("unique line " + string(rune('A'+i%26)) + "\n")
		} else {
			sb.WriteString("repeated line\n")
		}
	}
	input := sb.String()

	f := New(DefaultConfig())
	result := f.Apply(input)

	if result.FilteredBytes == 0 {
		t.Error("filtered output should not be empty for large input")
	}
	if result.Ratio >= 1.0 {
		t.Errorf("ratio = %.4f, expected < 1.0 for repetitive large input", result.Ratio)
	}
	if result.DupsCollapsed == 0 {
		t.Error("expected some duplicates to be collapsed")
	}
}

func TestFilter_MaxBytesAndMaxLines(t *testing.T) {
	t.Parallel()
	// Both MaxLines and MaxBytes active — both should apply.
	f := New(FilterConfig{MaxLines: 100, MaxBytes: 20, TailLines: 0})
	result := f.Apply("abcdefghij\nabcdefghij\nabcdefghij\nabcdefghij")
	if !result.Truncated {
		t.Error("expected truncation from MaxBytes")
	}
	if len(result.Output) > 20 {
		t.Errorf("output length = %d, should be <= 20", len(result.Output))
	}
}

func TestFilter_CollapseBlankOnly(t *testing.T) {
	t.Parallel()
	f := New(FilterConfig{CollapseBlank: true})
	result := f.Apply("\n\n\n\na\n\n\n\nb\n\n\n\n")
	// Consecutive blank lines should be collapsed to single blanks.
	lines := strings.Split(result.Output, "\n")
	prevBlank := false
	for i, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			t.Errorf("consecutive blank lines at index %d", i)
		}
		prevBlank = blank
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	cfg := FilterConfig{StripANSI: true, MaxLines: 50}
	f := New(cfg)
	if f == nil {
		t.Fatal("New returned nil")
	}
	if !f.cfg.StripANSI {
		t.Error("filter should have StripANSI enabled")
	}
	if f.cfg.MaxLines != 50 {
		t.Errorf("MaxLines = %d, want 50", f.cfg.MaxLines)
	}
}
