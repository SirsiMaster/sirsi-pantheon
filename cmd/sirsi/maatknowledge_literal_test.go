package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestMaatKnowledgeCLILiteralSearch(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	archive, err := knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := archive.Close(); err != nil {
			t.Error(err)
		}
	})
	raw, err := os.ReadFile("../../internal/maat/knowledge/testdata/literal-search.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Records []string
		Cases   []struct {
			Name, Query string
			Want        []int
		}
		Invalid []string
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	records := make([]seshat.SourceRecord, len(fixture.Records))
	for i, body := range fixture.Records {
		records[i] = seshat.SourceRecord{Source: "claude", InstanceID: "literal-fixture", NativeItemID: fmt.Sprint(i), Schema: "fixture-v1", Title: fmt.Sprintf("Literal fixture %02d", i), Summary: "Retained reference", Content: body}
	}
	if _, err := archive.ImportSource(context.Background(), "claude", records, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	oldJSON := JsonOutput
	JsonOutput = true
	t.Cleanup(func() { JsonOutput = oldJSON })
	invoke := func(query string) (knowledge.View, error) {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		cmd.Flags().String("query", "", "")
		cmd.Flags().String("classification", "", "")
		cmd.Flags().String("resolution", "", "")
		cmd.Flags().String("source", "", "")
		cmd.Flags().String("cursor", "", "")
		cmd.Flags().Int("limit", 20, "")
		cmd.Flags().Bool("history", false, "")
		cmd.Flags().Bool("withheld", false, "")
		var output bytes.Buffer
		cmd.SetOut(&output)
		err := maatKnowledgeCmd.RunE(cmd, []string{query})
		var page knowledge.View
		if err == nil {
			err = json.Unmarshal(output.Bytes(), &page)
		}
		return page, err
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			page, err := invoke(tc.Query)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				got = append(got, item.Title)
			}
			want := make([]string, 0, len(tc.Want))
			for _, index := range tc.Want {
				want = append(want, fmt.Sprintf("Literal fixture %02d", index))
			}
			sort.Strings(got)
			sort.Strings(want)
			if page.Total != len(want) || !reflect.DeepEqual(got, want) {
				t.Fatalf("literal %q total=%d got=%v want=%v", tc.Query, page.Total, got, want)
			}
		})
	}
	for i, query := range fixture.Invalid {
		t.Run(fmt.Sprintf("invalid_%d", i), func(t *testing.T) {
			_, err := invoke(query)
			if !errors.Is(err, knowledge.ErrInvalidSearchQuery) {
				t.Fatalf("query %q: %v", query, err)
			}
		})
	}
}

func TestMaatKnowledgeCLIInvalidLiteralBeforeArchiveRead(t *testing.T) {
	home := filepath.Join(t.TempDir(), "absent-home")
	t.Setenv("HOME", home)
	cmd := &cobra.Command{}
	cmd.Flags().String("query", "", " ")
	if err := maatKnowledgeCmd.RunE(cmd, []string{"---"}); !errors.Is(err, knowledge.ErrInvalidSearchQuery) {
		t.Fatalf("invalid literal=%v", err)
	}
	if _, err := os.Lstat(home); !os.IsNotExist(err) {
		t.Fatalf("invalid literal touched archive home: %v", err)
	}
}
