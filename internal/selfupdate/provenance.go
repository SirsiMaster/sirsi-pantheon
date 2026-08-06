// Package selfupdate — provenance.go
//
// Candidate provenance and version-probe gates for the serialized self-update
// flow. All gates fail closed — no silent advisory downgrade.
//
// Provenance: the source binary must be a clean, fully-stamped release build.
// A dirty build (uncommitted changes) or an unstamped build (plain `go build`
// without -ldflags) must not propagate to other CLI install locations because
// its hash is non-reproducible and its cdhash is unpredictable after a re-sign.
//
// Version probe: the source binary must answer `<binary> version --json` with a
// parseable version.Info payload. This is a liveness check — it proves the
// binary speaks the version protocol.
//
// Schema ceiling: the live router store's PRAGMA user_version must not exceed
// the candidate binary's RouterSchemaMax. This is the P0 gate that prevents a
// downgrade from installing a binary that cannot open the already-migrated store
// (the 2026-08-06 incident: a v14 binary over a v15 store stopped every agent).
package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// Sentinel errors for gate failures. Callers use errors.Is.
var (
	// ErrDirtyBuild is returned when the source binary was built from a
	// working tree with uncommitted changes. Its hash is not reproducible.
	ErrDirtyBuild = errors.New("refusing dirty build: install a clean stamped release")

	// ErrUnstampedBuild is returned when the source binary was built without
	// -ldflags version stamping (Version == "dev" or Commit == "none").
	ErrUnstampedBuild = errors.New("refusing unstamped build: install a release build (goreleaser or go build -ldflags)")

	// ErrVersionProbeFailed is returned when the source binary does not respond
	// to `version --json` with a parseable version.Info payload. This is a
	// version-protocol liveness check, not a store schema compatibility check.
	ErrVersionProbeFailed = errors.New("version probe failed: `version --json` did not return a valid payload")

	// ErrSchemaIncompatible is returned when the live router store is at a schema
	// version the candidate binary cannot open. Installing such a binary would
	// stop every agent that tries to open the store (2026-08-06 P0 incident class).
	ErrSchemaIncompatible = errors.New("schema incompatible: candidate ceiling is below live store schema")
)

// CheckProvenance verifies that info represents a clean, stamped release
// binary. It is the provenance half of the two-gate candidate check.
//
// A dirty build means vcs.modified was true at build time — the binary
// content is non-reproducible. An unstamped build means the -ldflags version
// and commit placeholders ("dev" / "none") were not overridden — the binary
// cannot participate in version-keyed drift detection.
func CheckProvenance(info version.Info) error {
	if info.Dirty {
		return fmt.Errorf("%w (version=%s commit=%s)", ErrDirtyBuild, info.Version, info.Commit)
	}
	if info.Version == "" || info.Version == "dev" {
		return fmt.Errorf("%w (version=%q)", ErrUnstampedBuild, info.Version)
	}
	if info.Commit == "" || info.Commit == "none" {
		return fmt.Errorf("%w (commit=%q)", ErrUnstampedBuild, info.Commit)
	}
	return nil
}

// CheckVersionProbe runs `version --json` on the binary at path and returns
// its Info on success. If the binary is absent, does not respond, or returns
// an unparseable payload, the error wraps ErrVersionProbeFailed.
//
// RouterSchemaMax in the returned Info is the candidate's schema ceiling.
// A zero value means the binary predates the ceiling contract and must be
// treated as ceiling=0 (i.e. it fails CheckSchemaCeiling if the live store
// is already at any version > 0).
func CheckVersionProbe(path string) (version.Info, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return version.Info{}, fmt.Errorf("%w: resolve path: %v", ErrVersionProbeFailed, err)
	}
	if _, statErr := os.Stat(abs); statErr != nil {
		return version.Info{}, fmt.Errorf("%w: binary not found at %s", ErrVersionProbeFailed, abs)
	}
	info, err := probeFn(abs)
	if err != nil {
		return version.Info{}, fmt.Errorf("%w: probe %s: %v", ErrVersionProbeFailed, abs, err)
	}
	if info.Version == "" {
		return version.Info{}, fmt.Errorf("%w: probe %s returned empty version field", ErrVersionProbeFailed, abs)
	}
	return info, nil
}

// CheckSchemaCeiling rejects a candidate binary whose schema ceiling is below
// the live store's current schema version. Failing closed means: if the
// candidate does not declare RouterSchemaMax (zero — a pre-contract binary),
// it is rejected whenever the live store is already at any versioned schema.
//
// liveSchema is the live store's PRAGMA user_version; use ReadLiveSchemaVersion
// (routerstore package) to obtain it. A liveSchema of 0 (absent/fresh store)
// always passes — any binary can bootstrap an empty store.
func CheckSchemaCeiling(info version.Info, liveSchema int) error {
	if liveSchema <= 0 {
		return nil // fresh/absent store — any binary wins
	}
	if info.RouterSchemaMax <= 0 {
		return fmt.Errorf("%w (live=%d candidate_max=0: pre-contract binary)",
			ErrSchemaIncompatible, liveSchema)
	}
	if info.RouterSchemaMax < liveSchema {
		return fmt.Errorf("%w (live=%d candidate_max=%d)",
			ErrSchemaIncompatible, liveSchema, info.RouterSchemaMax)
	}
	return nil
}
