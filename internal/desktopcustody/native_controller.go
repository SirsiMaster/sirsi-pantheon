package desktopcustody

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// nativeRead is bounded and uses fixed executable/argument vectors. It is
// private: the production CLI accepts no command, sudo or executable override.
func nativeRead(ctx context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, path, args...)
	var b bytes.Buffer
	c.Stdout = &boundedNativeOutput{buffer: &b, remaining: 16 << 20}
	var stderr bytes.Buffer
	c.Stderr = &boundedNativeOutput{buffer: &stderr, remaining: 4096}
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("native read %s: %w (%s)", path, err, stderr.String())
	}
	if stderr.Len() != 0 {
		return nil, fmt.Errorf("native read %s: incomplete evidence or visibility warning (%s)", path, stderr.String())
	}
	return b.Bytes(), nil
}

type boundedNativeOutput struct {
	buffer    *bytes.Buffer
	remaining int
}

func (w *boundedNativeOutput) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, errors.New("native evidence exceeds bound")
	}
	w.remaining -= len(p)
	return w.buffer.Write(p)
}

func NativeIdentity(ctx context.Context, host string) (HostIdentity, error) {
	if runtime.GOOS != "darwin" || !identifierRE.MatchString(host) {
		return HostIdentity{}, errors.New("sleep custody: actual Darwin host identity required")
	}
	b, err := nativeRead(ctx, "/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	if err != nil {
		return HostIdentity{}, err
	}
	m := regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([A-Fa-f0-9-]{36})"`).FindSubmatch(b)
	if len(m) != 2 {
		return HostIdentity{}, errors.New("sleep custody: actual platform UUID unavailable")
	}
	boot, err := nativeRead(ctx, "/usr/sbin/sysctl", "-n", "kern.bootsessionuuid")
	if err != nil {
		return HostIdentity{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return HostIdentity{}, err
	}
	f, err := os.Open(exe)
	if err != nil {
		return HostIdentity{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return HostIdentity{}, err
	}
	i := HostIdentity{Host: host, PlatformUUID: string(m[1]), BootID: strings.TrimSpace(string(boot)), ExecutableSHA256: hex.EncodeToString(h.Sum(nil))}
	if i.BootID == "" {
		return HostIdentity{}, errors.New("sleep custody: boot session unavailable")
	}
	return i, nil
}

// NativeInventory is a digest of the reviewed process + open-write inventory,
// not a CPU-idle guess. New processes/start times/writers change the digest.
// Management must review the baseline alongside real participant custody;
// a matching digest alone is not an assertion of payload quiescence.
func NativeInventory(ctx context.Context) (string, error) {
	if runtime.GOOS != "darwin" || os.Geteuid() != 0 {
		return "", errors.New("sleep custody: existing privileged native custody service required for complete writer visibility; no elevation fallback")
	}
	ps, err := nativeRead(ctx, "/bin/ps", "-axo", "pid=,ppid=,lstart=,comm=")
	if err != nil {
		return "", err
	}
	files, err := nativeRead(ctx, "/usr/sbin/lsof", "-nP", "-F0pcfatn")
	if err != nil {
		return "", err
	}
	assertions, err := nativeRead(ctx, "/usr/bin/pmset", "-g", "assertions")
	if err != nil {
		return "", err
	}
	return inventoryDigest(ps, files, assertions, os.Getpid())
}

func inventoryDigest(ps, files, assertions []byte, self int) (string, error) {
	// Require an explicit OS assertion summary. No assertion parser fallback.
	for _, name := range []string{"PreventSystemSleep", "PreventUserIdleSystemSleep", "ExternalMedia", "UserIsActive"} {
		p := regexp.MustCompile(`(?m)^\s*` + name + `\s+([0-9]+)\s*$`).FindSubmatch(assertions)
		if len(p) != 2 || string(p[1]) != "0" {
			return "", fmt.Errorf("sleep custody: native assertion %s prevents quiet qualification", name)
		}
	}
	type row struct {
		pid, ppid int
		line      string
	}
	var rows []row
	ignore := map[int]bool{self: true}
	for _, line := range strings.Split(strings.TrimSpace(string(ps)), "\n") {
		f := strings.Fields(line)
		if len(f) < 8 {
			return "", errors.New("sleep custody: malformed process inventory")
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		if e1 != nil || e2 != nil {
			return "", errors.New("sleep custody: invalid process identity")
		}
		rows = append(rows, row{pid, ppid, strings.Join(f, " ")})
	}
	for changed := true; changed; {
		changed = false
		for _, r := range rows {
			if ignore[r.ppid] && !ignore[r.pid] {
				ignore[r.pid] = true
				changed = true
			}
		}
	}
	var stable []string
	for _, r := range rows {
		if !ignore[r.pid] {
			stable = append(stable, "process "+r.line)
		}
	}
	if !bytes.Contains(files, []byte{0}) {
		return "", errors.New("sleep custody: native lsof NUL field output required")
	}
	pid := 0
	var fd, access, kind, name string
	var parseErr error
	flush := func() {
		if pid != 0 && !ignore[pid] && fd != "" && kind == "REG" {
			if _, err := strconv.Atoi(fd); err == nil && access != "r" && access != "w" && access != "u" {
				parseErr = errors.New("sleep custody: regular descriptor access mode unavailable")
			}
			if access == "w" || access == "u" {
				if name == "" {
					parseErr = errors.New("sleep custody: writable descriptor name unavailable")
				} else {
					b, _ := json.Marshal(struct {
						PID                    int
						FD, Access, Kind, Name string
					}{pid, fd, access, kind, name})
					stable = append(stable, "writer "+string(b))
				}
			}
		}
		fd, access, kind, name = "", "", "", ""
	}
	for _, field := range strings.Split(string(files), "\x00") {
		// lsof adds a newline between process/file sets even in -F0 mode;
		// embedded filename newlines are preserved inside the n field.
		field = strings.TrimPrefix(field, "\n")
		if field == "" {
			continue
		}
		switch field[0] {
		case 'p':
			flush()
			var err error
			pid, err = strconv.Atoi(field[1:])
			if err != nil || pid <= 0 {
				return "", errors.New("sleep custody: invalid lsof process identity")
			}
		case 'f':
			flush()
			fd = field[1:]
		case 'a':
			access = field[1:]
		case 't':
			kind = field[1:]
		case 'n':
			name = field[1:]
		case 'c':
		default:
			return "", errors.New("sleep custody: unexpected lsof evidence field")
		}
	}
	flush()
	if parseErr != nil {
		return "", parseErr
	}

	if len(rows) == 0 || len(stable) == 0 {
		return "", errors.New("sleep custody: empty native inventory")
	}
	sort.Strings(stable)
	encoded, err := json.Marshal(stable)
	if err != nil {
		return "", err
	}
	return digestBytes(encoded), nil
}

// ParseNativeReturn requires actual Sleep followed by full Wake in the
// consumed interval. DarkWake, reachability, a new boot, or a planned event
// without native log evidence cannot qualify a return.
func ParseNativeReturn(log []byte, s CustodyState, actual HostIdentity, now time.Time) (ReturnEvidence, error) {
	d, a := s.Delegation, s.Delegation.Admission
	if actual.Host != a.Host || actual.PlatformUUID != a.PlatformUUID || actual.BootID != a.BootID || actual.ExecutableSHA256 != d.ExecutableSHA256 || s.ConsumedAt.IsZero() || s.Observation == nil {
		return ReturnEvidence{}, errors.New("sleep custody: unchanged native identity and independent observation required")
	}
	pattern := regexp.MustCompile(`(?m)^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} [+-]\d{4})\s+(Sleep|Wake)\s+([^\r\n]+)$`)
	var sleepLine, wakeLine string
	var sleepAt, wakeAt time.Time
	for _, m := range pattern.FindAllSubmatch(log, -1) {
		at, err := time.Parse("2006-01-02 15:04:05 -0700", string(m[1]))
		if err != nil || at.Before(s.ConsumedAt.Truncate(time.Second)) || at.After(now) {
			continue
		}
		kind, text := string(m[2]), string(m[3])
		if kind == "Sleep" && strings.Contains(text, "Entering Sleep state") {
			sleepLine = string(m[0])
			sleepAt = at
		}
		if kind == "Wake" && strings.Contains(text, "Wake from ") && !sleepAt.IsZero() && at.After(sleepAt) && !at.Before(d.Request.WakeAt) {
			wakeLine = string(m[0])
			wakeAt = at
			break
		}
	}
	if sleepLine == "" || wakeLine == "" {
		return ReturnEvidence{}, errors.New("sleep custody: actual Sleep/full-Wake pair unavailable")
	}
	o := s.Observation
	if o.RequestID != d.Request.ID || o.Host != a.Host || o.PlatformUUID != a.PlatformUUID || o.BootID != a.BootID || o.LostAt.Before(s.ConsumedAt) || o.ReturnedAt.Before(wakeAt) || o.ReturnedAt.After(now.Add(5*time.Second)) {
		return ReturnEvidence{}, errors.New("sleep custody: independent observation does not bind native return")
	}
	b, _ := json.Marshal(o)
	e := ReturnEvidence{RequestID: d.Request.ID, Host: a.Host, PlatformUUID: a.PlatformUUID, BootID: a.BootID, SleepRecordID: digestBytes([]byte(sleepLine)), WakeRecordID: "pantheon-sleep-" + d.Request.ID, FullWake: true, ObservedAt: wakeAt, ObserverReceiptDigest: digestBytes(b)}
	e.Digest = digestBytes([]byte(sleepLine + "\n" + wakeLine + "\n" + e.ObserverReceiptDigest))
	return e, nil
}

func NativeReturn(ctx context.Context, s CustodyState) (ReturnEvidence, error) {
	if s.ControllerLeaseDeadline.IsZero() || time.Now().After(s.ControllerLeaseDeadline) {
		return ReturnEvidence{}, errors.New("sleep custody: original proven controller lease elapsed; incident review required, not a continuity claim")
	}
	i, err := NativeIdentity(ctx, s.Delegation.Admission.Host)
	if err != nil {
		return ReturnEvidence{}, err
	}
	inv, err := NativeInventory(ctx)
	if err != nil {
		return ReturnEvidence{}, err
	}
	if inv != s.Delegation.InventorySHA256 {
		return ReturnEvidence{}, errors.New("sleep custody: process/writer continuity changed after return")
	}
	b, err := nativeRead(ctx, "/usr/bin/pmset", "-g", "log")
	if err != nil {
		return ReturnEvidence{}, err
	}
	return ParseNativeReturn(b, s, i, time.Now().UTC())
}

// ObserveNativeReturn is an independent M1-side remote observation. The only
// remote command is a fixed boot-id read through the pre-enrolled SSH alias.
// Strict host checking stays on; authentication errors never count as sleep.
func ObserveNativeReturn(ctx context.Context, authority OnlineAuthority, id string) (CustodyState, error) {
	s, err := authority.DesktopSleep(ctx, CustodyCall{Operation: "status", ID: id, Role: "observer"})
	if err != nil {
		return s, err
	}
	ctx, cancel := context.WithDeadline(ctx, s.Delegation.Admission.NotAfter)
	defer cancel()
	if s.Delegation.Admission.Host != "m5" {
		return s, errors.New("sleep custody: no enrolled observer probe for target")
	}
	probe := func() ([]byte, error) {
		return nativeRead(ctx, "/usr/bin/ssh", "-n", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "ConnectTimeout=2", "sirsi-m5", "/usr/sbin/sysctl -n kern.bootsessionuuid")
	}
	b, err := probe()
	if err != nil || strings.TrimSpace(string(b)) != s.Delegation.Admission.BootID {
		return s, errors.New("sleep custody: independent baseline boot probe failed")
	}
	start := time.Now().UTC()
	e := ObserverEvidence{RequestID: id, Host: s.Delegation.Admission.Host, PlatformUUID: s.Delegation.Admission.PlatformUUID, BootID: s.Delegation.Admission.BootID}
	for time.Now().UTC().Before(s.Delegation.Admission.NotAfter) {
		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-time.After(time.Second):
		}
		current, err := authority.DesktopSleep(ctx, CustodyCall{Operation: "status", ID: id, Role: "observer"})
		if err != nil {
			return s, err
		}
		s = current
		b, err = probe()
		at := time.Now().UTC()
		if err != nil {
			if strings.Contains(err.Error(), "Permission denied") || strings.Contains(err.Error(), "Host key verification failed") {
				return s, err
			}
			if !s.ConsumedAt.IsZero() && e.LostAt.IsZero() {
				e.LostAt = at
			}
			continue
		}
		if strings.TrimSpace(string(b)) != e.BootID {
			return s, errors.New("sleep custody: independent observer detected changed boot")
		}
		if s.ConsumedAt.IsZero() {
			if _, err := authority.DesktopSleep(ctx, CustodyCall{Operation: "ready", ID: id, Role: "observer", QuietDigest: digestBytes([]byte(start.Format(time.RFC3339Nano) + "\n" + e.BootID))}); err != nil {
				return s, err
			}
		}
		if !e.LostAt.IsZero() && !at.Before(s.Delegation.Request.WakeAt) {
			e.ReturnedAt = at
			e.ContinuitySHA256 = digestBytes([]byte(start.Format(time.RFC3339Nano) + "\n" + e.BootID + "\n" + at.Format(time.RFC3339Nano)))
			return authority.DesktopSleep(ctx, CustodyCall{Operation: "observed-return", ID: id, Role: "observer", Observation: &e})
		}
	}
	return s, errors.New("sleep custody: no bounded loss/return observation; no power proof")
}
