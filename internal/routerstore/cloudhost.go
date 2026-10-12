package routerstore

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var cloudExecutorHostPattern = regexp.MustCompile(`^cloud-[a-z0-9][a-z0-9-]{0,120}$`)

func validCloudExecutorHost(host string) bool { return cloudExecutorHostPattern.MatchString(host) }

// ThreadOwnerHost is the record ownership identity, not the physical machine ID.
// Cloud clients must use the same scoped logical host for authentication and
// thread records. Physical clients retain their existing hostname semantics.
func ThreadOwnerHost() (string, error) {
	host := strings.TrimSpace(os.Getenv(EnvCloudHost))
	if host == "" {
		return os.Hostname()
	}
	if !validCloudExecutorHost(host) || strings.TrimSpace(os.Getenv(EnvUseMachineIDHost)) != "" {
		return "", fmt.Errorf("invalid or conflicting cloud executor host identity")
	}
	return host, nil
}
