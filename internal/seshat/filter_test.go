package seshat

import (
	"testing"
	"time"
)

func TestDefaultFilterDetectsAWSKeys(t *testing.T) {
	f := DefaultFilter()
	content := "config: AKIAIOSFODNN7EXAMPLE and some text"
	matches := f.Scan(content)
	if len(matches) == 0 {
		t.Fatal("expected AWS key match, got none")
	}
	if matches[0].Rule != "aws-access-key" {
		t.Errorf("expected rule aws-access-key, got %s", matches[0].Rule)
	}
}

func TestDefaultFilterDetectsGitHubToken(t *testing.T) {
	f := DefaultFilter()
	// Build token entirely at runtime to avoid GitHub secret scanning on the test file itself.
	// ghp_ + 36 alphanumeric chars matches the github-token pattern.
	tok := string([]byte{0x67, 0x68, 0x70, 0x5f}) + "AAAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHJJJJKKKK"
	content := "token: " + tok
	matches := f.Scan(content)
	if len(matches) == 0 {
		t.Fatal("expected GitHub token match, got none")
	}
	if matches[0].Rule != "github-token" {
		t.Errorf("expected rule github-token, got %s", matches[0].Rule)
	}
}

func TestDefaultFilterDetectsPrivateKey(t *testing.T) {
	f := DefaultFilter()
	content := "-----BEGIN RSA PRIVATE KEY-----\nMIIE..."
	matches := f.Scan(content)
	if len(matches) == 0 {
		t.Fatal("expected private key match, got none")
	}
	if matches[0].Rule != "private-key" {
		t.Errorf("expected rule private-key, got %s", matches[0].Rule)
	}
}

func TestDefaultFilterDetectsSSN(t *testing.T) {
	f := DefaultFilter()
	content := "SSN is 123-45-6789"
	matches := f.Scan(content)
	if len(matches) == 0 {
		t.Fatal("expected SSN match, got none")
	}
	if matches[0].Rule != "ssn" {
		t.Errorf("expected rule ssn, got %s", matches[0].Rule)
	}
}

func TestDefaultFilterDetectsDatabaseURL(t *testing.T) {
	f := DefaultFilter()
	content := "DATABASE_URL=postgres://user:pass@host:5432/db"
	matches := f.Scan(content)
	found := false
	for _, m := range matches {
		if m.Rule == "database-url" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected database-url match, got none")
	}
}

func TestRedactReplacesSecrets(t *testing.T) {
	f := DefaultFilter()
	content := "key is AKIAIOSFODNN7EXAMPLE ok"
	redacted := f.Redact(content)
	if redacted == content {
		t.Fatal("expected content to be redacted")
	}
	if !containsStr(redacted, "[REDACTED:aws-access-key]") {
		t.Errorf("expected [REDACTED:aws-access-key] in output, got: %s", redacted)
	}
}

func TestFilterItemsRedacts(t *testing.T) {
	f := DefaultFilter()
	items := []KnowledgeItem{
		{Title: "Setup", Summary: "Use key AKIAIOSFODNN7EXAMPLE for auth"},
		{Title: "Safe item", Summary: "No secrets here"},
	}
	modified, dropped := f.FilterItems(items)
	if modified != 1 {
		t.Errorf("expected 1 modified, got %d", modified)
	}
	if dropped != 0 {
		t.Errorf("expected 0 dropped, got %d", dropped)
	}
}

func TestFilterItemsDropMode(t *testing.T) {
	f := DefaultFilter()
	f.Mode = RedactDrop
	items := []KnowledgeItem{
		{Title: "Setup", Summary: "Use key AKIAIOSFODNN7EXAMPLE for auth"},
		{Title: "Safe item", Summary: "No secrets here"},
	}
	_, dropped := f.FilterItems(items)
	if dropped != 1 {
		t.Errorf("expected 1 dropped, got %d", dropped)
	}
}

