//go:build darwin

package setup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

func TestRootRelaySummaryAgeCoverageAndCountsNeverProveQueues(t *testing.T) {
	for _, name := range []string{"stale", "partial", "nonzero"} {
		t.Run(name, func(t *testing.T) {
			root := relayMetadataRoot(t, 0700)
			raw := publishRelayMetadataFixture(t, root, 0600, func(s *routerstore.RelayOutboxObservation) {
				switch name {
				case "stale":
					s.StartedAt = time.Unix(1, 0).UTC()
					s.ObservedAt = s.StartedAt
				case "partial":
					s.Complete = false
					s.Queued = nil
					s.Lanes[0].State = "unavailable"
					s.Lanes[0].Queued = nil
					s.Lanes[0].ErrorCode = "unavailable"
				case "nonzero":
					n := 7
					s.Queued = &n
					s.Lanes[0].Queued = &n
				}
			})
			before, err := os.Lstat(filepath.Join(root, rootRelaySummaryFixtureName))
			if err != nil {
				t.Fatal(err)
			}
			if err := rootRelaySpoolQuiescentAt(context.Background(), root, nil); err != nil {
				t.Fatalf("structurally valid %s metadata was confused with a queue: %v", name, err)
			}
			assertRelayMetadataUntouched(t, root, before, raw)
		})
	}
}

func TestRootRelayRootNamespaceAllowsOnlyValidatedAtomicSummaryReplacement(t *testing.T) {
	for _, name := range []string{"valid-replacement", "in-place-write", "unsafe-replacement", "summary-removed", "unknown-arrival", "new-lane", "staging-arrival", "staging-lookalike", "lane-replaced", "root-replaced", "late-queue-marker", "missing-queue-arrival"} {
		t.Run(name, func(t *testing.T) {
			root := relayMetadataRoot(t, 0700)
			publishRelayMetadataFixture(t, root, 0600, nil)
			path := filepath.Join(root, rootRelaySummaryFixtureName)
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if name == "missing-queue-arrival" {
				if err := os.Remove(filepath.Join(root, "codex-pantheon", "inflight")); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			err = rootRelaySpoolQuiescentAt(context.Background(), root, func(lane string) {
				if called {
					t.Fatal("fixture mutation was repeated")
				}
				called = true
				var err error
				switch name {
				case "valid-replacement":
					publishRelayMetadataFixture(t, root, 0600, nil)
				case "in-place-write":
					raw, readErr := os.ReadFile(path)
					if readErr != nil {
						t.Fatal(readErr)
					}
					err = os.WriteFile(path, append(raw, '\n'), 0600)
				case "unsafe-replacement":
					publishRelayMetadataFixture(t, root, 0600, func(s *routerstore.RelayOutboxObservation) { s.ProducerUID++ })
				case "summary-removed":
					err = os.Remove(path)
				case "unknown-arrival":
					err = os.WriteFile(filepath.Join(root, ".unknown.json"), []byte("retained"), 0600)
				case "new-lane":
					err = os.Mkdir(filepath.Join(root, "unexpected-lane"), 0700)
				case "staging-arrival":
					err = os.WriteFile(filepath.Join(root, ".relay-health-new-"+strings.Repeat("c", 32)), nil, 0600)
				case "staging-lookalike":
					err = os.WriteFile(filepath.Join(root, ".relay-health-new-not-a-token"), nil, 0600)
				case "lane-replaced":
					p := filepath.Join(root, lane)
					if err = os.Rename(p, filepath.Join(t.TempDir(), "retained-lane")); err == nil {
						err = os.Mkdir(p, 0700)
					}
				case "root-replaced":
					if err = os.Rename(root, filepath.Join(t.TempDir(), "retained-root")); err == nil {
						err = os.Symlink(t.TempDir(), root)
					}
				case "late-queue-marker":
					err = unix.Mkfifo(filepath.Join(root, lane, "outbox", "late.json"), 0600)
				case "missing-queue-arrival":
					err = os.Mkdir(filepath.Join(root, lane, "inflight"), 0700)
				}
				if err != nil {
					t.Fatal(err)
				}
			})
			if !called {
				t.Fatal("fixture did not reach the retained-directory seam")
			}
			if name == "valid-replacement" {
				if err != nil {
					t.Fatalf("valid producer rename remained a permanent busy condition: %v", err)
				}
				after, statErr := os.Lstat(path)
				if statErr != nil || os.SameFile(before, after) {
					t.Fatal("fixture did not replace the metadata inode")
				}
			} else if err == nil {
				t.Fatal("non-metadata namespace change escaped final verification")
			}
			if name == "staging-arrival" && !errors.Is(err, errRelayMetadataPublicationBusy) {
				t.Fatalf("publication debt lost its explicit classification: %v", err)
			}
		})
	}
}

func TestRootRelayMetadataCannotMaskSecondPassQueueArrival(t *testing.T) {
	for _, queue := range []string{"req", "outbox", "inflight"} {
		t.Run(queue, func(t *testing.T) {
			root := relayMetadataRoot(t, 0700)
			raw := publishRelayMetadataFixture(t, root, 0600, nil)
			before, err := os.Lstat(filepath.Join(root, rootRelaySummaryFixtureName))
			if err != nil {
				t.Fatal(err)
			}
			f, b := newRelayNativeFixture(t)
			b.relay.spoolClear = func(ctx context.Context) error {
				f.spoolReads++
				if f.spoolReads == 2 {
					if err := unix.Mkfifo(filepath.Join(root, "codex-pantheon", queue, "late.json"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				return rootRelaySpoolQuiescentAt(ctx, root, nil)
			}
			if err := b.Refresh(context.Background(), relayInstanceFixture()); err == nil {
				t.Fatal("late forwarding marker accepted")
			}
			if f.spoolReads != 2 || f.childReads != 3 || f.bootouts+f.bootstraps != 0 {
				t.Fatal("metadata bypassed the actual second pre-bootout probe")
			}
			assertRelayMetadataUntouched(t, root, before, raw)
			got, err := os.ReadFile(filepath.Join(root, rootRelaySummaryFixtureName))
			if err != nil || !bytes.Equal(got, raw) {
				t.Fatal("refusal changed metadata")
			}
		})
	}
}
