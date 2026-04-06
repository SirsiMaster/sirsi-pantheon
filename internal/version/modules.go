package version

// ModuleVersion tracks the version of each deity module.
var Modules = map[string]string{
	"anubis": "1.1.0", // v1.1.0: scan, clean, ghosts (ka), dedup (hathor)
	"isis":   "2.0.0", // v2.0.0: absorbs sekhmet — health, network, remediation
	"thoth":  "1.1.0", // v1.1.0: Stele integration — sync/compact events inscribed
	"maat":   "1.1.0", // v1.1.0: Stele integration — weigh/pulse events inscribed
	"seshat": "2.1.0", // v2.1.0: Stele integration — ingest events inscribed
	"hapi":   "1.1.0", // v1.1.0: Stele integration — detect events inscribed
	"seba":   "1.2.0", // v1.2.0: absorbs khepri — infra mapping + fleet discovery
	"net":    "1.1.0", // v1.1.0: scope weaving, alignment (formerly neith)
	"ra":     "1.1.0", // v1.1.0: ProtectGlyph, Stele deploy events
	"stele":  "1.0.0", // append-only hash-chained event ledger (ADR-014)
	"osiris": "0.5.0", // state snapshots, checkpoints (in development)
}

// Get returns the version of a module, or "unknown" if not registered.
func Get(module string) string {
	if v, ok := Modules[module]; ok {
		return v
	}
	return "unknown"
}
