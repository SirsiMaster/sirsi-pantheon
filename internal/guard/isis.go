package guard

import (
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/hapi"
)

// TokenizeResult contains the output of the Isis tokenization service.
type TokenizeResult struct {
	Tokens   []int  `json:"tokens"`
	Count    int    `json:"count"`
	Accel    string `json:"accelerator"`
	Duration string `json:"duration"`
}

// Tokenize processes text into tokens using the best available accelerator (ANE, GPU, or CPU).
func Tokenize(text string) (*TokenizeResult, error) {
	profile := hapi.DetectAccelerators()
	primary := profile.Primary

	// If ANE is available, use it for "Warrior" class throughput
	tokens, err := primary.Tokenize(text)
	if err != nil {
		return nil, fmt.Errorf("isis tokenize failed: %w", err)
	}

	return &TokenizeResult{
		Tokens:   tokens,
		Count:    len(tokens),
		Accel:    string(primary.Type()),
		Duration: "0ms", // TODO: Measure actual latency
	}, nil
}
