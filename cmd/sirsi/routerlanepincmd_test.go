package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// The operator surface sets, lists, retires (history kept) and refuses what it should, against a SQLite store.
func TestRouterLanePinSetListRetire(t *testing.T) {
	db := filepath.Join(t.TempDir(), "router.db")
	old := routerLanePinStore
	routerLanePinStore = db
	defer func() { routerLanePinStore = old }()

	var out bytes.Buffer
	routerLanePinSetCmd.SetOut(&out)
	_ = routerLanePinSetCmd.Flags().Set("by", "ra")
	if err := routerLanePinSetCmd.RunE(routerLanePinSetCmd, []string{"lane-a", "mach-1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "report-only") {
		t.Fatalf("set must say it is report-only: %s", out.String())
	}
	out.Reset()
	routerLanePinListCmd.SetOut(&out)
	if err := routerLanePinListCmd.RunE(routerLanePinListCmd, nil); err != nil || !strings.Contains(out.String(), "lane-a") || !strings.Contains(out.String(), "active") {
		t.Fatalf("list: %v %q", err, out.String())
	}
	if err := routerLanePinRetireCmd.RunE(routerLanePinRetireCmd, []string{"lane-a", "mach-1"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := routerLanePinListCmd.RunE(routerLanePinListCmd, nil); err != nil || strings.Contains(out.String(), "lane-a") {
		t.Fatalf("retired pin must leave the default list: %v %q", err, out.String())
	}
	_ = routerLanePinListCmd.Flags().Set("all", "true")
	defer func() { _ = routerLanePinListCmd.Flags().Set("all", "false") }()
	out.Reset()
	if err := routerLanePinListCmd.RunE(routerLanePinListCmd, nil); err != nil || !strings.Contains(out.String(), "retired") {
		t.Fatalf("--all must show the retired row: %v %q", err, out.String())
	}
	if err := routerLanePinRetireCmd.RunE(routerLanePinRetireCmd, []string{"lane-a", "mach-1"}); err == nil {
		t.Fatal("retiring an already retired pin must fail")
	}
}
