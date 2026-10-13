package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type sharedFake struct {
	items map[string]routerstore.Item
	reads []string
	sends int
}

func (s *sharedFake) GetContext(ctx context.Context, id string) (routerstore.Item, error) {
	if e := ctx.Err(); e != nil {
		return routerstore.Item{}, e
	}
	s.reads = append(s.reads, id)
	r, ok := s.items[id]
	if !ok {
		return r, errors.New("not found")
	}
	return r, nil
}
func sharedFixture() (*sharedFake, sharedGemmaProducer) {
	s := &sharedFake{items: map[string]routerstore.Item{"original": {ID: "original", From: "builder", To: gemmaWorkerAgent, Status: "open"}, "unrelated": {ID: "unrelated", From: "someone", To: "builder", Status: "open"}}}
	p := sharedGemmaProducer{store: s, actor: "builder", validate: func(context.Context) error { return nil }, poll: time.Millisecond, send: func(context.Context, string) (string, error) { s.sends++; return "original", nil }}
	return s, p
}
func sharedComplete(s *sharedFake) {
	reply := routerstore.Item{ID: "reply", From: gemmaWorkerAgent, To: "builder", Title: "NONBINDING Gemma draft for original: draft", Status: "closed", Instructions: gemmaWorkerReplyEnvelope("original", "fixture-lease", "draft")}
	s.items["reply"] = reply
	o := s.items["original"]
	o.Status = "completed"
	ref, _ := json.Marshal(gemmaSharedCompletionRef{Kind: "nonbinding_draft", OriginalItemID: "original", ReplyItemID: "reply", LeaseCorrelation: sharedGemmaReplyFingerprint(reply, "builder", "original")})
	o.Result = string(ref)
	s.items["original"] = o
}
func TestGemmaSharedPendingCancelResumeNoDuplicate(t *testing.T) {
	s, p := sharedFixture()
	out, e := p.run(context.Background(), "full identity", "", 0)
	if e != nil || out.State != "pending" || out.OriginalID != "original" || s.sends != 1 {
		t.Fatal("submit contract failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, e = p.run(ctx, "", "original", time.Minute)
	if e == nil && out.OriginalID != "original" {
		t.Fatal("cancel lost resume ID")
	}
	if s.sends != 1 {
		t.Fatal("resume submitted duplicate")
	}
	out, e = p.run(context.Background(), "", "original", 5*time.Millisecond)
	if e != nil || out.State != "pending" || s.items["original"].Status != "open" {
		t.Fatal("timeout altered original")
	}
	sharedComplete(s)
	out, e = p.run(context.Background(), "", "original", 0)
	if e != nil || out.State != "completed" || out.ReplyID != "reply" || s.sends != 1 {
		t.Fatalf("closed exact reply/resume failed: %+v %v", out, e)
	}
	for _, id := range s.reads {
		if id != "original" && id != "reply" {
			t.Fatal("unrelated item read")
		}
	}
	if s.items["unrelated"].Status != "open" {
		t.Fatal("unrelated inbox consumed")
	}
}
func TestGemmaSharedWrongOwnerRecipientReplyAndPartialPublication(t *testing.T) {
	for _, kind := range []string{"owner", "recipient", "replysender", "replyrecipient", "replyoriginal", "partial", "legacy"} {
		t.Run(kind, func(t *testing.T) {
			s, p := sharedFixture()
			sharedComplete(s)
			o := s.items["original"]
			r := s.items["reply"]
			switch kind {
			case "owner":
				o.From = "foreign"
			case "recipient":
				o.To = "another"
			case "replysender":
				r.From = "foreign"
			case "replyrecipient":
				r.To = "foreign"
			case "replyoriginal":
				r.Instructions = gemmaWorkerReplyEnvelope("other", "fixture", "draft")
			case "partial":
				o.Status = "open"
			case "legacy":
				o.Result = "Gemma draft delivered to builder; response is authoritative."
			}
			s.items["original"] = o
			s.items["reply"] = r
			out, e := p.run(context.Background(), "", "original", 0)
			if kind == "owner" || kind == "recipient" {
				if e == nil {
					t.Fatal("foreign original admitted")
				}
			} else if e != nil || out.State == "completed" {
				t.Fatal("invalid/unlinked/partial result falsely completed")
			}
			if kind == "partial" || kind == "legacy" {
				for _, id := range s.reads {
					if id != "original" {
						t.Fatal("partial/legacy swept reply")
					}
				}
			}
		})
	}
}
func TestGemmaSharedPromptActorWaitAndNoFallback(t *testing.T) {
	s, p := sharedFixture()
	for _, n := range []int{8193} {
		if _, e := p.run(context.Background(), strings.Repeat("x", n), "", 0); e == nil {
			t.Fatal("oversize prompt trimmed/admitted")
		}
	}
	if _, e := p.run(context.Background(), "x", "", 901*time.Second); e == nil {
		t.Fatal("unbounded wait admitted")
	}
	p.validate = func(context.Context) error { return errors.New("foreign actor") }
	if _, e := p.run(context.Background(), "x", "", 0); e == nil || s.sends != 0 {
		t.Fatal("foreign actor submitted")
	}
	cmd := newGemmaSharedCmd()
	if cmd.Flags().Lookup("from") != nil {
		t.Fatal("impersonation flag exists")
	}
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_DB", "forbidden")
	if sharedGemmaRemoteConfigured() == nil {
		t.Fatal("local fallback allowed")
	}
}
func TestGemmaSharedPermanentAndRetryableStatesPreserveOriginal(t *testing.T) {
	for _, status := range []string{"blocked", "open", "dead_letter"} {
		s, p := sharedFixture()
		o := s.items["original"]
		o.Status = status
		o.Result = "retained failure"
		s.items["original"] = o
		out, e := p.run(context.Background(), "", "original", 0)
		if e != nil || s.items["original"].Status != status || out.State == "completed" {
			t.Fatal("failure state consumed/closed")
		}
	}
}
func TestGemmaSharedCurrentActorThreadCannotBorrowForeignRuntime(t *testing.T) {
	th := router.Thread{ThreadID: "thread", AgentID: "builder", Status: router.ThreadStatusActive, MachineID: "machine", PID: 42}
	b, _ := json.Marshal(th)
	records := []routerstore.ThreadRecord{{ThreadID: "thread", Agent: "builder", Status: "active", Payload: b}}
	owns := func(t router.Thread) bool { return t.PID == 42 }
	if e := sharedGemmaActorThread("builder", "thread", "machine", records, owns); e != nil {
		t.Fatal(e)
	}
	if sharedGemmaActorThread("foreign", "thread", "machine", records, owns) == nil || sharedGemmaActorThread("builder", "thread", "foreign-host", records, owns) == nil || sharedGemmaActorThread("builder", "thread", "machine", records, func(router.Thread) bool { return false }) == nil {
		t.Fatal("foreign actor/host/runtime borrowed")
	}
}

func TestGemmaSharedTerminalMissingCorrelationReturnsImmediately(t *testing.T) {
	s, p := sharedFixture()
	o := s.items["original"]
	o.Status = "completed"
	o.Result = "legacy completed result, no exact reply"
	s.items["original"] = o
	started := time.Now()
	out, err := p.run(context.Background(), "", "original", 5*time.Minute)
	if err != nil || out.State != "correlation_unavailable" || time.Since(started) > time.Second {
		t.Fatalf("terminal waited: %+v %v", out, err)
	}
	if len(s.reads) != 1 || s.reads[0] != "original" || s.sends != 0 {
		t.Fatalf("unexpected read/send: %v %d", s.reads, s.sends)
	}
}

func TestGemmaSharedRuntimeUsesRegisteredDesktopAnchorAndStart(t *testing.T) {
	for _, tc := range []struct {
		name, origin, surface, recorded, actual string
		ancestor                                bool
		anchor                                  int
		resolveErr                              bool
		want                                    bool
	}{
		{"claude ancestor", "", "claude", "start", "start", true, 0, false, true},
		{"desktop unique child", "Codex Desktop", "codex", "start", "start", false, 42, false, true},
		{"untrusted origin", "", "codex", "start", "start", false, 42, false, false},
		{"other surface", "Codex Desktop", "claude", "start", "start", false, 42, false, false},
		{"other host child", "Codex Desktop", "codex", "start", "start", false, 99, false, false},
		{"ambiguous host", "Codex Desktop", "codex", "start", "start", false, 0, true, false},
		{"pid reuse", "Codex Desktop", "codex", "old", "new", true, 42, false, false},
		{"missing start", "Codex Desktop", "codex", "", "", true, 42, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := router.Thread{PID: 42, Surface: tc.surface, StartTime: tc.recorded}
			resolve := func(surface string) (int, error) {
				if surface != "codex" {
					t.Fatal("wrong resolver surface")
				}
				if tc.resolveErr {
					return 0, errors.New("ambiguous")
				}
				return tc.anchor, nil
			}
			got := sharedGemmaRuntimeOwned(record, tc.origin, func(int) bool { return tc.ancestor }, resolve, func(int) string { return tc.actual })
			if got != tc.want {
				t.Fatalf("ownership=%v want=%v", got, tc.want)
			}
		})
	}
}
