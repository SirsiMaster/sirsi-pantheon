package desktopcustody

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNativePowerUsesOnlyExactOwnedWakeAndSingleInvocation(t *testing.T) {
	f, _ := newFixture(t)
	var calls [][]string
	present := false
	p := &DarwinPower{zone: func() (*time.Location, error) { return time.UTC, nil }}
	p.run = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "schedule":
			present = args[1] != "cancel"
		case "-g":
			if present {
				return []byte("[0] wake at 10/15/2026 17:23:00 by 'pantheon-sleep-m5-sleep-1'\n"), nil
			}
		}
		return nil, nil
	}
	w, err := p.ScheduleWake(context.Background(), f.a, f.req)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.ReadWake(context.Background(), f.a, f.req, w); err != nil {
		t.Fatal(err)
	}
	if err := p.Sleep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Sleep(context.Background()); err == nil {
		t.Fatal("native sleep invoked twice")
	}
	if err := p.CancelWake(context.Background(), f.a, f.req, w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls[0], []string{"schedule", "wake", "10/15/26 17:23:00", "pantheon-sleep-m5-sleep-1"}) {
		t.Fatal(calls[0])
	}
	if !reflect.DeepEqual(calls[len(calls)-2], []string{"schedule", "cancel", "wake", "10/15/26 17:23:00", "pantheon-sleep-m5-sleep-1"}) {
		t.Fatal(calls)
	}
}

func TestLostWakeMutationReceiptIsUnknownAndNeverRetried(t *testing.T) {
	f, s := newFixture(t)
	f.p.scheduleErr = ErrWakeScheduleUnknown
	if err := s.Dispatch(context.Background(), f.req); !errors.Is(err, ErrSleepUnknown) {
		t.Fatal(err)
	}
	events := phase(t, f, SleepUnknown)
	if !events[0].WakeAt.Equal(f.req.WakeAt) {
		t.Fatal("lost planned wake timestamp")
	}
	_, _, sleeps, cancels := f.p.counts()
	if sleeps != 0 || cancels != 0 {
		t.Fatal("unknown wake mutation triggered another action")
	}
	if err := s.Dispatch(context.Background(), f.req); err == nil {
		t.Fatal("unknown transaction retried")
	}
}

func TestNativeReadbackRejectsSubstitutionAndDuplicate(t *testing.T) {
	f, _ := newFixture(t)
	for _, text := range []string{
		"[0] wake at 10/15/2026 17:24:00 by 'pantheon-sleep-m5-sleep-1'",
		"[0] poweron at 10/15/2026 17:23:00 by 'pantheon-sleep-m5-sleep-1'",
		"[0] wake at 10/15/2026 17:23:00 by 'someone-else'",
		"[0] wake at 10/15/2026 17:23:00 by 'pantheon-sleep-m5-sleep-1'\n[1] wake at 10/15/2026 17:23:00 by 'pantheon-sleep-m5-sleep-1'",
	} {
		p := &DarwinPower{zone: func() (*time.Location, error) { return time.UTC, nil }, run: func(context.Context, ...string) ([]byte, error) { return []byte(text), nil }}
		w := WakeReceipt{ID: "pantheon-sleep-" + f.req.ID, OwnerTag: "pantheon-sleep-" + f.req.ID, Host: f.req.Host, WakeAt: f.req.WakeAt}
		if _, err := p.readOwned(context.Background(), f.req, w); err == nil {
			t.Fatal(text)
		}
	}
}

func TestNativeScheduleRefusesAmbiguousLocalTime(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	p := &DarwinPower{zone: func() (*time.Location, error) { return zone, nil }, run: func(context.Context, ...string) ([]byte, error) {
		t.Fatal("unexpected native command")
		return nil, nil
	}}
	for _, hour := range []int{5, 6} {
		if _, _, err := p.stamp(time.Date(2026, 11, 1, hour, 30, 0, 0, time.UTC)); err == nil {
			t.Fatal("ambiguous local timestamp accepted")
		}
	}
}
