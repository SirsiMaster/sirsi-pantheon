package fixture

import "regexp"

var nonDigit = regexp.MustCompile(`\D`)

// Class G: '-500' becomes '500' — a refund turned into a charge.
func parseAmount(amount string) string {
	return nonDigit.ReplaceAllString(amount, "") // want G
}

func parseZip(zip string) string {
	return nonDigit.ReplaceAllString(zip, "")
}
