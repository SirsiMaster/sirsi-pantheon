package fixture

const maxRows = 500

// Clean: bounds from a server constant and contiguous validated input.
func rows(values []string) []string {
	n := len(values)
	if n > maxRows {
		n = maxRows
	}
	out := make([]string, n)
	copy(out, values)
	return out
}
