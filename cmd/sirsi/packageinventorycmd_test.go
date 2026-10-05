package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestPackageInventoryCommandUsesProductionSummaryWithoutExecutingPayload(t *testing.T) {
	if packageInventoryCmd.Parent() != rootCmd {
		t.Fatal("package-inventory is not registered on the canonical sirsi command")
	}
	for _, name := range []string{"app", "version", "build", "info-plist", "pkg-info", "launch-agent", "require-code-signature"} {
		if packageInventoryCmd.Flags().Lookup(name) == nil {
			t.Errorf("package-inventory is missing --%s", name)
		}
	}
	if err := packageInventoryCmd.Args(packageInventoryCmd, []string{"unexpected"}); err == nil {
		t.Fatal("package-inventory accepted an unexpected positional argument")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "Pantheon.app")
	for _, dir := range []string{"Contents/MacOS", "Contents/Resources", "Contents/_CodeSignature"} {
		if err := os.MkdirAll(filepath.Join(app, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	const version, build = "0.23.9-beta", "20260908"
	info := []byte(`<?xml version="1.0"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>ai.sirsi.pantheon</string>
<key>LSUIElement</key><true/>
<key>CFBundleShortVersionString</key><string>0.23.9-beta</string>
<key>CFBundleVersion</key><string>20260908</string>
</dict></plist>`)
	if err := os.WriteFile(filepath.Join(app, "Contents/Info.plist"), info, 0o644); err != nil {
		t.Fatal(err)
	}

	canonicalPath := func(name string) string {
		t.Helper()
		path, err := filepath.Abs(filepath.Join("..", "sirsi-menubar", "bundle", name))
		if err != nil {
			t.Fatalf("resolve canonical %s path: %v", name, err)
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatalf("resolve canonical %s: %v", name, err)
		}
		return path
	}
	pkgInfoPath := canonicalPath("PkgInfo")
	launchAgentPath := canonicalPath("ai.sirsi.pantheon.plist")
	for rel, data := range map[string][]byte{
		"Contents/PkgInfo":                           mustRead(t, pkgInfoPath),
		"Contents/MacOS/sirsi":                       []byte("#!/bin/sh\nprintf invoked > \"" + filepath.Join(root, "payload-ran") + "\"\n"),
		"Contents/MacOS/sirsi-menubar":               []byte("menubar payload bytes"),
		"Contents/Resources/ai.sirsi.pantheon.plist": mustRead(t, launchAgentPath),
		// This exercises the inventory command's structural signature-payload
		// requirement only; cryptographic identity is checked by the preceding
		// release identity gate, not inferred from this fixture.
		"Contents/_CodeSignature/CodeResources": []byte("test signature resource seal"),
	} {
		mode := os.FileMode(0o644)
		if rel == "Contents/MacOS/sirsi" || rel == "Contents/MacOS/sirsi-menubar" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(app, filepath.FromSlash(rel)), data, mode); err != nil {
			t.Fatal(err)
		}
	}

	previous := struct {
		app, version, build, infoPlist, pkgInfo, launchAgent string
		requireCodeSignature                                 bool
		jsonOutput                                           bool
	}{
		app: packageInventoryApp, version: packageInventoryVersion, build: packageInventoryBuild,
		infoPlist: packageInventoryInfoPlist, pkgInfo: packageInventoryPkgInfo,
		launchAgent: packageInventoryLaunchAgent, requireCodeSignature: packageInventoryRequireCodeSignature,
		jsonOutput: JsonOutput,
	}
	type priorFlag struct {
		name, value string
		changed     bool
	}
	priorFlags := make([]priorFlag, 0, packageInventoryCmd.Flags().NFlag())
	for _, name := range []string{"app", "version", "build", "info-plist", "pkg-info", "launch-agent", "require-code-signature"} {
		flag := packageInventoryCmd.Flags().Lookup(name)
		priorFlags = append(priorFlags, priorFlag{name: name, value: flag.Value.String(), changed: flag.Changed})
	}
	t.Cleanup(func() {
		for _, prior := range priorFlags {
			flag := packageInventoryCmd.Flags().Lookup(prior.name)
			if err := flag.Value.Set(prior.value); err != nil {
				t.Errorf("restore --%s: %v", prior.name, err)
			}
			flag.Changed = prior.changed
		}
		packageInventoryApp = previous.app
		packageInventoryVersion = previous.version
		packageInventoryBuild = previous.build
		packageInventoryInfoPlist = previous.infoPlist
		packageInventoryPkgInfo = previous.pkgInfo
		packageInventoryLaunchAgent = previous.launchAgent
		packageInventoryRequireCodeSignature = previous.requireCodeSignature
		JsonOutput = previous.jsonOutput
	})
	for _, flag := range []struct{ name, value string }{
		{name: "app", value: app},
		{name: "version", value: version},
		{name: "build", value: build},
		{name: "info-plist", value: filepath.Join(app, "Contents/Info.plist")},
		{name: "pkg-info", value: pkgInfoPath},
		{name: "launch-agent", value: launchAgentPath},
		{name: "require-code-signature", value: "true"},
	} {
		if err := packageInventoryCmd.Flags().Set(flag.name, flag.value); err != nil {
			t.Fatalf("set --%s: %v", flag.name, err)
		}
	}
	JsonOutput = false

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := packageInventoryCmd.RunE(command, nil); err != nil {
		t.Fatalf("package-inventory command failed: %v", err)
	}
	const expectedSummary = "package inventory accepted=true entries=10 engines=1 executables=2 python_free=true\n"
	if output.String() != expectedSummary {
		t.Fatalf("command output %q; want %q", output.String(), expectedSummary)
	}
	if _, err := os.Lstat(filepath.Join(root, "payload-ran")); !os.IsNotExist(err) {
		t.Fatalf("inventory executed the packaged CLI payload; sentinel stat error=%v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
