//go:build !darwin

package trashreceipt

import "fmt"

func Preview() Report {
	return Failure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}
func Apply(Manifest, bool) Report {
	return Failure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}

func Browse(BrowseRequest) Page {
	return pageFailure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}
func Select(SelectionRequest) Report {
	return Failure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}
