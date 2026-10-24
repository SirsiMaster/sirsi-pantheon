//go:build darwin

package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type mcpMigrationFixture struct {
	migration      *RouterMCPMigration
	selection      RouterMCPSelection
	before         []byte
	engine, legacy string
}

func fixtureMCPIdentity(path string) (RouterMCPEngine, error) {
	f, info, err := openCanonicalPath(path, false, false)
	if err != nil {
		return RouterMCPEngine{}, err
	}
	defer func() { _ = f.Close() }()
	if !info.Mode().IsRegular() || !ownedByCurrentUser(info) || linkCount(info) != 1 || info.Mode().Perm()&0022 != 0 {
		return RouterMCPEngine{}, errors.New("unsafe fixture payload")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return RouterMCPEngine{}, err
	}
	st := info.Sys().(*syscall.Stat_t)
	return RouterMCPEngine{Path: path, SHA256: routerMCPHash(b), Device: uint64(st.Dev), Inode: st.Ino}, nil
}

func newMCPMigrationFixture(t *testing.T, client, project string) *mcpMigrationFixture {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".claude.json")
	if client == "cursor" {
		path = filepath.Join(home, ".cursor", "mcp.json")
	}
	legacy := filepath.Join(home, ".local", "bin", "sirsi-router-mcp")
	engine := filepath.Join(home, "managed", "sirsi")
	for _, p := range []string{path, legacy, engine} {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{legacy, engine} {
		if err := os.WriteFile(p, []byte("inert fixture payload; never executed"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	quoted, _ := json.Marshal(legacy)
	entry := `{"command":` + string(quoted) + `,"args":[],"env":{"TOKEN":"owned-private-secret"},"cwd":"owned-private-cwd","disabled":false}`
	servers := `{"router":` + entry + `,"sirsi":{"command":"sirsi","args":["mcp"]},"other":{"command":"unchanged","env":{"OTHER":"keep-private"}}}`
	data := `{"mcpServers":` + servers + `,"settings":{"number":1.20e+2}}`
	if project != "" {
		key, _ := json.Marshal(project)
		data = `{"projects":{` + string(key) + `:{"mcpServers":` + servers + `,"trusted":true}},"mcpServers":{"other":{"command":"top-unchanged"}}}`
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	// Per-instance payload qualification probes are doubled. Real selected
	// filesystem reads, backup/staging, swap, fsync and readback are not doubled.
	m := &RouterMCPMigration{home: home, engine: func() (RouterMCPEngine, error) { return fixtureMCPIdentity(engine) }, legacy: fixtureMCPIdentity}
	return &mcpMigrationFixture{migration: m, selection: RouterMCPSelection{Client: client, Config: path, Server: "router", Project: project}, before: []byte(data), engine: engine, legacy: legacy}
}

func TestRouterMCPMigrationPlanApplyReadbackAndNoop(t *testing.T) {
	for _, tc := range []struct{ client, project string }{{"claude-code", ""}, {"claude-code", "/owned/project"}, {"cursor", ""}} {
		t.Run(tc.client+tc.project, func(t *testing.T) {
			f := newMCPMigrationFixture(t, tc.client, tc.project)
			plan, err := f.migration.Plan(f.selection)
			if err != nil {
				t.Fatal(err)
			}
			preview, _ := json.Marshal(plan)
			for _, secret := range []string{"owned-private-secret", "owned-private-cwd", "keep-private"} {
				if bytes.Contains(preview, []byte(secret)) {
					t.Fatal("preview leaked preserved values")
				}
			}
			if b, err := os.ReadFile(f.selection.Config); err != nil || !bytes.Equal(b, f.before) {
				t.Fatal("plan mutated config")
			}
			if _, err := os.Stat(plan.Backup); !os.IsNotExist(err) {
				t.Fatal("plan created backup")
			}
			result, err := f.migration.Apply(f.selection, plan.Digest)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "configuration_migrated_client_reconnect_pending" {
				t.Fatal("migration falsely implied client reconnection")
			}
			want, _, _, err := migrateMCPBytes(f.before, f.selection, f.legacy, f.engine)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(f.selection.Config)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatal("selected publication bytes differ")
			}
			for _, path := range []string{result.Backup, result.RetainedStage} {
				b, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(b, f.before) {
					t.Fatal("original not recoverable")
				}
				info, err := os.Lstat(path)
				if err != nil || info.Mode().Perm() != 0600 || linkCount(info) != 1 {
					t.Fatal("original backup/stage not private single-link")
				}
			}
			next, err := f.migration.Plan(f.selection)
			if err != nil {
				t.Fatal(err)
			}
			beforeEntries, _ := os.ReadDir(filepath.Dir(f.selection.Config))
			noop, err := f.migration.Apply(f.selection, next.Digest)
			if err != nil || noop.Status != "already_migrated_client_reconnect_unobserved" {
				t.Fatalf("verified no-op refused: %v", err)
			}
			afterEntries, _ := os.ReadDir(filepath.Dir(f.selection.Config))
			if len(beforeEntries) != len(afterEntries) {
				t.Fatal("no-op created artifacts")
			}
		})
	}
}

func TestRouterMCPMigrationRefusesDriftAndRetainsRecovery(t *testing.T) {
	for _, which := range []string{"source-bytes", "source-identity", "backup-collision", "engine-before-apply", "engine-before-publish", "legacy-before-publish", "source-before-publish", "replacement-before-publish", "stage-before-publish", "write-failure", "sync-failure", "readback-failure", "readback-replacement"} {
		t.Run(which, func(t *testing.T) {
			f := newMCPMigrationFixture(t, "cursor", "")
			plan, err := f.migration.Plan(f.selection)
			if err != nil {
				t.Fatal(err)
			}
			want := f.before
			changed := []byte(`{"external":"preserve concurrent edit"}`)
			switch which {
			case "source-bytes":
				if err := os.WriteFile(f.selection.Config, changed, 0600); err != nil {
					t.Fatal(err)
				}
				want = changed
			case "source-identity":
				if err := os.Rename(f.selection.Config, f.selection.Config+".held"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(f.selection.Config, f.before, 0600); err != nil {
					t.Fatal(err)
				}
			case "backup-collision":
				if err := os.WriteFile(plan.Backup, []byte("occupied"), 0600); err != nil {
					t.Fatal(err)
				}
			case "engine-before-apply":
				if err := os.WriteFile(f.engine, []byte("changed engine"), 0700); err != nil {
					t.Fatal(err)
				}
			case "engine-before-publish":
				f.migration.beforePublish = func() error { return os.WriteFile(f.engine, []byte("changed engine"), 0700) }
			case "legacy-before-publish":
				f.migration.beforePublish = func() error { return os.WriteFile(f.legacy, []byte("changed legacy"), 0700) }
			case "source-before-publish":
				f.migration.beforePublish = func() error { return os.WriteFile(f.selection.Config, changed, 0600) }
				want = changed
			case "replacement-before-publish":
				f.migration.beforePublish = func() error {
					if err := os.Rename(f.selection.Config, f.selection.Config+".held"); err != nil {
						return err
					}
					return os.WriteFile(f.selection.Config, f.before, 0600)
				}
			case "stage-before-publish":
				f.migration.beforePublish = func() error {
					entries, err := os.ReadDir(filepath.Dir(f.selection.Config))
					if err != nil {
						return err
					}
					for _, entry := range entries {
						if strings.HasSuffix(entry.Name(), ".stage.json") {
							path := filepath.Join(filepath.Dir(f.selection.Config), entry.Name())
							if err := os.Rename(path, path+".held"); err != nil {
								return err
							}
							return os.WriteFile(path, []byte("unknown replacement stage"), 0600)
						}
					}
					return errors.New("fixture stage absent")
				}
			case "write-failure":
				f.migration.writePrivate = func(file *os.File, b []byte) error {
					_, _ = file.Write(b[:1])
					return errors.New("injected write failure")
				}
			case "sync-failure":
				f.migration.writePrivate = func(file *os.File, b []byte) error {
					if _, err := file.Write(b); err != nil {
						return err
					}
					return errors.New("injected fsync failure")
				}
			case "readback-failure":
				f.migration.afterPublish = func() error { return errors.New("injected readback failure") }
				want, _, _, err = migrateMCPBytes(f.before, f.selection, f.legacy, f.engine)
				if err != nil {
					t.Fatal(err)
				}
			case "readback-replacement":
				f.migration.afterPublish = func() error {
					if err := os.Rename(f.selection.Config, f.selection.Config+".published"); err != nil {
						return err
					}
					return os.WriteFile(f.selection.Config, changed, 0600)
				}
				want = changed
			}
			result, err := f.migration.Apply(f.selection, plan.Digest)
			if err == nil || result.Status == "configuration_migrated_client_reconnect_pending" {
				t.Fatal("drift/failure reported successful migration")
			}
			got, readErr := os.ReadFile(f.selection.Config)
			if readErr != nil || !bytes.Equal(got, want) {
				t.Fatal("failure overwrote selected or concurrent config")
			}
			if which == "backup-collision" {
				got, _ := os.ReadFile(plan.Backup)
				if string(got) != "occupied" {
					t.Fatal("occupied backup overwritten")
				}
			}
			if strings.Contains(which, "publish") || which == "readback-replacement" {
				if _, err := os.Stat(plan.Backup); err != nil {
					t.Fatal("failure discarded recovery backup")
				}
			}
			if which == "stage-before-publish" {
				b, err := os.ReadFile(result.RetainedStage)
				if err != nil || string(b) != "unknown replacement stage" {
					t.Fatal("unknown stage replacement was removed or overwritten")
				}
			}
		})
	}
}

func TestRouterMCPMigrationRefusesUnsafeSelectedFilesystem(t *testing.T) {
	for _, which := range []string{"symlink-config", "hardlink-config", "fifo-config", "writable-config", "writable-parent", "symlink-parent", "missing-server", "generic-mcp", "bad-confirmation", "payload-qualification"} {
		t.Run(which, func(t *testing.T) {
			f := newMCPMigrationFixture(t, "cursor", "")
			switch which {
			case "symlink-config":
				if err := os.Rename(f.selection.Config, f.selection.Config+".held"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(f.selection.Config+".held", f.selection.Config); err != nil {
					t.Fatal(err)
				}
			case "hardlink-config":
				if err := os.Link(f.selection.Config, f.selection.Config+".linked"); err != nil {
					t.Fatal(err)
				}
			case "fifo-config":
				if err := os.Remove(f.selection.Config); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(f.selection.Config, 0600); err != nil {
					t.Fatal(err)
				}
			case "writable-config":
				if err := os.Chmod(f.selection.Config, 0666); err != nil {
					t.Fatal(err)
				}
			case "writable-parent":
				if err := os.Chmod(filepath.Dir(f.selection.Config), 0777); err != nil {
					t.Fatal(err)
				}
			case "symlink-parent":
				parent := filepath.Dir(f.selection.Config)
				if err := os.Rename(parent, parent+".held"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(parent+".held", parent); err != nil {
					t.Fatal(err)
				}
			case "missing-server":
				f.selection.Server = "missing"
			case "generic-mcp":
				f.selection.Server = "sirsi"
			case "payload-qualification":
				f.migration.engine = func() (RouterMCPEngine, error) { return RouterMCPEngine{}, errors.New("injected unsigned target") }
			case "bad-confirmation":
				if _, err := f.migration.Apply(f.selection, strings.Repeat("0", 64)); err == nil {
					t.Fatal("unconfirmed apply accepted")
				}
				return
			}
			if _, err := f.migration.Plan(f.selection); err == nil {
				t.Fatal("unsafe selection accepted")
			}
		})
	}
}

func TestRouterMCPLegacyQualificationRefusesWrapperWithoutExecution(t *testing.T) {
	f := newMCPMigrationFixture(t, "claude-code", "")
	if err := os.WriteFile(f.legacy, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyRouterMCPEngine(f.legacy); err == nil {
		t.Fatal("script wrapper accepted as known Go standalone")
	}
}

type foreignMCPInfo struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (f foreignMCPInfo) Sys() interface{} { return &f.stat }

func TestRouterMCPMigrationRejectsForeignOwnerWithoutPrivilegeChanges(t *testing.T) {
	f := newMCPMigrationFixture(t, "cursor", "")
	info, err := os.Lstat(f.selection.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !validRouterMCPConfigInfo(info) {
		t.Fatal("owned baseline refused")
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid = uint32(os.Getuid()) + 1
	if validRouterMCPConfigInfo(foreignMCPInfo{FileInfo: info, stat: stat}) {
		t.Fatal("production metadata predicate accepted foreign ownership")
	}
}
