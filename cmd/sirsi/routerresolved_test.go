package main

import "testing"

// The safety of close-resolved rests on two rules: resolvedBy decides which
// items LOOK done, and isAcknowledgement decides the only ones that may be
// CLOSED without a human reading them. Every case below is a way the command
// could silently destroy real work.

func TestItemWithNoPRIsNeverResolved(t *testing.T) {
	// The common case in the measured queue: 35 of 49 items named no PR.
	if _, ok := resolvedBy("Router doctor manufactures a wake-dead alarm", map[int]string{}); ok {
		t.Fatal("an item naming no PR must never be resolvable — this command cannot know whether its work landed")
	}
}

func TestAllReferencedPRsMustHaveLanded(t *testing.T) {
	states := map[int]string{339: "MERGED", 340: "OPEN", 341: "MERGED"}
	refs, ok := resolvedBy("Orchestrate exact-head dispositions for PRs #341 #339 #340", states)
	if ok {
		t.Fatal("one still-OPEN PR must block — the item's work is not finished")
	}
	if len(refs) != 3 {
		t.Fatalf("expected all three refs collected, got %v", refs)
	}
}

func TestResolvesWhenEveryPRLanded(t *testing.T) {
	states := map[int]string{339: "MERGED", 340: "MERGED", 341: "CLOSED"}
	if _, ok := resolvedBy("dispositions for PRs #341 #339 #340", states); !ok {
		t.Fatal("all merged/closed must resolve")
	}
}

// An unknown number is absence of evidence, not evidence of completion.
func TestUnknownPRNumberBlocksClosure(t *testing.T) {
	if _, ok := resolvedBy("fix per PR #9999", map[int]string{339: "MERGED"}); ok {
		t.Fatal("a PR this repo has never seen must not count as landed")
	}
}

func TestOpenStatesBlockClosure(t *testing.T) {
	for _, st := range []string{"OPEN", "DRAFT", ""} {
		if _, ok := resolvedBy("blocked on PR #500", map[int]string{500: st}); ok {
			t.Fatalf("state %q must not resolve an item", st)
		}
	}
}

func TestRepeatedReferencesAreDeduped(t *testing.T) {
	refs, ok := resolvedBy("PR #401 … see #401 again … #401", map[int]string{401: "MERGED"})
	if !ok || len(refs) != 1 {
		t.Fatalf("repeated refs must dedupe to one, got %v ok=%v", refs, ok)
	}
}

// Bare numbers must not be harvested — an item mentioning a size or percentage
// would otherwise resolve against an unrelated PR.
func TestOnlyHashPrefixedNumbersCount(t *testing.T) {
	if _, ok := resolvedBy("swap at 93% used, 18432 MB total", map[int]string{93: "MERGED", 18432: "MERGED"}); ok {
		t.Fatal("bare numbers must not be treated as PR references")
	}
}

// The rule that stops this command destroying work. The first draft closed on
// PR state alone; a live dry run proposed 32 closures including items that
// referenced merged PRs while reporting NEW problems about them. Only titles
// that ANNOUNCE themselves as acknowledgements may be auto-closed.
func TestOnlyAcknowledgementsAreAutoClosable(t *testing.T) {
	for _, title := range []string{
		"ACK: PR #391 corrected head merged",
		"RESPONSE: #391 MERGED (0b9fdd9) — your bind carried it",
		"Re: binding-hold cleared",
	} {
		if !isAcknowledgement(title) {
			t.Fatalf("%q is an acknowledgement and should be auto-closable", title)
		}
	}

	// Every one of these referenced only merged PRs in the live dry run.
	for _, title := range []string{
		"thoth payload fix is merged but bypassable: binary predates it, and npm delegation skips it",
		"router doctor: 'N already-armed' is the open-ITEM count, not agents",
		"binding-hold wedge: rule on timeout-minutes vs canonizing the label toggle (PR #398 merged)",
		"PR #401 CI-green but now CONFLICTING on CHANGELOG — yours to resolve and bind",
		"Merge approved PR #398 and own binding-hold source fix",
	} {
		if isAcknowledgement(title) {
			t.Fatalf("%q raises live work and must never be auto-closed", title)
		}
	}
}

// "response"/"ack" mid-title must not qualify — only a self-announcing title.
func TestAcknowledgementMustBeAnnouncedAtTheStart(t *testing.T) {
	for _, title := range []string{
		"No response from codex on PR #401 — chase it",
		"Awaiting ack for PR #398 before proceeding",
	} {
		if isAcknowledgement(title) {
			t.Fatalf("%q merely mentions ack/response; it is not one", title)
		}
	}
}
