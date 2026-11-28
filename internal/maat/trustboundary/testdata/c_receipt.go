package fixture

// Class C: the client sends the bytes AND the hash the server checks them against.
type SignRequest struct {
	PDF     []byte `json:"pdf"`
	Receipt string `json:"receipt"` // want C
	SHA256  string `json:"hash"`    // want C
}
