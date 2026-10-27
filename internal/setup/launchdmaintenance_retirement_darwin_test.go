//go:build darwin && cgo

package setup

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/servicedrain"
	"golang.org/x/sys/unix"
)

func TestMaintenanceRetirementRequiresIndependentKernelAndCompleteJobAbsence(t *testing.T) {
	sentinel := errors.New("complete system list unavailable")
	cases := []struct {
		name, start          string
		startErr, existsErr  error
		listed               bool
		listErr              error
		want                 string
		wantErr, probe, list bool
	}{
		{name: "same live identity", start: "123.000001", want: "original_alive"},
		{name: "reused PID", start: "124.000001", want: "pid_reused_or_changed", wantErr: true},
		{name: "EIO absent", startErr: unix.EIO, existsErr: unix.ESRCH, want: "retired", probe: true, list: true},
		{name: "ESRCH independently absent", startErr: unix.ESRCH, existsErr: unix.ESRCH, want: "retired", probe: true, list: true},
		{name: "EIO live PID", startErr: unix.EIO, want: "outcome_unknown", wantErr: true, probe: true},
		{name: "ESRCH live PID", startErr: unix.ESRCH, want: "outcome_unknown", wantErr: true, probe: true},
		{name: "EIO inaccessible PID", startErr: unix.EIO, existsErr: unix.EPERM, want: "outcome_unknown", wantErr: true, probe: true},
		{name: "EIO unknown kernel error", startErr: unix.EIO, existsErr: unix.EINVAL, want: "outcome_unknown", wantErr: true, probe: true},
		{name: "identity permission failure", startErr: unix.EPERM, existsErr: unix.ESRCH, want: "outcome_unknown", wantErr: true},
		{name: "absent PID but job present", startErr: unix.EIO, existsErr: unix.ESRCH, listed: true, want: "selected_job_still_loaded_or_changed", probe: true, list: true},
		{name: "absent PID but incomplete job read", startErr: unix.EIO, existsErr: unix.ESRCH, listErr: sentinel, want: "outcome_unknown", wantErr: true, probe: true, list: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := MaintenancePlan{}
			p.Original.PID = 555
			p.KernelStart = "123.000001"
			probed, listed := false, false
			state, err := maintenanceRetirementWith(context.Background(), p, func(pid int) (string, error) {
				if pid != 555 {
					t.Fatal("wrong selected PID")
				}
				return tc.start, tc.startErr
			}, func(pid int) error {
				probed = true
				if pid != 555 {
					t.Fatal("wrong existence PID")
				}
				return tc.existsErr
			}, func(context.Context) (bool, error) { listed = true; return tc.listed, tc.listErr })
			if state != tc.want || (err != nil) != tc.wantErr || probed != tc.probe || listed != tc.list {
				t.Fatalf("state=%s err=%v probe=%v list=%v; want %s err=%v probe=%v list=%v", state, err, probed, listed, tc.want, tc.wantErr, tc.probe, tc.list)
			}
		})
	}
}

func TestMaintenanceRetirementRefusesInvalidPIDAndCancelledContext(t *testing.T) {
	for _, pid := range []int{0, -1} {
		p := MaintenancePlan{}
		p.Original.PID = pid
		state, err := maintenanceRetirementWith(context.Background(), p, func(int) (string, error) { t.Fatal("invalid PID identity probe"); return "", nil }, func(int) error { t.Fatal("invalid PID signal probe"); return nil }, func(context.Context) (bool, error) { t.Fatal("invalid PID job read"); return false, nil })
		if state != "outcome_unknown" || err == nil {
			t.Fatalf("invalid PID accepted: %s %v", state, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state, err := maintenanceRetirementWith(ctx, MaintenancePlan{}, func(int) (string, error) { t.Fatal("canceled identity read"); return "", nil }, func(int) error { t.Fatal("canceled existence probe"); return nil }, func(context.Context) (bool, error) { t.Fatal("canceled job read"); return false, nil })
	if state != "outcome_unknown" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %s %v", state, err)
	}
}

func TestMaintenanceRetirementDarwinAbsentKernelPID(t *testing.T) {
	child := exec.Command("/usr/bin/true")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pid := child.Process.Pid
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Kill(pid, 0); !errors.Is(err, unix.ESRCH) {
		t.Fatalf("fixture PID not independently absent: %v", err)
	}
	_, identityErr := servicedrain.MaintenanceStart(pid)
	if !errors.Is(identityErr, unix.EIO) && !errors.Is(identityErr, unix.ESRCH) {
		t.Fatalf("unexpected absent PID identity result: %v", identityErr)
	}
	p := MaintenancePlan{}
	p.Original.PID = pid
	p.KernelStart = "fixture:exited"
	state, err := maintenanceRetirementWith(context.Background(), p, servicedrain.MaintenanceStart, func(pid int) error { return unix.Kill(pid, 0) }, func(context.Context) (bool, error) { return false, nil })
	if state != "retired" || err != nil {
		t.Fatalf("absent kernel PID refused: state=%s err=%v (identity=%v)", state, err, identityErr)
	}
	t.Logf("owned exited PID=%d identity=%v, independent kill0=ESRCH, fixture complete job absence", pid, identityErr)
}
