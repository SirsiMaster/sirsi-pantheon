package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-inference/apollo/fleetservice"
)

func TestApolloNativeAdmissionCleanupOutcomes(t *testing.T) {
	workErr := errors.New("fixture admitted work failed")
	releaseErr := errors.New("fixture release failed")
	persistErr := errors.New("fixture metrics persistence failed")
	for _, tc := range []struct {
		name                            string
		early                           bool
		workErr, releaseErr, persistErr error
	}{
		{name: "early work error", early: true, workErr: workErr},
		{name: "work and release errors joined", workErr: workErr, releaseErr: releaseErr},
		{name: "early work and release errors joined", early: true, workErr: workErr, releaseErr: releaseErr},
		{name: "release error prevents persistence", releaseErr: releaseErr},
		{name: "metrics read then release then persist"},
		{name: "persistence failure follows release", persistErr: persistErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			measured := fleetservice.RunMetrics{InputTokens: 8, GeneratedTokens: 4, TimeToFirstTokenMS: 12.5, Completed: true}
			wantMetrics := measured
			if tc.early {
				wantMetrics = fleetservice.RunMetrics{}
			}
			var order []string
			releases, persists := 0, 0
			metrics, err := withApolloNativeAdmission(func() (fleetservice.RunMetrics, error) {
				order = append(order, "work")
				if tc.early {
					return fleetservice.RunMetrics{}, tc.workErr
				}
				order = append(order, "read metrics")
				return measured, tc.workErr
			}, func(ctx context.Context) error {
				releases++
				order = append(order, "release")
				if persists != 0 {
					t.Error("metrics persisted before release")
				}
				// Match a backend that discards its metrics during release: the
				// caller must already own the measured value returned by work.
				measured = fleetservice.RunMetrics{}
				return tc.releaseErr
			})
			persist := func(metrics fleetservice.RunMetrics) error {
				persists++
				order = append(order, "persist")
				if releases != 1 || !reflect.DeepEqual(metrics, wantMetrics) {
					t.Error("persistence preceded successful release or lost measured metrics")
				}
				return tc.persistErr
			}
			// Production also persists outside the cleanup owner, only after a
			// normal successful return. This callback uses no filesystem/SDK.
			if err == nil {
				err = persist(metrics)
			}
			if releases != 1 || !reflect.DeepEqual(metrics, wantMetrics) {
				t.Fatalf("cleanup/metrics mismatch: releases=%d metrics=%+v", releases, metrics)
			}
			wantOrder := []string{"work"}
			if !tc.early {
				wantOrder = append(wantOrder, "read metrics")
			}
			wantOrder = append(wantOrder, "release")
			wantPersists := 0
			if tc.workErr == nil && tc.releaseErr == nil {
				wantOrder = append(wantOrder, "persist")
				wantPersists = 1
			}
			if persists != wantPersists || !reflect.DeepEqual(order, wantOrder) {
				t.Fatalf("order=%v persists=%d, want %v/%d", order, persists, wantOrder, wantPersists)
			}
			for _, wantErr := range []error{tc.workErr, tc.releaseErr, tc.persistErr} {
				if wantErr != nil && !errors.Is(err, wantErr) {
					t.Errorf("result lost error %v: %v", wantErr, err)
				}
			}
			if tc.workErr == nil && tc.releaseErr == nil && tc.persistErr == nil && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApolloNativeAdmissionCleanupUsesFreshContextAfterCancellation(t *testing.T) {
	type workKey struct{}
	workContext, cancel := context.WithCancel(context.WithValue(context.Background(), workKey{}, "work-only"))
	defer cancel()
	var cleanupContext context.Context
	var workFinished time.Time
	releases := 0
	_, err := withApolloNativeAdmission(func() (fleetservice.RunMetrics, error) {
		cancel()
		workFinished = time.Now()
		return fleetservice.RunMetrics{}, workContext.Err()
	}, func(ctx context.Context) error {
		releases++
		cleanupContext = ctx
		if workContext.Err() != context.Canceled || ctx.Err() != nil || ctx.Value(workKey{}) != nil {
			t.Error("cleanup inherited the canceled work context")
		}
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(workFinished.Add(15*time.Second)) || deadline.After(time.Now().Add(15*time.Second)) {
			t.Error("cleanup did not receive a fresh fifteen-second deadline at release")
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || releases != 1 || cleanupContext == nil || cleanupContext.Err() != context.Canceled {
		t.Fatalf("cancellation cleanup mismatch: err=%v releases=%d context=%v", err, releases, cleanupContext)
	}
}

func TestApolloNativeAdmissionPanicReleasesWithoutPersistence(t *testing.T) {
	panicValue := errors.New("fixture admitted work panic")
	var recovered any
	releases, persists := 0, 0
	func() {
		defer func() { recovered = recover() }()
		_, err := withApolloNativeAdmission(func() (fleetservice.RunMetrics, error) {
			panic(panicValue)
		}, func(ctx context.Context) error {
			releases++
			if ctx.Err() != nil {
				t.Error("panic cleanup context already canceled")
			}
			return errors.New("fixture panic cleanup error")
		})
		if err == nil {
			persists++
		}
	}()
	if recovered != panicValue || releases != 1 || persists != 0 {
		t.Fatalf("panic=%v releases=%d persists=%d", recovered, releases, persists)
	}
}

func TestApolloNativeAdmissionRankCountRefusedBeforeBinding(t *testing.T) {
	for _, count := range []int{1, 3} {
		t.Run(fmt.Sprintf("admitted_%d_prepared_2", count), func(t *testing.T) {
			prepared := fleetservice.Prepared{Plan: nativeProfileStructFixture().Plan}
			original := prepared
			original.Plan.Ranks = append([]fleetservice.Rank(nil), prepared.Plan.Ranks...)
			admission := fleetservice.Admission{Ranks: make([]fleetservice.RankFence, count)}
			releases, verified := 0, false
			_, err := withApolloNativeAdmission(func() (fleetservice.RunMetrics, error) {
				if err := bindApolloNativeAdmissionPlan(&prepared, admission); err != nil {
					return fleetservice.RunMetrics{}, err
				}
				verified = true
				return fleetservice.RunMetrics{}, nil
			}, func(context.Context) error {
				releases++
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), "rank count") || releases != 1 || verified || !reflect.DeepEqual(prepared, original) {
				t.Fatalf("rank mismatch reached binding/work or escaped cleanup: err=%v releases=%d verified=%t", err, releases, verified)
			}
		})
	}
}

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
	if got.Schema != apolloNativeMetricsSchema || got.RequestID != "run_42" || got.ModelID != profile.Model.ID || !reflect.DeepEqual(got.Metrics, metrics) || strings.Join(got.HostRoute, ",") != "studio,m1" {
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
