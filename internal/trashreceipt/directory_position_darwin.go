//go:build darwin

package trashreceipt

import "errors"

var errStatelessBrowseUnsupported = errors.New("stateless Trash browse is unsupported: native paging requires a retained directory session; select/apply remain canonical Go operations")

// A fresh FD cannot reconstruct getattrlistbulk's per-open buffered state.
// Deliberately return no names, even for an apparently valid seek cookie.
func readDirectoryChunk(int, uint64) (directoryChunk, error) {
	return directoryChunk{}, errStatelessBrowseUnsupported
}
