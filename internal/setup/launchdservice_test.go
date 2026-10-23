package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeServiceBackend struct {
	before, after          ServiceInstance
	reads, refreshes       int
	inspectErr, refreshErr error
}

func (b *fakeServiceBackend) Inspect(context.Context, string) (ServiceInstance, error) {
	b.reads++
	if b.inspectErr != nil {
		return ServiceInstance{}, b.inspectErr
	}
	if b.refreshes > 0 {
		return b.after, nil
	}
	return b.before, nil
}
func (b *fakeServiceBackend) Refresh(context.Context, ServiceInstance) error {
	b.refreshes++
	return b.refreshErr
}
func serviceFixture() ServiceInstance {
	return ServiceInstance{Label: "ai.sirsi.apollo.observer", Target: "gui/501/ai.sirsi.apollo.observer", Plist: "/private/service.plist", PlistSHA256: strings.Repeat("a", 64), Program: "/private/observer", ProgramSHA256: strings.Repeat("b", 64), PID: 123, Started: "original-start"}
}
func TestServiceControllerRefusesBeforeMutation(t *testing.T) {
	for _, name := range []string{"missing-authority", "denied", "changed-pid", "changed-start", "changed-plist", "changed-program", "inspect-failure", "incomplete-plan"} {
		t.Run(name, func(t *testing.T) {
			p := serviceFixture()
			b := &fakeServiceBackend{before: p}
			c := ServiceController{Backend: b, Authorize: func(context.Context, string) error { return nil }}
			switch name {
			case "missing-authority":
				c.Authorize = nil
			case "denied":
				c.Authorize = func(context.Context, string) error { return errors.New("foreign actor") }
			case "changed-pid":
				b.before.PID++
			case "changed-start":
				b.before.Started = "reused-pid"
			case "changed-plist":
				b.before.PlistSHA256 = strings.Repeat("c", 64)
			case "changed-program":
				b.before.ProgramSHA256 = strings.Repeat("d", 64)
			case "inspect-failure":
				b.inspectErr = errors.New("unavailable")
			case "incomplete-plan":
				p.ProgramSHA256 = ""
			}
			if _, err := c.Refresh(context.Background(), p); err == nil {
				t.Fatal("unsafe refresh accepted")
			}
			if b.refreshes != 0 {
				t.Fatal("refreshed before authorization and identity checks")
			}
		})
	}
}
func TestServiceControllerReadback(t *testing.T) {
	for _, name := range []string{"success", "refresh-failure", "same-instance", "different-artifact", "different-target"} {
		t.Run(name, func(t *testing.T) {
			p := serviceFixture()
			next := p
			next.PID++
			next.Started = "new-start"
			b := &fakeServiceBackend{before: p, after: next}
			switch name {
			case "refresh-failure":
				b.refreshErr = errors.New("bootstrap failed")
			case "same-instance":
				b.after = p
			case "different-artifact":
				b.after.ProgramSHA256 = strings.Repeat("c", 64)
			case "different-target":
				b.after.Target = "system/another"
			}
			c := ServiceController{Backend: b, Authorize: func(_ context.Context, label string) error {
				if label != p.Label {
					t.Fatal("different label authorized")
				}
				return nil
			}}
			got, err := c.Refresh(context.Background(), p)
			if (err == nil) != (name == "success") {
				t.Fatalf("result %v", err)
			}
			if b.refreshes != 1 {
				t.Fatalf("refresh count %d", b.refreshes)
			}
			if name == "success" && got != next {
				t.Fatal("incorrect readback")
			}
		})
	}
}
