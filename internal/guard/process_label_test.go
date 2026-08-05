package guard

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

func TestProcessDisplayName(t *testing.T) {
	const command = "ps -p 94812 -o args="
	tests := []struct {
		name string
		proc ProcessInfo
		argv string
		want string
	}{
		{
			name: "sanctioned MLX broker names the service",
			proc: ProcessInfo{PID: 94812, Name: "Python"},
			argv: "/Library/Frameworks/Python Python ~/.sirsi/gemma-capped-server.py --model gemma",
			want: "Gemma local AI broker",
		},
		{
			name: "unrelated Python remains honest",
			proc: ProcessInfo{PID: 94812, Name: "Python"},
			argv: "/usr/bin/python3 build_docs.py",
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
			m := &platform.Mock{NameStr: "mock", CommandResults: map[string]string{command: tt.argv}}
			if got := processDisplayName(m, tt.proc); got != tt.want {
				t.Fatalf("processDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
