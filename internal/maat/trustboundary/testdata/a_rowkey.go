package fixture

import "strconv"

// Class A: a row key supplied by the client sizes the allocation.
func buildRows(values map[string]string) []string {
	max := 0
	for k := range values {
		n, err := strconv.Atoi(k)
		if err != nil { // trust-boundary: malformed keys are rejected earlier by the schema
			continue
		}
		if n > max {
			max = n
		}
	}
	rows := make([]string, max+1) // want A
	for i := 0; i <= max; i++ {   // want A
		rows[i] = values[strconv.Itoa(i)]
	}
	return rows
}
