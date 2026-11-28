package fixture

import "strconv"

// Allowlisted: the reason is recorded on the line above the finding.
func sized(limit string) []int {
	n, _ := strconv.Atoi(limit)
	// trust-boundary: limit is clamped to serverMaxRows by validateLimit before this call
	return make([]int, n)
}