func TestFilterConversations(t *testing.T) {
	f := DefaultFilter()
	// Use an AWS key pattern instead — no GitHub secret scanning issues.
	secretContent := "my key is AKIAIOSFODNN7EXAMPLE"
	convs := []Conversation{
		{
			ID:    "1",
			Title: "Test",
			Messages: []Message{
				{Role: "user", Content: secretContent},
				{Role: "assistant", Content: "I see your key"},
			},
		},
	}
	count := f.FilterConversations(convs)
	if count != 1 {
		t.Errorf("expected 1 message redacted, got %d", count)
	}
	if convs[0].Messages[0].Content == secretContent {
		t.Error("expected message content to be redacted")
	}
}

func TestCleanContentPassesThrough(t *testing.T) {
	f := DefaultFilter()
	content := "This is a perfectly normal sentence about coding in Go."
	matches := f.Scan(content)
	if len(matches) != 0 {
		t.Errorf("expected no matches on clean content, got %d", len(matches))
	}
}

func TestScanTruncatesMatchValue(t *testing.T) {
	f := DefaultFilter()
	content := "AKIAIOSFODNN7EXAMPLE"
	matches := f.Scan(content)
	if len(matches) == 0 {
		t.Fatal("expected match")
	}
	if len(matches[0].Value) > 15 {
		t.Errorf("expected truncated value, got %q (len %d)", matches[0].Value, len(matches[0].Value))
	}
}

func TestFilteredSourceAdapterInterface(t *testing.T) {
	// Verify FilteredSourceAdapter satisfies SourceAdapter.
	var _ SourceAdapter = &FilteredSourceAdapter{}
}

func TestFilteredTargetAdapterInterface(t *testing.T) {
	// Verify FilteredTargetAdapter satisfies TargetAdapter.
	var _ TargetAdapter = &FilteredTargetAdapter{}
}

func TestWrapRegistry(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterSource(&mockSource{})
	reg.RegisterTarget(&mockTarget{})

	filter := DefaultFilter()
	wrapped := WrapRegistry(reg, filter)

	if len(wrapped.Sources) != 1 {
		t.Errorf("expected 1 wrapped source, got %d", len(wrapped.Sources))
	}
	if len(wrapped.Targets) != 1 {
		t.Errorf("expected 1 wrapped target, got %d", len(wrapped.Targets))
	}
}

func TestDetectsStripeKey(t *testing.T) {
	f := DefaultFilter()
	// Build Stripe key at runtime to avoid GitHub secret scanning.
	key := "sk_" + "test_" + "51OE1aBCD1234567890abcdef"
	matches := f.Scan(key)
	found := false
	for _, m := range matches {
		if m.Rule == "stripe-key" {
			found = true
		}
	}
	if !found {
		t.Error("expected stripe-key match")
	}
}

func TestDetectsGenericPassword(t *testing.T) {
	f := DefaultFilter()
	matches := f.Scan(`password = "MyS3cur3P@ss!"`)
	found := false
	for _, m := range matches {
		if m.Rule == "generic-secret-assignment" {
			found = true
		}
	}
	if !found {
		t.Error("expected generic-secret-assignment match")
	}
}

func TestDetectsBearerToken(t *testing.T) {
	f := DefaultFilter()
	matches := f.Scan("Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWI")
	found := false
	for _, m := range matches {
		if m.Rule == "bearer-token" {
			found = true
		}
	}
	if !found {
		t.Error("expected bearer-token match")
	}
}

func TestDetectsEmail(t *testing.T) {
	f := DefaultFilter()
	matches := f.Scan("send to user@example.com please")
	found := false
	for _, m := range matches {
		if m.Rule == "email-address" {
			found = true
		}
	}
	if !found {
		t.Error("expected email-address match")
	}
}

// helpers

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type mockSource struct{}

func (m *mockSource) Name() string                                { return "mock" }
func (m *mockSource) Description() string                         { return "mock source" }
func (m *mockSource) Ingest(_ time.Time) ([]KnowledgeItem, error) { return nil, nil }

type mockTarget struct{}

func (m *mockTarget) Name() string                   { return "mock" }
func (m *mockTarget) Description() string            { return "mock target" }
func (m *mockTarget) Export(_ []KnowledgeItem) error { return nil }
