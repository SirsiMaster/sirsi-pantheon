package vitals

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// NOTE: these tests swap the package-level runCommand seam and mutate the
// package-level network-delta state, so none of them may use t.Parallel().

// fakeCommands swaps runCommand for a canned-output lookup keyed by
// "name arg1 arg2 ...". Unknown commands return an error, which exercises
// each collector's error path. The original seam is restored via t.Cleanup.
func fakeCommands(t *testing.T, outputs map[string]string) {
	t.Helper()
	orig := runCommand
	t.Cleanup(func() { runCommand = orig })
	runCommand = func(name string, args ...string) ([]byte, error) {
		key := strings.TrimSpace(name + " " + strings.Join(args, " "))
		out, ok := outputs[key]
		if !ok {
			return nil, fmt.Errorf("no canned output for %q", key)
		}
		return []byte(out), nil
	}
}

// resetNetState zeroes the package-level network delta state before and
// after a test so collectNetwork tests are order-independent.
func resetNetState(t *testing.T) {
	t.Helper()
	reset := func() {
		netMu.Lock()
		prevNetDown, prevNetUp = 0, 0
		prevNetTime = time.Time{}
		netMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

const vmStatOut = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                              100000.
Pages active:                           1048576.
Pages inactive:                          200000.
Pages speculative:                        50000.
Pages wired down:                        524288.
Pages purgeable:                          10000.
`

func TestCollectRAM(t *testing.T) {
	// 16 GiB total; active 1048576 pages + wired 524288 pages at the
	// hardcoded 4096 bytes/page = 6 GiB used = 37.5% => low pressure.
	fakeCommands(t, map[string]string{
		"sysctl -n hw.memsize": "17179869184\n",
		"vm_stat":              vmStatOut,
	})

	var s Snapshot
	collectRAM(&s)

	if !almostEqual(s.RAMPercent, 37.5, 0.01) {
		t.Errorf("RAMPercent = %v, want 37.5", s.RAMPercent)
	}
	if !almostEqual(s.RAMTotalGB, 16, 0.01) {
		t.Errorf("RAMTotalGB = %v, want 16", s.RAMTotalGB)
	}
	if !almostEqual(s.RAMUsedGB, 6, 0.01) {
		t.Errorf("RAMUsedGB = %v, want 6", s.RAMUsedGB)
	}
	if s.RAMPressure != "low" || s.RAMIcon != "🟢" {
		t.Errorf("pressure = %q icon = %q, want low 🟢", s.RAMPressure, s.RAMIcon)
	}
}

func TestCollectRAMPressureLevels(t *testing.T) {
	tests := []struct {
		name         string
		activePages  int64
		wiredPages   int64
		wantPressure string
		wantIcon     string
	}{
		// total is fixed at 16 GiB; pages are 4096 bytes in the parser.
		{"medium at 75 percent", 2621440, 524288, "medium", "🟡"}, // 10+2 GiB
		{"high at 93.75 percent", 3407872, 524288, "high", "🔴"},  // 13+2 GiB
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := fmt.Sprintf(
				"Pages free: 1000.\nPages active: %d.\nPages wired down: %d.\n",
				tt.activePages, tt.wiredPages)
			fakeCommands(t, map[string]string{
				"sysctl -n hw.memsize": "17179869184",
				"vm_stat":              vm,
			})
			var s Snapshot
			collectRAM(&s)
			if s.RAMPressure != tt.wantPressure || s.RAMIcon != tt.wantIcon {
				t.Errorf("pressure = %q icon = %q, want %q %q",
					s.RAMPressure, s.RAMIcon, tt.wantPressure, tt.wantIcon)
			}
		})
	}
}

func TestCollectRAMErrorPaths(t *testing.T) {
	// Command failure leaves the snapshot untouched.
	fakeCommands(t, map[string]string{})
	var s Snapshot
	collectRAM(&s)
	if s.RAMPercent != 0 || s.RAMPressure != "" {
		t.Errorf("expected zero snapshot on command failure, got %+v", s)
	}

	// Zero total RAM bails out before running vm_stat.
	fakeCommands(t, map[string]string{
		"sysctl -n hw.memsize": "0\n",
	})
	s = Snapshot{}
	collectRAM(&s)
	if s.RAMPercent != 0 {
		t.Errorf("expected early return on zero memsize, got %+v", s)
	}
}

func TestCollectGit(t *testing.T) {
	fakeCommands(t, map[string]string{
		"git branch --show-current": "main\n",
		"git status --porcelain":    " M foo.go\n?? bar.go\nA  baz.go\n",
		"git log -1 --format=%cr":   "2 hours ago\n",
	})

	var s Snapshot
	collectGit(&s)

	if s.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want main", s.GitBranch)
	}
	if s.Uncommitted != 3 {
		t.Errorf("Uncommitted = %d, want 3", s.Uncommitted)
	}
	if s.LastCommit != "2 hours ago" {
		t.Errorf("LastCommit = %q, want '2 hours ago'", s.LastCommit)
	}
}

func TestCollectGitCleanTree(t *testing.T) {
	fakeCommands(t, map[string]string{
		"git branch --show-current": "test/maat-coverage-85\n",
		"git status --porcelain":    "",
		"git log -1 --format=%cr":   "5 minutes ago\n",
	})

	var s Snapshot
	collectGit(&s)

	if s.Uncommitted != 0 {
		t.Errorf("Uncommitted = %d, want 0 for clean tree", s.Uncommitted)
	}
	if s.GitBranch != "test/maat-coverage-85" {
		t.Errorf("GitBranch = %q", s.GitBranch)
	}
}

func TestCollectAccelerator(t *testing.T) {
	tests := []struct {
		name  string
		brand string
		want  string
	}{
		{"apple silicon shortened", "Apple M5 Max\n", "M5 Max"},
		{"intel kept verbatim", "Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz\n", "Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz"},
		{"bare apple kept", "Apple\n", "Apple"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeCommands(t, map[string]string{
				"sysctl -n machdep.cpu.brand_string": tt.brand,
			})
			var s Snapshot
			collectAccelerator(&s)
			if s.Accelerator != tt.want {
				t.Errorf("Accelerator = %q, want %q", s.Accelerator, tt.want)
			}
		})
	}
}

const topOut = `Processes: 500 total, 2 running, 498 sleeping, 2500 threads
Load Avg: 2.50, 2.80, 3.10
CPU usage: 12.34% user, 5.67% sys, 81.99% idle
SharedLibs: 400M resident, 50M data, 20M linkedit.
`

func TestCollectCPU(t *testing.T) {
	fakeCommands(t, map[string]string{
		"top -l 1 -n 0 -s 0":      topOut,
		"sysctl -n hw.logicalcpu": "8\n",
	})

	var s Snapshot
	collectCPU(&s)

	if !almostEqual(s.CPUPercent, 18.01, 0.001) {
		t.Errorf("CPUPercent = %v, want 18.01", s.CPUPercent)
	}
	if len(s.CPUCores) != 8 {
		t.Fatalf("CPUCores len = %d, want 8", len(s.CPUCores))
	}
	for i, c := range s.CPUCores {
		if !almostEqual(c, 18.01, 0.001) {
			t.Errorf("CPUCores[%d] = %v, want 18.01", i, c)
		}
	}
}

func TestCollectCPUBadCoreCount(t *testing.T) {
	fakeCommands(t, map[string]string{
		"top -l 1 -n 0 -s 0":      topOut,
		"sysctl -n hw.logicalcpu": "not-a-number\n",
	})

	var s Snapshot
	collectCPU(&s)

	if !almostEqual(s.CPUPercent, 18.01, 0.001) {
		t.Errorf("CPUPercent = %v, want 18.01 even when core count fails", s.CPUPercent)
	}
	if s.CPUCores != nil {
		t.Errorf("CPUCores = %v, want nil on unparsable core count", s.CPUCores)
	}
}

const psOut = ` %CPU    RSS   PID COMM
 45.3 204800   123 /Applications/Foo.app/Contents/MacOS/Foo
 30.0 102400   200 kernel_task
 12.5  51200   300 /usr/bin/bar
  0.0   1024   400 idleproc
  5.0  20480   500 launchd
  4.0  10240   600 WindowServer
  3.5   8192   700 baz
  2.0   4096   800 qux
  1.5   2048   900 quux
  1.0   1024  1000 corge
`

func TestCollectTopProcs(t *testing.T) {
	fakeCommands(t, map[string]string{
		"ps -Ao %cpu,rss,pid,comm -r": psOut,
	})

	var s Snapshot
	collectTopProcs(&s)

	// Header is unparsable, kernel_task/launchd/WindowServer are filtered,
	// zero-CPU rows are skipped, and the list caps at 5.
	wantNames := []string{"Foo", "bar", "baz", "qux", "quux"}
	if len(s.TopProcs) != len(wantNames) {
		t.Fatalf("TopProcs len = %d, want %d (%+v)", len(s.TopProcs), len(wantNames), s.TopProcs)
	}
	for i, want := range wantNames {
		if s.TopProcs[i].Name != want {
			t.Errorf("TopProcs[%d].Name = %q, want %q", i, s.TopProcs[i].Name, want)
		}
	}

	first := s.TopProcs[0]
	if !almostEqual(first.CPUPercent, 45.3, 0.001) {
		t.Errorf("TopProcs[0].CPUPercent = %v, want 45.3", first.CPUPercent)
	}
	if !almostEqual(first.MemMB, 200, 0.001) {
		t.Errorf("TopProcs[0].MemMB = %v, want 200", first.MemMB)
	}
	if first.PID != 123 {
		t.Errorf("TopProcs[0].PID = %d, want 123", first.PID)
	}
}

// netstat -ib canned output: header, loopback, one short row, and en0.
const netstatOut = `Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes
lo0        16384 <Link#1>                          9000     0     900000     9000     0     900000
en0        1500  <Link#4>
en0        1500  <Link#4>      aa:bb:cc:dd:ee:ff   1000     0     500000      800     0     250000
`

func TestCollectNetworkFirstSampleHasNoRate(t *testing.T) {
	resetNetState(t)
	fakeCommands(t, map[string]string{"netstat -ib": netstatOut})

	var s Snapshot
	collectNetwork(&s)

	if s.NetDownBps != 0 || s.NetUpBps != 0 {
		t.Errorf("first sample should report zero rates, got down=%v up=%v",
			s.NetDownBps, s.NetUpBps)
	}

	netMu.Lock()
	down, up := prevNetDown, prevNetUp
	netMu.Unlock()
	if down != 500000 || up != 250000 {
		t.Errorf("prev counters = %d/%d, want 500000/250000", down, up)
	}
}

func TestCollectNetworkComputesRates(t *testing.T) {
	resetNetState(t)
	fakeCommands(t, map[string]string{"netstat -ib": netstatOut})

	// Seed a previous sample 10s ago, 400000 bytes down / 200000 up behind.
	netMu.Lock()
	prevNetDown = 100000
	prevNetUp = 50000
	prevNetTime = time.Now().Add(-10 * time.Second)
	netMu.Unlock()

	var s Snapshot
	collectNetwork(&s)

	// dt is ~10s (slightly more), so rates are just under 40000/20000 B/s.
	if s.NetDownBps < 35000 || s.NetDownBps > 40000 {
		t.Errorf("NetDownBps = %v, want ~40000", s.NetDownBps)
	}
	if s.NetUpBps < 17500 || s.NetUpBps > 20000 {
		t.Errorf("NetUpBps = %v, want ~20000", s.NetUpBps)
	}
}

func TestCollectNetworkClampsNegativeRates(t *testing.T) {
	resetNetState(t)
	fakeCommands(t, map[string]string{"netstat -ib": netstatOut})

	// Previous counters larger than current (interface reset) => clamp to 0.
	netMu.Lock()
	prevNetDown = 900000
	prevNetUp = 600000
	prevNetTime = time.Now().Add(-10 * time.Second)
	netMu.Unlock()

	var s Snapshot
	collectNetwork(&s)

	if s.NetDownBps != 0 || s.NetUpBps != 0 {
		t.Errorf("expected clamped zero rates, got down=%v up=%v",
			s.NetDownBps, s.NetUpBps)
	}
}

const dfOut = `Filesystem     1G-blocks Used Available Capacity iused      ifree %iused  Mounted on
/dev/disk3s5         926  400       500    45%  500000 5000000000     0%   /
`

func TestCollectDisk(t *testing.T) {
	fakeCommands(t, map[string]string{"df -g /": dfOut})

	var s Snapshot
	collectDisk(&s)

	if s.DiskTotalGB != 926 || s.DiskUsedGB != 400 || s.DiskFreeGB != 500 {
		t.Errorf("disk = total %v used %v free %v, want 926/400/500",
			s.DiskTotalGB, s.DiskUsedGB, s.DiskFreeGB)
	}
	want := 400.0 / 926.0 * 100
	if !almostEqual(s.DiskPercent, want, 0.001) {
		t.Errorf("DiskPercent = %v, want %v", s.DiskPercent, want)
	}
}

func TestCollectDiskMalformedOutput(t *testing.T) {
	fakeCommands(t, map[string]string{"df -g /": "Filesystem only-header\n"})
	var s Snapshot
	collectDisk(&s)
	if s.DiskTotalGB != 0 || s.DiskPercent != 0 {
		t.Errorf("expected zero disk stats on malformed output, got %+v", s)
	}
}

func TestCollectLoadAvg(t *testing.T) {
	fakeCommands(t, map[string]string{
		"sysctl -n vm.loadavg": "{ 1.23 4.56 7.89 }\n",
	})

	var s Snapshot
	collectLoadAvg(&s)

	want := [3]float64{1.23, 4.56, 7.89}
	for i := range want {
		if !almostEqual(s.CPULoadAvg[i], want[i], 0.001) {
			t.Errorf("CPULoadAvg[%d] = %v, want %v", i, s.CPULoadAvg[i], want[i])
		}
	}
}

func TestCollectUptime(t *testing.T) {
	tests := []struct {
		name   string
		uptime time.Duration
		want   string
	}{
		{"days and hours", 3*24*time.Hour + 12*time.Hour + 30*time.Minute, "3d 12h"},
		{"hours and minutes", 2*time.Hour + 30*time.Minute + 5*time.Second, "2h 30m"},
		{"minutes only", 45*time.Minute + 5*time.Second, "45m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boot := time.Now().Add(-tt.uptime)
			out := fmt.Sprintf("{ sec = %d, usec = 123456 } Thu Jun 25 10:00:00 2026\n", boot.Unix())
			fakeCommands(t, map[string]string{
				"sysctl -n kern.boottime": out,
			})
			var s Snapshot
			collectUptime(&s)
			if s.UptimeStr != tt.want {
				t.Errorf("UptimeStr = %q, want %q", s.UptimeStr, tt.want)
			}
		})
	}
}

func TestCollectUptimeMalformed(t *testing.T) {
	fakeCommands(t, map[string]string{
		"sysctl -n kern.boottime": "{ sec = not-a-number, usec = 0 }\n",
	})
	var s Snapshot
	collectUptime(&s)
	if s.UptimeStr != "" {
		t.Errorf("UptimeStr = %q, want empty on parse failure", s.UptimeStr)
	}
}

func TestCollectMachineInfo(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"MacBookPro18,2", "MacBook Pro"},
		{"MacBookAir10,1", "MacBook Air"},
		{"Macmini9,1", "Mac mini"},
		{"MacPro7,1", "Mac Pro"},
		{"iMac21,1", "iMac"},
		{"Mac14,12", "Mac14,12"}, // unknown identifier kept verbatim
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			fakeCommands(t, map[string]string{
				"sysctl -n hw.model":      tt.model + "\n",
				"sw_vers -productVersion": "15.5\n",
			})
			var s Snapshot
			collectMachineInfo(&s)
			if s.ModelName != tt.want {
				t.Errorf("ModelName = %q, want %q", s.ModelName, tt.want)
			}
			if s.OSVersion != "macOS 15.5" {
				t.Errorf("OSVersion = %q, want 'macOS 15.5'", s.OSVersion)
			}
		})
	}
}

func TestCollectFullSnapshot(t *testing.T) {
	resetNetState(t)
	fakeCommands(t, map[string]string{
		"sysctl -n hw.memsize":               "17179869184\n",
		"vm_stat":                            vmStatOut,
		"git branch --show-current":          "main\n",
		"git status --porcelain":             " M a.go\n",
		"git log -1 --format=%cr":            "3 days ago\n",
		"sysctl -n machdep.cpu.brand_string": "Apple M5 Max\n",
		"top -l 1 -n 0 -s 0":                 topOut,
		"sysctl -n hw.logicalcpu":            "12\n",
		"ps -Ao %cpu,rss,pid,comm -r":        psOut,
		"netstat -ib":                        netstatOut,
		"df -g /":                            dfOut,
		"sysctl -n vm.loadavg":               "{ 0.50 0.75 1.00 }\n",
		"sysctl -n kern.boottime":            fmt.Sprintf("{ sec = %d, usec = 0 } x\n", time.Now().Add(-25*time.Hour).Unix()),
		"sysctl -n hw.model":                 "MacBookPro18,2\n",
		"sw_vers -productVersion":            "15.5\n",
	})

	s := Collect()

	if s.RAMPressure != "low" {
		t.Errorf("RAMPressure = %q, want low", s.RAMPressure)
	}
	if s.GitBranch != "main" || s.Uncommitted != 1 || s.LastCommit != "3 days ago" {
		t.Errorf("git fields = %q/%d/%q", s.GitBranch, s.Uncommitted, s.LastCommit)
	}
	if s.Accelerator != "M5 Max" {
		t.Errorf("Accelerator = %q, want 'M5 Max'", s.Accelerator)
	}
	if len(s.CPUCores) != 12 {
		t.Errorf("CPUCores len = %d, want 12", len(s.CPUCores))
	}
	if len(s.TopProcs) != 5 {
		t.Errorf("TopProcs len = %d, want 5", len(s.TopProcs))
	}
	if s.DiskTotalGB != 926 {
		t.Errorf("DiskTotalGB = %v, want 926", s.DiskTotalGB)
	}
	if !almostEqual(s.CPULoadAvg[0], 0.50, 0.001) {
		t.Errorf("CPULoadAvg[0] = %v, want 0.50", s.CPULoadAvg[0])
	}
	if s.UptimeStr != "1d 1h" {
		t.Errorf("UptimeStr = %q, want '1d 1h'", s.UptimeStr)
	}
	if s.ModelName != "MacBook Pro" || s.OSVersion != "macOS 15.5" {
		t.Errorf("machine info = %q / %q", s.ModelName, s.OSVersion)
	}
}

func TestSortProcsByCPU(t *testing.T) {
	procs := []ProcInfo{
		{Name: "low", CPUPercent: 1.5},
		{Name: "high", CPUPercent: 88.0},
		{Name: "mid", CPUPercent: 42.0},
	}
	SortProcsByCPU(procs)

	wantOrder := []string{"high", "mid", "low"}
	for i, want := range wantOrder {
		if procs[i].Name != want {
			t.Errorf("procs[%d].Name = %q, want %q", i, procs[i].Name, want)
		}
	}
}
