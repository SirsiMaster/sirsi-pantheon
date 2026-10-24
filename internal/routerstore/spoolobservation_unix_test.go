//go:build darwin || linux

package routerstore

import (
	"context"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRelayObservationExistingLoopPublicationIsNotTransportAuthority(t *testing.T) {
	for _, failPublication := range []bool{false, true} {
		t.Run(strconv.FormatBool(failPublication), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			r := relayObservationTestRoot(t)
			if failPublication {
				// The reserved leaf is invalid. The metadata path must refuse it,
				// never follow it or stop the existing discovery loop.
				if err := os.Symlink("missing-owned-fixture", filepath.Join(r.path, relayObservationName)); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			passes := 0
			rl := Relay{Spool: r.path, Base: "https://fixture.invalid", Token: "owned-fixture-token", Client: &http.Client{}}
			rl.discoverFn = func(context.Context) int {
				passes++
				if passes == 2 {
					cancel()
				}
				return 0
			}
			if err := rl.Serve(ctx); err != nil {
				t.Fatal(err)
			}
			if passes != 2 {
				t.Fatalf("projection failure stopped existing loop: %d", passes)
			}
			if !failPublication {
				if _, err := r.read(time.Now, nil); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRelayObservationNeverBlocksOnNonregularSummary(t *testing.T) {
	r := relayObservationTestRoot(t)
	if err := unix.Mkfifo(filepath.Join(r.path, relayObservationName), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.read(time.Now, nil); err == nil {
		t.Fatal("FIFO summary accepted")
	}
}

func relayObservationTestRoot(t *testing.T) *relayObservationRoot {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "relay")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	setOwnedSpoolFixtureGroupAndMode(t, root, 0o700)
	r, err := openRelayObservationRoot(root, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.close)
	return r
}

func relayObservationTestLane(t *testing.T, r *relayObservationRoot, name string) string {
	t.Helper()
	out := filepath.Join(r.path, name, "outbox")
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRelayObservationScanNoPayloadOpen(t *testing.T) {
	r := relayObservationTestRoot(t)
	out := relayObservationTestLane(t, r, "codex-pantheon")
	// Unreadable bytes and invalid JSON are intentionally irrelevant: the
	// scanner may stat names, but must never open/decode a request payload.
	held := filepath.Join(out, "owned-marker.json")
	if err := os.WriteFile(held, []byte("not-json: PRIVATE-BODY-SENTINEL"), 0000); err != nil {
		t.Fatal(err)
	}
	relayObservationTestLane(t, r, "empty")
	if err := os.Mkdir(filepath.Join(r.path, "absent"), 0700); err != nil {
		t.Fatal(err)
	}
	s := r.scan(time.Now())
	if err := s.validate(time.Now(), uint64(r.stat.Dev), uint64(r.stat.Ino), r.uid); err != nil {
		t.Fatal(err)
	}
	if !s.Complete || s.Queued == nil || *s.Queued != 1 || len(s.Lanes) != 3 {
		t.Fatalf("wrong bounded snapshot: %+v", s)
	}
	if s.Lanes[0].State != "absent" || s.Lanes[1].Agent != "codex-pantheon" || *s.Lanes[1].Queued != 1 || *s.Lanes[2].Queued != 0 {
		t.Fatalf("wrong per-lane coverage: %+v", s.Lanes)
	}
	if err := r.publish(s); err != nil {
		t.Fatal(err)
	}
	read, err := r.read(time.Now, nil)
	if err != nil || read.Queued == nil || *read.Queued != 1 {
		t.Fatalf("read=%+v err=%v", read, err)
	}
	st, err := os.Stat(held)
	if err != nil || st.Mode().Perm() != 0 {
		t.Fatalf("payload metadata changed: info=%v err=%v", st, err)
	}
	if health := CollectRelayOutboxHealth(r.path, ""); len(health.Outbox) != 3 || health.Observation == nil {
		t.Fatalf("projection=%+v", health)
	} else {
		if !health.Coverage.Complete {
			t.Fatal("complete direct inventory lost coverage")
		}
		for _, row := range health.Outbox {
			if row.Scope == nil || row.Scope.CanonicalSpool != r.path || row.Scope.RootDevice != uint64(r.stat.Dev) || row.Scope.RootInode != uint64(r.stat.Ino) {
				t.Fatalf("missing/mismatched row scope: %+v", row)
			}
		}
	}
}

func TestRelayObservationIncompleteLaneKeepsEligibleSibling(t *testing.T) {
	r := relayObservationTestRoot(t)
	out := relayObservationTestLane(t, r, "fresh")
	if err := os.WriteFile(filepath.Join(out, "held.json"), []byte("opaque"), 0000); err != nil {
		t.Fatal(err)
	}
	bad := relayObservationTestLane(t, r, "unavailable")
	if err := os.Symlink(out, filepath.Join(bad, "not-a-request.json")); err != nil {
		t.Fatal(err)
	}
	s := r.scan(time.Now())
	if s.Complete || s.Queued != nil || len(s.Lanes) != 2 || s.Lanes[0].Queued == nil || *s.Lanes[0].Queued != 1 || s.Lanes[1].State != "unavailable" || s.Lanes[1].Queued != nil {
		t.Fatalf("partial scan hid sibling or promoted sum: %+v", s)
	}
	if err := s.validate(time.Now(), uint64(r.stat.Dev), uint64(r.stat.Ino), r.uid); err != nil {
		t.Fatal(err)
	}
	health := CollectRelayOutboxHealth(r.path, "")
	if health.Coverage.Complete || len(health.Outbox) != 2 || health.Outbox[0].RetryCount == nil || *health.Outbox[0].RetryCount != 1 {
		t.Fatalf("partial inventory hidden by eligible row: %+v", health)
	}
}

func TestRelayObservationCompleteDirectCoverageDoesNotRequirePublisher(t *testing.T) {
	r := relayObservationTestRoot(t)
	relayObservationTestLane(t, r, "codex-pantheon")
	health := CollectRelayOutboxHealth(r.path, "")
	if !health.Coverage.Complete || health.Coverage.Source != "direct-metadata" || health.Coverage.StartedAt == nil || health.Observation != nil || health.ErrorCode == "" {
		t.Fatalf("publisher capability confused with inventory coverage: %+v", health)
	}
	if len(health.Outbox) != 1 || health.Outbox[0].RetryCount == nil || *health.Outbox[0].RetryCount != 0 || health.Outbox[0].Scope == nil {
		t.Fatalf("missing explicit zero or root identity: %+v", health)
	}
}

func TestRelayObservationReadRefusesSubstitutionAndStaleness(t *testing.T) {
	for _, kind := range []string{"root", "summary", "mode", "hardlink", "stale-at-completion", "future"} {
		t.Run(kind, func(t *testing.T) {
			r := relayObservationTestRoot(t)
			relayObservationTestLane(t, r, "codex-pantheon")
			s := r.scan(time.Now())
			if err := r.publish(s); err != nil {
				t.Fatal(err)
			}
			now := time.Now
			change := func() {
				switch kind {
				case "root":
					if err := os.Rename(r.path, r.path+"-retained"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(r.path, 0700); err != nil {
						t.Fatal(err)
					}
				case "summary":
					if err := os.Rename(filepath.Join(r.path, relayObservationName), filepath.Join(r.path, "old-summary")); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(r.path, relayObservationName), []byte("{}"), 0600); err != nil {
						t.Fatal(err)
					}
				case "mode":
					if err := os.Chmod(filepath.Join(r.path, relayObservationName), 0644); err != nil {
						t.Fatal(err)
					}
				case "hardlink":
					if err := os.Link(filepath.Join(r.path, relayObservationName), filepath.Join(r.path, "linked")); err != nil {
						t.Fatal(err)
					}
				case "stale-at-completion":
					now = func() time.Time { return s.StartedAt.Add(16 * time.Second) }
				case "future":
					now = func() time.Time { return s.StartedAt.Add(-3 * time.Second) }
				}
			}
			if _, err := r.read(func() time.Time { return now() }, change); err == nil {
				t.Fatal("ambiguous/stale observation accepted")
			}
		})
	}
}

func TestRelayObservationProducerIdentityNotGroupOrSelfClaim(t *testing.T) {
	lookup := func(name string) (*user.User, error) {
		if name != "_sirsipantheon" {
			t.Fatal(name)
		}
		return &user.User{Uid: "502"}, nil
	}
	for _, tc := range []struct {
		name, path          string
		owner, parent, self uint32
		bad                 bool
	}{
		{"service", "/private/var/sirsipantheon/relay", 502, 502, 501, false},
		{"group-member-is-not-producer", "/private/var/sirsipantheon/relay", 501, 501, 501, true},
		{"unknown-foreign-root", "/other/relay", 502, 502, 501, true},
		{"ordinary-private", "/owned/relay", 501, 501, 501, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := expectedRelayObservationUID(tc.path, unix.Stat_t{Uid: tc.owner, Gid: 20}, unix.Stat_t{Uid: tc.parent, Gid: 20}, int(tc.self), lookup)
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v bad=%v", err, tc.bad)
			}
		})
	}
}

func TestRelayObservationExactDeclaredGroupAndNoExistingPermissionChange(t *testing.T) {
	r := relayObservationTestRoot(t)
	if err := os.Chmod(r.path, 0770); err != nil {
		t.Fatal(err)
	}
	g, err := user.LookupGroupId(strconv.Itoa(int(r.stat.Gid)))
	if err != nil {
		t.Fatal(err)
	}
	groupRoot, err := openRelayObservationRoot(r.path, g.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer groupRoot.close()
	if err := groupRoot.publish(groupRoot.scan(time.Now())); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(r.path, relayObservationName))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0640 {
		t.Fatalf("new metadata mode=%o", st.Mode().Perm())
	}
	var actual unix.Stat_t
	if err := unix.Stat(filepath.Join(r.path, relayObservationName), &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Gid != r.stat.Gid {
		t.Fatal("metadata inherited ambient group instead of exact declaration")
	}
	if _, err := openRelayObservationRoot(r.path, ""); err == nil {
		t.Fatal("group root accepted without explicit policy")
	}
	if _, err := openRelayObservationRoot(r.path, "sirsi-fixture-nonexistent-group"); err == nil {
		t.Fatal("unknown group accepted")
	}
	if st, err := os.Stat(r.path); err != nil || st.Mode().Perm() != 0770 {
		t.Fatal("read changed existing root permissions")
	}
}

func TestRelayObservationMalformedDirectoryAndRepairPreflight(t *testing.T) {
	r := relayObservationTestRoot(t)
	out := relayObservationTestLane(t, r, "codex-pantheon")
	if err := os.Chmod(out, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(out, 0700) })
	if err := InspectSpoolOutboxRepair(r.path, "codex-pantheon"); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(out); err != nil || st.Mode().Perm() != 0 {
		t.Fatal("preflight changed permissions")
	}
	alias := filepath.Join(filepath.Dir(r.path), "alias")
	if err := os.Symlink(r.path, alias); err != nil {
		t.Fatal(err)
	}
	if err := InspectSpoolOutboxRepair(alias, "codex-pantheon"); err == nil {
		t.Fatal("unsupported symlink repair offered")
	}
	if err := os.Mkdir(filepath.Join(r.path, "bad-lane"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(r.path, "bad-lane", "outbox")); err != nil {
		t.Fatal(err)
	}
	s := r.scan(time.Now())
	for _, lane := range s.Lanes {
		if lane.Agent == "bad-lane" && lane.State != "unavailable" {
			t.Fatal("symlink directory followed")
		}
	}
	budget := relayScanBudget{deadline: time.Now().Add(-time.Second)}
	if budget.take("ordinary-lane") {
		t.Fatal("expired scan budget accepted")
	}
	_, err := r.read(time.Now, nil)
	if err == nil {
		t.Fatal("missing publisher accepted")
	}
}
