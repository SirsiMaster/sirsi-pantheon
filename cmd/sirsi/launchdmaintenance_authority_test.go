package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestMaintenanceAuthorizationRetainsRegisteredRuntimeAndGrant(t *testing.T) {
	for _, kind := range []string{"allowed", "no_grant", "foreign_actor", "foreign_runtime", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(home)
			t.Setenv("SIRSI_AGENT_ID", "owned-maintainer")
			t.Setenv("SIRSI_THREAD_ID", "owned-thread")
			t.Setenv("SIRSI_ROUTER_AGENT", "")
			config := router.AgentConfig{ID: "owned-maintainer", LifecycleServices: []string{"ai.sirsi.router.relay"}}
			if kind == "no_grant" {
				config.LifecycleServices = nil
			}
			reg := &router.Registry{Agents: map[string]router.AgentConfig{"owned-maintainer": config}}
			root := filepath.Join(home, ".agents", "idea-router")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := router.SaveRegistry(root, reg); err != nil {
				t.Fatal(err)
			}
			thread := router.Thread{ThreadID: "owned-thread", AgentID: "owned-maintainer", Status: router.ThreadStatusActive, MachineID: router.MachineID(), PID: os.Getpid(), StartTime: router.PIDStartTimeOf(os.Getpid())}
			if kind == "foreign_actor" {
				thread.AgentID = "foreign"
			}
			if kind == "foreign_runtime" {
				thread.StartTime = "not-current"
			}
			payload, err := json.Marshal(thread)
			if err != nil {
				t.Fatal(err)
			}
			reads := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
			}
			err = authorizeLifecycleWithReader(ctx, "ai.sirsi.router.relay", func(ctx context.Context, repo, actor, tid string) (routerstore.ThreadRecord, error) {
				reads++
				if actor != "owned-maintainer" || tid != "owned-thread" {
					t.Fatal("audience changed")
				}
				return routerstore.ThreadRecord{ThreadID: tid, Agent: actor, Status: "active", Payload: payload}, nil
			})
			if (err == nil) != (kind == "allowed") {
				t.Fatalf("%s: %v", kind, err)
			}
			if (kind == "cancel" || kind == "no_grant") && reads != 0 {
				t.Fatal("authority consulted before intent/grant checks")
			}
		})
	}
}
