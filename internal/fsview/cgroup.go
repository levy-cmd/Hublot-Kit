package fsview

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
)

// CgroupHasID reports whether the content of /proc/<pid>/cgroup places the
// process inside the container with this full ID (spec §5.1 step 3).
//
// Each line's path is anchored on its outermost Docker-shaped component:
// "docker-<64 hex>.scope" (systemd driver) or "<64 hex>" right under a
// "docker" component (cgroupfs driver). Child cgroups of the anchor are
// accepted (docker:dind moves its PID 1 to ".../init"). A ".." component
// anywhere, an anchor for another ID or a malformed line refuses the whole
// content. A line without any Docker-shaped component (a cgroup v1
// controller left at "/") proves nothing and is skipped; at least one line
// must match.
func CgroupHasID(cgroup, id string) bool {
	if !isDockerID(id) {
		return false
	}
	matched := false
	for _, line := range strings.Split(cgroup, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, ":", 3) // hierarchy:controllers:path
		if len(fields) != 3 {
			return false
		}
		comps := strings.Split(fields[2], "/")
		if slices.Contains(comps, "..") {
			return false
		}
		switch anchor := outermostID(comps); anchor {
		case "":
			continue
		case id:
			matched = true
		default:
			return false
		}
	}
	return matched
}

// outermostID returns the ID of the first Docker-shaped component from the
// root, or "" if there is none.
func outermostID(comps []string) string {
	for i, c := range comps {
		if s, ok := strings.CutPrefix(c, "docker-"); ok {
			if s, ok := strings.CutSuffix(s, ".scope"); ok && isDockerID(s) {
				return s
			}
		}
		if i > 0 && comps[i-1] == "docker" && isDockerID(c) {
			return c
		}
	}
	return ""
}

// isDockerID: exactly 64 lowercase hexadecimal characters.
func isDockerID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}

var privateCgroupNS atomic.Bool

// NoteCgroupNamespace records, from the content of /proc/self/cgroup, whether
// the agent runs in a private cgroup namespace ("0::/", spec §8), and reports
// it. Every later ErrContainerChanged then carries the --cgroupns=host hint.
func NoteCgroupNamespace(selfCgroup string) bool {
	private := slices.Contains(strings.Split(selfCgroup, "\n"), "0::/")
	privateCgroupNS.Store(private)
	return private
}

// changed builds an ErrContainerChanged with its detail.
func changed(format string, a ...any) error {
	msg := fmt.Sprintf(format, a...)
	if privateCgroupNS.Load() {
		msg += " ; lancez l'agent avec --cgroupns=host"
	}
	return fmt.Errorf("%w : %s", ErrContainerChanged, msg)
}
