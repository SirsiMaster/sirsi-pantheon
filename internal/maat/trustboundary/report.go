package trustboundary

import "strings"

// Checklist is the Trust-Boundary Law report skeleton (ADR-076 §3). Agents
// paste it into the PR body / router report and fill one verdict per line:
// `satisfied` or `n/a — <reason>`. Rule letters match the lint's findings.
var Checklist = []struct{ Rule, Item string }{
	{"A", "Bounds: every loop/allocation bound is a server constant or contiguous validated input — never a client-supplied number (row key 250000 → 250k rows; max-of-keys loop bound)"},
	{"B", "Ceilings: every client-driven output (pages, carried text, attachments) has a hard server ceiling with a typed refusal above it; no quadratic work on untrusted input"},
	{"C", "Provenance: the server attests provenance (HMAC / signed token / server record); no client-sent hash, receipt or tag is trusted, even when it matches the bytes the client also sent"},
	{"D", "Sibling gates: a new path through a resource calls the same gate functions as the existing paths — named here: <gate functions>"},
	{"E", "Replace-not-merge: persisted maps carrying user data are replaced whole, never merged; a stale-key test proves a prior version's keys do not survive"},
	{"F", "Reader classification: every stored field is classified client-readable vs server-only; PII is server-only and encrypted; nothing sensitive sits on a viewer-readable document path"},
	{"G", "No silent loss / no meaning change: accepted input is never dropped without a typed refusal; no normalisation turns a value into a different meaning ('-500' → '500')"},
	{"H", "Process: verifiers gate by their own exit status (never `… | tail && push`); zero self-exemptions in the pushed range; algorithms are bounded-but-complete (iterative with cycle detection, no depth cap that rejects valid input) and order-independent"},
}

// Report renders the checklist skeleton as Markdown.
func Report() string {
	var b strings.Builder
	b.WriteString("### Trust-Boundary Law checklist (ADR-076) — one line per item: `satisfied` | `n/a — <reason>`\n\n")
	for _, c := range Checklist {
		b.WriteString("- [ ] **")
		b.WriteString(c.Rule)
		b.WriteString(".** ")
		b.WriteString(c.Item)
		b.WriteString(" — _verdict:_ \n")
	}
	b.WriteString("\nLint: `sirsi maat gate --base <base> --head <head>` → <pass | findings listed below>\n")
	return b.String()
}
