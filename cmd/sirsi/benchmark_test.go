package main

import "testing"

func TestValidateBenchmarkConfig(t *testing.T) {
	tests := []struct {
		name    string
		blocks  int
		size    int
		wantErr bool
	}{
		{name: "one byte", blocks: 1, size: 1},
		{name: "exact safety limit", blocks: 1, size: maxBenchmarkInputBytes},
		{name: "exact block count limit", blocks: maxBenchmarkBlocks, size: 1},
		{name: "nonpositive blocks", blocks: 0, size: 1, wantErr: true},
		{name: "negative blocks", blocks: -1, size: 1, wantErr: true},
		{name: "over block count limit", blocks: maxBenchmarkBlocks + 1, size: 1, wantErr: true},
		{name: "nonpositive size", blocks: 1, size: 0, wantErr: true},
		{name: "negative size", blocks: 1, size: -1, wantErr: true},
		{name: "over safety limit", blocks: 2, size: maxBenchmarkInputBytes, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBenchmarkConfig(tt.blocks, tt.size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateBenchmarkConfig(%d, %d) error = %v, wantErr %v", tt.blocks, tt.size, err, tt.wantErr)
			}
		})
	}
}
