//go:build !darwin

package platform

// The Darwin type compiles on every platform (darwin.go is untagged), so its
// trash methods need a non-darwin counterpart or the type stops satisfying
// Platform off-macOS — which is exactly what CI lint caught. Same pattern as
// fda_darwin.go / fda_other.go.
//
// These refuse rather than no-op: a caller asking to permanently delete must
// never be told the work happened by a build that cannot do it.

// TrashContents is unavailable off macOS.
func (d *Darwin) TrashContents() ([]TrashEntry, error) { return nil, nil }

// EmptyTrash is unavailable off macOS.
func (d *Darwin) EmptyTrash([]string) ([]string, int64, error) {
	return nil, 0, errTrashUnsupported
}
