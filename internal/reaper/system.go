package reaper

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// RealDeps wires the reaper to the live OS: `ps` for the process table,
// syscall.Kill for signals, signal-0 for the survivor liveness re-check.
func RealDeps() Deps {
	return Deps{
		Procs:   psProcs,
		SelfPID: os.Getpid(),
		Kill:    func(pid, sig int) error { return syscall.Kill(pid, syscall.Signal(sig)) },
		Alive:   func(pid int) bool { return syscall.Kill(pid, 0) == nil },
	}
}

// psProcs enumerates the process table via one `ps` pass. Fields are emitted
// with trailing `=` (no headers); command is last so it can contain spaces.
func psProcs() ([]Proc, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,etime=,rss=,command=").Output()
	if err != nil {
		return nil, err
	}
	var procs []Proc
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		rssKB, err3 := strconv.Atoi(fields[3])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		procs = append(procs, Proc{
			PID:     pid,
			PPID:    ppid,
			AgeSec:  etimeSecs(fields[2]),
			RSSMB:   rssKB / 1024,
			Command: strings.Join(fields[4:], " "),
		})
	}
	return procs, nil
}

// etimeSecs parses ps etime ("MM:SS", "HH:MM:SS", or "DD-HH:MM:SS") to seconds.
func etimeSecs(s string) int {
	days := 0
	if i := strings.IndexByte(s, '-'); i >= 0 {
		days, _ = strconv.Atoi(s[:i])
		s = s[i+1:]
	}
	parts := strings.Split(s, ":")
	secs := 0
	for _, p := range parts {
		v, _ := strconv.Atoi(p)
		secs = secs*60 + v
	}
	return days*86400 + secs
}
