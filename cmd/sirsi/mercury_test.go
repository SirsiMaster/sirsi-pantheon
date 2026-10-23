package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mercury"
)

func TestMercuryCommandJSONIsExactSharedProjection(t *testing.T) {
	previous := JsonOutput
	JsonOutput = true
	defer func() { JsonOutput = previous }()

	want := mercury.Read{State: "stale", Message: "last sample", Headline: &mercury.Headline{Wire: mercury.Cell{Number: "12.3", Unit: "GB/s"}}}
	cmd := newMercuryCmdWithRead(func(context.Context) mercury.Read { return want })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got mercury.Read
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON projection differs from shared Read DTO:\n got %#v\nwant %#v", got, want)
	}
}

func TestMercuryCommandHumanRendersSharedCellsAndContext(t *testing.T) {
	previous := JsonOutput
	JsonOutput = false
	defer func() { JsonOutput = previous }()
	want := mercury.Read{
		State: "ok", Message: "Mercury snapshot · host",
		Headline: &mercury.Headline{
			Wire:       mercury.Cell{Number: "12.3", Unit: "GB/s", Note: "measured"},
			Compressed: mercury.Cell{Number: "18.4", Unit: "GB/s"},
			Gain:       mercury.Cell{Number: "1.50", Unit: "×"},
			Context:    "M1 → M2", RunStatus: "Leg 1 of 1",
		},
	}
	cmd := newMercuryCmdWithRead(func(context.Context) mercury.Read { return want })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Mercury: ok", "Wire: 12.3 GB/s · measured", "Compressed: 18.4 GB/s", "Gain: 1.50 ×", "M1 → M2", "Leg 1 of 1"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("human output missing %q: %s", expected, out.String())
		}
	}
}

func TestMercuryCommandRejectsPositionals(t *testing.T) {
	called := false
	cmd := newMercuryCmdWithRead(func(context.Context) mercury.Read {
		called = true
		return mercury.Read{State: "unexpected"}
	})
	cmd.SetArgs([]string{"ignored"})
	if err := cmd.Execute(); err == nil || (!strings.Contains(err.Error(), "accepts") && !strings.Contains(err.Error(), "unknown command")) {
		t.Fatalf("expected positional argument refusal, got %v", err)
	}
	if called {
		t.Fatal("positional argument refusal happened after a Mercury read")
	}
}
