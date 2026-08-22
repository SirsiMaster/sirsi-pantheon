package dashboard

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAssistantSafetensorsRequiresFormalIdentity(t *testing.T) {
	root := t.TempDir()
	assistant := []byte("assistant")
	digest := fmt.Sprintf("%x", sha256.Sum256(assistant))
	if err := os.WriteFile(filepath.Join(root, "model.safetensors"), assistant, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "model.json")
	data := fmt.Sprintf(`{"execution":{"mode":"mtp","assistant":{"checkpoint_sha256":"%s"}},"artifacts":{"integrity":{"files":[{"root":"assistant","path":"model.safetensors","role":"assistant","sha256":"%s"}]}}}`, digest, digest)
	if err := os.WriteFile(manifest, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveAssistantSafetensors(manifest, root)
	if err != nil || resolved != filepath.Join(root, "model.safetensors") {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	if err := os.WriteFile(filepath.Join(root, "model.safetensors"), []byte("drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveAssistantSafetensors(manifest, root); err == nil {
		t.Fatal("assistant drift was accepted")
	}
}
