package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-inference/apollo/fleetservice"
)

func TestInstalledApolloNativeProfileRejectsTrailingAndUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native-profile.json")
	old := installedApolloNativeProfilePath
	installedApolloNativeProfilePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { installedApolloNativeProfilePath = old })
	valid := nativeProfileFixture()
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadInstalledApolloNativeProfile(); err != nil {
		t.Fatalf("valid installed profile rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte(valid+" trailing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadInstalledApolloNativeProfile(); err == nil {
		t.Fatal("trailing profile data accepted")
	}
	if err := os.WriteFile(path, []byte(`{"schema":"`+apolloNativeProfileSchema+`","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadInstalledApolloNativeProfile(); err == nil {
		t.Fatal("unknown profile field accepted")
	}
}

func TestNativeProfileRequiresExactRankResourceBinding(t *testing.T) {
	profile := apolloNativeProfile{
		Schema:    apolloNativeProfileSchema,
		Model:     apolloNativeModel{ID: "gemma", CheckpointSHA256: strings.Repeat("a", 64), TokenizerSHA256: strings.Repeat("b", 64), TemplateSHA256: strings.Repeat("c", 64), Precision: "Q8", TokenizerPath: "/installed/tokenizer.json", Capacity: 128},
		Fleet:     fleetservice.Fleet{Hosts: []fleetservice.Host{{ID: "m5"}, {ID: "m1"}}},
		Plan:      fleetservice.Plan{Model: strings.Repeat("a", 64), Tokenizer: strings.Repeat("b", 64), Precision: "Q8", Context: 128, Epoch: 1, Ranks: []fleetservice.Rank{{Index: 0, Host: "m5", Budget: fleetservice.Budget{Admitted: 1}}, {Index: 1, Host: "m1", Budget: fleetservice.Budget{Admitted: 1}}}},
		Resources: map[string]string{"m5": "mem.m5"},
	}
	if err := profile.validate(); err == nil {
		t.Fatal("profile with an unbound rank resource accepted")
	}
}

func TestNativeProfileRejectsUnqualifiedContextAboveLivePrefix(t *testing.T) {
	profile := apolloNativeProfile{
		Schema:    apolloNativeProfileSchema,
		Model:     apolloNativeModel{ID: "gemma", CheckpointSHA256: strings.Repeat("a", 64), TokenizerSHA256: strings.Repeat("b", 64), TemplateSHA256: strings.Repeat("c", 64), Precision: "Q8", TokenizerPath: "/installed/tokenizer.json", Capacity: apolloNativeQualifiedContext + 1},
		Fleet:     fleetservice.Fleet{Hosts: []fleetservice.Host{{ID: "m5"}, {ID: "m1"}}},
		Plan:      fleetservice.Plan{Model: strings.Repeat("a", 64), Tokenizer: strings.Repeat("b", 64), Precision: "Q8", Context: apolloNativeQualifiedContext + 1, Epoch: 1, Ranks: []fleetservice.Rank{{Index: 0, Host: "m5", Budget: fleetservice.Budget{Admitted: 1}}, {Index: 1, Host: "m1", Budget: fleetservice.Budget{Admitted: 1}}}},
		Resources: map[string]string{"m5": "mem.m5", "m1": "mem.m1"},
	}
	if err := profile.validate(); err == nil {
		t.Fatal("unqualified context above the live prefix was accepted")
	}
}

func TestNativeProfileRejectsUnpackagedRawTransport(t *testing.T) {
	profile := nativeProfileStructFixture()
	profile.Plan.Links[0].Codec = "raw"
	if err := profile.validate(); err == nil {
		t.Fatal("raw link accepted without a packaged raw-rank artifact qualification")
	}
}

func TestNativeProfileAcceptsRawTransportWithBoundCoordinatorAndRankArtifacts(t *testing.T) {
	profile := nativeProfileStructFixture()
	profile.Plan.Links[0].Codec = "raw"
	profile.RawRankBinarySHA256 = map[string]string{
		"studio": strings.Repeat("e", 64),
		"m1":     strings.Repeat("f", 64),
	}
	profile.Plan.Ranks[0].Engine = profile.RawRankBinarySHA256["studio"]
	profile.Plan.Ranks[1].Engine = profile.RawRankBinarySHA256["m1"]
	if err := profile.validate(); err != nil {
		t.Fatalf("fully bound raw profile rejected: %v", err)
	}
}

func TestNativeProfileRejectsCoordinatorTokenizerHashMismatch(t *testing.T) {
	profile := nativeProfileStructFixture()
	profile.CoordinatorTokenizerSHA256 = strings.Repeat("a", 64)
	if err := profile.validate(); err == nil {
		t.Fatal("coordinator tokenizer with a different content identity accepted")
	}
}

func TestNativeProfileAcceptsOneLocalRankWithoutFleetTransport(t *testing.T) {
	profile := nativeProfileStructFixture()
	profile.Fleet.Hosts = profile.Fleet.Hosts[:1]
	profile.Fleet.Coordinator = nil
	profile.Plan.Ranks = profile.Plan.Ranks[:1]
	profile.Plan.Links = nil
	profile.Resources = map[string]string{"studio": "mem.studio"}
	profile.CoordinatorTokenizerSHA256 = ""
	if err := profile.validate(); err != nil {
		t.Fatalf("single local Apollo profile rejected: %v", err)
	}
}

func TestNativeProfileRejectsSingleRankFleetResidue(t *testing.T) {
	profile := nativeProfileStructFixture()
	profile.Fleet.Hosts = profile.Fleet.Hosts[:1]
	profile.Plan.Ranks = profile.Plan.Ranks[:1]
	profile.Plan.Links = nil
	profile.Resources = map[string]string{"studio": "mem.studio"}
	if err := profile.validate(); err == nil {
		t.Fatal("single local Apollo profile accepted a coordinator")
	}
	profile.Fleet.Coordinator = nil
	profile.CoordinatorTokenizerSHA256 = ""
	profile.Plan.Links = []fleetservice.Link{{From: 0, To: 0, Transport: "tcp", Codec: "raw", Profile: strings.Repeat("d", 64)}}
	if err := profile.validate(); err == nil {
		t.Fatal("single local Apollo profile accepted a transport edge")
	}
}

func TestApolloNativeChatResultJSONIncludesMeasuredMetrics(t *testing.T) {
	raw, err := json.Marshal(apolloNativeChatResult{Contract: fleetservice.Contract, Text: "ok", Metrics: fleetservice.RunMetrics{InputTokens: 4, GeneratedTokens: 3, TimeToFirstTokenMS: 12.5, DecodeTokensPerSecond: 7.5, Completed: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"contract"`, `"text"`, `"metrics"`, `"time_to_first_token_ms"`, `"decode_tokens_per_second"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("native JSON result omitted %s: %s", field, raw)
		}
	}
}

func TestPersistApolloNativeRunMetricsPublishesBoundMeasurementsOnly(t *testing.T) {
	root := t.TempDir()
	old := apolloNativeMetricsRoot
	apolloNativeMetricsRoot = func() (string, error) { return root, nil }
	t.Cleanup(func() { apolloNativeMetricsRoot = old })
	profile := nativeProfileStructFixture()
	metrics := fleetservice.RunMetrics{InputTokens: 8, GeneratedTokens: 4, TimeToFirstTokenMS: 10.5, DecodeTokensPerSecond: 9.25, Completed: true}
	if err := persistApolloNativeRunMetrics("run_42", profile, metrics); err != nil {
		t.Fatalf("persist Apollo run metrics: %v", err)
	}
	path := filepath.Join(root, "run_42", "metrics.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got apolloNativeRunMetrics
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != apolloNativeMetricsSchema || got.RequestID != "run_42" || got.ModelID != profile.Model.ID || got.Metrics != metrics || strings.Join(got.HostRoute, ",") != "studio,m1" {
		t.Fatalf("persisted metrics mismatch: %+v", got)
	}
	if err := persistApolloNativeRunMetrics("run_42", profile, metrics); err == nil {
		t.Fatal("existing metrics evidence was overwritten")
	}
	if err := persistApolloNativeRunMetrics("../unsafe", profile, metrics); err == nil {
		t.Fatal("unsafe request id accepted for metrics persistence")
	}
}

func TestApolloOpaqueIDRejectsPathCharacters(t *testing.T) {
	if _, err := apolloOpaqueID("../request"); err == nil {
		t.Fatal("unsafe Apollo id accepted")
	}
}

func nativeProfileStructFixture() apolloNativeProfile {
	return apolloNativeProfile{
		Schema:                     apolloNativeProfileSchema,
		Model:                      apolloNativeModel{ID: "gemma", CheckpointSHA256: strings.Repeat("a", 64), TokenizerSHA256: strings.Repeat("b", 64), TemplateSHA256: strings.Repeat("c", 64), Precision: "Q8", TokenizerPath: "/installed/tokenizer.json", Capacity: 128},
		Fleet:                      fleetservice.Fleet{Hosts: []fleetservice.Host{{ID: "studio", Local: true}, {ID: "m1"}}, Coordinator: &fleetservice.Coordinator{HostID: "studio", Target: "operator@studio", Binary: "/installed/apollo-fleet-agent", BinarySHA256: strings.Repeat("d", 64), TokenizerPath: "/installed/remote-tokenizer.json"}},
		Plan:                       fleetservice.Plan{Model: strings.Repeat("a", 64), Tokenizer: strings.Repeat("b", 64), Precision: "Q8", Context: 128, Epoch: 1, Ranks: []fleetservice.Rank{{Index: 0, Host: "studio", Engine: strings.Repeat("e", 64), Runtime: strings.Repeat("f", 64), Budget: fleetservice.Budget{Admitted: 1}}, {Index: 1, Host: "m1", Engine: strings.Repeat("f", 64), Runtime: strings.Repeat("e", 64), Budget: fleetservice.Budget{Admitted: 1}}}, Links: []fleetservice.Link{{From: 0, To: 1, Transport: "tcp", Codec: "mercury-cpu", Profile: strings.Repeat("d", 64)}}},
		Resources:                  map[string]string{"studio": "mem.studio", "m1": "mem.m1"},
		CoordinatorTokenizerSHA256: strings.Repeat("b", 64),
	}
}

func nativeProfileFixture() string {
	return `{"schema":"` + apolloNativeProfileSchema + `","model":{"id":"gemma","checkpoint_sha256":"` + strings.Repeat("a", 64) + `","tokenizer_sha256":"` + strings.Repeat("b", 64) + `","template_sha256":"` + strings.Repeat("c", 64) + `","precision":"Q8","tokenizer_path":"/installed/tokenizer.json","context_capacity":128},"fleet":{"hosts":[{"host_id":"studio","local":true},{"host_id":"m1"}],"coordinator":{"host_id":"studio","ssh_target":"operator@studio","binary":"/installed/apollo-fleet-agent","binary_sha256":"` + strings.Repeat("d", 64) + `","tokenizer_path":"/installed/remote-tokenizer.json"}},"plan":{"model_sha256":"` + strings.Repeat("a", 64) + `","tokenizer_sha256":"` + strings.Repeat("b", 64) + `","precision":"Q8","context_capacity":128,"epoch":1,"ranks":[{"rank":0,"host_id":"studio","engine_sha256":"` + strings.Repeat("e", 64) + `","runtime_sha256":"` + strings.Repeat("f", 64) + `","budget":{"admitted_bytes":1}},{"rank":1,"host_id":"m1","engine_sha256":"` + strings.Repeat("f", 64) + `","runtime_sha256":"` + strings.Repeat("e", 64) + `","budget":{"admitted_bytes":1}}],"links":[{"source_rank":0,"destination_rank":1,"transport":"tcp","codec":"mercury-cpu","profile_sha256":"` + strings.Repeat("d", 64) + `"}]},"maat_resources":{"studio":"mem.studio","m1":"mem.m1"},"raw_rank_binary_sha256":{},"coordinator_tokenizer_sha256":"` + strings.Repeat("b", 64) + `"}`
}
