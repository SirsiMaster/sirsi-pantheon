package desktopcustody

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"time"
)

func NativeExecutionBudget(context.Context) (ExecutionBudget, error) {
	if runtime.GOOS != "darwin" || os.Geteuid() != 0 {
		return ExecutionBudget{}, errors.New("sleep custody: existing privileged native supervisor required")
	}
	path := os.Getenv("SIRSI_MAAT_EXECUTION_RECEIPT")
	if path == "" {
		return ExecutionBudget{}, errors.New("sleep custody: actual normal supervisor execution fact missing; no caller-created grant fallback")
	}
	data, err := readNativeBudgetFile(path)
	if err != nil {
		return ExecutionBudget{}, err
	}
	return parseExecutionBudget(data, os.Getppid(), nativeParentStart(os.Getppid()), os.Args, time.Now().UTC())
}

func parseExecutionBudget(data []byte, parent int, birth string, argv []string, now time.Time) (ExecutionBudget, error) {
	var b ExecutionBudget
	if err := json.Unmarshal(data, &b); err != nil {
		return b, err
	}
	canonical, err := json.Marshal(b)
	if err != nil || !bytes.Equal(canonical, data) {
		return b, errors.New("sleep custody: noncanonical execution fact")
	}
	start, e1 := time.Parse(time.RFC3339Nano, b.ExecutionStarted)
	end, e2 := time.Parse(time.RFC3339Nano, b.ExecutionDeadline)
	if parent <= 1 || birth == "" || b.SupervisorPID != parent || b.SupervisorStart != birth || b.ArgvSHA256 != digestBytes([]byte(strings.Join(argv, "\x00"))) || b.BudgetSeconds < 1 || b.BudgetSeconds > 7200 || e1 != nil || e2 != nil || start.After(now.Add(5*time.Second)) || !end.After(now) || end.Sub(start) > 2*time.Hour || b.AdmissionID == "" || b.ThreadID == "" || b.Work == "" || b.Agent == "" {
		return b, errors.New("sleep custody: actual parent/start/argv/execution deadline differs")
	}
	return b, nil
}
