package guard

import "testing"

// The real argv observed live 2026-07-30 on the owner's machine. These are the
// strings `ps -axo args` actually produces — NOT the census's Command field,
// which holds only comm ("Python", "gemma") and cannot identify a broker at all.
// An earlier version of this test fed full argv into ProcessInfo.Command and
// passed while the shipped check found nothing.
const (
	cappedArgv = "/Library/Frameworks/Python.framework/Versions/3.12/Resources/Python.app/Contents/MacOS/Python /Users/x/.sirsi/gemma-capped-server.py 22320611328 --model mlx-community/gemma-4-12B-it-8bit --host 127.0.0.1 --port 8765 --prompt-cache-bytes 4294967296"
	orphanArgv = "./gemma serve /Users/x/.cache/huggingface/hub/models--mlx-community--gemma-4-12B-it-8bit/snapshots/200bb6db :8477"
)

func TestBrokerCommandMatchesBothRealShapes(t *testing.T) {
	if !brokerCommand.MatchString(cappedArgv) {
		t.Fatal("the capped broker's real argv must be recognized")
	}
	if !brokerCommand.MatchString(orphanArgv) {
		t.Fatal("the orphan's real argv must be recognized — it matched neither the pidfile nor the port file, so argv is the only handle on it")
	}
}

// comm is what the process census exposes, and it is useless here. Pinning that
// so nobody re-points discovery at Command and reintroduces a silent check.
func TestCommIsNotEnoughToIdentifyABroker(t *testing.T) {
	for _, comm := range []string{"Python", "gemma"} {
		if brokerCommand.MatchString(comm) {
			t.Fatalf("%q must not match on its own — matching bare comm would flag every Python process", comm)
		}
	}
}

func TestDiscoversOtherServerImplementations(t *testing.T) {
	for _, argv := range []string{
		"python -m mlx_lm.server --model foo --port 9000",
		"/opt/llama.cpp/llama-server -m model.gguf --port 9001",
		"python -m vllm.entrypoints.openai.api_server --model bar",
	} {
		if !brokerCommand.MatchString(argv) {
			t.Fatalf("did not discover a broker in %q — anchoring to one implementation is how the orphan stayed invisible", argv)
		}
	}
}

func TestUnrelatedProcessesAreNotBrokers(t *testing.T) {
	for _, argv := range []string{
		"/Applications/Claude.app/Contents/MacOS/Claude",
		"com.apple.Virtualization.VirtualMachine",
		"sirsi router doctor --fix",
	} {
		if brokerCommand.MatchString(argv) {
			t.Fatalf("%q must not be classified as a model broker", argv)
		}
	}
}

func TestUncappedBrokerIsMarked(t *testing.T) {
	if capNote(orphanArgv) == "" {
		t.Fatal("the uncapped broker must be marked — nothing bounds it, which makes it the more dangerous of a duplicated pair")
	}
	if capNote(cappedArgv) != "" {
		t.Fatalf("the capped broker must not be marked uncapped, got %q", capNote(cappedArgv))
	}
}

// claude-home's review of #403: `sirsi gemma serve --stop` — the sanctioned
// graceful stop, likely running DURING an incident — matches the argv pattern.
// A CLI wrapper is tens of MB; only a GB-scale footprint can be a resident
// model, so the floor excludes it where a nonzero check could not.
func TestCLIWrapperIsNotABroker(t *testing.T) {
	if brokerFootprintFloor <= 100*1024*1024 {
		t.Fatalf("floor %d is small enough to admit a CLI wrapper", brokerFootprintFloor)
	}
	if brokerFootprintFloor > 2*(int64(1)<<30) {
		t.Fatalf("floor %d would exclude a small real model (3B 4-bit ≈ 1.5 GB)", brokerFootprintFloor)
	}
}

// Observed live 2026-08-03: omlx-server held a 23.3 GB peak footprint against
// 18 MB RSS — its entire working set in swap, 90% of the swap file — and matched
// NOTHING in brokerCommand, so the duplicate-broker check reported a clean
// machine. An enumeration of names is the weakest part of that file; these pin
// the servers we know serve models locally.
func TestBrokerCommandCoversKnownServers(t *testing.T) {
	for _, argv := range []string{
		"omlx-server",
		"/opt/homebrew/bin/mlx-server --model foo",
		"ollama serve",
		"text-generation-launcher --model-id bar",
		"python -m mlx_lm.server --model baz",
	} {
		if !brokerCommand.MatchString(argv) {
			t.Fatalf("a local model server went undiscovered: %q", argv)
		}
	}
}
