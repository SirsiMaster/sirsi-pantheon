package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	name string
	tier Tier
	caps Caps
	up   bool
	resp Response
	err  error
}

func (f *fakeProvider) Name() string                   { return f.name }
func (f *fakeProvider) Tier() Tier                     { return f.tier }
func (f *fakeProvider) Caps() Caps                     { return f.caps }
func (f *fakeProvider) Available(context.Context) bool { return f.up }
func (f *fakeProvider) Complete(context.Context, Request) (Response, error) {
	return f.resp, f.err
}

type memoryDecisionLog struct{ records []DecisionRecord }

func (m *memoryDecisionLog) Append(r DecisionRecord) error {
	m.records = append(m.records, r)
	return nil
}

func TestModelRouterJudgmentFallsDownToLocal(t *testing.T) {
	caps := Caps{Streaming: true}
	local := &fakeProvider{name: "sne", tier: TierLocal, caps: caps, up: true,
		resp: Response{Text: "local answer", Tier: TierLocal, Provider: "sne", Model: "gemma"}}
	remote := &fakeProvider{name: "remote", tier: TierRemote, caps: caps, up: true,
		err: errors.New("rate limit exhausted")}
	log := &memoryDecisionLog{}
	r := &ModelRouter{Local: local, Remote: remote, Log: log, Now: func() time.Time { return time.Unix(1, 0) }}

	resp, decision, err := r.Run(context.Background(), PolicyRequest{
		Task: TaskJudgment, Privacy: PrivacyShareable, Latency: LatencyInteractive,
	}, Request{Prompt: "judge"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "local answer" || decision.Lane != LaneLocal {
		t.Fatalf("resp=%+v decision=%+v", resp, decision)
	}
	if len(log.records) != 1 || log.records[0].Outcome != "fallback-completed" {
		t.Fatalf("records=%+v", log.records)
	}
}

func TestModelRouterLocalOnlyNeverUsesRemote(t *testing.T) {
	remote := &fakeProvider{name: "remote", tier: TierRemote, caps: Caps{Streaming: true}, up: true}
	log := &memoryDecisionLog{}
	r := &ModelRouter{Remote: remote, Log: log}
	_, _, err := r.Run(context.Background(), PolicyRequest{
		Task: TaskGeneration, Privacy: PrivacyLocalOnly,
	}, Request{Prompt: "private"})
	if !errors.Is(err, ErrNoQualifiedLane) {
		t.Fatalf("err=%v, want ErrNoQualifiedLane", err)
	}
	if len(log.records) != 1 || log.records[0].Outcome != "rejected" {
		t.Fatalf("records=%+v", log.records)
	}
}

func TestJSONLDecisionLogger(t *testing.T) {
	path := t.TempDir() + "/router/decisions.jsonl"
	log := JSONLDecisionLogger{Path: path}
	if err := log.Append(DecisionRecord{At: time.Unix(1, 0), Outcome: "completed"}); err != nil {
		t.Fatal(err)
	}
}
