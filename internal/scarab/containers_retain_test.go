package scarab

import "testing"

// The hedera-local consensus ledger lives in an ANONYMOUS docker volume, so its
// name is a bare hash and carries no hint — the compose labels are the only
// signal. This pins that an anonymous ledger volume is never counted as
// reclaimable, while an ordinary dangling volume still is.
func TestIsRetainedVolume(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"anonymous ledger volume, identified only by compose label",
			"9f3c1e77a2b4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e\tcom.docker.compose.project=hedera-local,com.docker.compose.volume=network-node-data", true},
		{"named network-node volume", "hedera-local_network-node-data\t", true},
		{"mirror-node volume", "mirror-node-db\t", true},
		{"label case is not normalised by compose", "abc123\tcom.docker.compose.project=Hedera-Local", true},
		{"ordinary build cache is still reclaimable", "9c2f_buildkit_cache\t", false},
		{"unlabelled anonymous volume stays reclaimable", "a1b2c3d4e5f6\t", false},
	}
	for _, c := range cases {
		if got := isRetainedVolume(c.line); got != c.want {
			t.Errorf("%s: isRetainedVolume(%q) = %v, want %v", c.name, c.line, got, c.want)
		}
	}
}
