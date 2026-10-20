package desktopcustody

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func nativeReturnFixture() (CustodyState, HostIdentity, []byte, time.Time) {
	start := time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC)
	i := HostIdentity{Host: "m5", PlatformUUID: "platform", BootID: "boot", ExecutableSHA256: strings.Repeat("a", 64)}
	s := CustodyState{Delegation: Delegation{Request: SleepRequest{ID: "sleep-one", Host: "m5", WakeAt: start.Add(3 * time.Minute)}, Admission: SleepAdmission{Host: i.Host, PlatformUUID: i.PlatformUUID, BootID: i.BootID}, ExecutableSHA256: i.ExecutableSHA256}, ConsumedAt: start, NativeAuthorizedAt: start, Observation: &ObserverEvidence{RequestID: "sleep-one", Host: i.Host, PlatformUUID: i.PlatformUUID, BootID: i.BootID, LostAt: start.Add(2 * time.Second), ReturnedAt: start.Add(181 * time.Second), ContinuitySHA256: strings.Repeat("c", 64)}}
	log := []byte("2026-10-20 06:00:01 +0000 Sleep Entering Sleep state due to 'Software Sleep pid=1'\n2026-10-20 06:03:00 +0000 Wake Wake from Normal Sleep due to RTC\n")
	return s, i, log, start.Add(182 * time.Second)
}

func TestNativeReturnRequiresRealPairAndIndependentUnchangedBoot(t *testing.T) {
	s, i, log, now := nativeReturnFixture()
	if e, err := ParseNativeReturn(log, s, i, now); err != nil || !e.FullWake || e.SleepRecordID == "" {
		t.Fatalf("evidence=%+v err=%v", e, err)
	}
	for _, mode := range []string{"darkwake", "old-boot", "old-native", "no-observer", "wrong-observer-boot", "observer-before-wake", "no-sleep", "no-consume", "no-native-authorization", "native-before-consume", "aborted", "native-after-sleep"} {
		t.Run(mode, func(t *testing.T) {
			s, i, log, now := nativeReturnFixture()
			switch mode {
			case "darkwake":
				log = []byte(strings.ReplaceAll(string(log), " Wake ", " DarkWake "))
			case "old-boot":
				i.BootID = "other"
			case "old-native":
				log = []byte(strings.ReplaceAll(string(log), "2026-10-20", "2026-10-19"))
			case "no-observer":
				s.Observation = nil
			case "wrong-observer-boot":
				s.Observation.BootID = "other"
			case "observer-before-wake":
				s.Observation.ReturnedAt = s.Delegation.Request.WakeAt.Add(-time.Second)
			case "no-sleep":
				log = []byte(strings.Split(string(log), "\n")[1])
			case "no-consume":
				s.ConsumedAt = time.Time{}
			case "no-native-authorization":
				s.NativeAuthorizedAt = time.Time{}
			case "native-before-consume":
				s.NativeAuthorizedAt = s.ConsumedAt.Add(-time.Second)
			case "aborted":
				s.AbortedAt = now
			case "native-after-sleep":
				s.NativeAuthorizedAt = s.ConsumedAt.Add(2 * time.Second)
			}
			if _, err := ParseNativeReturn(log, s, i, now); err == nil {
				t.Fatal("unproved native return accepted")
			}
		})
	}
}

