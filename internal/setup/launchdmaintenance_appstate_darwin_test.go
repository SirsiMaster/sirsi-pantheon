//go:build darwin && cgo

package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func appStateFixture(t *testing.T) (maintenanceJournal, MaintenanceAppAdmission) {
	t.Helper()
	path, _ := maintenanceOwnedDirectory(t)
	j := maintenanceJournal{path, uint32(os.Getuid())}
	ticket := MaintenanceAppTicket{Schema: 1, Generation: 1, ID: strings.Repeat("1", 32), Nonce: strings.Repeat("2", 32),
		Operation: "confirm", Plan: fixtureMaintenancePlan(), Transaction: strings.Repeat("2", 32)}
	return j, MaintenanceAppAdmission{Schema: 1, HighWater: 1, Ticket: ticket, State: "reserved"}
}

func TestMaintenanceAppStateDurableFenceAndLateArrival(t *testing.T) {
	j, reserved := appStateFixture(t)
	if err := j.persistAppState(MaintenanceAppAdmission{}, reserved); err != nil {
		t.Fatal(err)
	}
	observed, err := j.appState()
	if err != nil || observed != reserved {
		t.Fatal(observed, err)
	}
	running := reserved
	running.State = "running"
	if err := j.persistAppState(reserved, running); err != nil {
		t.Fatal(err)
	}
	fenced := running
	fenced.HighWater++
	fenced.State = "fenced"
	fenced.Error = "admitted-past-outcome-unknown"
	if err := j.persistAppState(running, fenced); err != nil {
		t.Fatal(err)
	}
	if err := j.syncAppState(fenced); err != nil {
		t.Fatal("durability reconciliation", err)
	}
	if err := j.syncAppState(running); err == nil {
		t.Fatal("stale record synchronized as current authority")
	}
	// Fresh journal object models a process restart, not an in-memory latch.
	restarted := maintenanceJournal{j.directory, j.owner}
	if got, err := restarted.appState(); err != nil || got != fenced {
		t.Fatal(got, err)
	}
	if err := restarted.persistAppState(reserved, running); err == nil {
		t.Fatal("late original reset durable fence")
	}
	if got, err := restarted.appState(); err != nil || got != fenced {
		t.Fatal("failed CAS changed fence", got, err)
	}
	info, err := os.Lstat(filepath.Join(j.directory, maintenanceAppStateLeaf))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private state precondition", info, err)
	}
}

func TestMaintenanceAppStateRefusesNamespaceAndModeDrift(t *testing.T) {
	for _, name := range []string{"public-mode", "symlink", "hardlink", "fifo", "truncated", "duplicate-json", "parent-renamed"} {
		t.Run(name, func(t *testing.T) {
			j, reserved := appStateFixture(t)
			if err := j.persistAppState(MaintenanceAppAdmission{}, reserved); err != nil {
				t.Fatal(err)
			}
			leaf := filepath.Join(j.directory, maintenanceAppStateLeaf)
			switch name {
			case "public-mode":
				if err := os.Chmod(leaf, 0755); err != nil {
					t.Fatal(err)
				}
				if st, err := os.Stat(leaf); err != nil || st.Mode().Perm() != 0755 {
					t.Fatal("negative fixture mode", st, err)
				}
			case "symlink":
				if err := os.Rename(leaf, leaf+".retained"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(leaf+".retained", leaf); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(leaf, leaf+".second-name"); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := os.Rename(leaf, leaf+".retained"); err != nil {
					t.Fatal(err)
				}
				if err := unix.Mkfifo(leaf, 0600); err != nil {
					t.Fatal(err)
				}
			case "truncated":
				if err := os.WriteFile(leaf, []byte("{\"schema\":"), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate-json":
				b, err := os.ReadFile(leaf)
				if err != nil {
					t.Fatal(err)
				}
				b = append([]byte("{\"schema\":1,"), b[1:]...)
				if err := os.WriteFile(leaf, b, 0600); err != nil {
					t.Fatal(err)
				}
			case "parent-renamed":
				if err := os.Rename(j.directory, j.directory+".retained"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Rename(j.directory+".retained", j.directory) })
			}
			if _, err := j.appState(); err == nil {
				t.Fatal("untrusted admission namespace accepted")
			}
			running := reserved
			running.State = "running"
			if err := j.persistAppState(reserved, running); err == nil {
				t.Fatal("unknown state overwritten")
			}
		})
	}
}

func TestMaintenanceAppStateRetainedStageDebtIsBounded(t *testing.T) {
	j, reserved := appStateFixture(t)
	for i := 0; i < 32; i++ {
		if err := os.WriteFile(filepath.Join(j.directory, fmt.Sprintf("%s.%032x.stage", maintenanceAppStateLeaf, i)), []byte("inert retained debt"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.persistAppState(MaintenanceAppAdmission{}, reserved); err == nil {
		t.Fatal("staging debt bound ignored")
	}
	if _, err := os.Lstat(filepath.Join(j.directory, maintenanceAppStateLeaf)); !os.IsNotExist(err) {
		t.Fatal("state published despite debt", err)
	}
	entries, err := os.ReadDir(j.directory)
	if err != nil || len(entries) != 32 {
		t.Fatal("retained debt removed/expanded", len(entries), err)
	}
}
