// Package selfupdate — provenance.go
//
// Candidate provenance and schema-compatibility gates for the serialized
// self-update flow. Both gates are HARD failures by default — no silent
// advisory downgrade (owner/codex-home P0 acceptance contract).
//
// Provenance: the source binary must be a clean, fully-stamped release build.
// A dirty build (uncommitted changes) or an unstamped build (plain `go build`
// without -ldflags) must not propagate to other CLI install locations because
// its hash is non-reproducible and its cdhash is unpredictable after a re-sign.
//
// Schema compatibility: the source binary must respond to `<binary> version
// --json` with a valid version.Info payload. An empty or malformed response
// means the binary cannot participate in the drift-detection contract, so
// installing it would break the next drift check silently.
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

	// ErrSchemaIncompatible is returned when the source binary does not
	// respond to `version --json` with a parseable version.Info payload.
	// A binary that can't speak the schema breaks the drift-detection contract.
	ErrSchemaIncompatible = errors.New("refusing schema-incompatible binary: `version --json` did not return a valid payload")
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

// CheckSchema probes the binary at path via `version --json` and returns
// its Info on success. If the binary is absent, does not respond, or returns
// an unparseable payload the error is ErrSchemaIncompatible.
//
// This is the schema-compatibility half of the two-gate check. It is separate
// from CheckProvenance so each gate is individually testable and its failure
// is attributed correctly.
func CheckSchema(path string) (version.Info, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return version.Info{}, fmt.Errorf("%w: resolve path: %v", ErrSchemaIncompatible, err)
	}
	if _, statErr := os.Stat(abs); statErr != nil {
		return version.Info{}, fmt.Errorf("%w: binary not found at %s", ErrSchemaIncompatible, abs)
	}
	info, err := probeFn(abs)
	if err != nil {
		return version.Info{}, fmt.Errorf("%w: probe %s: %v", ErrSchemaIncompatible, abs, err)
	}
	if info.Version == "" {
		return version.Info{}, fmt.Errorf("%w: probe %s returned empty version field", ErrSchemaIncompatible, abs)
	}
	return info, nil
}
