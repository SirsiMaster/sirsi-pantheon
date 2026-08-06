// Package selfupdate — provenance.go
//
// Candidate provenance and version-probe gates for the serialized self-update
// flow. Both gates are HARD failures by default — no silent advisory downgrade.
//
// Provenance: the source binary must be a clean, fully-stamped release build.
// A dirty build (uncommitted changes) or an unstamped build (plain `go build`
// without -ldflags) must not propagate to other CLI install locations because
// its hash is non-reproducible and its cdhash is unpredictable after a re-sign.
//
// Version probe: the source binary must answer `<binary> version --json` with a
// parseable version.Info payload. This is a liveness check — it proves the
// binary speaks the version protocol. It does NOT check store schema
// compatibility; the real schema ceiling gate (PRAGMA user_version vs highest
// migration entry) is tracked separately as ledger task
// `selfupdate-real-schema-ceiling-gate`.
package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// Sentinel errors for provenance failures. Callers use errors.Is.
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

	// ErrSchemaIncompatible is returned before replacement when the live
	// router database is newer than the candidate binary can understand.
	ErrSchemaIncompatible = errors.New("router schema incompatible: candidate ceiling is below live schema")
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

// CheckSchemaCeiling rejects candidates that cannot open the live router
// store. A missing/zero ceiling also fails closed because pre-contract binaries
// cannot prove compatibility with a versioned production store.
func CheckSchemaCeiling(info version.Info, liveSchema int) error {
	if liveSchema < 0 {
		return fmt.Errorf("%w: invalid live schema %d", ErrSchemaIncompatible, liveSchema)
	}
	if info.RouterSchemaMax <= 0 || info.RouterSchemaMax < liveSchema {
		return fmt.Errorf("%w (live=%d candidate_max=%d)", ErrSchemaIncompatible, liveSchema, info.RouterSchemaMax)
	}
	return nil
}

// CheckVersionProbe runs `version --json` on the binary at path and returns
// its Info on success. If the binary is absent, does not respond, or returns
// an unparseable payload, the error wraps ErrVersionProbeFailed.
//
// This is a version-protocol liveness check, not a store schema compatibility
// gate. It proves the binary speaks the version protocol. The real schema
// ceiling gate (PRAGMA user_version vs highest migration entry) requires a new
// field on the probe payload and is tracked as `selfupdate-real-schema-ceiling-gate`.
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
