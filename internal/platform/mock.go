package platform

import (
	"os"
	"strings"
)

// Mock implements Platform for testing.
// All operations are recorded and no system calls are made.
type Mock struct {
	// State for recording calls
	TrashCalls     []string
	KillCalls      []int
	OpenBrowserURL string

	// Error injection for failure-path tests (Rule A16): when set, the
	// corresponding operation still records the call, then returns the error —
	// so a test can exercise the "trash/kill failed" branch deterministically.
	TrashErr error
	KillErr  error

	// State for simulating returns
	NameStr         string
	Env             map[string]string
	HomeDir         string
	WorkDir         string
	ProcessList     []string
	PickFolderPath  string
	PickFolderError error

	// NoTrash simulates a platform without trash support (Linux, Android, iOS).
	NoTrash bool

	// Command output mapping: "cmd args..." -> output
	CommandResults map[string]string
	CommandError   error

	// ReadDir simulation: dirname -> entries
	DirEntries map[string][]os.DirEntry
}

func (m *Mock) ReadDir(dirname string) ([]os.DirEntry, error) {
	if entries, ok := m.DirEntries[dirname]; ok {
		return entries, nil
	}
	// Return empty list if directory not found in mock
	return []os.DirEntry{}, nil
}

func (m *Mock) Getenv(key string) string {
	if m.Env == nil {
		return ""
	}
	return m.Env[key]
}

func (m *Mock) UserHomeDir() (string, error) {
	return m.HomeDir, nil
}

func (m *Mock) Getwd() (string, error) {
	return m.WorkDir, nil
}

func (m *Mock) Command(name string, args ...string) ([]byte, error) {
	if m.CommandError != nil {
		return nil, m.CommandError
	}
	full := name + " " + strings.Join(args, " ")
	if res, ok := m.CommandResults[full]; ok {
		return []byte(res), nil
	}
	// Fallback to name-only match if full command not found
	if res, ok := m.CommandResults[name]; ok {
		return []byte(res), nil
	}
	return nil, nil
}

func (m *Mock) Processes() ([]string, error) {
	return m.ProcessList, nil
}

func (m *Mock) Name() string {
	if m.NameStr != "" {
		return m.NameStr
	}
	return "mock"
}

func (m *Mock) SupportsTrash() bool {
	return !m.NoTrash
}

func (m *Mock) MoveToTrash(path string) error {
	m.TrashCalls = append(m.TrashCalls, path)
	return m.TrashErr
}

func (m *Mock) ProtectedPrefixes() []string {
	return []string{
		"/System/",
		"/usr/",
		"/bin/",
	}
}

func (m *Mock) PickFolder() (string, error) {
	return m.PickFolderPath, m.PickFolderError
}

func (m *Mock) OpenBrowser(url string) error {
	m.OpenBrowserURL = url
	return nil
}

func (m *Mock) Kill(pid int) error {
	m.KillCalls = append(m.KillCalls, pid)
	return m.KillErr
}
