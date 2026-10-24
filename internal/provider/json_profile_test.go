package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerationRequirementsProfileDeclarationDoesNotLeakAcrossSelection(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{"SIRSI_REMOTE_API_KEY", "OPENAI_API_KEY", "SIRSI_REMOTE_ENDPOINT"} {
		t.Setenv(key, "")
	}
	write := func(name, endpoint, model, declaration string) string {
		t.Helper()
		path := filepath.Join(home, name+".json")
		b := fmt.Sprintf(`{"endpoint":%q,"model":%q,"context_tokens":1024,"output_tokens":256,"streaming":false%s}`, endpoint, model, declaration)
		if err := os.WriteFile(path, []byte(b), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	first := write("first", "http://127.0.0.1:11111", "first-exact-model", `,"json_object":true`)
	second := write("second", "http://127.0.0.1:22222", "second-exact-model", "")
	t.Setenv("SIRSI_LOCAL_PROFILE", first)
	ladder := Ladder(context.Background(), home)
	if len(ladder) != 1 || ladder[0].Caps().JSONObject != CapabilitySupported {
		t.Fatal("first exact profile declaration not mapped")
	}
	t.Setenv("SIRSI_LOCAL_PROFILE", second)
	ladder = Ladder(context.Background(), home)
	if len(ladder) != 1 || ladder[0].Caps().JSONObject != CapabilityUnknown {
		t.Fatal("prior profile declaration leaked across selection")
	}
	p := Local(home, Conf{})
	if p.Endpoint != "http://127.0.0.1:22222/v1" || p.Model != "second-exact-model" {
		t.Fatalf("second profile binding changed: %+v", p)
	}
}
