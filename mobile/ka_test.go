package mobile

import (
	"encoding/json"
	"testing"
)

func TestKaHunt(t *testing.T) {
	if testing.Short() {
		t.Skip("slow filesystem scan — skipped in short mode")
	}
	result := KaHunt(false)

	var resp Response
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Ka scan should succeed (may find 0 ghosts on a clean system).
	if !resp.OK {
		t.Fatalf("expected ok=true, got error: %s", resp.Error)
	}
}
