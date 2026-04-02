package seba

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Register a mock guard audit so runtime diagram tests can run.
	RegisterGuardAudit(func() (*AuditResult, error) {
		return &AuditResult{
			TotalRAM: 32 * 1024 * 1024 * 1024, // 32GB
			UsedRAM:  16 * 1024 * 1024 * 1024, // 16GB
			FreeRAM:  16 * 1024 * 1024 * 1024,
			Groups: []ProcessGroup{
				{
					Name:       "IDE",
					TotalRSS:   4 * 1024 * 1024 * 1024,
					TotalCount: 5,
					Processes: []ProcessInfo{
						{PID: 1234, Name: "code", Command: "code", RSS: 2 * 1024 * 1024 * 1024, User: "test", CPUPercent: 5.0, Group: "IDE"},
					},
				},
			},
			Orphans:      nil,
			TotalOrphans: 0,
			OrphanRSS:    0,
		}, nil
	})

	os.Exit(m.Run())
}
