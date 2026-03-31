package maat

import (
	"runtime"
	"testing"
)

func TestGetActualPlatform(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("GetActualPlatform uses macOS sysctl — skipping on " + runtime.GOOS)
	}
	info, err := GetActualPlatform()
	if err != nil {
		t.Fatalf("Failed to get platform info: %v", err)
	}

	if info.Model == "" {
		t.Error("Actual hardware model was empty")
	}
	if info.OSVersion == "" {
		t.Error("Actual OS version was empty")
	}
}

func TestCheckPlatformIntegrity(t *testing.T) {
	tests := []struct {
		name    string
		claimed string
		want    bool // want error if incorrect
	}{
		{"Correct M1 Max Tahoe", "Measured on Apple M1 Max, macOS Tahoe (v26.3.1)", false},
		{"Incorrect M4 Max Sequoia", "Measured on Apple M4 Max, macOS Sequoia", true},
		{"Incorrect OS only", "Measured on Apple M1 Max, macOS Sequoia", true},
		{"Incorrect HW only", "Measured on Apple M4 Max, macOS Tahoe", true},
		{"Partial correct", "M1 Max Tahoe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPlatformIntegrity(tt.claimed)
			isErr := err != nil
			if isErr != tt.want {
				t.Errorf("CheckPlatformIntegrity(%q) error = %v, want error = %v", tt.claimed, err, tt.want)
			}
		})
	}
}
