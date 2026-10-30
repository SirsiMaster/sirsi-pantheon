package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventory"
)

func TestRunEmitsNonExecutingInventory(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "Pantheon.app")
	for _, dir := range []string{"Contents/MacOS", "Contents/Resources/StackLab/v2"} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	info := []byte(`<?xml version="1.0"?><plist><dict><key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string><key>CFBundleShortVersionString</key><string>0.23.9-beta</string><key>CFBundleVersion</key><string>20260908</string></dict></plist>`)
	pkgInfo := []byte("APPL????")
	launchAgent := []byte("Label=ai.sirsi.pantheon\n")
	brandLogo := []byte("canonical Sirsi application-mark bytes\n")
	receiptPath := filepath.Join(root, "frozen-digests.txt")
	files := map[string][]byte{
		"Contents/Info.plist":                                                      info,
		"Contents/PkgInfo":                                                         pkgInfo,
		"Contents/MacOS/sirsi":                                                     []byte("cli bytes"),
		"Contents/MacOS/sirsi-menubar":                                             []byte("menubar bytes"),
		"Contents/Resources/ai.sirsi.pantheon.plist":                               launchAgent,
		"Contents/Resources/sirsi-logo-white.png":                                  brandLogo,
		"Contents/Resources/StackLab/anubis-trash-receipt-v1.json":                 []byte(`{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.anubis-trash-receipt"}`),
		"Contents/Resources/StackLab/apollo-sne-telemetry-v1.json":                 []byte(`{"schema":"sirsi.stacklab.apollo-telemetry.v1"}`),
		"Contents/Resources/StackLab/maat-system-one-recipe-v1.json":               []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/maat-pod-authority-service-recipe-v1.json":    []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/maat-wing-v1.json":                            []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/mercury-consumer-recipe-v1.json":              []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/model-router-generation-requirements-v1.json": []byte(`{"schema":"stacklab.module.model-router-generation-requirements.v1","component":"model-router-generation-requirements-v1"}`),
		"Contents/Resources/StackLab/model-router-v1-recipe-v1.json":               []byte(`{"schema":"sirsi.stacklab.recipe.v1","id":"stacklab.recipe.model-router-v1"}`),
		"Contents/Resources/StackLab/native-stacklab-surface-recipe-v1.json":       []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/pantheon-native-workspace-v1.json":            []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/pantheon.pt-wing-001-wing-v1.json":            []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/pantheon-release-artifact-recipe-v1.json":     []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/ra-horus-fabric-recipe-v1.json":               []byte(`{"schema":"sirsi.stacklab.recipe.v1"}`),
		"Contents/Resources/StackLab/ra-horus-fabric-wing-v1.json":                 []byte(`{"schema":"sirsi.stacklab.wing.v1"}`),
		"Contents/Resources/StackLab/v2/PROVENANCE.md":                             []byte("Stack Lab provenance\n"),
		"Contents/Resources/StackLab/v2/wing.schema.json":                          []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema"}`),
	}
	for rel, data := range files {
		mode := os.FileMode(0o644)
		if rel == "Contents/MacOS/sirsi" || rel == "Contents/MacOS/sirsi-menubar" {
			mode = 0o755
		}
		path := filepath.Join(app, filepath.FromSlash(rel))
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := filepath.WalkDir(app, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	args := []string{
		"--app", app, "--version", "0.23.9-beta", "--build", "20260908",
		"--info-plist", filepath.Join(app, "Contents/Info.plist"),
		"--pkg-info", filepath.Join(app, "Contents/PkgInfo"),
		"--launch-agent", filepath.Join(app, "Contents/Resources/ai.sirsi.pantheon.plist"),
		"--brand-logo", filepath.Join(app, "Contents/Resources/sirsi-logo-white.png"),
		"--apollo-capability", "legacy-development",
		"--write-frozen-digests", receiptPath,
	}
	code := run(args, &out, &errOut)
	if code != 0 {
		t.Fatalf("run code=%d stderr=%s", code, errOut.String())
	}
	var report packageinventory.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != packageinventory.Schema || !report.PythonFree || report.EngineCount != 2 {
		t.Fatalf("unexpected report: %s", out.String())
	}
	receipt, err := os.ReadFile(receiptPath)
	if err != nil || string(receipt) != fmt.Sprintf("info_sha256=%x\napollo_profile_sha256=absent\n", sha256.Sum256(info)) {
		t.Fatalf("standalone inventory did not write the exact initial digest pair: receipt=%q err=%v", receipt, err)
	}
	if len(report.Entries) != 27 {
		t.Fatalf("inventory contains %d entries, want complete 27-entry unsigned payload", len(report.Entries))
	}
	wantPaths := map[string]string{}
	for _, dir := range []string{"Contents", "Contents/MacOS", "Contents/Resources", "Contents/Resources/StackLab", "Contents/Resources/StackLab/v2"} {
		wantPaths[dir] = "directory"
	}
	for rel := range files {
		wantPaths[rel] = "regular"
	}
	for i, entry := range report.Entries {
		if i > 0 && report.Entries[i-1].Path >= entry.Path {
			t.Fatalf("inventory paths are duplicated or not sorted: %+v", report.Entries)
		}
		if wantPaths[entry.Path] != entry.Type {
			t.Fatalf("unexpected inventory entry: %+v", entry)
		}
		if data, ok := files[entry.Path]; ok {
			if len(data) == 0 || entry.Size != int64(len(data)) || entry.Nlink != 1 || entry.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
				t.Fatalf("inventory did not bind fixture bytes: %+v", entry)
			}
		}
		delete(wantPaths, entry.Path)
	}
	if len(wantPaths) != 0 {
		t.Fatalf("inventory omitted fixture paths: %v", wantPaths)
	}

	// Each added contract remains required. Inventory only reads these regular
	// fixture files; neither fake engine contains executable application code.
	for _, leaf := range []string{"anubis-trash-receipt-v1.json", "mercury-consumer-recipe-v1.json", "model-router-v1-recipe-v1.json", "model-router-generation-requirements-v1.json", "maat-pod-authority-service-recipe-v1.json", "pantheon-native-workspace-v1.json"} {
		t.Run("missing_"+leaf, func(t *testing.T) {
			rel := "Contents/Resources/StackLab/" + leaf
			path := filepath.Join(app, filepath.FromSlash(rel))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(path, files[rel], 0o644); err != nil {
					t.Error(err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Error(err)
				}
			})
			var rejectedOut, rejectedErr bytes.Buffer
			if code := run(args, &rejectedOut, &rejectedErr); code != 1 || rejectedOut.Len() != 0 || !strings.Contains(rejectedErr.String(), "missing required entry") || !strings.Contains(rejectedErr.String(), rel) {
				t.Fatalf("missing contract: code=%d stdout=%s stderr=%s", code, rejectedOut.String(), rejectedErr.String())
			}
		})
	}

	// Developer callers use the legacy capability contract with an ad-hoc
	// signed bundle and no release digest flags. Preserve signature-structure
	// inventory through the standalone CLI.
	signatureDir := filepath.Join(app, "Contents", "_CodeSignature")
	if err := os.Mkdir(signatureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(signatureDir, "CodeResources"), []byte("fixture signature layout"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := standaloneMachOFixture()
	for _, path := range []string{
		filepath.Join(app, "Contents", "MacOS", "sirsi"),
		filepath.Join(app, "Contents", "MacOS", "sirsi-menubar"),
	} {
		if err := os.WriteFile(path, engine, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	signedArgs := append([]string(nil), args[:len(args)-2]...)
	signedArgs = append(signedArgs, "--require-code-signature")
	var signedOut, signedErr bytes.Buffer
	if code := run(signedArgs, &signedOut, &signedErr); code != 0 {
		t.Fatalf("legacy signed development inventory without frozen flags failed: code=%d stdout=%s stderr=%s", code, signedOut.String(), signedErr.String())
	}
	var signedReport packageinventory.Report
	if err := json.Unmarshal(signedOut.Bytes(), &signedReport); err != nil {
		t.Fatal(err)
	}
	if signedReport.EngineCount != 2 || len(signedReport.Entries) != 29 {
		t.Fatalf("signed development inventory omitted signature or engines: engines=%d entries=%d", signedReport.EngineCount, len(signedReport.Entries))
	}
}

func standaloneMachOFixture() []byte {
	const headerSize = 32
	const dylibHeaderSize = 24
	const loadDylib = 0xc
	name := "/usr/lib/libSystem.B.dylib"
	commandSize := dylibHeaderSize + len(name) + 1
	for commandSize%8 != 0 {
		commandSize++
	}
	data := make([]byte, headerSize+commandSize)
	binary.LittleEndian.PutUint32(data[0:], 0xfeedfacf)
	binary.LittleEndian.PutUint32(data[4:], 0x0100000c)
	binary.LittleEndian.PutUint32(data[12:], 2)
	binary.LittleEndian.PutUint32(data[16:], 1)
	binary.LittleEndian.PutUint32(data[20:], uint32(commandSize))
	binary.LittleEndian.PutUint32(data[headerSize:], loadDylib)
	binary.LittleEndian.PutUint32(data[headerSize+4:], uint32(commandSize))
	binary.LittleEndian.PutUint32(data[headerSize+8:], dylibHeaderSize)
	copy(data[headerSize+dylibHeaderSize:], name)
	return data
}

func TestRunRequiresIndependentApolloCapabilityArgument(t *testing.T) {
	args := []string{
		"--app", "/fixture/Pantheon.app", "--version", "0.24.147", "--build", "0.24.147",
		"--info-plist", "/fixture/Info.plist", "--pkg-info", "/fixture/PkgInfo",
		"--launch-agent", "/fixture/LaunchAgent.plist", "--brand-logo", "/fixture/logo.png",
	}
	var out, errOut bytes.Buffer
	if code := run(args, &out, &errOut); code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "explicit --apollo-capability is required") {
		t.Fatalf("missing independent expectation did not fail closed: code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

func TestRunRequiresFrozenDigestsOnlyForExplicitReleaseVerification(t *testing.T) {
	args := []string{
		"--app", "/fixture/Pantheon.app", "--version", "0.24.147", "--build", "0.24.147",
		"--info-plist", "/fixture/Info.plist", "--pkg-info", "/fixture/PkgInfo",
		"--launch-agent", "/fixture/LaunchAgent.plist", "--brand-logo", "/fixture/logo.png",
		"--apollo-capability", "disabled", "--require-code-signature", "--require-frozen-digests",
	}
	var out, errOut bytes.Buffer
	if code := run(args, &out, &errOut); code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "requires frozen Info.plist and Apollo profile digests") {
		t.Fatalf("release standalone inventory accepted without frozen digests: code=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
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
