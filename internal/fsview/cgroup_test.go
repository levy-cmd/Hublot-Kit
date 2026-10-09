package fsview

import (
	"errors"
	"strings"
	"testing"
)

func TestCgroupHasID(t *testing.T) {
	id := strings.Repeat("ab", 32)
	other := strings.Repeat("cd", 32)
	cases := []struct {
		name, cgroup string
		want         bool
	}{
		{"systemd", "0::/system.slice/docker-" + id + ".scope\n", true},
		{"cgroupfs", "0::/docker/" + id + "\n", true},
		{"docker:dind child", "0::/system.slice/docker-" + id + ".scope/init\n", true},
		{"cgroupfs child", "0::/docker/" + id + "/child\n", true},
		{"systemd with --cgroup-parent", "0::/custom.slice/docker-" + id + ".scope\n", true},
		{"namespace root", "0::/\n", false},
		{"empty", "", false},
		{"short id", "0::/docker/" + id[:12] + "\n", false},
		{"substring in a scope", "0::/system.slice/not" + id + "evil.scope\n", false},
		{"id followed by junk", "0::/docker/" + id + "x\n", false},
		{"scope followed by junk", "0::/system.slice/docker-" + id + ".scopex\n", false},
		{"uppercase id", "0::/docker/" + strings.ToUpper(id) + "\n", false},
		{"dotdot cgroupfs", "0::/../" + id + "\n", false},
		{"dotdot systemd", "0::/../docker-" + id + ".scope\n", false},
		{"dotdot after the anchor", "0::/docker/" + id + "/..\n", false},
		{"nested scope, other outer", "0::/system.slice/docker-" + other + ".scope/docker-" + id + ".scope\n", false},
		{"nested cgroupfs, other outer", "0::/docker/" + other + "/docker/" + id + "\n", false},
		{"cgroupfs with --cgroup-parent (v0.1 limit)", "0::/myparent/" + id + "\n", false},
		{"cgroup v1, every controller", "12:pids:/docker/" + id + "\n11:memory:/docker/" + id + "\n0::/docker/" + id + "\n", true},
		{"cgroup v1, a controller at /", "12:rdma:/\n11:memory:/docker/" + id + "\n", true},
		{"cgroup v1, conflicting lines", "12:pids:/docker/" + other + "\n11:memory:/docker/" + id + "\n", false},
		{"malformed line", "garbage\n0::/docker/" + id + "\n", false},
	}
	for _, c := range cases {
		if got := CgroupHasID(c.cgroup, id); got != c.want {
			t.Errorf("%s: CgroupHasID(%q) = %v, want %v", c.name, c.cgroup, got, c.want)
		}
	}
	if !CgroupHasID("0::/system.slice/docker-"+other+".scope/docker-"+id+".scope\n", other) {
		t.Error("the outer container must still match its own nested cgroups")
	}
	if CgroupHasID("0::/docker/abc\n", "abc") {
		t.Error("a short id must never match")
	}
}

func TestChangedHintInPrivateNamespace(t *testing.T) {
	t.Cleanup(func() { NoteCgroupNamespace("") })
	if !NoteCgroupNamespace("0::/\n") {
		t.Fatal("0::/ is the root of a private cgroup namespace")
	}
	err := changed("pid %d", 7)
	if !errors.Is(err, ErrContainerChanged) || !strings.Contains(err.Error(), "--cgroupns=host") {
		t.Fatalf("got %v", err)
	}
	if NoteCgroupNamespace("0::/system.slice/docker-" + strings.Repeat("ab", 32) + ".scope\n") {
		t.Fatal("a host cgroup path is not a private namespace")
	}
	if err := changed("pid %d", 7); strings.Contains(err.Error(), "--cgroupns=host") {
		t.Fatalf("hint only in a private namespace: %v", err)
	}
}
