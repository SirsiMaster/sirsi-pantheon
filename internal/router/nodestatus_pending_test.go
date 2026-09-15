package router

import (
	"os"
	"strings"
	"testing"
)

// node-status's pending read must be the indexed OPEN-items query, never the
// whole corpus: on 2026-09-15 ListAll moved 36 MB through the M5 relay for a
// 113-item answer and every `ctr`/`node-status` there waited the full spool
// timeout (SHA 20260914-234610). Scope the read to the claim (A35).
func TestNodeStatusPendingReadsOpenItemsOnly(t *testing.T) {
	src, err := os.ReadFile("nodestatus.go")
	if err != nil {
		t.Fatal(err)
	}
	fn := string(src)
	i := strings.Index(fn, "func CollectNodeStatus(")
	if i < 0 {
		t.Fatal("CollectNodeStatus not found")
	}
	fn = fn[i:]
	if strings.Contains(fn, "store.ListAll(") {
		t.Fatal("CollectNodeStatus reads the whole item corpus for a pending count — use store.Inbox(\"\")")
	}
	if !strings.Contains(fn, `store.Inbox("")`) {
		t.Fatal("CollectNodeStatus no longer sources pending from the open-items index")
	}
}
