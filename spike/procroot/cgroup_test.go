package procroot

import (
	"strings"
	"testing"
)

func TestCgroupHasID(t *testing.T) {
	id := strings.Repeat("ab", 32)
	cases := []struct {
		cgroup string
		want   bool
	}{
		{"0::/system.slice/docker-" + id + ".scope\n", true},
		{"0::/docker/" + id + "\n", true},
		{"0::/../docker-" + id + ".scope\n", true}, // vu depuis un espace de noms cgroup privé
		{"0::/\n", false},
		{"0::/docker/" + id[:12] + "\n", false},
	}
	for _, c := range cases {
		if got := CgroupHasID(c.cgroup, id); got != c.want {
			t.Errorf("CgroupHasID(%q) = %v, want %v", c.cgroup, got, c.want)
		}
	}
	if CgroupHasID("0::/docker/abc", "abc") {
		t.Error("a short id must never match")
	}
}
