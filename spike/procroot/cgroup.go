package procroot

import (
	"errors"
	"strings"
)

// ErrChanged means the process no longer is (or never was) the container's main process.
var ErrChanged = errors.New("container changed")

// CgroupHasID reports whether a /proc/<pid>/cgroup content belongs to the container
// with this full 64-character ID.
func CgroupHasID(cgroup, id string) bool {
	return len(id) == 64 && strings.Contains(cgroup, id)
}
