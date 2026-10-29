package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
	"github.com/spf13/cobra"
)

func TestApolloInstalledAppCommandDispatch(t *testing.T) {
	oldDeps := getApolloAppDependencies()
	oldModel, oldMode, oldPrecision, oldPrompt, oldRequest, oldSession := apolloModelID, apolloMode, apolloPrecision, apolloPrompt, apolloRequestID, apolloSessionID
	oldOutput, oldStdin, oldJSON := apolloMaxOutput, apolloReadStdin, JsonOutput
	t.Cleanup(func() {
		setApolloAppDependencies(oldDeps)
		apolloModelID, apolloMode, apolloPrecision, apolloPrompt, apolloRequestID, apolloSessionID = oldModel, oldMode, oldPrecision, oldPrompt, oldRequest, oldSession
		apolloMaxOutput, apolloReadStdin, JsonOutput = oldOutput, oldStdin, oldJSON
	})
	for _, tc := range []struct {
		name, mode, precision, request string
		homeErr, chatErr               bool
		wantErr                        string
	}{
		{name: "plain", mode: "plain", precision: apollo.InstalledAppPrecision},
		{name: "flash", mode: "flash", precision: apollo.InstalledAppPrecision},
		{name: "certified", mode: "mlx-certified", precision: apollo.InstalledAppPrecision},
		{name: "no mode", precision: apollo.InstalledAppPrecision, wantErr: "explicit"},
		{name: "no precision", mode: "plain", wantErr: "explicit"},
		{name: "native session ID", mode: "plain", precision: apollo.InstalledAppPrecision, request: "native-id", wantErr: "not supported"},
		{name: "home failure", mode: "plain", precision: apollo.InstalledAppPrecision, homeErr: true, wantErr: "fixture home"},
		{name: "app failure", mode: "plain", precision: apollo.InstalledAppPrecision, chatErr: true, wantErr: "fixture app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.Flags().String("mode", "plain", "")
			cmd.Flags().String("precision", "Q8", "")
			if tc.mode != "" {
				if err := cmd.Flags().Set("mode", tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			if tc.precision != "" {
				if err := cmd.Flags().Set("precision", tc.precision); err != nil {
					t.Fatal(err)
				}
			}
			apolloModelID, apolloMode, apolloPrecision, apolloPrompt = "qwen3.5-9b", tc.mode, tc.precision, "fixture prompt"
			apolloMaxOutput, apolloReadStdin, JsonOutput = 64, false, true
			apolloRequestID, apolloSessionID = tc.request, ""
			homeFn := func() (string, error) {
				if tc.homeErr {
					return "", errors.New("fixture home")
				}
				return "/fixture", nil
			}
			calls := 0
			chatFn := func(_ context.Context, home string, in apollo.InstalledAppRequest) (apollo.InstalledAppResult, error) {
				calls++
				if tc.wantErr != "" && !tc.chatErr {
					t.Fatal("refused input reached app")
				}
				if home != "/fixture" || in.Model != "qwen3.5-9b" || in.Precision != apollo.InstalledAppPrecision || in.Mode != tc.mode || in.MaxOutputTokens != 64 {
					t.Fatalf("unbound input %+v", in)
				}
				if tc.chatErr {
					return apollo.InstalledAppResult{}, errors.New("fixture app")
				}
				return apollo.InstalledAppResult{Contract: "sirsi.apollo.installed-loopback.v1", Text: "fixture answer"}, nil
			}
			setApolloAppDependencies(apolloAppDependencies{home: homeFn, chat: chatFn})
			err := apolloChatCmd.RunE(cmd, nil)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil || calls != 1 || !strings.Contains(out.String(), "fixture answer") {
				t.Fatalf("err=%v calls=%d output=%s", err, calls, &out)
			}
			if strings.Contains(out.String(), "fences released") {
				t.Fatal("app path claimed native-fence cleanup")
			}
		})
	}
}
