// Package platform provides an abstraction layer over OS-specific operations.
package platform

import (
	"os"
)

// Platform abstracts OS-specific operations.
// All system interactions that differ across platforms go through this interface.
type Platform interface {
	// Getenv retrieves the value of the environment variable named by the key.
	Getenv(key string) string

	// UserHomeDir returns the current user's home directory.
	UserHomeDir() (string, error)

	// Getwd returns a rooted path name corresponding to the current directory.
	Getwd() (string, error)

	// Command executes a system command and returns its combined output.
	Command(name string, args ...string) ([]byte, error)

	// Processes returns a list of running process names.
	Processes() ([]string, error)

	// Name returns the platform identifier ("darwin", "linux", "windows", "mock").
	Name() string

	// SupportsTrash returns true if the platform supports reversible trash.
	SupportsTrash() bool
	// TrashContents lists what is in the trash (read-only). Platforms without
	// a trash return nil.
	TrashContents() ([]TrashEntry, error)
	// EmptyTrash PERMANENTLY deletes the named trash entries. The only
	// operation with no undo, so implementations must enforce containment
	// themselves rather than trusting the caller.
	EmptyTrash(paths []string) (deleted []string, freed int64, err error)

	// MoveToTrash moves a file to the OS trash or recycling bin.
	MoveToTrash(path string) error

	// ProtectedPrefixes returns a list of system-wide protected directory prefixes.
	ProtectedPrefixes() []string

	// OpenBrowser opens the default browser to the given URL.
	OpenBrowser(url string) error

	// PickFolder opens a native folder picker dialog and returns the selected path.
	PickFolder() (string, error)

	// ReadDir reads the directory named by dirname and returns a list of directory entries.
	ReadDir(dirname string) ([]os.DirEntry, error)

	// Kill terminates a process by its PID (Rule A1).
	Kill(pid int) error
}
