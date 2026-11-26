//go:build darwin

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

type noPendingAppUpdateStore struct{ maatAppUpdateStore }

func (noPendingAppUpdateStore) PendingAppUpdateAttempt() (maat.AppUpdateAttempt, error) {
	return maat.AppUpdateAttempt{}, maat.ErrNoIncompleteAppUpdateAttempt
}
func (noPendingAppUpdateStore) Close() error { return nil }

func TestAppUpdateFirstUseStoreInitialization(t *testing.T) {
	cases := []struct {
		name       string
		state      maat.DefaultStoreState
		initErr    error
		wantInit   int
		wantOpened bool
		wantRefuse bool
	}{
		{"uninitialized initializes and proceeds", maat.DefaultStoreUninitialized, nil, 1, true, false},
		{"uninitialized with failing init still refuses", maat.DefaultStoreUninitialized, errors.New("disk full"), 1, false, true},
		{"incomplete is never initialized", maat.DefaultStoreIncomplete, nil, 0, false, true},
		{"ready is untouched", maat.DefaultStoreReady, nil, 0, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := maatAppUpdateDependencies
			t.Cleanup(func() { maatAppUpdateDependencies = old })
			state, inits, opened := tc.state, 0, false
			maatAppUpdateDependencies = maatAppUpdateOps{
				storeStatus: func() maat.DefaultStoreStatus { return maat.DefaultStoreStatus{State: state} },
				initStore: func() (string, error) {
					inits++
					if tc.initErr == nil {
						state = maat.DefaultStoreReady
					}
					return "uuid", tc.initErr
				},
				openStore: func() (maatAppUpdateStore, error) { opened = true; return noPendingAppUpdateStore{}, nil },
			}
			_, err := reconcilePendingMaatAppUpdate()
			if inits != tc.wantInit || opened != tc.wantOpened {
				t.Fatalf("inits=%d opened=%v, want %d/%v", inits, opened, tc.wantInit, tc.wantOpened)
			}
			if tc.wantRefuse != (err != nil && strings.Contains(err.Error(), "requires the existing initialized failure-memory store")) {
				t.Fatalf("refusal mismatch: err=%v wantRefuse=%v", err, tc.wantRefuse)
			}
		})
	}
}
