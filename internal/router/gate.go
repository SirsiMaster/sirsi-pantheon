package router

// gate.go — the Honest Gate: the deterministic boundary between work the
// continuous loop may do on its own and work that genuinely needs the owner
// (ADR-039). Code embodiment of "models in effort at all times except when there
// is an honest user gate."
//
// SAFETY POSTURE (mirrors internal/cleaner/safety.go): the dangerous classes are
// matched by HARDCODED rules, never by a model's judgment — a model can be talked
// out of a gate; this table cannot.
//
// DESIGN (v2, after an adversarial under-gating audit): matching is by
// case-insensitive REGEX with word boundaries — NOT substring bigrams. Substring
// bigrams produced false-NEGATIVES (the dangerous direction): "wire transfer"
// missed "wire $5k", "rm -rf" missed "rm -fr" / "rm --force", "make public"
// missed "world-readable". The floor must be robust to ordinary phrasing and flag
// variance, so danger ROOTS are matched as single high-signal tokens and CLI
// verbs as flag-insensitive patterns. The table is deliberately BROAD: a false
// gate costs the owner a glance; a missed gate could ship an irreversible action,
// so over-gating is intentional. The Tier-0 model may only ADD gates (fuzzy
// business/ambiguity); it may NEVER clear a gate matched here.

