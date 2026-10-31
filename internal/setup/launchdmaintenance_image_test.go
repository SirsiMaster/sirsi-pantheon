package setup

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestMaintenanceImageCDHashUsesExactKernelReadback(t *testing.T) {
	candidate := bytes.Repeat([]byte{0x31}, 20)
	loadedCalls := 0
	got, err := maintenanceImageCDHash("/retained/sirsi", 4321,
		func(path string) ([]byte, error) {
			if path != "/retained/sirsi" {
				t.Fatalf("static image path = %q", path)
			}
			return append([]byte(nil), candidate...), nil
		},
		func(pid int) ([]byte, error) {
			loadedCalls++
			if pid != 4321 {
				t.Fatalf("loaded image pid = %d", pid)
			}
			return append([]byte(nil), candidate...), nil
		})
	if err != nil || got != strings.Repeat("31", 20) || loadedCalls != 1 {
		t.Fatalf("exact kernel image = %q, calls=%d, err=%v", got, loadedCalls, err)
	}
}

func TestMaintenanceImageCDHashRejectsWrongKernelHash(t *testing.T) {
	candidate := bytes.Repeat([]byte{0x31}, 20)
	wrong := bytes.Repeat([]byte{0x32}, 20)
	got, err := maintenanceImageCDHash("/retained/sirsi", 4321,
		func(string) ([]byte, error) { return candidate, nil },
		func(int) ([]byte, error) { return wrong, nil })
	if err == nil || got != "" || !strings.Contains(err.Error(), "does not match kernel-loaded CDHash") {
		t.Fatalf("wrong kernel image accepted: %q, err=%v", got, err)
	}
}

func TestMaintenanceImageCDHashRefusesUnknownKernelReadback(t *testing.T) {
	candidate := bytes.Repeat([]byte{0x31}, 20)
	wantErr := errors.New("injected csops refusal")
	got, err := maintenanceImageCDHash("/retained/sirsi", 4321,
		func(string) ([]byte, error) { return candidate, nil },
		func(int) ([]byte, error) { return nil, wantErr })
	if got != "" || !errors.Is(err, wantErr) {
		t.Fatalf("unknown kernel readback accepted: %q, err=%v", got, err)
	}
}

func TestMaintenanceImageCDHashRejectsMalformedKernelReadback(t *testing.T) {
	candidate := bytes.Repeat([]byte{0x31}, 20)
	got, err := maintenanceImageCDHash("/retained/sirsi", 4321,
		func(string) ([]byte, error) { return candidate, nil },
		func(int) ([]byte, error) { return candidate[:19], nil })
	if got != "" || err == nil || !strings.Contains(err.Error(), "malformed 20-byte CDHash") {
		t.Fatalf("malformed kernel hash accepted: %q, err=%v", got, err)
	}
}

func TestMaintenanceImageCDHashRequiresKernelReaderForLoadedPID(t *testing.T) {
	candidate := bytes.Repeat([]byte{0x31}, 20)
	got, err := maintenanceImageCDHash("/retained/sirsi", 4321,
		func(string) ([]byte, error) { return candidate, nil }, nil)
	if got != "" || err == nil || !strings.Contains(err.Error(), "reader unavailable") {
		t.Fatalf("missing kernel reader accepted: %q, err=%v", got, err)
	}
}

func TestMaintenanceImageCDHashArtifactModeDoesNotClaimLoadedIdentity(t *testing.T) {
	candidate := bytes.Repeat([]byte{0x31}, 20)
	loadedCalls := 0
	got, err := maintenanceImageCDHash("/retained/sirsi", 0,
		func(string) ([]byte, error) { return candidate, nil },
		func(int) ([]byte, error) {
			loadedCalls++
			return nil, errors.New("must not be called for artifact-only identity")
		})
	if err != nil || got != strings.Repeat("31", 20) || loadedCalls != 0 {
		t.Fatalf("artifact-only identity = %q, calls=%d, err=%v", got, loadedCalls, err)
	}
}

func TestMaintenanceLoadedImageIdentityReturnsKernelHashAndStableStart(t *testing.T) {
	loaded := bytes.Repeat([]byte{0x41}, 20)
	startCalls, loadedCalls := 0, 0
	got, err := maintenanceLoadedImageIdentity(539,
		func(pid int) (string, error) {
			startCalls++
			if pid != 539 {
				t.Fatalf("start PID = %d", pid)
			}
			return "1792847484.000000", nil
		},
		func(pid int) ([]byte, error) {
			loadedCalls++
			if pid != 539 {
				t.Fatalf("loaded PID = %d", pid)
			}
			return loaded, nil
		})
	if err != nil || got.PID != 539 || got.KernelStart != "1792847484.000000" || got.CDHash != strings.Repeat("41", 20) || startCalls != 2 || loadedCalls != 1 {
		t.Fatalf("loaded identity = %+v, starts=%d, hashes=%d, err=%v", got, startCalls, loadedCalls, err)
	}
}

