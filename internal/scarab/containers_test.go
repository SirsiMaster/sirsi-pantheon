package scarab

import (
	"fmt"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

func TestAuditContainers_DockerNotRunning(t *testing.T) {
	m := &platform.Mock{
		CommandError: fmt.Errorf("docker not found"),
	}

	audit, err := AuditContainersWith(m)
	if err != nil {
		t.Fatalf("AuditContainersWith failed: %v", err)
	}
	if audit.DockerRunning {
		t.Error("Expected DockerRunning to be false")
	}
}

func TestAuditContainers_Success(t *testing.T) {
	m := &platform.Mock{
		CommandResults: map[string]string{
			"docker info": "OK",
			"docker ps -a --format {{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}": "id1\tname1\timg1\tUp 2 hours\t80:80\nid2\tname2\timg2\tExited (0)\t",
			"docker images -f dangling=true -q":                                              "img1\nimg2\n",
			// Two dangling volumes: an ordinary one, and the hedera-local consensus
			// ledger — anonymous, so identifiable ONLY by its compose label.
			"docker volume ls -f dangling=true --format {{.Name}}\t{{.Labels}}": "vol1\t\n9f3c1e77a2b4\tcom.docker.compose.project=hedera-local\n",
		},
	}

	audit, err := AuditContainersWith(m)
	if err != nil {
		t.Fatalf("AuditContainersWith failed: %v", err)
	}

	if !audit.DockerRunning {
		t.Error("Expected DockerRunning to be true")
	}
	if audit.RunningCount != 1 {
		t.Errorf("RunningCount = %d, want 1", audit.RunningCount)
	}
	if audit.StoppedCount != 1 {
		t.Errorf("StoppedCount = %d, want 1", audit.StoppedCount)
	}
	if audit.DanglingImages != 2 {
		t.Errorf("DanglingImages = %d, want 2", audit.DanglingImages)
	}
	// The ledger volume must NOT be counted as reclaimable, and must still be
	// visible as retained — an operator has to see it exists and see it was
	// protected deliberately.
	if audit.UnusedVolumes != 1 {
		t.Errorf("UnusedVolumes = %d, want 1 (the ledger volume must not be counted)", audit.UnusedVolumes)
	}
	if audit.RetainedVolumes != 1 {
		t.Errorf("RetainedVolumes = %d, want 1 (the hedera ledger volume)", audit.RetainedVolumes)
	}
	if len(audit.Containers) != 2 {
		t.Errorf("Containers count = %d, want 2", len(audit.Containers))
	}
}
