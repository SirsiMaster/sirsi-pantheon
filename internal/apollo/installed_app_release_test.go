package apollo

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Earlier documented route IDs (2.2.0, 2.3.0, 2.3.1, 2.3.2) stay accepted as input and are always sent as the current route.
func TestInstalledAppLegacyRouteNormalized(t *testing.T) {
	for _, model := range []string{InstalledAppModel + "@apollo:2.2.0", InstalledAppModel + "@apollo:2.3.0", InstalledAppModel + "@apollo:2.3.1", InstalledAppModel + "@apollo:2.3.2", InstalledAppRoute, InstalledAppModel, "qwen3.5-9b"} {
		in := appRequestFixture()
		in.Model = model
		wire, err := appWireRequest(in)
		if err != nil {
			t.Fatalf("%s refused: %v", model, err)
		}
		if wire.Model != InstalledAppRoute {
			t.Fatalf("%s sent as %q, want %q", model, wire.Model, InstalledAppRoute)
		}
	}
	if IsInstalledAppModel(InstalledAppModel + "@apollo:2.1.0") {
		t.Fatal("an undocumented route ID must not select the installed app")
	}
}

// Since 2.3.2, Apollo serves Plain on the native front over mlx_qwen (router/qwenfront.go). Its apollo block carries extra
// fields (front, runtime, exact, timings, hashes) and an explicit null drafter; the binding must still accept it.
func TestInstalledAppAcceptsNativeFrontPlainMetadata(t *testing.T) {
	body := `{"id":"chatcmpl-x","object":"chat.completion","created":1,"model":"` + InstalledAppModel + `","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5},` +
		`"apollo":{"engine":"apollo-qwen","tier":"plain","exact":true,"front":"native","exactness":"Plain","drafter":null,"runtime":"mlx_qwen","model":"` + InstalledAppModel + `",` +
		`"prefill_s":0.1,"ttft_s":0.1,"decode_tok_s":40.5,"tokens_per_round":null,"stop_reason":"stop","wall_s":0.2,"hash_tokens":"a","hash_logprobs":"b"},` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":"fixture answer"},"finish_reason":"stop","logprobs":null}]}`
	client := &http.Client{Transport: appRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	got, err := chatInstalledApp(context.Background(), client, appRequestFixture())
	if err != nil || got.Tier != "plain" || got.Text != "fixture answer" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// Release-contract control: every pinned artifact hash must equal the contents manifest published with the
// GitHub release apollo-v2.3.9 (testdata is the read-back copy of Apollo-2.3.9-contents.sha256, sha256 8478e605…,
// itself listed in that release's SHA256SUMS). This is sourced independently of installedAppFiles.
func TestInstalledAppFilesMatchPublishedContents(t *testing.T) {
	f, err := os.Open("testdata/Apollo-2.3.9-contents.sha256")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	published := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		published[strings.TrimPrefix(fields[1], "Contents/")] = fields[0]
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for path, want := range installedAppFiles {
		if got, ok := published[path]; !ok || got != want {
			t.Fatalf("%s pinned %s, published manifest has %q", path, want, got)
		}
	}
}
