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
	// EVERY digit-bearing token is factual. There is no small-integer
	// exemption: an invented "48" or "99" is a fabricated measurement no
	// matter how few characters it takes to write. The earlier exemption went
	// unnoticed because its test used "4", which happened to be present in the
	// grounding — the case proved nothing about the exemption it justified.
	if strings.ContainsAny(word, "0123456789") {
		return true
	}
	// Separators strictly inside the word — a trailing period is punctuation.
	return strings.ContainsAny(strings.Trim(word, "./_-"), "./_-")
}

// normalizeToken strips surrounding punctuation and case so grounding and
// summary are tokenized the same way. Both sides MUST go through this, or a
// comparison is really a comparison of two different tokenizations.
func normalizeToken(word string) string {
	return strings.ToLower(strings.Trim(word, ".,;:!?()[]{}\"'"))
}

func groundingTokens(grounding string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(grounding) {
		if t := normalizeToken(w); t != "" {
			set[t] = true
		}
	}
	return set
}

// unverifiableTokens returns the factual words in summary that are not present
// in the grounding as WHOLE tokens.
//
// Whole-token equality, not substring containment — ruled by codex-pantheon on
// PR #494. Substring matching let `sne-server-macos` pass against a report that
// says `sne-server-macos-arm64`: a truncated identifier is precisely the class
// this gate exists to withhold, and the exact value is already rendered in the
// selected finding directly beneath the summary. There is no legitimate need
// for a partial identifier, because the prompt tells the model not to restate
// identifiers at all.
func unverifiableTokens(summary, grounding string) []string {
	set := groundingTokens(grounding)
	var bad []string
	seen := make(map[string]bool)
	for _, raw := range strings.Fields(summary) {
		key := normalizeToken(raw)
		if !isFactual(key) || seen[key] {
			continue
		}
		seen[key] = true
		if !set[key] {
			// Report the token AS THE MODEL WROTE IT. Echoing the normalized
			// form back would show the operator a lowercased identifier that
			// nothing actually produced — a small lie inside the message whose
			// entire job is to flag a lie.
			bad = append(bad, strings.Trim(raw, ".,;:!?()[]{}\"'"))
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
