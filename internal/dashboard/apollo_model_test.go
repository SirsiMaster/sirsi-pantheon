package dashboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApolloDashboardSelectedModel(t *testing.T) {
	for _, model := range []string{"", "mlx-community/gemma-4-12B-it-8bit"} {
		t.Run(model, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
				t.Fatal(err)
			}
			if model != "" {
				if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-model.conf"), []byte(model+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want := model
			if want == "" {
				want = "local"
			}
			if got := sneModel(); got != want {
				t.Fatalf("model=%q want=%q", got, want)
			}
		})
	}
}
