package main

import (
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalReceiptActualEntryRejectsBeforeAnyRemoteCobraEffect(t *testing.T) {
	old := rootCmd
	t.Cleanup(func() { rootCmd = old })
	effects := 0
	rootCmd = &cobra.Command{Use: "sirsi"}
	rootCmd.AddCommand(&cobra.Command{Use: "router", RunE: func(*cobra.Command, []string) error { effects++; return nil }})
	path := filepath.Join(t.TempDir(), "fact.json")
	if err := os.WriteFile(path, []byte(`{"schema":"sirsi.maat.local-execution.v1","scope":"local"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_MAAT_LOCAL_EXECUTION_RECEIPT", path)
	if err := executeSirsi([]string{"router"}); err == nil || effects != 0 {
		t.Fatalf("local receipt bypassed real entry: effects=%d err=%v", effects, err)
	}
}
