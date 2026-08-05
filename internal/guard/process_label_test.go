package guard

import (
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"testing"
)

func TestProcessDisplayName(t *testing.T) {
	tests := []struct {
		name string
		proc ProcessInfo
		want string
	}{
		{
			name: "native SNE names the service",
			proc: ProcessInfo{PID: 94812, Name: "sirsi-inference"},
			want: "SNE local inference",
		},
		{
			name: "Python remains honest",
			proc: ProcessInfo{PID: 94812, Name: "Python"},
			want: "Python",
		},
		{
			name: "native process avoids argv probe",
			proc: ProcessInfo{PID: 42, Name: "sirsi"},
			want: "sirsi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &platform.Mock{NameStr: "mock"}
			if got := processDisplayName(m, tt.proc); got != tt.want {
				t.Fatalf("processDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
