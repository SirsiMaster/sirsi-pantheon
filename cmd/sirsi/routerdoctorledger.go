package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// The ledger arm of `sirsi router doctor`. The doctor already checks the
// THREAD fabric (armed watchers, stranded inboxes, stale records) but had no
// check on the TASK ledger, which is the other half of A36's three sources —
// so a lane could sit on rows that were terminal-in-fact and stale-in-status
// indefinitely and every surface still read healthy.
//
// Measured 2026-08-06: claude-home's registry held 76 non-done rows, 51 of
// them (67%) terminal-in-fact — bodies reading DONE/MERGED/RESOLVED while the
// status column stayed pending or in-progress. Reconciled by hand; a hand pass
// does not scale and re-rots within days.
//
// SCOPE (A35 — the check must not claim more than it reads): this reads the
// task table ONLY. It does not query GitHub, so it cannot tell you a cited PR
// merged; it reports what is locally decidable — rows whose derived liveness
// is `stalled`, and rows whose own subject asserts a terminal verdict while
// their status does not. Both are report-only: no status is auto-rewritten,
// because "the body says DONE" is a signal, never a verdict.

// terminalVerdict matches the words a lane writes into a task subject when it
// has actually finished — deliberately uppercase-anchored, because these are
// verdict shouts in practice ("PR #506 MERGED", "PREMISE DISSOLVED") and the
// lowercase words appear constantly in ordinary prose ("blocked on the merged
// branch") where they mean nothing about status.
var terminalVerdict = regexp.MustCompile(`\b(DONE|MERGED|RESOLVED|RETRACTED|TOMBSTONE|CONSOLIDATED|SUPERSEDED|PREMISE DISSOLVED)\b`)

// verdictWindow is how far into a subject the matcher looks. A lane that has
// finished LEADS with the verdict — "DONE: ADR-054 …", "PR #506 MERGED this
// pass", "DONE in verified reality: …" — while a row that merely *discusses*
// those words buries them in prose. Measured against the live ledger: an
// unwindowed matcher flagged 11 rows, of which the one false positive was a
// task whose body described the words themselves; every one of the 10 true
// positives put its verdict inside the first 48 characters.
const verdictWindow = 48

func assertsTerminalVerdict(subject string) bool {
	if len(subject) > verdictWindow {
		subject = subject[:verdictWindow]
	}
	return terminalVerdict.MatchString(subject)
}

// ledgerFinding is one row worth reporting, with the reason it was picked.
type ledgerFinding struct {
	Agent   string
	TaskID  string
	Reason  string
	Updated string
}

// findLedgerRot classifies non-done tasks. Returns stalled rows and
// contradiction rows separately — they need different remedies, so collapsing
// them into one count would tell an operator nothing actionable.
func findLedgerRot(tasks []routerstore.Task) (stalled, contradicted []ledgerFinding) {
	for _, t := range tasks {
		if t.Status == "done" {
			continue
		}
		if assertsTerminalVerdict(t.Subject) {
			contradicted = append(contradicted, ledgerFinding{t.Agent, t.TaskID, "subject asserts a terminal verdict but status=" + t.Status, t.Updated})
			continue
		}
		if t.Liveness == "stalled" {
			stalled = append(stalled, ledgerFinding{t.Agent, t.TaskID, "status=" + t.Status + ", untouched since", t.Updated})
		}
	}
	return stalled, contradicted
}

