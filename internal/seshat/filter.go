package seshat

import (
	"regexp"
	"time"
)

// RedactionMode controls how matched secrets are handled.
type RedactionMode int

const (
	// RedactMask replaces matched text with [REDACTED].
	RedactMask RedactionMode = iota
	// RedactDrop removes the entire knowledge item.
	RedactDrop
)

// FilterRule defines a named pattern that detects sensitive content.
type FilterRule struct {
	Name     string
	Pattern  *regexp.Regexp
	Severity string // "critical", "high", "medium"
}

// FilterMatch records where a rule matched in content.
type FilterMatch struct {
	Rule  string
	Value string // the matched text (truncated for safety)
	Start int
	End   int
}

// SecretsFilter scans and redacts sensitive content from knowledge items.
type SecretsFilter struct {
	Rules []*FilterRule
	Mode  RedactionMode
}

// DefaultFilter returns a filter with standard secret detection patterns.
func DefaultFilter() *SecretsFilter {
	return &SecretsFilter{
		Rules: defaultRules(),
		Mode:  RedactMask,
	}
}

// Scan checks content for secrets and returns all matches.
func (f *SecretsFilter) Scan(content string) []FilterMatch {
	var matches []FilterMatch
	for _, rule := range f.Rules {
		locs := rule.Pattern.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			matched := content[loc[0]:loc[1]]
			// Truncate matched value to avoid leaking full secrets in logs.
			display := matched
			if len(display) > 12 {
				display = display[:8] + "..."
			}
			matches = append(matches, FilterMatch{
				Rule:  rule.Name,
				Value: display,
				Start: loc[0],
				End:   loc[1],
			})
		}
	}
	return matches
}

// Redact replaces all matched secrets in content with [REDACTED:<rule>].
func (f *SecretsFilter) Redact(content string) string {
	for _, rule := range f.Rules {
		content = rule.Pattern.ReplaceAllStringFunc(content, func(match string) string {
			return "[REDACTED:" + rule.Name + "]"
		})
	}
	return content
}

// FilterItems applies the secrets filter to a slice of knowledge items in place.
// Items with critical matches are dropped entirely when Mode is RedactDrop.
// Returns the count of items modified and items dropped.
func (f *SecretsFilter) FilterItems(items []KnowledgeItem) (modified, dropped int) {
	var kept []KnowledgeItem
	for i := range items {
		item := &items[i]
		titleMatches := f.Scan(item.Title)
		summaryMatches := f.Scan(item.Summary)

		if f.Mode == RedactDrop && hasCritical(titleMatches, summaryMatches) {
			dropped++
			continue
		}

		if len(titleMatches) > 0 || len(summaryMatches) > 0 {
			item.Title = f.Redact(item.Title)
			item.Summary = f.Redact(item.Summary)
			modified++
		}

		kept = append(kept, *item)
	}

	// Compact the slice in place.
	copy(items, kept)
	// Return via the counts — caller uses kept length.
	return modified, dropped
}

// FilterConversations applies the secrets filter to conversation messages.
func (f *SecretsFilter) FilterConversations(convs []Conversation) int {
	total := 0
	for i := range convs {
		for j := range convs[i].Messages {
			matches := f.Scan(convs[i].Messages[j].Content)
			if len(matches) > 0 {
				convs[i].Messages[j].Content = f.Redact(convs[i].Messages[j].Content)
				total++
			}
		}
	}
	return total
}

func hasCritical(matchSets ...[]FilterMatch) bool {
	for _, matches := range matchSets {
		for _, m := range matches {
			for _, rule := range defaultRules() {
				if rule.Name == m.Rule && rule.Severity == "critical" {
					return true
				}
			}
		}
	}
	return false
}

