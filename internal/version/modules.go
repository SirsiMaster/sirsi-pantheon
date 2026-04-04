package version

// ModuleVersion tracks the version of each deity module.
var Modules = map[string]string{
	"ka":      "1.1.0", // v1.1.0: multi-layer ghost matching, app enumerator, uninstaller
	"anubis":  "1.0.0", // scanning, cleaning, safety
	"thoth":   "1.1.0", // v1.1.0: Stele integration — sync/compact events inscribed
	"maat":    "1.1.0", // v1.1.0: Stele integration — weigh/pulse events inscribed
	"seshat":  "2.1.0", // v2.1.0: Stele integration — ingest events inscribed
	"hapi":    "1.1.0", // v1.1.0: Stele integration — detect events inscribed
	"seba":    "1.1.0", // v1.1.0: Stele integration — render events inscribed
	"horus":   "1.0.0", // filesystem index, sight
	"sekhmet": "1.1.0", // v1.1.0: Stele integration — guard start events inscribed
	"khepri":  "1.0.0", // network scan, container audit
	"isis":    "1.0.0", // remediation engine
	"neith":   "1.1.0", // v1.1.0: Stele integration — weave/drift events inscribed
	"ra":      "1.1.0", // v1.1.0: ProtectGlyph, Stele deploy events
	"stele":   "1.0.0", // append-only hash-chained event ledger (ADR-014)
	"osiris":  "0.5.0", // FinalWishes checkpoint (partial)
	"hathor":  "1.0.0", // mirror dedup
}

// Get returns the version of a module, or "unknown" if not registered.
func Get(module string) string {
	if v, ok := Modules[module]; ok {
		return v
	}
	return "unknown"
}
