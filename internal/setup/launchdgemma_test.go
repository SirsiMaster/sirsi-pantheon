package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func gemmaLifecycleFixture() (gemmaWorkerOperations, GemmaWorkerPlan, *gemmaWorkerDebt, *[]string) {
	now := time.Now()
	p := GemmaWorkerPlan{Instance: ServiceInstance{Label: GemmaNativeWorkerLabel, Target: "gui/501/" + GemmaNativeWorkerLabel, Plist: "/owned/worker.plist", PlistSHA256: strings.Repeat("a", 64), Program: "/owned/sirsi", ProgramSHA256: strings.Repeat("b", 64), PID: 70069, Started: "old"}, Thread: "worker", Machine: "native", KernelStart: "1.000001", CDHash: strings.Repeat("c", 40), ReplacementCDHash: strings.Repeat("c", 40), Parent: 1}
	p.Digest = gemmaPlanDigest(p)
	debt := &gemmaWorkerDebt{}
	events := &[]string{}
	started := false
	d := gemmaWorkerOperations{now: func() time.Time { return now }, load: func() (gemmaWorkerDebt, error) {
		if debt.Phase == "" {
			return *debt, ErrGemmaNoDebt
		}
		return *debt, nil
	}, save: func(s gemmaWorkerDebt) error { *debt = s; *events = append(*events, "save:"+s.Phase); return nil }, inspect: func(context.Context, string) (GemmaWorkerPlan, error) {
		q := p
		if started {
			q.Instance.PID++
			q.KernelStart = "2.000001"
			q.Digest = gemmaPlanDigest(q)
		}
		return q, nil
	}, barrier: func(_ context.Context, r routerstore.LaneClaimBarrierRequest) (routerstore.LaneClaimBarrierResult, error) {
		*events = append(*events, r.Action)
		return routerstore.LaneClaimBarrierResult{Paused: r.Action != "release"}, nil
	}, ack: func(string) (GemmaWorkerPauseAck, error) {
		return GemmaWorkerPauseAck{Plan: p, At: now.Add(time.Second)}, nil
	}, joined: func(GemmaWorkerPlan) error { *events = append(*events, "joined"); return nil }, start: func(context.Context) error { *events = append(*events, "start"); started = true; return nil }, wait: func(context.Context) error { return context.DeadlineExceeded }}
	return d, p, debt, events
}
func TestGemmaLifecycleJoinedOrder(t *testing.T) {
	d, p, s, ev := gemmaLifecycleFixture()
	if e := applyGemmaWorker(context.Background(), d, p, "ssa"); e != nil {
		t.Fatal(e)
	}
	if s.Phase != "done" {
		t.Fatal(s.Phase)
	}
	order := strings.Join(*ev, ",")
	if !strings.Contains(order, "joined,save:release-intent,release,save:start-intent,start") {
		t.Fatal(order)
	}
}
func TestGemmaLifecycleRefusals(t *testing.T) {
	for _, which := range []string{"foreign-label", "foreign-thread", "reused-pid", "active-item", "active-task", "active-child", "join-failed", "substitution", "auth-outage", "stale-ack"} {
		t.Run(which, func(t *testing.T) {
			d, p, s, ev := gemmaLifecycleFixture()
			switch which {
			case "foreign-label":
				p.Instance.Label = "ai.sirsi.gemma-broker"
				p.Digest = gemmaPlanDigest(p)
			case "foreign-thread":
				d.ack = func(string) (GemmaWorkerPauseAck, error) {
					q := p
					q.Thread = "foreign"
					return GemmaWorkerPauseAck{Plan: q, At: time.Now().Add(time.Second)}, nil
				}
			case "reused-pid", "substitution":
				d.inspect = func(context.Context, string) (GemmaWorkerPlan, error) {
					q := p
					q.KernelStart = "reused"
					return q, nil
				}
			case "active-item", "active-task":
				d.barrier = func(context.Context, routerstore.LaneClaimBarrierRequest) (routerstore.LaneClaimBarrierResult, error) {
					return routerstore.LaneClaimBarrierResult{Paused: true, ActiveItems: 1, ActiveTasks: 1}, nil
				}
			case "active-child", "join-failed":
				d.joined = func(GemmaWorkerPlan) error { return errors.New("not joined") }
			case "auth-outage":
				d.barrier = func(context.Context, routerstore.LaneClaimBarrierRequest) (routerstore.LaneClaimBarrierResult, error) {
					return routerstore.LaneClaimBarrierResult{}, errors.New("auth")
				}
			case "stale-ack":
				d.ack = func(string) (GemmaWorkerPauseAck, error) {
					return GemmaWorkerPauseAck{Plan: p, At: time.Unix(1, 0)}, nil
				}
			}
			if e := applyGemmaWorker(context.Background(), d, p, "ssa"); e == nil {
				t.Fatal("unsafe apply accepted")
			}
			for _, x := range *ev {
				if x == "start" || x == "release" {
					t.Fatalf("unsafe mutation %s debt=%s", x, s.Phase)
				}
			}
		})
	}
}
func TestGemmaLifecycleUnknownStartNeverReplayed(t *testing.T) {
	d, p, s, ev := gemmaLifecycleFixture()
	d.start = func(context.Context) error { *ev = append(*ev, "start"); return errors.New("lost reply") }
	if applyGemmaWorker(context.Background(), d, p, "ssa") == nil {
		t.Fatal("missing failure")
	}
	if s.Phase != "start-intent" {
		t.Fatal(s.Phase)
	}
	_ = recoverGemmaWorker(context.Background(), d, "ssa")
	n := 0
	for _, x := range *ev {
		if x == "start" {
			n++
		}
	}
	if n != 1 {
		t.Fatal("replayed", n)
	}
}
func TestGemmaLifecycleSaveFailureBeforeRelease(t *testing.T) {
	d, p, _, ev := gemmaLifecycleFixture()
	save := d.save
	d.save = func(s gemmaWorkerDebt) error {
		if s.Phase == "release-intent" {
			return errors.New("disk")
		}
		return save(s)
	}
	if applyGemmaWorker(context.Background(), d, p, "ssa") == nil {
		t.Fatal("missing refusal")
	}
	for _, x := range *ev {
		if x == "release" || x == "start" {
			t.Fatal(x)
		}
	}
}