// defaultRules returns the built-in secret detection patterns.
func defaultRules() []*FilterRule {
	return []*FilterRule{
		// API Keys
		{
			Name:     "aws-access-key",
			Pattern:  regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			Severity: "critical",
		},
		{
			Name:     "aws-secret-key",
			Pattern:  regexp.MustCompile(`(?i)aws[_\-]?secret[_\-]?access[_\-]?key\s*[:=]\s*[A-Za-z0-9/+=]{40}`),
			Severity: "critical",
		},
		{
			Name:     "openai-api-key",
			Pattern:  regexp.MustCompile(`sk-[A-Za-z0-9]{20,}T3BlbkFJ[A-Za-z0-9]{20,}`),
			Severity: "critical",
		},
		{
			Name:     "anthropic-api-key",
			Pattern:  regexp.MustCompile(`sk-ant-[A-Za-z0-9\-]{80,}`),
			Severity: "critical",
		},
		{
			Name:     "google-api-key",
			Pattern:  regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
			Severity: "critical",
		},
		{
			Name:     "github-token",
			Pattern:  regexp.MustCompile(`(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36,}`),
			Severity: "critical",
		},
		{
			Name:     "stripe-key",
			Pattern:  regexp.MustCompile(`(?:sk|pk|rk)_(?:test|live)_[A-Za-z0-9]{24,}`),
			Severity: "critical",
		},
		{
			Name:     "slack-token",
			Pattern:  regexp.MustCompile(`xox[bposatr]-[0-9]{10,}-[A-Za-z0-9\-]+`),
			Severity: "high",
		},
		{
			Name:     "firebase-key",
			Pattern:  regexp.MustCompile(`(?i)firebase[_\-]?(?:api[_\-]?key|token)\s*[:=]\s*[A-Za-z0-9\-_]{20,}`),
			Severity: "high",
		},

		// Generic secrets
		{
			Name:     "generic-secret-assignment",
			Pattern:  regexp.MustCompile(`(?i)(?:password|passwd|secret|token|api[_\-]?key)\s*[:=]\s*["']?[A-Za-z0-9!@#$%^&*]{8,}["']?`),
			Severity: "high",
		},
		{
			Name:     "bearer-token",
			Pattern:  regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`),
			Severity: "high",
		},
		{
			Name:     "basic-auth-header",
			Pattern:  regexp.MustCompile(`(?i)basic\s+[A-Za-z0-9+/]{20,}={0,2}`),
			Severity: "high",
		},

		// Private keys
		{
			Name:     "private-key",
			Pattern:  regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
			Severity: "critical",
		},

		// Connection strings
		{
			Name:     "database-url",
			Pattern:  regexp.MustCompile(`(?i)(?:postgres|mysql|mongodb|redis|amqp)://[^\s"']+@[^\s"']+`),
			Severity: "critical",
		},

		// PII patterns
		{
			Name:     "ssn",
			Pattern:  regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			Severity: "critical",
		},
		{
			Name:     "credit-card",
			Pattern:  regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`),
			Severity: "critical",
		},
		{
			Name:     "email-address",
			Pattern:  regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
			Severity: "medium",
		},
		{
			Name:     "phone-us",
			Pattern:  regexp.MustCompile(`\b(?:\+1[\-\s]?)?\(?\d{3}\)?[\-\s]?\d{3}[\-\s]?\d{4}\b`),
			Severity: "medium",
		},

		// Environment variable leaks
		{
			Name:     "env-var-secret",
			Pattern:  regexp.MustCompile(`(?i)(?:export\s+)?[A-Z_]*(?:SECRET|TOKEN|PASSWORD|API_KEY|PRIVATE_KEY)[A-Z_]*\s*=\s*[^\s]+`),
			Severity: "high",
		},
	}
}

// FilteredSourceAdapter wraps a SourceAdapter and applies secrets filtering to ingested items.
type FilteredSourceAdapter struct {
	Adapter SourceAdapter
	Filter  *SecretsFilter
}

func (f *FilteredSourceAdapter) Name() string        { return f.Adapter.Name() }
func (f *FilteredSourceAdapter) Description() string { return f.Adapter.Description() + " (filtered)" }
func (f *FilteredSourceAdapter) Ingest(since time.Time) ([]KnowledgeItem, error) {
	items, err := f.Adapter.Ingest(since)
	if err != nil {
		return nil, err
	}
	f.Filter.FilterItems(items)
	return items, nil
}

// FilteredTargetAdapter wraps a TargetAdapter and applies secrets filtering before export.
type FilteredTargetAdapter struct {
	Adapter TargetAdapter
	Filter  *SecretsFilter
}

func (f *FilteredTargetAdapter) Name() string        { return f.Adapter.Name() }
func (f *FilteredTargetAdapter) Description() string { return f.Adapter.Description() + " (filtered)" }
func (f *FilteredTargetAdapter) Export(items []KnowledgeItem) error {
	f.Filter.FilterItems(items)
	return f.Adapter.Export(items)
}

// WrapRegistry returns a new registry where all adapters are wrapped with secrets filtering.
func WrapRegistry(reg *AdapterRegistry, filter *SecretsFilter) *AdapterRegistry {
	wrapped := NewRegistry()
	for name, src := range reg.Sources {
		wrapped.Sources[name] = &FilteredSourceAdapter{Adapter: src, Filter: filter}
	}
	for name, tgt := range reg.Targets {
		wrapped.Targets[name] = &FilteredTargetAdapter{Adapter: tgt, Filter: filter}
	}
	return wrapped
}
