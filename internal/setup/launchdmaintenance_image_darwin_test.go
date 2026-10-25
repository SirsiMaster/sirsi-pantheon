//go:build darwin && cgo

package setup

import (
	"bufio"
	"context"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
)

const maintenanceCDHashChildEnv = "SIRSI_MAINTENANCE_CDHASH_SELF_CHILD"

func TestMaintenanceKernelCDHashOwnedSelfChild(t *testing.T) {
	if os.Getenv(maintenanceCDHashChildEnv) == "1" {
		if _, err := io.WriteString(os.Stdout, "ready\n"); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMaintenanceKernelCDHashOwnedSelfChild$")
	child.Env = append(os.Environ(), maintenanceCDHashChildEnv+"=1")
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !waited {
			_ = child.Wait()
		}
	})
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(ready) != "ready" {
		t.Fatalf("owned child readiness = %q, err=%v", ready, err)
	}

	pid := child.Process.Pid
	identity, err := maintenanceLoadedImageIdentity(pid, servicedrain.MaintenanceStart, maintenanceKernelCDHash)
	if err != nil || identity.PID != pid || identity.KernelStart == "" || len(identity.CDHash) != 40 {
		t.Fatalf("owned child kernel identity = %+v, err=%v", identity, err)
	}
	candidate, err := hex.DecodeString(identity.CDHash)
	if err != nil || len(candidate) != 20 {
		t.Fatalf("owned child CDHash encoding invalid: %q, err=%v", identity.CDHash, err)
	}
	loaded, err := maintenanceKernelCDHash(pid)
	if err != nil {
		t.Fatalf("owned child raw kernel hash: %v", err)
	}
	if err := maintenanceCompareCDHash(candidate, loaded); err != nil {
		t.Fatalf("exact owned-child CDHash rejected: %v", err)
	}
	wrong := append([]byte(nil), candidate...)
	wrong[0] ^= 0xff
	if err := maintenanceCompareCDHash(wrong, loaded); err == nil {
		t.Fatal("wrong owned-child CDHash accepted")
	}

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		waited = true
		t.Fatalf("owned child exit: %v", err)
	}
	waited = true
}