import (
	"regexp"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// GateClass is why an item is (or is not) owner-gated, in taxonomy order.
type GateClass int

const (
	GateNone         GateClass = iota // the loop may act on this itself
	GateSafety                        // money / access-control / IAM / destructive / credentials
	GateFounder                       // founder / business / terms / investor / brand / strategy
	GateIrreversible                  // publish / send / merge-to-prod / deploy / outward-facing
	GateEscalate                      // explicit ESCALATE / needs-owner, or addressed to the owner
)

func (c GateClass) String() string {
	switch c {
	case GateSafety:
		return "safety"
	case GateFounder:
		return "founder"
	case GateIrreversible:
		return "irreversible"
	case GateEscalate:
		return "escalate"
	default:
		return "none"
	}
}

// GateDecision is the classifier's verdict for one item.
type GateDecision struct {
	Gated  bool      `json:"gated"`
	Class  GateClass `json:"class"`
	Reason string    `json:"reason,omitempty"` // the matched rule / why it gates
}

type gateRule struct {
	re    *regexp.Regexp
	class GateClass
	label string
}

// mk compiles a case-insensitive rule. Patterns already assume (?i).
func mk(class GateClass, label, pat string) gateRule {
	return gateRule{re: regexp.MustCompile("(?i)" + pat), class: class, label: label}
}

// gateRules are evaluated in order; the FIRST match wins, so Safety rules are
// listed before Irreversible before Founder — the most dangerous class a mixed
// item touches is the one reported. Breadth is intentional (see file header).
var gateRules = []gateRule{
	// ── Safety · money movement ─────────────────────────────────────────────
	mk(GateSafety, "money", `\b(pay|paying|payment|payout|payable|remit|reimburse|deposit|withdraw|withdrawal|invoice|refund)\b`),
	mk(GateSafety, "charge", `\bcharg(e|ing)\b.{0,15}\b(card|customer|client|account)\b|\$`),
	mk(GateSafety, "money", `\b(wire|wiring|ach|swift|venmo|paypal|zelle|cashapp)\b`),
	mk(GateSafety, "money", `\bstripe\b.*\b(charge|payment|payout|refund|transfer)\b`),
	mk(GateSafety, "money amount", `\$\s?\d`),
	mk(GateSafety, "send funds", `\bsend\b.{0,25}\b(money|funds|payment|cash|\$)`),
	mk(GateSafety, "transfer funds", `\btransfer\b.{0,25}\b(money|funds|\$|to (the )?(account|vendor|supplier))`),
	mk(GateSafety, "securities", `\b(buy|sell|purchase|short|trade)\b.{0,20}\b(stock|share|securit|bond|option)\b`),
	mk(GateSafety, "crypto", `\b(crypto|cryptocurrency|bitcoin|ethereum|stablecoin|usdc|usdt|wallet)\b`),
	mk(GateSafety, "crypto", `\b(eth|btc|sol|xrp)\b`),
	mk(GateSafety, "crypto tx", `\b(sign|broadcast|submit)\b.{0,20}\btransaction\b|\bseed phrase\b|\bprivate key\b`),
	mk(GateSafety, "card", `\b(credit|debit)\s+card\b|\bcard on file\b`),

	// ── Safety · credentials / exfiltration ─────────────────────────────────
	mk(GateSafety, "credential", `\b(secret|secrets|credential|creds|password|passphrase|passwd)\b`),
	mk(GateSafety, "token/key", `\b(api[_ -]?key|access[_ -]?key|secret[_ -]?key|private[_ -]?key|client[_ -]?secret|aws[_ -]?secret|access[_ -]?token|auth[_ -]?token|bearer token|service account key)\b`),
	mk(GateSafety, "cred file", `\.(env|pem|p12|pfx|key|keystore)\b|\b(id_rsa|authorized_keys|\.aws/credentials|\.npmrc)\b`),
	mk(GateSafety, "ssh key", `\bssh\s+key\b|\bgpg\s+key\b`),
	mk(GateSafety, "env dump", `\b(print|cat|echo|dump|paste|leak|exfil)\b.{0,30}\b(env|secret|cred|token|key|password)`),
	mk(GateSafety, "env var", `\benv(ironment)?\s+var(iable)?s?\b`),

	// ── Safety · destructive ────────────────────────────────────────────────
	mk(GateSafety, "rm -rf", `\brm\s+-{1,2}\S*(r|f|recursive|force)`),
	mk(GateSafety, "sql drop", `\b(drop|truncate)\s+(table|database|schema|index)\b|\bdelete\s+from\b`),
	mk(GateSafety, "destroy", `\b(destroy|wipe|erase|obliterate|nuke|flushall|flushdb)\b`),
	mk(GateSafety, "iac teardown", `\bterraform\s+destroy\b|\bkubectl\s+delete\b|\bhelm\s+(uninstall|delete)\b`),
	mk(GateSafety, "git destructive", `\bgit\s+reset\s+--hard\b|\bgit\s+clean\s+-\S*(f|d)|\bgit\s+push\s+.*(-f\b|--force|-{1,2}force)`),
	mk(GateSafety, "force push", `\bforce[- ]?push\b`),
	mk(GateSafety, "delete resource", `\bdelete\b.{0,25}\b(database|prod|production|namespace|deployment|cluster|bucket|volume|snapshot|repo|repository|branch|table|user|account|record)`),
	mk(GateSafety, "purge/hard-delete", `\bpurge\b|\bhard[- ]?delete\b|\bempty the trash\b|\bpermanently delete\b`),

	// ── Safety · access-control / IAM / exposure ────────────────────────────
	mk(GateSafety, "iam", `\b(iam|add-iam-policy-binding|setiampolicy|getiampolicy)\b`),
	mk(GateSafety, "grant", `\bgrant\b|\broles/\w`),
	mk(GateSafety, "make public", `\bmake\b.{0,25}\bpublic\b|\bworld[- ]?readable\b|\bpublic[- ]?read\b|\ballusers\b|\ballauthenticatedusers\b|\bpublicly\s+(readable|accessible|writable)\b`),
	mk(GateSafety, "repo public", `\bvisibility\b.{0,15}\bpublic\b|\brepo(sitory)?\s+public\b|\bmake\b.{0,15}\brepo`),
	mk(GateSafety, "open firewall", `\b0\.0\.0\.0/0\b|\ballow\s+ingress\b|\bopen\s+(the\s+)?(port|firewall)\b`),
	mk(GateSafety, "disable auth", `\b(disable|remove|bypass|turn off|skip)\b.{0,15}\b(auth|authentication|2fa|mfa|security|gate)\b`),
	mk(GateSafety, "privilege", `\bsudo\b|\bmake\b.{0,10}\badmin\b|\badmin\s+access\b|\bgrant\s+admin\b|\bbucket policy\b|\bputobjectacl\b`),
	mk(GateSafety, "chmod", `\bchmod\s+-?\S*\s*[0-7]*7{2,3}\b|\bchmod\s+-R\b`),
	mk(GateSafety, "branch protection", `\bbranch\s+protection\b|\badd\s+collaborator\b|\brecovery\s+contact\b`),

	// ── Founder / business ──────────────────────────────────────────────────
	mk(GateFounder, "founder/business", `\b(term[- ]?sheet|investor|valuation|cap\s+table|equity|fundrais|pre[- ]?money|post[- ]?money|founder\s+decision|go[- ]?to[- ]?market|\bgtm\b|rebrand|press\s+release|board\s+meeting)\b`),
	mk(GateFounder, "pricing", `\bpricing\b|\bprice\s+point\b|\bdiscount\b.{0,15}\bcustomer\b`),

	// ── Irreversible / outward-facing ───────────────────────────────────────
	mk(GateIrreversible, "deploy", `\bdeploy(ing|ment)?\b|\brollout\b|\broll\s+out\b|\bpromote\b.{0,15}\bprod|\bship\b.{0,15}\b(prod|production|live|it live)\b|\bgo[- ]?live\b|\bcut(ting)?\s+(a\s+)?release\b|\btag\s+(a\s+)?release\b|\brelease\s+to\b`),
	mk(GateIrreversible, "deploy cli", `\bfirebase\s+deploy\b|\bterraform\s+apply\b|\bkubectl\s+apply\b|\bhelm\s+(install|upgrade)\b|\bdocker\s+push\b|\bnpm\s+publish\b|\bgh\s+release\b|\bgh\s+pr\s+merge\b|\bgit\s+push\s+(origin\s+)?(main|master|prod)\b`),
	mk(GateIrreversible, "merge-prod", `\bmerge\b.{0,20}\b(to\s+)?(main|master|prod|production)\b|\bmerge\s+to\s+(main|prod)\b`),
	mk(GateIrreversible, "publish", `\bpublish(ing|ed)?\b|\bpost(ing)?\s+to\b|\btweet\b|\bgo\s+public\b`),
	mk(GateIrreversible, "outward comms", `\b(send|email|slack|dm|text|sms|message|notify|contact|reply)\b.{0,25}\b(customer|client|investor|user|vendor|supplier|the email|the message)\b`),
	mk(GateIrreversible, "send email", `\bsend\b.{0,15}\bemail\b|\bemail\b.{0,15}\b(the|to)\b`),
}

// ClassifyGate returns whether an item needs the owner and why. It is the single
// authority every autonomous executor MUST consult before acting on an item.
// Deterministic and side-effect free.
func ClassifyGate(item work.Item) GateDecision {
	// Addressed to the human is an explicit escalation — the owner IS the assignee.
	switch strings.ToLower(strings.TrimSpace(item.To)) {
	case "user", "owner", "cylton", "sirsimaster", "cylton-collymore":
		return GateDecision{Gated: true, Class: GateEscalate, Reason: "item is addressed to the owner"}
	}

	// Scan title + instructions + result (result can carry woken-item content).
	hay := item.Title + "\n" + item.Instructions + "\n" + item.Result

	// Explicit escalation marker anywhere wins as an escalate gate.
	if escalateRe.MatchString(hay) {
		return GateDecision{Gated: true, Class: GateEscalate, Reason: "explicit escalate/needs-owner marker"}
	}

	for _, r := range gateRules {
		if r.re.MatchString(hay) {
			return GateDecision{Gated: true, Class: r.class, Reason: r.label}
		}
	}
	return GateDecision{Gated: false, Class: GateNone}
}

var escalateRe = regexp.MustCompile(`(?i)\b(escalate|needs? owner|owner[- ]gated|owner directive needed|ask (the )?owner|human decision)\b`)

// GatePatternCount reports how many hard-gate rules are armed — doctor/tests use
// it to prove the table is non-empty (a silently empty table would make
// everything auto-executable, a critical safety failure).
func GatePatternCount() int { return len(gateRules) }
