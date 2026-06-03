package platform

// DiskAccessLevel is the spectrum of how much of the workstation Sirsi can
// actually see — not a boolean. macOS grants disk access in tiers, so a hygiene
// tool's visibility is all / some / none.
type DiskAccessLevel int

const (
	// AccessNone: blind to every protected location — scans/cleans are materially
	// incomplete.
	AccessNone DiskAccessLevel = iota
	// AccessSome: piecemeal per-folder grants (e.g. Desktop yes, Documents no) but
	// not full access — Sirsi sees part of the machine.
	AccessSome
	// AccessFull: full disk access — Sirsi can see everything. The level a
	// workstation hygiene tool wants.
	AccessFull
)

func (l DiskAccessLevel) String() string {
	switch l {
	case AccessFull:
		return "full"
	case AccessSome:
		return "some"
	default:
		return "none"
	}
}

// DiskAccess is the resolved visibility status: the level plus which probed
// locations are readable vs blind, so a surface can show exactly what's missing.
type DiskAccess struct {
	Level      DiskAccessLevel
	Accessible []string // human names of readable probes
	Blocked    []string // human names of blocked probes
}
