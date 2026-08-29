package main

import (
	"context"
	"testing"
)

func TestInitializeResidentControlPlanePublishesBeforeGraphicalReady(t *testing.T) {
	previous := residentControlPlane
	t.Cleanup(func() { residentControlPlane = previous })
	want := &controlPlane{}

	got, err := initializeResidentControlPlane(func() (*controlPlane, error) { return want, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got != want || residentControlPlane != want {
		t.Fatal("initialized control plane was not published for the later graphical callback")
	}
}

func TestControlPlaneStopIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cp := &controlPlane{ctx: ctx, cancel: cancel}

	cp.stop()
	cp.stop()
}
