//go:build windows

package maat

import (
	"os"
	"testing"
)

func historyPrefixCursorFixture(t *testing.T, path, query string, limit int) *historyCursor {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return &historyCursor{Version: historyCursorVersion, Snapshot: historySnapshotID(path, info), QueryKey: query, Phase: "scan", Limit: limit}
}