func TestGemmaLifecycleLegacy140IsRetirementNotPauseAck(t *testing.T) {
	d, p, s, ev := gemmaLifecycleFixture()
	p.Legacy140 = true
	p.CDHash = "c9fa4a594cab29a4a95c2180c0ca64e5dcd0de59"
	p.Digest = gemmaPlanDigest(p)
	inspect := d.inspect
	d.inspect = func(c context.Context, t string) (GemmaWorkerPlan, error) {
		q, e := inspect(c, t)
		if q.KernelStart == p.KernelStart {
			q = p
			q.Legacy140 = false
			q.Digest = gemmaPlanDigest(q)
			return q, e
		}
		return q, e
	}
	d.ack = func(string) (GemmaWorkerPauseAck, error) {
		return GemmaWorkerPauseAck{}, errors.New("old140 cannot ack")
	}
	d.legacyRetire = func(context.Context, GemmaWorkerPlan) error { *ev = append(*ev, "legacy-retire"); return nil }
	d.startLegacy = d.start
	if e := applyGemmaWorker(context.Background(), d, p, "ssa"); e != nil {
		t.Fatal(e)
	}
	if s.Phase != "done" || !strings.Contains(s.OldOutcome, "no cooperative pause acknowledgment") {
		t.Fatal(s.Phase, s.OldOutcome)
	}
	order := strings.Join(*ev, ",")
	if !strings.Contains(order, "save:legacy-retire-intent,legacy-retire,joined") {
		t.Fatal(order)
	}
}
func TestGemmaLifecycleLegacy140UnknownRetirementNeverReplayed(t *testing.T) {
	d, p, s, ev := gemmaLifecycleFixture()
	p.Legacy140 = true
	p.CDHash = "c9fa4a594cab29a4a95c2180c0ca64e5dcd0de59"
	p.Digest = gemmaPlanDigest(p)
	d.inspect = func(context.Context, string) (GemmaWorkerPlan, error) { return p, nil }
	d.legacyRetire = func(context.Context, GemmaWorkerPlan) error {
		*ev = append(*ev, "legacy-retire")
		return errors.New("lost reply")
	}
	d.startLegacy = d.start
	d.joined = func(GemmaWorkerPlan) error { return errors.New("old instance remains or absence unknown") }
	if applyGemmaWorker(context.Background(), d, p, "ssa") == nil {
		t.Fatal("unknown retirement accepted")
	}
	_ = recoverGemmaWorker(context.Background(), d, "ssa")
	n := 0
	for _, x := range *ev {
		if x == "legacy-retire" {
			n++
		}
		if x == "release" || x == "start" {
			t.Fatal("unknown old outcome released barrier", x)
		}
	}
	if n != 1 || !s.LegacyRetirementIssued {
		t.Fatal(n, s.Phase)
	}
}
func TestGemmaLifecycleReplacementDiskDiffersFromOldLoaded(t *testing.T) {
	d, p, _, _ := gemmaLifecycleFixture()
	p.CDHash = strings.Repeat("d", 40)
	p.Digest = gemmaPlanDigest(p)
	inspected := false
	d.inspect = func(context.Context, string) (GemmaWorkerPlan, error) {
		if !inspected {
			inspected = true
			return p, nil
		}
		q := p
		q.CDHash = p.ReplacementCDHash
		q.KernelStart = "replacement"
		q.Digest = gemmaPlanDigest(q)
		return q, nil
	}
	d.ack = func(string) (GemmaWorkerPauseAck, error) {
		return GemmaWorkerPauseAck{Plan: p, At: time.Now().Add(time.Second)}, nil
	}
	if e := applyGemmaWorker(context.Background(), d, p, "ssa"); e != nil {
		t.Fatal(e)
	}
}

func TestGemmaLifecycleLegacy140WrongLoadedHashRefused(t *testing.T) {
	d, p, _, ev := gemmaLifecycleFixture()
	p.Legacy140 = true
	p.CDHash = strings.Repeat("d", 40)
	p.Digest = gemmaPlanDigest(p)
	if applyGemmaWorker(context.Background(), d, p, "ssa") == nil {
		t.Fatal("unrecognized old image accepted")
	}
	if len(*ev) != 0 {
		t.Fatal("refusal touched controller", *ev)
	}
}
