package schedule

import (
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/scales"
)

func TestNetTrafficProbe_AttributesLoadToTheBusyLane(t *testing.T) {
	SetLocalMachineLabelFn(func() (string, error) { return "m1", nil })
	defer SetLocalMachineLabelFn(nil)

	call := 0
	snapshots := []map[string][2]int64{
		{"en1": {0, 0}, "en2": {0, 0}},
		{"en1": {0, 1_000_000}, "en2": {0, 100}}, // en1: 2MB/s out, en2: well under the floor
	}
	SetTrafficProviders(
		func() (map[string][2]int64, error) {
			s := snapshots[call]
			if call < len(snapshots)-1 {
				call++
			}
			return s, nil
		},
		func() ([]scales.TBLane, error) {
			return []scales.TBLane{
				{Iface: "en1", Port: "Thunderbolt 1", Active: true},
				{Iface: "en2", Port: "Thunderbolt 2", Active: true},
				{Iface: "en3", Port: "Thunderbolt 3", Active: false}, // idle port: never sampled
			}, nil
		},
	)
	defer SetTrafficProviders(nil, nil)
	origWindow := getTrafficSampleWindow()
	setTrafficSampleWindowForTest(0)
	defer setTrafficSampleWindowForTest(origWindow)

	actors, err := NetTrafficProbe("m1")
	if err != nil {
		t.Fatalf("NetTrafficProbe: %v", err)
	}
	if len(actors) != 1 {
		t.Fatalf("expected exactly 1 actor (en1 busy, en2 under floor, en3 inactive), got %#v", actors)
	}
	if actors[0].Iface != "en1" || actors[0].Kind != "traffic" {
		t.Fatalf("unexpected actor: %#v", actors[0])
	}
}

func TestNetTrafficProbe_RefusesCrossHost(t *testing.T) {
	SetLocalMachineLabelFn(func() (string, error) { return "m1", nil })
	defer SetLocalMachineLabelFn(nil)

	_, err := NetTrafficProbe("m5")
	if err == nil {
		t.Fatal("expected refusal probing a different machine, got nil error")
	}
}

func TestNetTrafficProbe_NoActiveLanesIsNotAnError(t *testing.T) {
	SetLocalMachineLabelFn(func() (string, error) { return "m1", nil })
	defer SetLocalMachineLabelFn(nil)
	SetTrafficProviders(
		func() (map[string][2]int64, error) { return nil, errors.New("must not be called") },
		func() ([]scales.TBLane, error) { return nil, nil },
	)
	defer SetTrafficProviders(nil, nil)

	actors, err := NetTrafficProbe("m1")
	if err != nil {
		t.Fatalf("NetTrafficProbe: %v", err)
	}
	if len(actors) != 0 {
		t.Fatalf("expected no actors, got %#v", actors)
	}
}

func TestReadNetstatIB_ParsesLinkRowsOnly(t *testing.T) {
	// Mirrors real `netstat -ib` output shapes: a virtual interface with a
	// blank Address column (lo0) and a hardware interface with a MAC in that
	// column (en1) parse to the same counter positions, read from the end of
	// the row rather than a fixed index.
	sample := "Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll\n" +
		"lo0        16384 <Link#1>                      10     0        100       20     0        200     0\n" +
		"lo0        16384 127           localhost       10     -        100       20     -        200     -\n" +
		"en1        65518 <Link#13>   36:14:af:01:a5:00 30     0        300       40     0        400     0\n" +
		"en1        65518 10.96.140/24  10.96.140.1     30     -        300       40     -        400     -\n"

	counters, err := parseNetstatIB(sample)
	if err != nil {
		t.Fatal(err)
	}
	if counters["lo0"] != [2]int64{100, 200} {
		t.Fatalf("lo0 (blank-address link row) = %v, want [100 200]", counters["lo0"])
	}
	if counters["en1"] != [2]int64{300, 400} {
		t.Fatalf("en1 (MAC-address link row) = %v, want [300 400]", counters["en1"])
	}
	if len(counters) != 2 {
		t.Fatalf("expected only the 2 link rows counted, got %#v", counters)
	}
}