func TestMaintenanceLoadedImageIdentityRefusesPIDReplacement(t *testing.T) {
	startCalls := 0
	got, err := maintenanceLoadedImageIdentity(539,
		func(int) (string, error) {
			startCalls++
			if startCalls == 1 {
				return "first-start", nil
			}
			return "replacement-start", nil
		},
		func(int) ([]byte, error) { return bytes.Repeat([]byte{0x41}, 20), nil })
	if err == nil || got != (MaintenanceImageIdentity{}) || startCalls != 2 {
		t.Fatalf("replaced PID identity accepted: %+v, starts=%d, err=%v", got, startCalls, err)
	}
}

func TestMaintenanceLoadedImageIdentityRefusesStartOrHashFailure(t *testing.T) {
	startErr := errors.New("injected process observation failure")
	loadedCalls := 0
	got, err := maintenanceLoadedImageIdentity(539,
		func(int) (string, error) { return "", startErr },
		func(int) ([]byte, error) {
			loadedCalls++
			return bytes.Repeat([]byte{0x41}, 20), nil
		})
	if err == nil || got != (MaintenanceImageIdentity{}) || loadedCalls != 0 {
		t.Fatalf("unknown PID observation accepted: %+v, calls=%d, err=%v", got, loadedCalls, err)
	}

	got, err = maintenanceLoadedImageIdentity(539,
		func(int) (string, error) { return "stable-start", nil },
		func(int) ([]byte, error) { return nil, errors.New("injected csops refusal") })
	if err == nil || got != (MaintenanceImageIdentity{}) {
		t.Fatalf("unknown CDHash accepted: %+v, err=%v", got, err)
	}
}

func TestMaintenanceLoadedImageIdentityRejectsInvalidPIDAndHashLength(t *testing.T) {
	readerCalls := 0
	start := func(int) (string, error) {
		readerCalls++
		return "stable-start", nil
	}
	hash := func(int) ([]byte, error) {
		readerCalls++
		return bytes.Repeat([]byte{0x41}, 19), nil
	}
	got, err := maintenanceLoadedImageIdentity(1, start, hash)
	if err == nil || got != (MaintenanceImageIdentity{}) || readerCalls != 0 {
		t.Fatalf("invalid PID queried: %+v, calls=%d, err=%v", got, readerCalls, err)
	}
	got, err = maintenanceLoadedImageIdentity(539, start, hash)
	if err == nil || got != (MaintenanceImageIdentity{}) || readerCalls != 2 {
		t.Fatalf("malformed kernel hash accepted: %+v, calls=%d, err=%v", got, readerCalls, err)
	}
}

func TestMaintenanceImageIdentityRejectsPIDOverflowBeforeReaders(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("an int larger than Darwin pid_t is not representable")
	}
	oversized := int(int64(maintenanceMaximumPID) + 1)
	readerCalls := 0
	start := func(int) (string, error) { readerCalls++; return "start", nil }
	loaded := func(int) ([]byte, error) { readerCalls++; return bytes.Repeat([]byte{0x41}, 20), nil }
	identity, err := maintenanceLoadedImageIdentity(oversized, start, loaded)
	if err == nil || identity != (MaintenanceImageIdentity{}) || readerCalls != 0 {
		t.Fatalf("oversized kernel PID queried: %+v, calls=%d, err=%v", identity, readerCalls, err)
	}
	_, err = maintenanceImageCDHash("retained-fixture", oversized,
		func(string) ([]byte, error) { readerCalls++; return bytes.Repeat([]byte{0x41}, 20), nil }, loaded)
	if err == nil || readerCalls != 0 {
		t.Fatalf("oversized candidate PID reached readers: calls=%d, err=%v", readerCalls, err)
	}
}

func TestMaintenanceImageCDHashRejectsMalformedInputs(t *testing.T) {
	valid := bytes.Repeat([]byte{0x31}, 20)
	for _, tc := range []struct {
		name string
		path string
		pid  int
		read maintenanceStaticCDHashReader
	}{
		{name: "empty path", path: "", pid: 4321, read: func(string) ([]byte, error) { return valid, nil }},
		{name: "invalid pid", path: "/retained/sirsi", pid: 1, read: func(string) ([]byte, error) { return valid, nil }},
		{name: "negative pid", path: "/retained/sirsi", pid: -1, read: func(string) ([]byte, error) { return valid, nil }},
		{name: "short retained hash", path: "/retained/sirsi", pid: 0, read: func(string) ([]byte, error) { return valid[:19], nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := maintenanceImageCDHash(tc.path, tc.pid, tc.read, nil)
			if err == nil || got != "" {
				t.Fatalf("malformed request accepted: %q, err=%v", got, err)
			}
		})
	}
}
