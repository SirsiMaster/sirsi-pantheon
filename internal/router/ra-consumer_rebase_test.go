package router

import (
	"os"
	"testing"
)

func TestRebaseForeignHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := rebaseForeignHome("--add-dir /Users/no-such-user-xyz/.sirsi spool:///Users/no-such-user-xyz/.sirsi/relay")
	want := "--add-dir " + home + "/.sirsi spool://" + home + "/.sirsi/relay"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := rebaseForeignHome(home + "/x"); got != home+"/x" {
		t.Fatalf("own home must be untouched, got %q", got)
	}
}
