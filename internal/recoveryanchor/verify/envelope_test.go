package verify

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func goldenEnvelope() Envelope {
	return Envelope{
		NodeID: "node-m1", Verb: VerbRestartBroker, Target: "ai.sirsi.gemma-broker",
		ConfigDigest: strings.Repeat("a", 64), RequestID: "123e4567-e89b-42d3-a456-426614174000",
		IssuedAt: 1000000, ExpiresAt: 1000060, Epoch: 1,
	}
}

func TestEnvelopeGoldenFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/canonical-envelope-valid.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(goldenEnvelope().Canonical(), raw) {
		t.Fatalf("Canonical() differs from fixture:\n%s\nvs\n%s", goldenEnvelope().Canonical(), raw)
	}
	e, err := ParseEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if e != goldenEnvelope() {
		t.Fatalf("parsed %+v", e)
	}
}

func TestParseEnvelope(t *testing.T) {
	valid := string(goldenEnvelope().Canonical())
	sub := func(old, new string) string {
		if !strings.Contains(valid, old) {
			t.Fatalf("test bug: %q not in envelope", old)
		}
		return strings.Replace(valid, old, new, 1)
	}
	cases := []struct {
		name string
		in   string
		want Reason
	}{
		{"valid", valid, ReasonAdmitted},
		{"oversize", strings.Repeat("a", MaxEnvelopeBytes+1), ReasonEnvelopeOversize},
		{"empty", "", ReasonEnvelopeMalformed},
		{"not json", "hello\n", ReasonEnvelopeMalformed},
		{"array", "[]\n", ReasonEnvelopeMalformed},
		{"duplicate field", sub(`{"`, `{"epoch":1,"`), ReasonEnvelopeDuplicateField},
		{"unknown field", sub(`{"`, `{"extra":1,"`), ReasonEnvelopeUnknownField},
		{"unknown field beats missing", `{"extra":1}` + "\n", ReasonEnvelopeUnknownField},
		{"missing epoch", sub(`"epoch":1,`, ``), ReasonEnvelopeMissingField},
		{"missing verb", sub(`,"verb":"restart-broker"`, ``), ReasonEnvelopeMissingField},
		{"empty object", "{}\n", ReasonEnvelopeMissingField},
		{"epoch as string", sub(`"epoch":1`, `"epoch":"1"`), ReasonEnvelopeMalformed},
		{"epoch float", sub(`"epoch":1`, `"epoch":1.0`), ReasonEnvelopeMalformed},
		{"epoch exponent", sub(`"epoch":1`, `"epoch":1e0`), ReasonEnvelopeMalformed},
		{"epoch negative", sub(`"epoch":1`, `"epoch":-1`), ReasonEnvelopeMalformed},
		{"epoch zero", sub(`"epoch":1`, `"epoch":0`), ReasonEnvelopeMalformed},
		{"epoch leading zero", sub(`"epoch":1`, `"epoch":01`), ReasonEnvelopeMalformed},
		{"epoch overflow", sub(`"epoch":1`, `"epoch":99999999999999999999`), ReasonEnvelopeMalformed},
		{"epoch null", sub(`"epoch":1`, `"epoch":null`), ReasonEnvelopeMalformed},
		{"epoch bool", sub(`"epoch":1`, `"epoch":true`), ReasonEnvelopeMalformed},
		{"nested object", sub(`"epoch":1`, `"epoch":{}`), ReasonEnvelopeMalformed},
		{"nested array", sub(`"epoch":1`, `"epoch":[1]`), ReasonEnvelopeMalformed},
		{"verb as number", sub(`"verb":"restart-broker"`, `"verb":7`), ReasonEnvelopeMalformed},
		{"node_id with space", sub(`node-m1`, `node m1`), ReasonEnvelopeMalformed},
		{"target with slash", sub(`ai.sirsi.gemma-broker`, `ai/sirsi`), ReasonEnvelopeMalformed},
		{"verb uppercase", sub(`restart-broker`, `Restart-Broker`), ReasonEnvelopeMalformed},
		{"digest uppercase", sub(strings.Repeat("a", 64), strings.Repeat("A", 64)), ReasonEnvelopeMalformed},
		{"digest short", sub(strings.Repeat("a", 64), strings.Repeat("a", 63)), ReasonEnvelopeMalformed},
		{"request_id uppercase", sub(`123e4567`, `123E4567`), ReasonEnvelopeMalformed},
		{"request_id not v4", sub(`-42d3-`, `-12d3-`), ReasonEnvelopeMalformed},
		{"request_id bad variant", sub(`-a456-`, `-c456-`), ReasonEnvelopeMalformed},
		{"whitespace after colon", sub(`"epoch":1`, `"epoch": 1`), ReasonEnvelopeNonCanonical},
		{"leading space", " " + valid, ReasonEnvelopeNonCanonical},
		{"no trailing newline", strings.TrimSuffix(valid, "\n"), ReasonEnvelopeNonCanonical},
		{"extra trailing newline", valid + "\n", ReasonEnvelopeNonCanonical},
		{"CRLF", strings.TrimSuffix(valid, "\n") + "\r\n", ReasonEnvelopeNonCanonical},
		{"unicode escape", sub(`node-m1`, string(rune(92))+`u006eode-m1`), ReasonEnvelopeNonCanonical},
		{"keys out of order", `{"epoch":1,"config_digest":"` + strings.Repeat("a", 64) + `","expires_at":1000060,"issued_at":1000000,"node_id":"node-m1","request_id":"123e4567-e89b-42d3-a456-426614174000","target":"ai.sirsi.gemma-broker","verb":"restart-broker"}` + "\n", ReasonEnvelopeNonCanonical},
		{"trailing garbage", valid + "x", ReasonEnvelopeMalformed},
		{"second object", valid + valid, ReasonEnvelopeMalformed},
		{"trailing comma", sub(`"verb":"restart-broker"`, `"verb":"restart-broker",`), ReasonEnvelopeMalformed},
		{"unterminated", strings.TrimSuffix(valid, "}\n"), ReasonEnvelopeMalformed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseEnvelope([]byte(c.in))
			wantReason(t, reasonOf(err), c.want)
		})
	}
}
