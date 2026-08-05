package provider

import (
	"bufio"
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Conf mirrors ~/.sirsi/orchestrator.conf — the SAME file sirsi-brain.sh reads.
//
// Reusing the file rather than inventing a new one is deliberate: an existing
// configuration must keep working through this migration, and two config files
// disagreeing about which brain is active is precisely the dual-source class
// that produced the router's phantom-item P0.
type Conf struct {
	Provider string // gemma | claude | codex | gemini | openai
	Model    string
	Endpoint string
}

// LoadConf reads the orchestrator conf. A missing file is not an error — it
// means "unconfigured", and the local default must still work. Sirsi has to be
// useful on a machine where nothing has been set up, or the local premise is
// fiction.
func LoadConf(home string) Conf {
	c := Conf{}
	path := filepath.Join(home, ".sirsi", "orchestrator.conf")
	f, err := os.Open(path) //nolint:gosec // fixed path under the user's home
	if err != nil {
		return c
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "provider":
			c.Provider = v
		case "model":
			c.Model = v
		case "endpoint":
			c.Endpoint = v
		}
	}
	return c
}

// localEndpoint resolves the gemma broker's address from the port file the
// broker itself writes. Reading the port rather than assuming one is what makes
// this survive a port change — assuming 8080 is how a probe reports a healthy
// broker as dead.
func localEndpoint(home string) string {
	b, err := os.ReadFile(filepath.Join(home, ".sirsi", "gemma-server.port")) //nolint:gosec // fixed path
	if err != nil {
		return ""
	}
	port := strings.TrimSpace(string(b))
	if port == "" {
		return ""
	}
	return "http://127.0.0.1:" + port + "/v1"
}

// Local returns the on-device provider, or nil when no local broker is
// configured. Model is left empty when unset so the caller can resolve the
// served model from /v1/models rather than guessing a name — sending
// model:"default" to mlx_lm.server returns a 404 about HF_HUB_OFFLINE that
// reads like a broken broker and is not.
func Local(home string, conf Conf) *OpenAICompat {
	// RUNG ZERO IS UNCONDITIONAL. conf.Provider selects which REMOTE to prefer;
	// it must never be able to delete the local rung. The previous form only
	// built an endpoint when Provider was ""/gemma/local, so configuring ANY
	// remote provider silently dropped local out of the ladder entirely — no
	// error, no log, the rung simply vanished and Sirsi became cloud-only.
	//
	// That inverts the whole premise: the machine with a cloud key configured is
	// exactly the one whose owner still expects the zero-token offline path to
	// answer first. Local is dropped for ONE reason only — there is no local
	// broker to talk to (no port file, or a non-loopback address).
	localConf := conf.Provider == "" || conf.Provider == "gemma" || conf.Provider == "local"

	ep := localEndpoint(home)
	if localConf && isLoopbackEndpoint(conf.Endpoint) {
		ep = conf.Endpoint // explicit local override wins over the port file
	}
	if !isLoopbackEndpoint(ep) {
		return nil
	}

	// MODEL PROVENANCE. Making the rung PRESENT is not enough — it has to be
	// USABLE. conf.Model belongs to whichever provider conf names, so with
	// provider=openai and model=<a remote model>, handing that name to the local
	// broker produces a rung that exists and fails every request, then silently
	// escalates to remote. Same paid path as before the presence fix, wearing a
	// different shape (codex-pantheon, router item 20260729-191819).
	//
	// An empty model is not a gap: OpenAICompat resolves the served model from
	// /v1/models, which is the honest source — the broker names what it loaded.
	model := ""
	if localConf {
		model = conf.Model
	}
	return &OpenAICompat{
		ProviderName:           "local",
		Endpoint:               ep,
		Model:                  model,
		TierValue:              TierLocal,
		SupportsTools:          false, // mlx_lm.server has no tool-calling
		SupportsStreaming:      true,
		ContextTokens:          0,
		UseRealCompletionProbe: true, // MODEL-ROUTER-DESIGN.md: liveness = real completion, not /health
	}
}

func isLoopbackEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Ladder returns the providers to try, cheapest rung first.
//
// This is the certainties ladder in code (SIRSI_V2_APPLICATION §2c). The
// heuristic rung is not here because it is not a model — it runs before any
// provider is consulted, and most work never gets past it. What this returns is
// everything ABOVE heuristic, ordered local-then-remote so the zero-token,
// offline path is always preferred and remote is only ever an enhancement.
func Ladder(ctx context.Context, home string) []Provider {
	conf := LoadConf(home)
	var out []Provider
	if l := Local(home, conf); l != nil {
		out = append(out, l)
	}
	if r := remoteFromEnv(conf); r != nil {
		out = append(out, r)
	}
	return out
}

// remoteFromEnv builds a remote provider from an OpenAI-compatible endpoint and
// key in the environment. Absent credentials mean "no remote rung", never an
// error: a machine with no cloud access must still have a working Sirsi.
func remoteFromEnv(conf Conf) *OpenAICompat {
	key := firstNonEmpty(os.Getenv("SIRSI_REMOTE_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	ep := os.Getenv("SIRSI_REMOTE_ENDPOINT")
	if ep == "" && conf.Provider != "" && conf.Provider != "gemma" && conf.Provider != "local" {
		ep = conf.Endpoint
	}
	if key == "" || ep == "" {
		return nil
	}
	// A loopback endpoint is the LOCAL rung by definition, whatever the provider
	// is called. Labeling it TierRemote would make the ladder claim a remote,
	// paid, network-dependent rung for traffic that never leaves the machine —
	// and any caller reasoning about offline capability or cost from the tier
	// would be wrong in both directions (codex-pantheon, router item
	// 20260729-191819). Reject it rather than classify it dishonestly.
	if isLoopbackEndpoint(ep) {
		return nil
	}
	return &OpenAICompat{
		ProviderName:      "remote",
		Endpoint:          ep,
		Model:             firstNonEmpty(os.Getenv("SIRSI_REMOTE_MODEL"), conf.Model),
		APIKey:            key,
		TierValue:         TierRemote,
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsJSON:      true,
		ContextTokens:     128000,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
