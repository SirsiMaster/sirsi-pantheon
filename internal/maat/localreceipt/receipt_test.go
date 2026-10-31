package localreceipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestLocalFactClosedTier1IdentityAndDeadline(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	argv := []string{"/Applications/Pantheon.app/Contents/MacOS/sirsi", "vitals", "--json"}
	h := sha256.Sum256([]byte(argv[0] + "\x00vitals\x00--json"))
	f := Fact{Schema: Schema, Scope: "local", UID: 501, SupervisorPID: 123, SupervisorStart: "exact-start", ArgvSHA256: hex.EncodeToString(h[:]), StartedAt: now.Add(-time.Second), Deadline: now.Add(time.Minute), CPUCores: 1}
	data, _ := json.Marshal(f)
	if err := Validate(data, 501, 123, "exact-start", argv, now); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"root", "foreign-user", "foreign-parent", "changed-start", "expired", "future", "argv", "cores", "fabric", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			v := f
			uid, parent, birth := 501, 123, "exact-start"
			a := append([]string{}, argv...)
			switch kind {
			case "root":
				uid = 0
			case "foreign-user":
				uid = 502
			case "foreign-parent":
				parent = 124
			case "changed-start":
				birth = "new"
			case "expired":
				v.Deadline = now
			case "future":
				v.StartedAt = now.Add(time.Hour)
			case "argv":
				a[1] = "router"
			case "cores":
				v.CPUCores = 2
			case "fabric":
				v.Scope = "fabric"
			}
			b, _ := json.Marshal(v)
			if kind == "malformed" {
				b = []byte("{")
			}
			if err := Validate(b, uid, parent, birth, a, now); err == nil {
				t.Fatal("unbound local fact accepted")
			}
		})
	}
}

func TestLocalFactCannotAuthorizeFabricPrivilegedOrSNE(t *testing.T) {
	for _, a := range [][]string{{"router", "task", "complete"}, {"maat", "reserve", "studio"}, {"maat", "authority-serve"}, {"launchd", "maintenance-app"}, {"ask", "--local"}, {"apollo", "run"}, {"stacklab", "doctor"}, {"thoth", "brain"}, {"diagnose", "--fix"}, {"liveness-watch", "restore-disabled", "--confirm"}} {
		if Allowed(a) {
			t.Fatalf("remote/privileged/model operation admitted: %v", a)
		}
	}
	for _, a := range [][]string{{"version", "--json"}, {"vitals", "--json"}, {"maat", "triage", "--confirm", "--json"}, {"thoth", "status"}, {"stacklab", "local", "enable", "--provider", "self-hosted"}, {"liveness-watch", "run", "--local", "--json"}} {
		if !Allowed(a) {
			t.Fatalf("Tier1 route refused: %v", a)
		}
	}
}
