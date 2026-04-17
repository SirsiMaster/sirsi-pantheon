package main

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

func init() {
	// Wire guard.Audit into seba to break the import cycle.
	// seba cannot import guard directly because guard imports hapi which imports seba.
	seba.RegisterGuardAudit(func() (*seba.AuditResult, error) {
		result, err := guard.Audit()
		if err != nil {
			return nil, err
		}
		// Convert guard types to seba bridge types.
		out := &seba.AuditResult{
			TotalRAM:     result.TotalRAM,
			UsedRAM:      result.UsedRAM,
			FreeRAM:      result.FreeRAM,
			TotalOrphans: result.TotalOrphans,
			OrphanRSS:    result.OrphanRSS,
		}
		for _, g := range result.Groups {
			sg := seba.ProcessGroup{
				Name:       g.Name,
				TotalRSS:   g.TotalRSS,
				TotalCount: g.TotalCount,
			}
			for _, p := range g.Processes {
				sg.Processes = append(sg.Processes, seba.ProcessInfo{
					PID:        p.PID,
					Name:       p.Name,
					Command:    p.Command,
					RSS:        p.RSS,
					VSZ:        p.VSZ,
					User:       p.User,
					CPUPercent: p.CPUPercent,
					Group:      p.Group,
				})
			}
			out.Groups = append(out.Groups, sg)
		}
		for _, p := range result.Orphans {
			out.Orphans = append(out.Orphans, seba.ProcessInfo{
				PID:        p.PID,
				Name:       p.Name,
				Command:    p.Command,
				RSS:        p.RSS,
				VSZ:        p.VSZ,
				User:       p.User,
				CPUPercent: p.CPUPercent,
				Group:      p.Group,
			})
		}
		return out, nil
	})
}
