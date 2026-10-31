//go:build !darwin && !linux

package selfupdate

import (
	"context"
	"os"
	"path/filepath"
)

// The supported Mac/Linux scan uses the retained-descriptor byte comparator.
// Keep the previous conservative filesystem-identity behavior elsewhere.
func sameExecutable(left, right string) bool {
	return sameExecutableContext(context.Background(), left, right)
}

func sameExecutableContext(ctx context.Context, left, right string) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && leftInfo.Mode().IsRegular() &&
		rightInfo.Mode().IsRegular() && os.SameFile(leftInfo, rightInfo)
}
