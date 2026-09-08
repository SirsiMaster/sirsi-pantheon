package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEmitsNonExecutingInventory(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "Pantheon.app")
	for _, dir := range []string{"Contents/MacOS", "Contents/Resources"} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	info := []byte(`<?xml version="1.0"?><plist><dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string><key>CFBundleShortVersionString</key><string>0.23.9-beta</string><key>CFBundleVersion</key><string>20260908</string></dict></plist>`)
	pkgInfo := []byte("APPL????")
	launchAgent := []byte("Label=ai.sirsi.pantheon\n")
	files := map[string][]byte{
		"Contents/Info.plist":                        info,
		"Contents/PkgInfo":                           pkgInfo,
		"Contents/MacOS/sirsi":                       []byte("cli bytes"),
		"Contents/MacOS/sirsi-menubar":               []byte("menubar bytes"),
		"Contents/Resources/ai.sirsi.pantheon.plist": launchAgent,
	}
	for rel, data := range files {
		if err := os.WriteFile(filepath.Join(app, filepath.FromSlash(rel)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	code := run([]string{
		"--app", app, "--version", "0.23.9-beta", "--build", "20260908",
		"--info-plist", filepath.Join(app, "Contents/Info.plist"),
		"--pkg-info", filepath.Join(app, "Contents/PkgInfo"),
		"--launch-agent", filepath.Join(app, "Contents/Resources/ai.sirsi.pantheon.plist"),
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run code=%d stderr=%s", code, errOut.String())
	}
	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report["python_free"] != true || report["engine_count"] != float64(2) {
		t.Fatalf("unexpected report: %s", out.String())
	}
}

func TestRunRejectsPositionalAndMissingInputs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"unexpected"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "positional") {
		t.Fatalf("positional args: code=%d stderr=%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(nil, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "required") {
		t.Fatalf("missing inputs: code=%d stderr=%s", code, errOut.String())
	}
}
