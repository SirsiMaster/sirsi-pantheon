package desktopcustody

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNativeBudgetRequiresActualParentStartArgvAndBoundedDeadline(t *testing.T) {
	now := time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC)
	argv := []string{"/Applications/Pantheon/sirsi", "desktop-sleep", "dispatch", "request-one"}
	b := ExecutionBudget{AdmissionID: "normal-grant", Agent: "sha", BudgetSeconds: 600, ArgvSHA256: digestBytes([]byte(strings.Join(argv, "\x00"))), ExecutionDeadline: now.Add(10 * time.Minute).Format(time.RFC3339), ExecutionStarted: now.Format(time.RFC3339), Resource: "m5", SupervisorPID: 123, SupervisorStart: "Tue Oct 20 06:00:00 2026", ThreadID: "actual-thread", Work: "controller"}
	data, _ := json.Marshal(b)
	if _, err := parseExecutionBudget(data, 123, b.SupervisorStart, argv, now); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"parent", "birth", "argv", "expired", "noncanonical"} {
		t.Run(mode, func(t *testing.T) {
			pid, birth, args, at, raw := 123, b.SupervisorStart, argv, now, data
			switch mode {
			case "parent":
				pid = 124
			case "birth":
				birth = "reused-pid"
			case "argv":
				args = []string{"different-job"}
			case "expired":
				at = now.Add(time.Hour)
			case "noncanonical":
				raw = append([]byte(" "), data...)
			}
			if _, err := parseExecutionBudget(raw, pid, birth, args, at); err == nil {
				t.Fatal("unowned execution fact accepted")
			}
		})
	}
}
