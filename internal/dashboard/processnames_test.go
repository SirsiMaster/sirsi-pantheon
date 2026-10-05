package dashboard

import (
	"reflect"
	"testing"
)

func TestProcessLabelsFromCommandOutputUsesExactBasenames(t *testing.T) {
	got := ProcessLabelsFromCommandOutput("COMMAND\n"+
		"/Applications/Pantheon.app/Contents/MacOS/sirsi-menubar\n"+
		"guard-backup\n"+
		"my-sirsi-wrapper\n"+
		"/opt/anubis\n"+
		"/opt/anubis\n", PantheonComponentLabels())
	want := []string{"☥ Sirsi Menubar", "𓃣 Anubis"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProcessLabelsFromCommandOutput() = %#v, want %#v", got, want)
	}
}
