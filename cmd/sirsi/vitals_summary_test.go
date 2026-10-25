package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func fixtureVitalsProviders(t *testing.T) {
	t.Helper()
	vitalsProvidersMu.Lock()
	mem, summary, pressure, swap := vitalsMemFn, vitalsSummaryFn, vitalsPressureFn, vitalsSwapFn
	vitalsProvidersMu.Unlock()
	t.Cleanup(func() {
		vitalsProvidersMu.Lock()
		defer vitalsProvidersMu.Unlock()
		vitalsMemFn, vitalsSummaryFn, vitalsPressureFn, vitalsSwapFn = mem, summary, pressure, swap
	})
}

func TestVitalsActualSummaryCommandSkipsRankedProvider(t *testing.T) {
	fixtureVitalsProviders(t)
	oldJSON, oldTop := JsonOutput, vitalsTop
	flag := vitalsCmd.Flags().Lookup("summary")
	oldValue, changed := flag.Value.String(), flag.Changed
	t.Cleanup(func() { JsonOutput, vitalsTop = oldJSON, oldTop; _ = flag.Value.Set(oldValue); flag.Changed = changed })
	JsonOutput, vitalsTop = true, 0 // --top0 must not turn summary into ranking.
	if err := vitalsCmd.ParseFlags([]string{"--summary"}); err != nil {
		t.Fatal(err)
	}
	vitalsProvidersMu.Lock()
	vitalsMemFn = func() (guard.MemSample, error) {
		t.Fatal("summary reached ranked producer")
		return guard.MemSample{}, nil
	}
	vitalsSummaryFn = func() (guard.MemSample, error) {
		return guard.MemSample{TotalRAM: 48 << 30, FreeBytes: 16 << 30, Top: []guard.MemProc{}}, nil
	}
	vitalsPressureFn = func(free, total int64) (guard.PressureLevel, string) {
		if free != 16<<30 || total != 48<<30 {
			t.Fatal("pressure received different capacity")
		}
		return guard.PressureWarn, "fixture-pressure"
	}
	vitalsSwapFn = func() int64 { return 512 << 20 }
	vitalsProvidersMu.Unlock()
	var runErr error
	out := captureStdout(t, func() { runErr = vitalsCmd.RunE(vitalsCmd, vitalsCmd.Flags().Args()) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	var got vitalsReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Top == nil || len(got.Top) != 0 || got.TotalBytes != 48<<30 || got.FreeBytes != 16<<30 || got.UsedBytes != 32<<30 || got.SwapUsedBytes != 512<<20 || got.Pressure != "warn" {
		t.Fatalf("summary wire: %s", out)
	}
}

func TestVitalsSummaryParityAndExplicitBrowserRetainsRankedProducer(t *testing.T) {
	fixtureVitalsProviders(t)
	mem := guard.MemSample{TotalRAM: 48 << 30, FreeBytes: 16 << 30}
	rankedCalls, summaryCalls := 0, 0
	vitalsProvidersMu.Lock()
	vitalsSummaryFn = func() (guard.MemSample, error) { summaryCalls++; return mem, nil }
	vitalsMemFn = func() (guard.MemSample, error) {
		rankedCalls++
		m := mem
		m.Top = []guard.MemProc{{Name: "/owned/fixture-worker", PID: 123, RSS: 100, Footprint: 200}}
		return m, nil
	}
	vitalsPressureFn = func(int64, int64) (guard.PressureLevel, string) { return guard.PressureNormal, "fixture-pressure" }
	vitalsSwapFn = func() int64 { return 42 }
	vitalsProvidersMu.Unlock()
	summary, err := sampleVitalsReport(true, 5)
	if err != nil {
		t.Fatal(err)
	}
	ranked, err := sampleVitalsReport(false, 5)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := collectDashboardVitals()
	if err != nil {
		t.Fatal(err)
	}
	var got vitalsReport
	if err := json.Unmarshal(browser, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, ranked) || rankedCalls != 2 || summaryCalls != 1 || len(ranked.Top) != 1 || ranked.Top[0].RSSBytes != 200 || ranked.Top[0].Name != "fixture-worker" {
		t.Fatalf("explicit ranked evidence lost: %+v calls %d/%d", got, rankedCalls, summaryCalls)
	}
	oldJSON, oldTop := JsonOutput, vitalsTop
	flag := vitalsCmd.Flags().Lookup("summary")
	oldValue, changed := flag.Value.String(), flag.Changed
	t.Cleanup(func() { JsonOutput, vitalsTop = oldJSON, oldTop; _ = flag.Value.Set(oldValue); flag.Changed = changed })
	JsonOutput, vitalsTop = true, 5
	if err := flag.Value.Set("false"); err != nil {
		t.Fatal(err)
	}
	flag.Changed = false
	if err := vitalsCmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(t, func() { runErr = vitalsCmd.RunE(vitalsCmd, vitalsCmd.Flags().Args()) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, ranked) || rankedCalls != 3 || summaryCalls != 1 {
		t.Fatalf("actual default command lost ranked evidence: %s", out)
	}
	ranked.Top = []vitalsProc{}
	if !reflect.DeepEqual(summary, ranked) {
		t.Fatalf("capacity/pressure/swap differ: %+v vs %+v", summary, ranked)
	}
}

func TestVitalsSummarySamplingErrorDoesNotFallBackToRanking(t *testing.T) {
	fixtureVitalsProviders(t)
	want := errors.New("inert summary unavailable")
	vitalsProvidersMu.Lock()
	vitalsSummaryFn = func() (guard.MemSample, error) { return guard.MemSample{}, want }
	vitalsMemFn = func() (guard.MemSample, error) {
		t.Fatal("summary failure fell back to ranking")
		return guard.MemSample{}, nil
	}
	vitalsProvidersMu.Unlock()
	if _, err := sampleVitalsReport(true, 5); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}
