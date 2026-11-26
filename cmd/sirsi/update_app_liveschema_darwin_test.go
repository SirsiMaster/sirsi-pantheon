//go:build darwin

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestObserveLiveSchemaWith(t *testing.T) {
	noLocal := func() (string, error) { return "", errors.New("SIRSI_ROUTER_URL is set; no local ledger of record") }
	hasLocal := func() (string, error) { return "/db", nil }
	local := func(string) (int, error) { return 27, nil }
	svc := func(n int, err error) func() (int, error) { return func() (int, error) { return n, err } }
	cases := []struct {
		name    string
		path    func() (string, error)
		service func() (int, error)
		want    int
		wantErr string
	}{
		{"local ledger keeps today's path", hasLocal, svc(0, errors.New("must not be called")), 27, ""},
		{"service-backed host reads schema from service", noLocal, svc(30, nil), 30, ""},
		{"older service lacks the method", noLocal, svc(0, errors.New("no such store method: LedgerSchemaVersion")), 0, "older service"},
		{"service unreachable", noLocal, svc(0, errors.New("dial tcp: connection refused")), 0, "connection refused"},
		{"service reports nonsense", noLocal, svc(0, nil), 0, "invalid ledger schema"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := observeLiveSchemaWith(tc.path, local, tc.service)
			if tc.wantErr == "" && (err != nil || got != tc.want) {
				t.Fatalf("got %d, %v; want %d", got, err, tc.want)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v; want containing %q", err, tc.wantErr)
			}
		})
	}
}
