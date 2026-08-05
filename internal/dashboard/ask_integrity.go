package dashboard

import (
	"fmt"
	"regexp"
	"strings"
)

// Answer integrity.
//
// The first cut of /api/ask let the model write the answer prose and disclosed
// the risk in a footer. codex-pantheon ruled that insufficient on PR #494, and
// was right: a footer is disclosure AFTER false data has already rendered. The
// live test had already caught the model writing `action.runner…` for a finding
// that says `actions.runner…` — substance right, identifier wrong, and an
// identifier is precisely the part an operator would paste into a shell.
//
// So the model no longer owns any text that reaches the screen. It SELECTS
// findings by index; the server renders those findings verbatim from its own
// data. The model's one free-text field is a short framing sentence, and every
// factual-looking token in it is checked against the grounding before it is
// shown — fail closed, drop the sentence, keep the exact findings.

// isFactual reports whether a word is the kind a reader would act on: a
// measurement, or an identifier.
//
// Deliberately NOT a regex over the raw sentence — the obvious pattern also
// swallows ordinary words that merely end a sentence ("restart." contains a
// dot), and rejecting English vocabulary for absence from a diagnostic report
// would withhold every valid summary. A word qualifies only if it carries a
// digit or internal structure.
func isFactual(word string) bool {
	if word == "" {
		return false
	}
	hasDigit := strings.ContainsAny(word, "0123456789")
	// Separators strictly inside the word — a trailing period is punctuation.
	inner := strings.Trim(word, "./_-")
	hasStructure := strings.ContainsAny(inner, "./_-")

	if !hasDigit && !hasStructure {
		return false
	}
	// Bare small integers ("4 problems", "one of 3") are counting words the
	// model may legitimately compute. They carry no identity and cannot be
	// pasted into a shell.
	if hasDigit && !hasStructure && len(word) <= 2 {
		return false
	}
	return true
}

// unverifiableTokens returns the factual-looking words in summary that do not
// appear verbatim in the grounding text.
func unverifiableTokens(summary, grounding string) []string {
	lowerGround := strings.ToLower(grounding)
	var bad []string
	seen := map[string]bool{}
	for _, raw := range strings.Fields(summary) {
		t := strings.Trim(raw, ".,;:!?()[]\"'")
		if !isFactual(t) || seen[t] {
			continue
		}
		seen[t] = true
		if !strings.Contains(lowerGround, strings.ToLower(t)) {
			bad = append(bad, t)
		}
	}
	return bad
}

// controlToken matches the EXACT chat-template markers this model family emits.
// An earlier version used `<\|?[a-z_]+\|?>`, which also deleted ordinary
// lowercase placeholders like <target>, <path>, and <pid> — the very strings a
// remediation answer needs to show. The preservation test used uppercase
// <TARGET> and so never exercised the case it claimed to cover.
var controlToken = regexp.MustCompile(`<\|channel>|<channel\|>|<turn\|>|<\|turn>|<start_of_turn>|<end_of_turn>|<\|im_start\|>|<\|im_end\|>|<bos>|<eos>|<pad>`)

// cleanCompletion strips chat-template scaffolding from raw model output.
//
// The answer is whatever follows the LAST channel header. Scratch-channel-only
// or unterminated output is an ERROR, not a value: returning the scratch text
// puts the model's private reasoning on screen as though it were the response.
func cleanCompletion(s string) (string, error) {
	const chanOpen, chanClose = "<|channel>", "<channel|>"
	if i := strings.LastIndex(s, chanClose); i >= 0 {
		s = s[i+len(chanClose):]
	} else if strings.Contains(s, chanOpen) {
		return "", fmt.Errorf("local engine returned only scratch-channel output")
	}
	return strings.TrimSpace(controlToken.ReplaceAllString(s, "")), nil
}
