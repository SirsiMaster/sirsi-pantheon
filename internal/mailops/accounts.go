package mailops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Account is one registered mailbox: a human label, its address, and where
// its OAuth token lives. No secrets live in this struct or in
// configs/mailops.yaml — only the token file path.
type Account struct {
	Label     string `yaml:"label"`
	Email     string `yaml:"email"`
	TokenPath string `yaml:"token_path"`
}

type accountsFile struct {
	Accounts []Account `yaml:"accounts"`
}

// DefaultAccountsPath returns the standard account-registry config path.
func DefaultAccountsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sirsi", "mailops", "accounts.yaml"), nil
}

// LoadAccounts reads the account registry from path.
func LoadAccounts(path string) ([]Account, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mailops: read accounts %s: %w", path, err)
	}
	var f accountsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("mailops: parse accounts %s: %w", path, err)
	}
	for i := range f.Accounts {
		f.Accounts[i].TokenPath = expandHome(f.Accounts[i].TokenPath)
	}
	return f.Accounts, nil
}

// expandHome expands a leading "~/" to the user's home directory. Falls
// back to the literal path if the home directory can't be resolved.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// Find returns the account with the given label, or an error if unregistered.
func Find(accounts []Account, label string) (Account, error) {
	for _, a := range accounts {
		if a.Label == label {
			return a, nil
		}
	}
	return Account{}, fmt.Errorf("mailops: no account registered with label %q", label)
}
