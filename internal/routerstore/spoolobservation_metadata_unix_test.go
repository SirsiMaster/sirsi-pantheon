//go:build darwin || linux

package routerstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRelayReservedMetadataStructureDoesNotGrantFreshHealth(t *testing.T) {
	now := time.Now().UTC()
	for _, name := range []string{"stale", "future", "partial", "nonzero"} {
		t.Run(name, func(t *testing.T) {
			s := observationFixture(now)
			switch name {
			case "stale":
				s.StartedAt = now.Add(-time.Hour)
				s.ObservedAt = s.StartedAt
			case "future":
				s.StartedAt = now.Add(time.Hour)
				s.ObservedAt = s.StartedAt
			case "partial":
				s.Complete = false
				s.Queued = nil
				s.Lanes[1] = RelayLaneObservation{Agent: "quiet", State: "unavailable", ErrorCode: "unavailable"}
			}
			raw, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeRelayObservationMetadata(raw, 9, 10, 501); err != nil {
				t.Fatal(err)
			}
			if (name == "stale" || name == "future") && s.validate(now, 9, 10, 501) == nil {
				t.Fatal("namespace recognition weakened the existing fresh-health validator")
			}
		})
	}
}

func TestRelayReservedMetadataIndependentPolicyAndFileStat(t *testing.T) {
	lookup := func(name string) (*user.User, error) {
		if name != "_sirsipantheon" {
			t.Fatal(name)
		}
		return &user.User{Uid: "502"}, nil
	}
	root := unix.Stat_t{Mode: unix.S_IFDIR | 0770, Uid: 502, Gid: 501}
	parent := unix.Stat_t{Mode: unix.S_IFDIR | 0755, Uid: 502, Gid: 501}
	policy, err := relayObservationPolicyForRoot("/private/var/sirsipantheon/relay", root, parent, 0, lookup)
	if err != nil || policy.UID != 502 || policy.GID != 501 || policy.Mode != 0640 {
		t.Fatalf("independent deployed policy = %+v, err=%v", policy, err)
	}
	// Synthetic policy controls do not claim chown on a privileged host.
	for _, name := range []string{"valid", "owner", "group", "mode", "directory", "fifo", "device", "links", "negative-size", "oversize"} {
		t.Run(name, func(t *testing.T) {
			st := unix.Stat_t{Mode: unix.S_IFREG | 0640, Uid: 502, Gid: 501, Nlink: 1, Size: 952}
			switch name {
			case "owner":
				st.Uid = 0
			case "group":
				st.Gid++
			case "mode":
				st.Mode = unix.S_IFREG | 0644
			case "directory":
				st.Mode = unix.S_IFDIR | 0640
			case "fifo":
				st.Mode = unix.S_IFIFO | 0640
			case "device":
				st.Mode = unix.S_IFCHR | 0640
			case "links":
				st.Nlink = 2
			case "negative-size":
				st.Size = -1
			case "oversize":
				st.Size = RelayObservationMetadataLimit + 1
			}
			if (policy.validateStat(st) == nil) != (name == "valid") {
				t.Fatal("unsafe metadata stat was accepted or declared policy rejected")
			}
		})
	}
	root.Uid = 0
	if _, err := relayObservationPolicyForRoot("/private/var/sirsipantheon/relay", root, parent, 0, lookup); err == nil {
		t.Fatal("root controller privilege authenticated a foreign producer root")
	}
}

func TestRelayReservedMetadataReadRefusesRacingReplacementAndCancellation(t *testing.T) {
	for _, name := range []string{"replacement", "same-inode-write", "cancel"} {
		t.Run(name, func(t *testing.T) {
			r := relayObservationTestRoot(t)
			if err := r.publish(r.scan(time.Now())); err != nil {
				t.Fatal(err)
			}
			policy := RelayObservationFilePolicy{UID: r.uid, GID: uint32(r.gid), Mode: r.mode}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := readRelayObservationMetadataAt(ctx, r.fd, policy, func() {
				switch name {
				case "replacement":
					if err := r.publish(r.scan(time.Now())); err != nil {
						t.Fatal(err)
					}
				case "same-inode-write":
					path := filepath.Join(r.path, relayObservationName)
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
						t.Fatal(err)
					}
				case "cancel":
					cancel()
				}
			})
			if name == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			} else if !errors.Is(err, ErrRelayObservationMetadataChanged) {
				t.Fatalf("racing metadata was not classified changed: %v", err)
			}
		})
	}
}
