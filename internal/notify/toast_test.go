package notify

import (
	"sync"
	"testing"
	"time"
)

func TestEscapeAppleScript(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello world", "hello world"},
		{"quotes", `say "hello"`, `say \"hello\"`},
		{"backslash", `path\to\file`, `path\\to\\file`},
		{"mixed", `"test" \ done`, `\"test\" \\ done`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := escapeAppleScript(tt.in)
			if got != tt.want {
				t.Errorf("escapeAppleScript(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEscapeAppleScript_Truncation(t *testing.T) {
	t.Parallel()
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	got := escapeAppleScript(string(long))
	if len(got) > 200 {
		t.Errorf("expected truncation to <= 200 chars, got %d", len(got))
	}
}

func TestToast_Injectable(t *testing.T) {
	// Not parallel — modifies package-level state.
	var mu sync.Mutex
	var captured string

	old := getToastExecFn()
	setToastExecFn(func(script string) error {
		mu.Lock()
		defer mu.Unlock()
		captured = script
		return nil
	})

	// Reset rate limiter for test.
	lastToastMu.Lock()
	lastToast = time.Time{}
	lastToastMu.Unlock()

	Toast("Test Title", "Test Body")
	time.Sleep(200 * time.Millisecond) // Let goroutine run.

	mu.Lock()
	got := captured
	mu.Unlock()
	if got == "" {
		t.Error("expected toast exec to be called")
	}
	if got != `display notification "Test Body" with title "Test Title"` {
		t.Errorf("unexpected script: %s", got)
	}

	// Restore.
	setToastExecFn(old)
}

func TestToast_RateLimiting(t *testing.T) {
	// Not parallel — modifies package-level rate limiter state.

	var mu sync.Mutex
	callCount := 0

	old := getToastExecFn()
	setToastExecFn(func(_ string) error {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		return nil
	})

	// Set a meaningful gap for this test (init() disables it).
	oldGap := minToastGap
	minToastGap = 5 * time.Second

	// Reset rate limiter.
	lastToastMu.Lock()
	lastToast = time.Time{}
	lastToastMu.Unlock()

	// First toast should fire.
	Toast("A", "first")
	time.Sleep(50 * time.Millisecond)

	// Second toast within 5s should be rate-limited.
	Toast("B", "second")
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 toast (rate limited), got %d", count)
	}

	setToastExecFn(old)
	minToastGap = oldGap
}

func TestSeverityIcon(t *testing.T) {
	t.Parallel()
	tests := []struct {
		severity string
		want     string
	}{
		{SeveritySuccess, "✅"},
		{SeverityWarning, "⚠️"},
		{SeverityError, "❌"},
		{SeverityInfo, "ℹ️"},
		{"unknown", "ℹ️"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			t.Parallel()
			if got := SeverityIcon(tt.severity); got != tt.want {
				t.Errorf("SeverityIcon(%q) = %q, want %q", tt.severity, got, tt.want)
			}
		})
	}
}
