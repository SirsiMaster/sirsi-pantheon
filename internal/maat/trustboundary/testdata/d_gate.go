package fixture

// trust-boundary-gate: RequireDeliveryReview, AllowedRedirect

import "net/http"

func RequireDeliveryReview(r *http.Request) bool { return true }
func AllowedRedirect(u string) bool              { return true }

func sendHandler(w http.ResponseWriter, r *http.Request) {
	if !RequireDeliveryReview(r) || !AllowedRedirect(r.URL.Query().Get("next")) {
		return
	}
}

// Class D: the new signing path skips both sibling gates.
func signHandler(w http.ResponseWriter, r *http.Request) { // want D
	_ = r
}