func TestNativeInventoryBindsProcessStartWritersAndAssertions(t *testing.T) {
	ps := []byte("1 0 Tue Oct 20 06:00:00 2026 /sbin/launchd\n10 1 Tue Oct 20 06:00:00 2026 /Applications/owner\n90 1 Tue Oct 20 06:00:00 2026 /usr/local/bin/controller\n91 90 Tue Oct 20 06:00:00 2026 /bin/ps\n")
	files := []byte("p10\x00cowner\x00\nf3\x00aw\x00tREG\x00n/Users/owner/work\x00\np90\x00ccontroller\x00\nf4\x00aw\x00tREG\x00n/private/journal\x00\n")
	assertions := []byte(" PreventSystemSleep 0\n PreventUserIdleSystemSleep 0\n ExternalMedia 0\n UserIsActive 0\n")
	baseline, err := inventoryDigest(ps, files, assertions, 90)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ ps, files, assertions []byte }{{[]byte(strings.Replace(string(ps), "10 1 Tue Oct 20 06:00:00", "10 1 Tue Oct 20 06:00:01", 1)), files, assertions}, {ps, []byte(strings.ReplaceAll(string(files), "/Users/owner/work", "/Users/owner/different")), assertions}, {ps, files, []byte(strings.ReplaceAll(string(assertions), "ExternalMedia 0", "ExternalMedia 1"))}} {
		digest, err := inventoryDigest(change.ps, change.files, change.assertions, 90)
		if err == nil && digest == baseline {
			t.Fatal("changed native custody unchanged")
		}
	}
	if _, err := inventoryDigest(nil, nil, assertions, 90); err == nil {
		t.Fatal("empty inventory accepted")
	}
}

func TestExplicitReconcileUnknownRequiresPriorInvocationAndDoesNotSleepAgain(t *testing.T) {
	f, s := newFixture(t)
	f.p.sleepErr = context.DeadlineExceeded
	if err := s.Dispatch(context.Background(), f.req); !errors.Is(err, ErrSleepUnknown) {
		t.Fatal(err)
	}
	f.now = f.req.WakeAt.Add(time.Second)
	if err := s.VerifyReturn(context.Background(), f.req); err == nil {
		t.Fatal("ordinary return silently reconciled unknown")
	}
	if err := s.ReconcileReturn(context.Background(), f.req); err != nil {
		t.Fatal(err)
	}
	phase(t, f, SleepVerified)
	_, _, sleeps, _ := f.p.counts()
	if sleeps != 1 {
		t.Fatal("recovery redispatched")
	}
	f2, s2 := newFixture(t)
	f2.p.scheduleErr = ErrWakeScheduleUnknown
	if err := s2.Dispatch(context.Background(), f2.req); !errors.Is(err, ErrSleepUnknown) {
		t.Fatal(err)
	}
	f2.now = f2.req.WakeAt.Add(time.Second)
	if err := s2.ReconcileReturn(context.Background(), f2.req); err == nil {
		t.Fatal("recovered a sleep that was never invoked")
	}
}

func TestNativePowerExpiredWakeDeniesBeforeCommand(t *testing.T) {
	called := false
	at := time.Now()
	p := &DarwinPower{wake: &WakeReceipt{WakeAt: at}, now: func() time.Time { return at }, run: func(context.Context, ...string) ([]byte, error) { called = true; return nil, nil }}
	if err := p.Sleep(context.Background()); err == nil || called {
		t.Fatal("expired owned wake actuated")
	}
}

func TestNativeInventoryReadsLsofAccessModeAndNulPaths(t *testing.T) {
	// Bundled lsof(8), OUTPUT FOR OTHER PROGRAMS: f is the descriptor,
	// a is access mode, and -F0 uses NUL fields plus newline set separators.
	ps := []byte("1 0 Tue Oct 20 06:00:00 2026 /sbin/launchd\n10 1 Tue Oct 20 06:00:00 2026 /Applications/owner\n90 1 Tue Oct 20 06:00:00 2026 /usr/local/bin/controller\n")
	assertions := []byte("PreventSystemSleep 0\nPreventUserIdleSystemSleep 0\nExternalMedia 0\nUserIsActive 0\n")
	files := []byte("p10\x00cowner\x00\nf3\x00au\x00tREG\x00n/Users/owner/file\nwith-newline\x00\n")
	one, err := inventoryDigest(ps, files, assertions, 90)
	if err != nil {
		t.Fatal(err)
	}
	files = []byte(strings.ReplaceAll(string(files), "file\nwith-newline", "different\nfile"))
	two, err := inventoryDigest(ps, files, assertions, 90)
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatal("native lsof writable-file identity was omitted")
	}
}
