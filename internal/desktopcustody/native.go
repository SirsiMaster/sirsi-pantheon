package desktopcustody

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DarwinPower uses only Apple's fixed system executable and typed argument
// vectors. It grants no authority, elevates no privileges and changes no power
// settings. The controller must supply verified admission and final custody.
type DarwinPower struct {
	mu        sync.Mutex
	run       func(context.Context, ...string) ([]byte, error)
	zone      func() (*time.Location, error)
	now       func() time.Time
	wake      *WakeReceipt
	scheduled bool
}

func NewDarwinPower() (*DarwinPower, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("sleep custody: native power is available only on Darwin")
	}
	return &DarwinPower{now: time.Now, run: func(ctx context.Context, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		// No shell, caller executable, sudo, command fallback or environment
		// selector is accepted. OS privilege denial remains a denial.
		return exec.CommandContext(ctx, "/usr/bin/pmset", args...).CombinedOutput()
	}, zone: func() (*time.Location, error) {
		data, err := os.ReadFile("/etc/localtime")
		if err != nil {
			return nil, err
		}
		return time.LoadLocationFromTZData("system", data)
	}}, nil
}

func (p *DarwinPower) stamp(at time.Time) (string, string, error) {
	if p == nil || p.run == nil || p.zone == nil || at.IsZero() {
		return "", "", errors.New("sleep custody: native power adapter is unavailable")
	}
	zone, err := p.zone()
	if err != nil {
		return "", "", fmt.Errorf("sleep custody: system timezone unavailable: %w", err)
	}
	local := at.In(zone)
	// The native scheduler has no offset operand. Refuse repeated local
	// clock times rather than silently schedule the other DST occurrence.
	start, end := local.ZoneBounds()
	for _, boundary := range []time.Time{start, end} {
		if boundary.IsZero() {
			continue
		}
		_, before := boundary.Add(-time.Nanosecond).In(zone).Zone()
		_, after := boundary.In(zone).Zone()
		delta := time.Duration(after-before) * time.Second
		if delta == 0 {
			continue
		}
		for _, shift := range []time.Duration{delta, -delta} {
			if local.Format("2006-01-02 15:04:05") == at.Add(shift).In(zone).Format("2006-01-02 15:04:05") {
				return "", "", errors.New("sleep custody: native schedule timestamp is ambiguous across a timezone transition")
			}
		}
	}
	return local.Format("01/02/06 15:04:05"), local.Format("01/02/2006 15:04:05"), nil
}

func (p *DarwinPower) ScheduleWake(ctx context.Context, a SleepAdmission, r SleepRequest) (WakeReceipt, error) {
	if p == nil {
		return WakeReceipt{}, errors.New("sleep custody: native power adapter is unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scheduled {
		return WakeReceipt{}, errors.New("sleep custody: native wake mutation cannot be retried")
	}
	if r.ID != a.RequestID || r.Host != a.Host || a.Operation != SleepOperation || !identifierRE.MatchString(r.ID) || !r.WakeAt.Before(a.NotAfter) || r.WakeAt.Nanosecond() != 0 {
		return WakeReceipt{}, errors.New("sleep custody: native wake request binding is invalid")
	}
	date, _, err := p.stamp(r.WakeAt)
	if err != nil {
		return WakeReceipt{}, err
	}
	tag := "pantheon-sleep-" + r.ID
	p.scheduled = true
	if _, err := p.run(ctx, "schedule", "wake", date, tag); err != nil {
		return WakeReceipt{}, fmt.Errorf("%w: %v", ErrWakeScheduleUnknown, err)
	}
	w := WakeReceipt{ID: tag, Host: r.Host, WakeAt: r.WakeAt, OwnerTag: tag}
	line, err := p.readOwned(ctx, r, w)
	if err != nil {
		return WakeReceipt{}, fmt.Errorf("%w: %v", ErrWakeScheduleUnknown, err)
	}
	w.Readback = line
	p.wake = &w
	return w, nil
}

func (p *DarwinPower) readOwned(ctx context.Context, r SleepRequest, w WakeReceipt) (string, error) {
	_, stamp, err := p.stamp(r.WakeAt)
	if err != nil {
		return "", err
	}
	if w.Host != r.Host || !w.WakeAt.Equal(r.WakeAt) || w.OwnerTag != "pantheon-sleep-"+r.ID || w.ID != w.OwnerTag {
		return "", errors.New("sleep custody: owned wake receipt mismatch")
	}
	data, err := p.run(ctx, "-g", "sched")
	if err != nil {
		return "", fmt.Errorf("sleep custody: native wake readback failed: %w", err)
	}
	want := "wake at " + stamp + " by '" + w.OwnerTag + "'"
	pattern := regexp.MustCompile(`^\[[0-9]+\]\s+` + regexp.QuoteMeta(want) + `$`)
	var found string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "by '"+w.OwnerTag+"'") {
			if found != "" || !pattern.MatchString(strings.TrimSpace(line)) {
				return "", errors.New("sleep custody: duplicate or substituted owned wake event")
			}
			found = strings.TrimSpace(line)
		}
	}
	if found == "" {
		return "", os.ErrNotExist
	}
	return found, nil
}

func (p *DarwinPower) ReadWake(ctx context.Context, a SleepAdmission, r SleepRequest, w WakeReceipt) error {
	if a.RequestID != r.ID || a.Host != r.Host || a.Operation != SleepOperation {
		return errors.New("sleep custody: native readback admission mismatch")
	}
	_, err := p.readOwned(ctx, r, w)
	return err
}

func (p *DarwinPower) Sleep(ctx context.Context) error {
	if p == nil {
		return errors.New("sleep custody: native power adapter is unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.wake == nil {
		return errors.New("sleep custody: native sleep requires a read-back owned wake")
	}
	if p.now == nil || !p.wake.WakeAt.After(p.now()) || ctx.Err() != nil {
		return errors.New("sleep custody: native wake expired or action canceled")
	}
	// Consume this adapter instance before invocation. A timeout or native
	// return cannot enable an automatic retry.
	p.wake = nil
	_, err := p.run(ctx, "sleepnow")
	return err
}

func (p *DarwinPower) CancelWake(ctx context.Context, a SleepAdmission, r SleepRequest, w WakeReceipt) error {
	if a.RequestID != r.ID || a.Host != r.Host || a.Operation != SleepOperation {
		return errors.New("sleep custody: native cleanup admission mismatch")
	}
	if _, err := p.readOwned(ctx, r, w); errors.Is(err, os.ErrNotExist) {
		return nil // An already-fired event needs no removal.
	} else if err != nil {
		return err
	}
	date, _, err := p.stamp(r.WakeAt)
	if err != nil {
		return err
	}
	if _, err := p.run(ctx, "schedule", "cancel", "wake", date, w.OwnerTag); err != nil {
		return fmt.Errorf("sleep custody: exact owned wake cleanup failed: %w", err)
	}
	if _, err := p.readOwned(ctx, r, w); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return errors.New("sleep custody: owned wake remains after cleanup")
		}
		return fmt.Errorf("sleep custody: owned wake cleanup not confirmed: %w", err)
	}
	return nil
}

func digestBytes(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func EvidenceDigest(data []byte) string { return digestBytes(data) }