// printLedgerFindings renders one finding group. Returns 1 if it reported
// anything, so the caller can fold it into the doctor's issue count.
func printLedgerFindings(header string, findings []ledgerFinding, remedy string) int {
	if len(findings) == 0 {
		return 0
	}
	fmt.Printf("⚠ %s\n", fmt.Sprintf(header, len(findings)))
	// Group by agent so an operator sees WHOSE ledger rotted, which is the
	// unit of remediation — one lane reconciles its own rows.
	byAgent := map[string][]ledgerFinding{}
	for _, f := range findings {
		byAgent[f.Agent] = append(byAgent[f.Agent], f)
	}
	agents := make([]string, 0, len(byAgent))
	for a := range byAgent {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	for _, a := range agents {
		rows := byAgent[a]
		fmt.Printf("    %s: %d row(s)\n", a, len(rows))
		// Cap the per-agent detail, but SAY the cap — a silent truncation
		// reads as "that was all of them" (no-silent-caps).
		const show = 3
		for i, r := range rows {
			if i == show {
				fmt.Printf("        … %d more (see `sirsi router ledger %s`)\n", len(rows)-show, a)
				break
			}
			fmt.Printf("        %s\n", strings.TrimRight(fmt.Sprintf("%s  %s %s", r.TaskID, r.Reason, r.Updated), " "))
		}
	}
	fmt.Println("    → " + remedy)
	fmt.Println()
	return 1
}

// ledgerHandoff matches a row that declares its `done` is scoped to ITS OWN
// ledger rather than to the work.
//
// This is a real, deliberate pattern, not sloppiness: when the owner moves a
// workstream between lanes, the losing lane closes its copy with an explicit
// "CLOSED ON THIS LEDGER — not abandoned" and names where ownership went. The
// row is done AS A COMMITMENT OF THAT LANE while the work continues elsewhere.
//
// It has to be recognized or the check is a false-positive generator. Measured
// 2026-08-07: of the 10 ids whose statuses diverged, **8** were this exact
// pattern — the SNE set handed from claude-nexus to codex-inference on
// 2026-08-05, each carrying the marker in its subject. Reporting those as
// defects would have buried the 2 genuine unexplained splits under 8 correct
// handoffs, which is precisely how a truthful surface becomes one nobody reads.
var ledgerHandoff = regexp.MustCompile(`(?i)CLOSED ON THIS LEDGER|CLOSED HERE ONLY|OWNERSHIP (MOVED|TRANSFERRED)`)

// declaresLedgerHandoff reuses verdictWindow: a lane that is declaring scope
// LEADS with it, for the same reason a lane declaring a verdict does.
func declaresLedgerHandoff(subject string) bool {
	if len(subject) > verdictWindow {
		subject = subject[:verdictWindow]
	}
	return ledgerHandoff.MatchString(subject)
}

// findCrossAgentDivergence reports one task_id carried on more than one agent's
// ledger with DIFFERENT statuses.
//
// A shared id is not itself wrong: the fabric routinely mirrors a row onto a
// second lane so both can track it, and 14 of the 24 shared ids measured on
// 2026-08-07 agreed on status and are fine. What is wrong is the DIVERGENT
// case, and it is invisible by construction — each ledger is internally
// consistent and complete on its own terms, so no single-lane surface can see
// the split. `sirsi router ledger claude-nexus` says sne-05 is done; `sirsi
// router ledger codex-inference` says it is blocked; both are telling the truth
// about what they hold.
//
// Measured cost, 2026-08-07: 10 of 24 shared ids diverged, including sne-05,
// sne-27, sne-28, sne-29, sne-31, sne-39, sne-42 and sne-43 — all `done` on
// claude-nexus and `pending`/`blocked`/`in-progress` on codex-inference at the
// same instant. Acting on one ledger produced a routing decision that would
// have re-opened eight already-closed rows; it was caught only because the
// receiving lane cross-checked by hand before executing.
//
// Ids where a holder declares an explicit ledger-scoped handoff are suppressed:
// that row already carries its own explanation, and 8 of the 10 divergences
// measured on this host were exactly that.
//
// Report-only, and deliberately so. Which side is right is a question about the
// WORK, not about the rows — a checker that picked a winner would be guessing,
// and guessing is what produced the mis-route.
func findCrossAgentDivergence(tasks []routerstore.Task) []ledgerFinding {
	type holder struct {
		agent, status string
		handedOff     bool
	}
	byID := map[string][]holder{}
	for _, t := range tasks {
		byID[t.TaskID] = append(byID[t.TaskID], holder{t.Agent, t.Status, declaresLedgerHandoff(t.Subject)})
	}
	var out []ledgerFinding
	for id, hs := range byID {
		if len(hs) < 2 {
			continue
		}
		sort.Slice(hs, func(i, j int) bool { return hs[i].agent < hs[j].agent })
		// A row that says so IS the explanation. Suppress the whole id when any
		// holder declares a ledger-scoped close — see declaresLedgerHandoff.
		explained := false
		for _, h := range hs {
			if h.handedOff {
				explained = true
				break
			}
		}
		if explained {
			continue
		}
		diverged := false
		for _, h := range hs[1:] {
			if h.status != hs[0].status {
				diverged = true
				break
			}
		}
		if !diverged {
			continue
		}
		parts := make([]string, 0, len(hs))
		for _, h := range hs {
			parts = append(parts, h.agent+"="+h.status)
		}
		// Attributed to the FIRST holder alphabetically only so the grouped
		// output has a home; every holder is named in the reason.
		out = append(out, ledgerFinding{hs[0].agent, id, "held by " + strconv.Itoa(len(hs)) + " lanes with different statuses: " + strings.Join(parts, ", "), ""})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID < out[j].TaskID })
	return out
}

// checkLedgerRot is the doctor's ledger arm. Returns the number of issue
// groups it reported. Never fails the doctor on a read error — it says the
// ledger truth is UNKNOWN, because an unchecked ledger is not a clean one.
func checkLedgerRot() int {
	f, err := openTaskFacade()
	if err != nil {
		fmt.Printf("⚠ ledger health UNKNOWN — cannot open the task store: %v\n\n", err)
		return 1
	}
	defer f.Close()
	tasks, err := f.Store().ListTasks("")
	if err != nil {
		fmt.Printf("⚠ ledger health UNKNOWN — cannot list tasks: %v\n\n", err)
		return 1
	}
	stalled, contradicted := findLedgerRot(tasks)
	issues := printLedgerFindings(
		"%d task id(s) live on more than one lane's ledger with CONTRADICTING statuses:",
		findCrossAgentDivergence(tasks),
		"decide which lane actually owns the work, then reconcile the loser's row — until then any routing decision made from one ledger is made on half the record. Report-only: which side is right is a question about the work, not the rows.")
	issues += printLedgerFindings(
		"%d ledger row(s) whose SUBJECT asserts a terminal verdict while status is not done:",
		contradicted,
		"reconcile the status to match the body, or rewrite a body that overstates its verdict — `sirsi router task update <agent> <task-id> --status done`.")
	issues += printLedgerFindings(
		fmt.Sprintf("%%d ledger row(s) stalled — pending/in-progress and untouched for over %s:", routerstore.TaskStalledAfter),
		stalled,
		"work the row, re-state it with a fresh update, or close it. A row nobody touches is not an open commitment, it is a lie about one.")
	if issues == 0 && len(tasks) > 0 {
		fmt.Printf("ℹ ledger clean — %d task row(s), none stalled, no status/body contradictions.\n\n", len(tasks))
	}
	return issues
}
