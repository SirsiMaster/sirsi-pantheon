package stele

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRecoversChainFromBoundedTailOfLargeLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stele.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 64<<10)
	for i := range block {
		block[i] = 'x'
	}
	block[len(block)-1] = '\n'
	for i := 0; i < 256; i++ {
		if _, err = f.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	last := Entry{Seq: 42, Hash: "final-hash"}
	line, _ := json.Marshal(last)
	if _, err = f.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}

	ledger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.seq != 43 || ledger.prevHash != "final-hash" {
		t.Fatalf("chain recovery = (%d, %q), want (43, final-hash)", ledger.seq, ledger.prevHash)
	}
}
